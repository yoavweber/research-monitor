package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewApp_WiresArxivRoute verifies that NewApp assembles the runtime
// pipeline (byte fetcher → arxiv fetcher → query → route.ArxivConfig) such
// that GET /api/arxiv/fetch on the returned engine reaches a real handler
// instead of panicking on a nil Fetcher or missing route.
//
// Requirements exercised: 1.1 (authenticated client hits the fetch endpoint),
// 2.3 (query uses env categories), 3.1/3.2 (query uses env max_results + sort).
//
// This test mutates process-wide env via t.Setenv; it cannot use t.Parallel.
func TestNewApp_WiresArxivRoute(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "test.db"))
	// Leave ARXIV_BASE_URL unset so the env default applies.
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "1")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	app, err := NewApp(context.Background(), env)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if app == nil || app.Engine == nil {
		t.Fatalf("NewApp returned app=%v engine=%v", app, app.Engine)
	}

	// Authenticated request: the route must be registered AND the handler
	// must not nil-panic. With a valid wire-up the response is either 200
	// (arxiv returned XML), 502 (arxiv returned non-success / malformed),
	// or 504 (network timeout / DNS failure). A 500 here means the route
	// was reached but the use case hit a nil Fetcher — the bootstrap is
	// missing the arxiv wiring this task introduces.
	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	req.Header.Set("X-API-Token", env.APIToken)
	w := httptest.NewRecorder()
	app.Engine.ServeHTTP(w, req)

	switch w.Code {
	case http.StatusNotFound:
		t.Fatalf("GET /api/arxiv/fetch returned 404: route not registered on /api group; body=%s", w.Body.String())
	case http.StatusInternalServerError:
		t.Fatalf("GET /api/arxiv/fetch returned 500: handler reached but nil Fetcher caused a panic; body=%s", w.Body.String())
	case http.StatusUnauthorized:
		t.Fatalf("GET /api/arxiv/fetch returned 401 with a valid X-API-Token; body=%s", w.Body.String())
	}
	// Any remaining status (200, 502, 504, ...) proves the pipeline is
	// wired: the real fetcher ran and classified the upstream outcome.

	// Unauthenticated request must be rejected before the controller runs.
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	wNoAuth := httptest.NewRecorder()
	app.Engine.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/arxiv/fetch without token: got %d, want 401; body=%s", wNoAuth.Code, wNoAuth.Body.String())
	}
}

// TestNewApp_WiresPaperRoutes verifies the source-neutral /api/papers
// pipeline: NewApp must construct the persisted paper repository, thread it
// into route.Deps.Paper, and AutoMigrate the papers table so a freshly
// migrated DB serves an empty list (200) and a missing key returns 404.
//
// Requirements exercised: 4.3 / 4.4 (migration runs against a fresh schema
// so reads succeed), 5.1 (auth middleware still gates the route).
//
// This test mutates process-wide env via t.Setenv; it cannot use t.Parallel.
func TestNewApp_WiresPaperRoutes(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "1")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	app, err := NewApp(context.Background(), env)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	// Authenticated list against an empty (just-migrated) catalogue must
	// return 200 with the empty-array shape — proves the repo is wired and
	// the papers table exists. A 500 here means the migration did not
	// include the papers table or the repo was not threaded into Deps.
	reqList := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	reqList.Header.Set("X-API-Token", env.APIToken)
	wList := httptest.NewRecorder()
	app.Engine.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("GET /api/papers: status=%d, want 200; body=%s", wList.Code, wList.Body.String())
	}
	body := wList.Body.String()
	if !strings.Contains(body, `"papers":[]`) {
		t.Fatalf(`GET /api/papers body must contain "papers":[]; got=%s`, body)
	}
	if !strings.Contains(body, `"count":0`) {
		t.Fatalf(`GET /api/papers body must contain "count":0; got=%s`, body)
	}

	// Authenticated lookup for a key that does not exist must surface as 404,
	// not 500: the repo's ErrNotFound has to flow through the controller and
	// the ErrorEnvelope middleware that NewApp mounts on the engine.
	reqGet := httptest.NewRequest(http.MethodGet, "/api/papers/arxiv/unknown", nil)
	reqGet.Header.Set("X-API-Token", env.APIToken)
	wGet := httptest.NewRecorder()
	app.Engine.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusNotFound {
		t.Fatalf("GET /api/papers/arxiv/unknown: status=%d, want 404; body=%s", wGet.Code, wGet.Body.String())
	}

	// Auth still gates the route: missing token must be rejected at the
	// middleware layer before the controller runs.
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	wNoAuth := httptest.NewRecorder()
	app.Engine.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/papers without token: got %d, want 401; body=%s", wNoAuth.Code, wNoAuth.Body.String())
	}
}
