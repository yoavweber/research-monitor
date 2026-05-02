package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainextraction "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	extractionpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
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
	req.Header.Set(middleware.APITokenHeader, env.APIToken)
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
	reqList.Header.Set(middleware.APITokenHeader, env.APIToken)
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
	reqGet.Header.Set(middleware.APITokenHeader, env.APIToken)
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

// TestNewApp_ExtractionStartupRecovery covers the design's "Lifecycle, expiry,
// and restart recovery" 4-step bootstrap sequence: a row left in `running`
// from a prior process exit must be flipped to `failed: process_restart`
// BEFORE the worker starts and BEFORE any HTTP request is served, while a
// pending row from before the prior shutdown must be picked up by the worker
// after self-signal.
//
// Requirements exercised: 1.4 (extraction routes mounted via composed
// dependencies), 2.6 (worker drains pending after startup), 5.5 / 5.6
// (composition root wires extractor → repo → use case → worker → route deps,
// owns lifecycle), 6.5 / 6.6 (recovery flip runs before serving + fail-fast
// on recovery error).
//
// The test does not call LoadEnv (which would mutate process-wide env). It
// constructs an Env literal with a tempdir SQLite path so the test is
// hermetic and concurrency-safe across runs.
func TestNewApp_ExtractionStartupRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "extraction.db")

	// Phase 1: seed the on-disk DB with one row in `running` (orphaned from
	// a prior process) and one in `pending` (was waiting at shutdown). We
	// open the DB through the production OpenSQLite helper so the seeded
	// connection observes the same WAL / TranslateError flags NewApp will
	// later see — and so the SQLite driver registration matches.
	seedDB, err := persistence.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("seed open db: %v", err)
	}
	if err := persistence.AutoMigrate(seedDB); err != nil {
		t.Fatalf("seed automigrate: %v", err)
	}

	now := time.Now().UTC()
	runningID := uuid.NewString()
	pendingID := uuid.NewString()
	payloadJSON := mustMarshalRequestPayload(t, domainextraction.RequestPayload{
		SourceType: "paper",
		SourceID:   "seed-paper",
		// /nonexistent/mineru is also configured below; this PDF path is
		// never read because the extractor errors before opening the file.
		PDFPath: "/nonexistent/sample.pdf",
	})

	rows := []extractionpersist.Extraction{
		{
			ID:             runningID,
			SourceType:     "paper",
			SourceID:       "seed-running",
			Status:         domainextraction.JobStatusRunning,
			RequestPayload: payloadJSON,
			CreatedAt:      now.Add(-30 * time.Second),
			UpdatedAt:      now.Add(-30 * time.Second),
		},
		{
			ID:             pendingID,
			SourceType:     "paper",
			SourceID:       "seed-pending",
			Status:         domainextraction.JobStatusPending,
			RequestPayload: payloadJSON,
			CreatedAt:      now.Add(-10 * time.Second),
			UpdatedAt:      now.Add(-10 * time.Second),
		},
	}
	for i := range rows {
		if err := seedDB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed create row %d: %v", i, err)
		}
	}
	// Close the seed handle so NewApp opens its own connection without
	// fighting over the SQLite WAL writer.
	if sqlDB, err := seedDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	// Phase 2: construct an Env literal directly and call NewApp. AppEnv is
	// "test" so NewApp does not mutate the process-global gin mode, which
	// keeps this test safe to run in parallel with other tests in the
	// future. MineruPath points at a binary that does not exist so any
	// extraction the worker happens to dequeue fails fast with a typed
	// extractor error rather than blocking on a real subprocess.
	env := &Env{
		AppEnv:                 "test",
		HTTPPort:               0,
		APIToken:               "test-token",
		SQLitePath:             dbPath,
		ArxivBaseURL:           "http://localhost",
		ArxivCategories:        []string{"q-fin.MF"},
		ArxivMaxResults:        1,
		ExtractionMaxWords:     50000,
		ExtractionSignalBuffer: 10,
		ExtractionJobExpiry:    time.Hour,
		MineruPath:             "/nonexistent/mineru",
		MineruTimeout:          10 * time.Minute,
	}

	// The app context is the worker's lifetime. Cancelling it after NewApp
	// returns is the documented signal for the worker goroutine to exit;
	// app.Shutdown() then blocks on Worker.Stop() until run() observes
	// ctx.Done() and closes its done channel.
	appCtx, cancelApp := context.WithCancel(context.Background())
	app, err := NewApp(appCtx, env)
	if err != nil {
		cancelApp()
		t.Fatalf("NewApp: %v", err)
	}
	if app == nil {
		cancelApp()
		t.Fatalf("NewApp returned nil app with no error")
	}

	// Phase 3: signal shutdown and assert it returns within a deadline.
	// cancelApp() releases the worker's run loop; Shutdown blocks on Stop()
	// which waits for the goroutine to exit, then closes the DB. We wrap
	// Shutdown in a goroutine + timeout so a regression that caused
	// Worker.Stop to block forever surfaces as a clear test failure rather
	// than a hung test.
	cancelApp()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- app.Shutdown(context.Background())
	}()
	select {
	case sErr := <-shutdownDone:
		if sErr != nil {
			t.Fatalf("Shutdown returned error: %v", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Shutdown did not return within 5s — Worker.Stop is blocked")
	}

	// Phase 4: re-open the DB on a fresh handle and assert the recovery
	// flip ran. The was-running row MUST now be failed: process_restart;
	// that is the headline invariant of the bootstrap recovery sequence.
	verifyDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("verify open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := verifyDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	var runningRow extractionpersist.Extraction
	if err := verifyDB.First(&runningRow, "id = ?", runningID).Error; err != nil {
		t.Fatalf("read recovered running row: %v", err)
	}
	if runningRow.Status != domainextraction.JobStatusFailed {
		t.Fatalf("running row status: got %q, want %q", runningRow.Status, domainextraction.JobStatusFailed)
	}
	if runningRow.FailureReason != domainextraction.FailureReasonProcessRestart {
		t.Fatalf("running row failure_reason: got %q, want %q", runningRow.FailureReason, domainextraction.FailureReasonProcessRestart)
	}

	// The pending row must NOT be left in `running`: either the worker
	// never picked it up before shutdown (still pending), or it was picked
	// up and processed to a terminal state (most likely failed because the
	// configured MinerU binary path does not exist). A `running` status
	// here indicates a recovery / shutdown invariant violation.
	var pendingRow extractionpersist.Extraction
	if err := verifyDB.First(&pendingRow, "id = ?", pendingID).Error; err != nil {
		t.Fatalf("read pending row: %v", err)
	}
	if pendingRow.Status == domainextraction.JobStatusRunning {
		t.Fatalf("pending row left in running after shutdown: %#v", pendingRow)
	}
}

// mustMarshalRequestPayload mirrors the persistence layer's JSON encoding so
// the seeded rows are readable by the production code path. Failure here is
// a programmer error in the test, not a domain condition.
func mustMarshalRequestPayload(t *testing.T, p domainextraction.RequestPayload) string {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal request_payload: %v", err)
	}
	return string(b)
}
