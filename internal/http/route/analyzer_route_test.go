package route_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
)

// stubUseCase is the minimum analyzer.UseCase the route test exercises.
// Returning a sentinel-free 200 lets us assert reachability through the
// auth middleware without coupling to controller-level error mapping
// (covered in 3.1).
type stubUseCase struct {
	analyzeCalls int
}

func (s *stubUseCase) Analyze(_ context.Context, id string) (*domain.Analysis, error) {
	s.analyzeCalls++
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	return &domain.Analysis{ExtractionID: id, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *stubUseCase) Get(_ context.Context, id string) (*domain.Analysis, error) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	return &domain.Analysis{ExtractionID: id, CreatedAt: now, UpdatedAt: now}, nil
}

func newAnalyzerEngine(uc domain.UseCase) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	group := engine.Group("/api")
	group.Use(middleware.APIToken(testAPIToken))

	route.AnalyzerRouter(route.Deps{
		Group:    group,
		Analyzer: route.AnalyzerConfig{UseCase: uc},
	})
	return engine
}

func TestAnalyzerRouter_RegistersEndpoints(t *testing.T) {
	t.Parallel()

	engine := newAnalyzerEngine(&stubUseCase{})

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"POST_analyses", http.MethodPost, "/api/analyses", []byte(`{"extraction_id":"ex-1"}`)},
		{"GET_analyses_by_id", http.MethodGet, "/api/analyses/ex-1", nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := bytes.NewBuffer(nil)
			if tc.body != nil {
				body = bytes.NewBuffer(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set(middleware.APITokenHeader, testAPIToken)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Fatalf("%s %s: route not registered (got 404); body=%s",
					tc.method, tc.path, w.Body.String())
			}
		})
	}
}

func TestAnalyzerRouter_RejectsMissingToken(t *testing.T) {
	t.Parallel()

	engine := newAnalyzerEngine(&stubUseCase{})

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"POST_no_token", http.MethodPost, "/api/analyses", []byte(`{"extraction_id":"ex-1"}`)},
		{"GET_no_token", http.MethodGet, "/api/analyses/ex-1", nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := bytes.NewBuffer(nil)
			if tc.body != nil {
				body = bytes.NewBuffer(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s without token: got status %d, want 401; body=%s",
					tc.method, tc.path, w.Code, w.Body.String())
			}
		})
	}
}

func TestAnalyzerRouter_AuthenticatedPostReachesController(t *testing.T) {
	t.Parallel()

	uc := &stubUseCase{}
	engine := newAnalyzerEngine(uc)

	body := bytes.NewBufferString(`{"extraction_id":"ex-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/analyses", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, testAPIToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	if uc.analyzeCalls != 1 {
		t.Errorf("analyzeCalls=%d, want 1 — controller did not run", uc.analyzeCalls)
	}
}
