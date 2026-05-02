//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	persistanalyzer "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/analyzer"
	persistextraction "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// analysisWire mirrors the controller's AnalysisResponse so this test owns
// a copy of the wire contract. If the controller's shape drifts the test
// fails loudly, which is what Requirement 1.3 asks for.
type analysisWire struct {
	ExtractionID         string `json:"extraction_id"`
	ShortSummary         string `json:"short_summary"`
	LongSummary          string `json:"long_summary"`
	ThesisAngleFlag      bool   `json:"thesis_angle_flag"`
	ThesisAngleRationale string `json:"thesis_angle_rationale"`
	Model                string `json:"model"`
	PromptVersion        string `json:"prompt_version"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type analysisEnvelope struct {
	Data analysisWire `json:"data"`
}

// seedDoneExtraction inserts a row directly via the persistence model so the
// analyzer's FindByID + status gate sees a "done" extraction it can read.
// Returns the row's id.
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

// seedPendingExtraction inserts a row with status=pending so the analyzer's
// not-ready branch can be exercised end-to-end.
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

func doPostAnalyses(t *testing.T, env *setup.TestEnv, extractionID string) *http.Response {
	t.Helper()
	body := []byte(`{"extraction_id":"` + extractionID + `"}`)
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/analyses", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func doGetAnalyses(t *testing.T, env *setup.TestEnv, extractionID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/analyses/"+extractionID, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return resp
}

func decodeAnalysis(t *testing.T, resp *http.Response) analysisWire {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env analysisEnvelope
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

	resp := doPostAnalyses(t, env, id)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/analyses status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	got := decodeAnalysis(t, resp)

	if got.ExtractionID != id {
		t.Errorf("extraction_id = %q, want %q", got.ExtractionID, id)
	}
	if got.ShortSummary == "" || got.LongSummary == "" {
		t.Errorf("summaries empty: short=%q long=%q", got.ShortSummary, got.LongSummary)
	}
	if got.Model != "fake" {
		t.Errorf("model = %q, want %q (fake provider)", got.Model, "fake")
	}
	if !strings.Contains(got.PromptVersion, "thesis.v1") {
		t.Errorf("prompt_version = %q does not name the thesis prompt", got.PromptVersion)
	}

	// Persistence: exactly one row per extraction id, contents match.
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

	// Read-back via GET.
	getResp := doGetAnalyses(t, env, id)
	if getResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(getResp.Body)
		t.Fatalf("GET /api/analyses/:id status = %d, want 200; body=%s", getResp.StatusCode, raw)
	}
	gotGet := decodeAnalysis(t, getResp)
	if gotGet.ExtractionID != id || gotGet.ShortSummary != got.ShortSummary {
		t.Errorf("GET body = %+v, want fields matching POST response", gotGet)
	}
}

func TestAnalyzer_E2E_RerunOverwrites_PreservesCreatedAt(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	id := seedDoneExtraction(t, env, "First body.")

	first := decodeAnalysis(t, doPostAnalyses(t, env, id))
	// The fake's clock is the system clock; the second POST must observe a
	// later UpdatedAt. Sleeping a millisecond is sufficient because the
	// repository writes time at second-or-better resolution.
	time.Sleep(2 * time.Millisecond)
	second := decodeAnalysis(t, doPostAnalyses(t, env, id))

	if c := countAnalyses(t, env, id); c != 1 {
		t.Errorf("row count after rerun = %d, want 1", c)
	}
	if first.CreatedAt != second.CreatedAt {
		t.Errorf("created_at changed across rerun: first=%s second=%s", first.CreatedAt, second.CreatedAt)
	}
	if !(second.UpdatedAt >= first.UpdatedAt) {
		t.Errorf("updated_at did not advance: first=%s second=%s", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestAnalyzer_E2E_GetUnknownID_Returns404(t *testing.T) {
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{WireAnalyzer: true})
	defer env.Close()

	resp := doGetAnalyses(t, env, "does-not-exist")
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

	resp := doPostAnalyses(t, env, id)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 409; body=%s", resp.StatusCode, raw)
	}
	if c := countAnalyses(t, env, id); c != 0 {
		t.Errorf("rows for pending extraction = %d, want 0 (no write on precondition failure)", c)
	}
}
