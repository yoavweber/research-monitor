package route

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	paperrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// newPaperEngine assembles a minimal /api group identical to production
// modulo auth: the ErrorEnvelope middleware is mounted so the sentinel
// translation (paper.ErrNotFound → 404) materialises in w.Code, which is
// what the smoke tests assert. Auth is intentionally absent — these tests
// exercise route wiring, not the APIToken middleware.
func newPaperEngine(repo paper.Repository) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	group := engine.Group("/api")

	PaperRouter(Deps{
		Group:  group,
		Logger: nopLogger{},
		Clock:  fixedClock{t: time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)},
		Paper:  PaperConfig{Repo: repo},
	})
	return engine
}

// TestPaperRouter_RegistersEndpoints verifies both /api/papers routes are
// wired through to the controller and that sentinel errors flow through
// ErrorEnvelope. Requirements 2.1, 2.3, 3.1, 3.4, 5.2.
func TestPaperRouter_RegistersEndpoints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		// List against an empty real repo returns the empty-array envelope.
		{"List_OK", "/api/papers", http.StatusOK},
		// FindByKey against an empty real repo surfaces paper.ErrNotFound, which
		// the ErrorEnvelope middleware translates to 404.
		{"Get_NotFound", "/api/papers/arxiv/x", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := paperrepo.NewRepository(testdb.New(t))
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			newPaperEngine(repo).ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("GET %s: status=%d, want %d; body=%s", tc.path, w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}
