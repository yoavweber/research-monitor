package route_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testAPIToken = "test-token"

// newExtractionEngine assembles a minimal /api group with the production
// APIToken middleware mounted, then calls ExtractionRouter to register the
// extraction handlers. The fake use case records calls so the test can
// assert reachability through the auth chain without spinning up the full
// bootstrap composition (Task 4 owns that).
func newExtractionEngine(uc extraction.UseCase) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	group := engine.Group("/api")
	group.Use(middleware.APIToken(testAPIToken))

	route.ExtractionRouter(route.Deps{
		Group: group,
		Extraction: route.ExtractionConfig{
			UseCase: uc,
		},
	})
	return engine
}

// TestExtractionRouter_RegistersEndpoints verifies that both extraction
// endpoints are wired onto the /api group. We probe each path with a valid
// API token and assert the response is NOT 404 (i.e. the router matched the
// path); the precise status code depends on the fake use case, which is
// covered by the controller-level tests in Task 3.5.
// Requirements 1.4, 2.6 (404 absence proves route registration).
func TestExtractionRouter_RegistersEndpoints(t *testing.T) {
	t.Parallel()

	uc := &mocks.ExtractionUseCaseFake{
		SubmitResult: extraction.SubmitResult{ID: "ext-1", Status: extraction.JobStatusPending},
		GetResult: &extraction.Extraction{
			ID:     "ext-1",
			Status: extraction.JobStatusPending,
			RequestPayload: extraction.RequestPayload{
				SourceType: "arxiv",
				SourceID:   "2501.00001",
				PDFPath:    "/tmp/x.pdf",
			},
		},
	}
	engine := newExtractionEngine(uc)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"POST_extractions", http.MethodPost, "/api/extractions",
			[]byte(`{"source_type":"arxiv","source_id":"2501.00001","pdf_path":"/tmp/x.pdf"}`)},
		{"GET_extractions_by_id", http.MethodGet, "/api/extractions/ext-1", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body *bytes.Buffer
			if tc.body != nil {
				body = bytes.NewBuffer(tc.body)
			} else {
				body = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set(middleware.APITokenHeader, testAPIToken)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Fatalf("%s %s: route not registered (got 404); body=%s", tc.method, tc.path, w.Body.String())
			}
		})
	}
}

// TestExtractionRouter_RejectsMissingToken verifies the new endpoints
// inherit the APIToken middleware mounted on the /api group: a request
// without X-API-Token returns 401 from both POST /api/extractions and
// GET /api/extractions/:id. Requirements 1.4, 2.6.
func TestExtractionRouter_RejectsMissingToken(t *testing.T) {
	t.Parallel()

	engine := newExtractionEngine(&mocks.ExtractionUseCaseFake{})

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"POST_no_token", http.MethodPost, "/api/extractions",
			[]byte(`{"source_type":"arxiv","source_id":"2501.00001","pdf_path":"/tmp/x.pdf"}`)},
		{"GET_no_token", http.MethodGet, "/api/extractions/ext-1", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body *bytes.Buffer
			if tc.body != nil {
				body = bytes.NewBuffer(tc.body)
			} else {
				body = bytes.NewBuffer(nil)
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

// TestExtractionRouter_PostReachesController verifies an authenticated POST
// passes through APIToken and lands on Submit. The fake records the
// translated RequestPayload, proving the controller was invoked. Confirms
// the route is wired through the auth middleware end-to-end; exhaustive
// controller behavior is asserted in Task 3.5.
func TestExtractionRouter_PostReachesController(t *testing.T) {
	t.Parallel()

	uc := &mocks.ExtractionUseCaseFake{
		SubmitResult: extraction.SubmitResult{ID: "ext-1", Status: extraction.JobStatusPending},
	}
	engine := newExtractionEngine(uc)

	body := bytes.NewBufferString(`{"source_type":"arxiv","source_id":"2501.00001","pdf_path":"/tmp/x.pdf"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/extractions", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APITokenHeader, testAPIToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("POST /api/extractions with valid token: got status %d, want 202; body=%s",
			w.Code, w.Body.String())
	}

	submits, _, _ := uc.CallsSnapshot()
	if len(submits) != 1 {
		t.Fatalf("expected 1 Submit call, got %d", len(submits))
	}
	if submits[0].SourceType != "arxiv" || submits[0].SourceID != "2501.00001" || submits[0].PDFPath != "/tmp/x.pdf" {
		t.Fatalf("unexpected Submit payload: %+v", submits[0])
	}
}
