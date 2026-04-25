package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

// fakePaperRepo is an inline paper.Repository that the route-level smoke
// tests share. It records the calls received and returns whatever the
// individual test pre-loaded — Save is unused by the read endpoints but
// must be present to satisfy the interface; calling it from these tests
// is a programmer error and the helper panics so the regression surfaces
// immediately.
type fakePaperRepo struct {
	findEntry *paper.Entry
	findErr   error

	listEntries []paper.Entry
	listErr     error
}

func (f *fakePaperRepo) Save(_ context.Context, _ paper.Entry) (bool, error) {
	panic("fakePaperRepo.Save must not be called by route-level read tests")
}

func (f *fakePaperRepo) FindByKey(_ context.Context, _, _ string) (*paper.Entry, error) {
	return f.findEntry, f.findErr
}

func (f *fakePaperRepo) List(_ context.Context) ([]paper.Entry, error) {
	return f.listEntries, f.listErr
}

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
		repo       *fakePaperRepo
		wantStatus int
	}{
		{"List_OK", "/api/papers", &fakePaperRepo{listEntries: []paper.Entry{}}, http.StatusOK},
		{"Get_NotFound", "/api/papers/arxiv/x", &fakePaperRepo{findErr: paper.ErrNotFound}, http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			newPaperEngine(tc.repo).ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("GET %s: status=%d, want %d; body=%s", tc.path, w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}
