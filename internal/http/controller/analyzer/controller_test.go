package analyzer_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	analyzerctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newEngine(ctrl *analyzerctrl.Controller) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	engine.POST("/api/analyses", ctrl.Submit)
	engine.GET("/api/analyses/:extraction_id", ctrl.Get)
	return engine
}

func sample(t time.Time) *domain.Analysis {
	return &domain.Analysis{
		ExtractionID:         "ex-1",
		ShortSummary:         "short text",
		LongSummary:          "long text",
		ThesisAngleFlag:      true,
		ThesisAngleRationale: "promising",
		Model:                "fake",
		PromptVersion:        "short.v1+long.v1+thesis.v1",
		CreatedAt:            t,
		UpdatedAt:            t,
	}
}

func decodeAnalysis(t *testing.T, body []byte) analyzerctrl.AnalysisResponse {
	t.Helper()
	var env analyzerctrl.AnalysisEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	return env.Data
}

// errorEnvelope is the wire shape decoded from the standard error path.
// common.Envelope's Error field is unexported; decoding through this typed
// shape avoids the map[string]any soup the controller test would otherwise
// drown in for status-code/reason assertions.
type errorEnvelope struct {
	Error common.Error `json:"error"`
}

func decodeError(t *testing.T, body []byte) common.Error {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	return env.Error
}

func TestSubmit_HappyPath_Returns200WithAnalysisShape(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(_ context.Context, id string) (*domain.Analysis, error) {
			a := sample(now)
			a.ExtractionID = id
			return a, nil
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"ex-1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	got := decodeAnalysis(t, w.Body.Bytes())
	if got.ExtractionID != "ex-1" {
		t.Errorf("extraction_id = %q", got.ExtractionID)
	}
	if !got.ThesisAngleFlag {
		t.Errorf("thesis_angle_flag = false, want true")
	}
	if got.ShortSummary != "short text" || got.LongSummary != "long text" {
		t.Errorf("summary fields = %q / %q", got.ShortSummary, got.LongSummary)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("created_at not set")
	}
}

func TestGet_HappyPath_Returns200(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	fake := &mocks.AnalyzerUseCaseFake{
		GetFn: func(_ context.Context, id string) (*domain.Analysis, error) {
			a := sample(now)
			a.ExtractionID = id
			return a, nil
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/analyses/ex-7", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	got := decodeAnalysis(t, w.Body.Bytes())
	if got.ExtractionID != "ex-7" {
		t.Errorf("extraction_id = %q, want ex-7", got.ExtractionID)
	}
	if fake.GetCalls != 1 {
		t.Errorf("GetCalls = %d, want 1", fake.GetCalls)
	}
}

func TestSubmit_EmptyExtractionID_Returns400AndDoesNotInvokeUseCase(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
	if fake.AnalyzeCalls != 0 {
		t.Errorf("Analyze invoked despite bad request: calls=%d", fake.AnalyzeCalls)
	}
}

func TestSubmit_MalformedJSON_Returns400(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestSubmit_ExtractionNotFound_Returns404(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, domain.ErrExtractionNotFound
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404; body=%s", w.Code, w.Body.String())
	}
	got := decodeError(t, w.Body.Bytes())
	if reason, _ := got.Details["reason"].(string); reason != "extraction_not_found" {
		t.Errorf("details.reason = %q, want extraction_not_found", reason)
	}
}

func TestSubmit_ExtractionNotReady_Returns409(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, domain.ErrExtractionNotReady
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"ex-pending"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestSubmit_LLMUpstream_Returns502WithReason(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, errors.Join(domain.ErrLLMUpstream, errors.New("transport boom"))
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"ex-fail"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502; body=%s", w.Code, w.Body.String())
	}
	got := decodeError(t, w.Body.Bytes())
	if reason, _ := got.Details["reason"].(string); reason != "llm_upstream" {
		t.Errorf("details.reason = %q, want llm_upstream", reason)
	}
}

func TestSubmit_LLMMalformed_Returns502WithReason(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, domain.ErrAnalyzerMalformedResponse
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"ex-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502; body=%s", w.Code, w.Body.String())
	}
	got := decodeError(t, w.Body.Bytes())
	if reason, _ := got.Details["reason"].(string); reason != "llm_malformed_response" {
		t.Errorf("details.reason = %q, want llm_malformed_response", reason)
	}
}

func TestSubmit_CatalogueUnavailable_Returns500WithoutReason(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, domain.ErrCatalogueUnavailable
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(`{"extraction_id":"ex-fail"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500; body=%s", w.Code, w.Body.String())
	}
	got := decodeError(t, w.Body.Bytes())
	if got.Details != nil {
		t.Errorf("500 response should not carry details: %v", got.Details)
	}
}

func TestGet_AnalysisNotFound_Returns404(t *testing.T) {
	t.Parallel()

	fake := &mocks.AnalyzerUseCaseFake{
		GetFn: func(context.Context, string) (*domain.Analysis, error) {
			return nil, domain.ErrAnalysisNotFound
		},
	}
	ctrl := analyzerctrl.NewController(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/analyses/missing", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404; body=%s", w.Code, w.Body.String())
	}
	got := decodeError(t, w.Body.Bytes())
	if reason, _ := got.Details["reason"].(string); reason != "analysis_not_found" {
		t.Errorf("details.reason = %q, want analysis_not_found", reason)
	}
}
