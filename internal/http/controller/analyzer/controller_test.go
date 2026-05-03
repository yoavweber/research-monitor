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
		ThesisAngleRationale: "placeholder",
		Model:                "fake",
		PromptVersion:        "analyzer.short.v1+analyzer.long.v1",
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

func postAnalyses(t *testing.T, fake *mocks.AnalyzerUseCaseFake, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/analyses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newEngine(analyzerctrl.NewController(fake)).ServeHTTP(w, req)
	return w
}

func getAnalysis(t *testing.T, fake *mocks.AnalyzerUseCaseFake, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/analyses/"+id, nil)
	w := httptest.NewRecorder()
	newEngine(analyzerctrl.NewController(fake)).ServeHTTP(w, req)
	return w
}

func TestSubmit(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 and the documented analysis envelope on success", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
		fake := &mocks.AnalyzerUseCaseFake{
			AnalyzeFn: func(_ context.Context, id string) (*domain.Analysis, error) {
				a := sample(now)
				a.ExtractionID = id
				return a, nil
			},
		}

		w := postAnalyses(t, fake, `{"extraction_id":"ex-1"}`)

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
	})

	t.Run("returns 400 and skips the use case for an empty extraction_id", func(t *testing.T) {
		t.Parallel()

		fake := &mocks.AnalyzerUseCaseFake{}

		w := postAnalyses(t, fake, `{"extraction_id":""}`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
		}
		if fake.AnalyzeCalls != 0 {
			t.Errorf("Analyze invoked despite bad request: calls=%d", fake.AnalyzeCalls)
		}
	})

	t.Run("returns 400 for a malformed JSON body", func(t *testing.T) {
		t.Parallel()

		fake := &mocks.AnalyzerUseCaseFake{}

		w := postAnalyses(t, fake, `{not json`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("maps each sentinel onto the documented status code and reason", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name       string
			err        error
			wantStatus int
			wantReason string
		}{
			{"extraction not found", domain.ErrExtractionNotFound, http.StatusNotFound, "extraction_not_found"},
			{"extraction not ready", domain.ErrExtractionNotReady, http.StatusConflict, "extraction_not_ready"},
			{"llm transport error", errors.Join(domain.ErrLLMUpstream, errors.New("boom")), http.StatusBadGateway, "llm_upstream"},
			{"catalogue unavailable carries no reason", domain.ErrCatalogueUnavailable, http.StatusInternalServerError, ""},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fake := &mocks.AnalyzerUseCaseFake{
					AnalyzeFn: func(context.Context, string) (*domain.Analysis, error) { return nil, tc.err },
				}

				w := postAnalyses(t, fake, `{"extraction_id":"ex-x"}`)

				if w.Code != tc.wantStatus {
					t.Fatalf("status=%d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
				}
				got := decodeError(t, w.Body.Bytes())
				gotReason, _ := got.Details["reason"].(string)
				if gotReason != tc.wantReason {
					t.Errorf("details.reason = %q, want %q", gotReason, tc.wantReason)
				}
			})
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 with the documented envelope on success", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
		fake := &mocks.AnalyzerUseCaseFake{
			GetFn: func(_ context.Context, id string) (*domain.Analysis, error) {
				a := sample(now)
				a.ExtractionID = id
				return a, nil
			},
		}

		w := getAnalysis(t, fake, "ex-7")

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
	})

	t.Run("returns 404 with the analysis_not_found reason for a missing id", func(t *testing.T) {
		t.Parallel()

		fake := &mocks.AnalyzerUseCaseFake{
			GetFn: func(context.Context, string) (*domain.Analysis, error) { return nil, domain.ErrAnalysisNotFound },
		}

		w := getAnalysis(t, fake, "missing")

		if w.Code != http.StatusNotFound {
			t.Fatalf("status=%d, want 404; body=%s", w.Code, w.Body.String())
		}
		got := decodeError(t, w.Body.Bytes())
		if reason, _ := got.Details["reason"].(string); reason != "analysis_not_found" {
			t.Errorf("details.reason = %q, want analysis_not_found", reason)
		}
	})
}
