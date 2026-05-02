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
	analyzerctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// useCaseFake is an inline analyzer.UseCase double scoped to controller
// tests. AnalyzeFn / GetFn let each case program the response or sentinel
// the controller will see; defaults panic so unintended calls are loud.
type useCaseFake struct {
	AnalyzeFn func(ctx context.Context, id string) (*domain.Analysis, error)
	GetFn     func(ctx context.Context, id string) (*domain.Analysis, error)

	AnalyzeCalls int
	GetCalls     int
}

func (f *useCaseFake) Analyze(ctx context.Context, id string) (*domain.Analysis, error) {
	f.AnalyzeCalls++
	if f.AnalyzeFn == nil {
		panic("Analyze called but no AnalyzeFn programmed")
	}
	return f.AnalyzeFn(ctx, id)
}

func (f *useCaseFake) Get(ctx context.Context, id string) (*domain.Analysis, error) {
	f.GetCalls++
	if f.GetFn == nil {
		panic("Get called but no GetFn programmed")
	}
	return f.GetFn(ctx, id)
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

func decodeData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	d, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing or wrong type; raw=%s", body)
	}
	return d
}

func decodeError(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type; raw=%s", body)
	}
	return e
}

// --- success paths --------------------------------------------------------

func TestSubmit_HappyPath_Returns200WithAnalysisShape(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	fake := &useCaseFake{
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
	d := decodeData(t, w.Body.Bytes())
	if d["extraction_id"] != "ex-1" {
		t.Errorf("extraction_id = %v", d["extraction_id"])
	}
	if d["thesis_angle_flag"] != true {
		t.Errorf("thesis_angle_flag = %v, want true", d["thesis_angle_flag"])
	}
	if d["short_summary"] != "short text" || d["long_summary"] != "long text" {
		t.Errorf("summary fields = %v / %v", d["short_summary"], d["long_summary"])
	}
	if _, ok := d["created_at"].(string); !ok {
		t.Errorf("created_at not a string: %v", d["created_at"])
	}
}

func TestGet_HappyPath_Returns200(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	fake := &useCaseFake{
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
	d := decodeData(t, w.Body.Bytes())
	if d["extraction_id"] != "ex-7" {
		t.Errorf("extraction_id = %v, want ex-7", d["extraction_id"])
	}
	if fake.GetCalls != 1 {
		t.Errorf("GetCalls = %d, want 1", fake.GetCalls)
	}
}

// --- precondition / error paths -------------------------------------------

func TestSubmit_EmptyExtractionID_Returns400AndDoesNotInvokeUseCase(t *testing.T) {
	t.Parallel()

	fake := &useCaseFake{}
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

	fake := &useCaseFake{}
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

	fake := &useCaseFake{
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
	e := decodeError(t, w.Body.Bytes())
	if details, ok := e["details"].(map[string]any); ok {
		if details["reason"] != "extraction_not_found" {
			t.Errorf("details.reason = %v, want extraction_not_found", details["reason"])
		}
	} else {
		t.Errorf("details missing on 404 response: %v", e)
	}
}

func TestSubmit_ExtractionNotReady_Returns409(t *testing.T) {
	t.Parallel()

	fake := &useCaseFake{
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

	fake := &useCaseFake{
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
	e := decodeError(t, w.Body.Bytes())
	details, ok := e["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing on 502: %v", e)
	}
	if details["reason"] != "llm_upstream" {
		t.Errorf("details.reason = %v, want llm_upstream", details["reason"])
	}
}

func TestSubmit_LLMMalformed_Returns502WithReason(t *testing.T) {
	t.Parallel()

	fake := &useCaseFake{
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
	e := decodeError(t, w.Body.Bytes())
	details, ok := e["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing on 502: %v", e)
	}
	if details["reason"] != "llm_malformed_response" {
		t.Errorf("details.reason = %v, want llm_malformed_response", details["reason"])
	}
}

func TestSubmit_CatalogueUnavailable_Returns500WithoutReason(t *testing.T) {
	t.Parallel()

	fake := &useCaseFake{
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
	e := decodeError(t, w.Body.Bytes())
	if _, has := e["details"]; has {
		t.Errorf("500 response should not carry details.reason: %v", e)
	}
}

func TestGet_AnalysisNotFound_Returns404(t *testing.T) {
	t.Parallel()

	fake := &useCaseFake{
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
	e := decodeError(t, w.Body.Bytes())
	details, ok := e["details"].(map[string]any)
	if !ok || details["reason"] != "analysis_not_found" {
		t.Errorf("details.reason = %v, want analysis_not_found", details)
	}
}
