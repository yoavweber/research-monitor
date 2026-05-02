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
	persistextraction "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

func seedDoneExtraction(t *testing.T, env *setup.TestEnv, body string) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	row := persistextraction.Extraction{
		ID:                  id,
		SourceType:          "paper",
		SourceID:            id,
		Status:              "done",
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

func seedPendingExtraction(t *testing.T, env *setup.TestEnv) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	row := persistextraction.Extraction{
		ID:             id,
		SourceType:     "paper",
		SourceID:       id,
		Status:         "pending",
		RequestPayload: `{"SourceType":"paper","SourceID":"` + id + `","PDFPath":"/tmp/p.pdf"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := env.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed pending extraction: %v", err)
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

func TestAnalyzer_E2E_PostThenGet_RoundTripsAnalysis(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	id := seedDoneExtraction(t, env, "Body markdown for the analyzer test.")

	resp := doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/analyses status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	got := decodeAnalysisEnv(t, resp)

	if got.ExtractionID != id {
		t.Errorf("extraction_id = %q, want %q", got.ExtractionID, id)
	}
	if got.ShortSummary == "" || got.LongSummary == "" {
		t.Errorf("summaries empty: short=%q long=%q", got.ShortSummary, got.LongSummary)
	}
	if got.Model != "fake" {
		t.Errorf("model = %q, want %q (fake provider)", got.Model, "fake")
	}

	if c := countAnalyses(t, env, id); c != 1 {
		t.Errorf("rows for %s = %d, want 1", id, c)
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
		t.Fatalf("GET /api/analyses/:id status = %d, want 200; body=%s", getResp.StatusCode, raw)
	}
	gotGet := decodeAnalysisEnv(t, getResp)
	if gotGet.ExtractionID != id || gotGet.ShortSummary != got.ShortSummary {
		t.Errorf("GET body = %+v, want fields matching POST response", gotGet)
	}
}

func TestAnalyzer_E2E_RerunOverwrites_PreservesCreatedAt(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	id := seedDoneExtraction(t, env, "First body.")

	first := decodeAnalysisEnv(t, doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`))
	// SQLite writes time at second-or-better resolution; sleep so UpdatedAt
	// is reliably observable as advanced.
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
}

func TestAnalyzer_E2E_GetUnknownID_Returns404(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/analyses/does-not-exist")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 404; body=%s", resp.StatusCode, raw)
	}
}

func TestAnalyzer_E2E_PostNotReady_Returns409AndWritesNoRow(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	id := seedPendingExtraction(t, env)

	resp := doAuthenticatedPost(t, env.Server.URL+"/api/analyses", `{"extraction_id":"`+id+`"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 409; body=%s", resp.StatusCode, raw)
	}
	if c := countAnalyses(t, env, id); c != 0 {
		t.Errorf("rows for pending extraction = %d, want 0 (no write on precondition failure)", c)
	}
}
