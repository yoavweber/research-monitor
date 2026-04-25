package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakePaperFetcher is an inline fake satisfying paper.Fetcher.
// It returns the configured entries and error without touching the network.
type fakePaperFetcher struct {
	entries []paper.Entry
	err     error
}

func (f *fakePaperFetcher) Fetch(_ context.Context, _ paper.Query) ([]paper.Entry, error) {
	return f.entries, f.err
}

// nopLogger is a no-op shared.Logger for wiring tests where log output is
// not under assertion.
type nopLogger struct{}

func (nopLogger) InfoContext(_ context.Context, _ string, _ ...any)  {}
func (nopLogger) WarnContext(_ context.Context, _ string, _ ...any)  {}
func (nopLogger) ErrorContext(_ context.Context, _ string, _ ...any) {}
func (nopLogger) DebugContext(_ context.Context, _ string, _ ...any) {}
func (nopLogger) With(_ ...any) shared.Logger                        { return nopLogger{} }

// fixedClock satisfies shared.Clock with a deterministic time.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// TestArxivRouter_RegistersFetchEndpoint verifies that ArxivRouter wires
// GET /arxiv/fetch onto the provided /api group and routes it through to the
// controller (which calls the use case, which calls the fake fetcher).
// Requirement 1.1: authenticated client gets 200 with entries.
// Auth (requirement 1.2) is provided by the /api group's middleware; we do
// not mount APIToken here since this test exercises route wiring only.
func TestArxivRouter_RegistersFetchEndpoint(t *testing.T) {
	t.Parallel()

	engine := gin.New()
	group := engine.Group("/api")

	ArxivRouter(Deps{
		Group:  group,
		Logger: nopLogger{},
		Clock:  fixedClock{t: time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)},
		Arxiv: ArxivConfig{
			Fetcher: &fakePaperFetcher{entries: []paper.Entry{}},
			Query:   paper.Query{Categories: []string{"cs.LG"}, MaxResults: 10},
		},
		// Empty fetch result → repo.Save is never invoked; a nil-method fake
		// is sufficient to satisfy the new use-case dependency without making
		// this wiring test responsible for repository behaviour.
		Paper: PaperConfig{Repo: &fakePaperRepo{}},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/arxiv/fetch: got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}
