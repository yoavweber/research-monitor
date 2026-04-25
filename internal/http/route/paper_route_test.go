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

// TestPaperRouter_RegistersListEndpoint verifies GET /api/papers reaches
// the controller and returns 200 with the repo's (possibly empty) result.
// Requirements 2.1, 3.1: the route is registered and routes through to the
// list handler.
func TestPaperRouter_RegistersListEndpoint(t *testing.T) {
	t.Parallel()

	repo := &fakePaperRepo{listEntries: []paper.Entry{}}

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()
	newPaperEngine(repo).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/papers: status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestPaperRouter_RegistersGetEndpoint_NotFound verifies GET /api/papers/
// :source/:source_id reaches the controller, that paper.ErrNotFound from
// the repo flows through c.Error → ErrorEnvelope, and that the resulting
// status is 404. Requirements 2.3, 3.4, 5.2.
func TestPaperRouter_RegistersGetEndpoint_NotFound(t *testing.T) {
	t.Parallel()

	repo := &fakePaperRepo{findErr: paper.ErrNotFound}

	req := httptest.NewRequest(http.MethodGet, "/api/papers/arxiv/x", nil)
	w := httptest.NewRecorder()
	newPaperEngine(repo).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/papers/arxiv/x: status=%d, want 404; body=%s", w.Code, w.Body.String())
	}
}
