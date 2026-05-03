//go:build integration

package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	analyzerctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/analyzer"
	persistanalyzer "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	persistextraction "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

func seedExtraction(t *testing.T, env *setup.TestEnv, status, body string) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	row := persistextraction.Extraction{
		ID:                  id,
		SourceType:          "paper",
		SourceID:            id,
		Status:              extraction.JobStatus(status),
		RequestPayload:      `{"SourceType":"paper","SourceID":"` + id + `","PDFPath":"/tmp/p.pdf"}`,
		BodyMarkdown:        body,
		MetadataContentType: "paper",
		MetadataWordCount:   3,
		Title:               "Seeded paper",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := env.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed extraction: %v", err)
	}
	return id
}

func decodeAnalysisEnv(t *testing.T, resp *http.Response) analyzerctrl.AnalysisResponse {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env analyzerctrl.AnalysisEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, raw)
	}
	return env.Data
}

func countAnalyses(t *testing.T, env *setup.TestEnv, extractionID string) int64 {
	t.Helper()
	var count int64
	if err := env.DB.Table("analyses").Where("extraction_id = ?", extractionID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}

func TestAnalyzer_E2E(t *testing.T) {
	t.Run("POST then GET round-trip persists and reads the same analysis", func(t *testing.T) {
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
		defer env.Close()
		id := seedExtraction(t, env, "done", "Body markdown for the analyzer test.")

		postResp := doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`)
		if postResp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(postResp.Body)
			t.Fatalf("POST status = %d, want 200; body=%s", postResp.StatusCode, raw)
		}
		got := decodeAnalysisEnv(t, postResp)

		if got.ExtractionID != id || got.ShortSummary == "" || got.LongSummary == "" {
			t.Errorf("response missing required fields: %+v", got)
		}
		if got.Model != "fake" {
			t.Errorf("model = %q, want %q", got.Model, "fake")
		}
		if c := countAnalyses(t, env, id); c != 1 {
			t.Errorf("rows = %d, want 1", c)
		}
		persisted := persistanalyzer.Analysis{}
		if err := env.DB.Where("extraction_id = ?", id).First(&persisted).Error; err != nil {
			t.Fatalf("load row: %v", err)
		}
		if persisted.ShortSummary != got.ShortSummary {
			t.Errorf("DB.short = %q, response.short = %q", persisted.ShortSummary, got.ShortSummary)
		}

		getResp := doAuthenticatedGet(t, env.Server.URL+"/api/analyses/"+id)
		if getResp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(getResp.Body)
			t.Fatalf("GET status = %d, want 200; body=%s", getResp.StatusCode, raw)
		}
		gotGet := decodeAnalysisEnv(t, getResp)
		if gotGet.ExtractionID != id || gotGet.ShortSummary != got.ShortSummary {
			t.Errorf("GET body = %+v, want fields matching POST response", gotGet)
		}
	})

	t.Run("rerun overwrites the row, preserves created_at, advances updated_at", func(t *testing.T) {
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
		defer env.Close()
		id := seedExtraction(t, env, "done", "First body.")

		first := decodeAnalysisEnv(t, doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`))
		// SQLite writes time at second-or-better resolution; sleep so
		// UpdatedAt is reliably observable as advanced.
		time.Sleep(2 * time.Millisecond)
		second := decodeAnalysisEnv(t, doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`))

		if c := countAnalyses(t, env, id); c != 1 {
			t.Errorf("row count after rerun = %d, want 1", c)
		}
		if !first.CreatedAt.Equal(second.CreatedAt) {
			t.Errorf("created_at changed across rerun: first=%s second=%s", first.CreatedAt, second.CreatedAt)
		}
		if second.UpdatedAt.Before(first.UpdatedAt) {
			t.Errorf("updated_at went backwards: first=%s second=%s", first.UpdatedAt, second.UpdatedAt)
		}
	})

	t.Run("precondition failures return the documented status with no row written", func(t *testing.T) {
		env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
		defer env.Close()

		cases := []struct {
			name       string
			seedStatus string
			useUnknown bool
			method     string
			wantStatus int
		}{
			{"GET unknown extraction id returns 404", "", true, http.MethodGet, http.StatusNotFound},
			{"POST against a pending extraction returns 409", "pending", false, http.MethodPost, http.StatusConflict},
			{"POST against a failed extraction returns 409", "failed", false, http.MethodPost, http.StatusConflict},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				var (
					id   string
					resp *http.Response
				)
				if tc.useUnknown {
					id = "does-not-exist"
				} else {
					id = seedExtraction(t, env, tc.seedStatus, "")
				}
				if tc.method == http.MethodPost {
					resp = doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`)
				} else {
					resp = doAuthenticatedGet(t, env.Server.URL + "/api/analyses/" + id)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.wantStatus {
					raw, _ := io.ReadAll(resp.Body)
					t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, raw)
				}
				if !tc.useUnknown {
					if c := countAnalyses(t, env, id); c != 0 {
						t.Errorf("rows for precondition-failed extraction = %d, want 0", c)
					}
				}
			})
		}
	})
}
