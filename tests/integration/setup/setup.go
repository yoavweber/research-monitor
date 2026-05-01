//go:build integration || manual || mineru

package setup

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gormlogger "gorm.io/gorm/logger"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	extractionrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	paperrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	sourcerepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	"github.com/yoavweber/research-monitor/backend/internal/http/controller"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
)

const TestToken = "test-token"

// TestEnvOpts lets integration tests inject replacements for collaborators
// that the harness would otherwise omit. Zero value is valid: callers pass
// only the fields they care about.
type TestEnvOpts struct {
	// ArxivFetcher, if non-nil, causes the harness to wire the arxiv fetch
	// route onto the /api group using this fetcher together with ArxivQuery.
	// Leaving it nil keeps the harness arxiv-free (matches prior behavior).
	ArxivFetcher paper.Fetcher
	// ArxivQuery is the immutable query passed to the use case. Zero value
	// is fine when ArxivFetcher is nil.
	ArxivQuery paper.Query
	// PaperRepo, if non-nil, replaces the real SQLite-backed repository the
	// harness would otherwise build. The single repo instance is threaded
	// through both PaperRouter (read endpoints) and ArxivRouter (fetch +
	// persist), so failure-injection covers the full /api/papers and
	// /api/arxiv/fetch surface area for R5.5.
	PaperRepo paper.Repository

	// Extractor, if non-nil, causes the harness to wire the extraction
	// stack (repository -> use case -> worker -> controller) using this
	// fake in place of the production MinerU adapter. The extraction routes
	// (POST /api/extractions, GET /api/extractions/:id) are mounted only
	// when this is non-nil so non-extraction tests retain their existing
	// behavior. Used by Task 5.1's hermetic integration suite.
	Extractor extraction.Extractor

	// ExtractionMaxWords overrides the post-normalize word-count threshold
	// used by the extraction use case. Zero (the default) is replaced with
	// 50000 to mirror the bootstrap default; tests that exercise the
	// too_large failure path set this to a small value (e.g. 1).
	ExtractionMaxWords int

	// ExtractionJobExpiry overrides the worker's pickup-time expiry
	// duration. Zero is replaced with 1 hour to mirror the bootstrap
	// default; the hermetic suite never relies on the expiry path so this
	// is effectively a passthrough for symmetry with bootstrap.
	ExtractionJobExpiry time.Duration

	// ExtractionSignalBuffer overrides the wake-channel buffer size. Zero
	// is replaced with 10 to mirror the bootstrap default.
	ExtractionSignalBuffer int
}

type TestEnv struct {
	Server   *httptest.Server
	SourceUC domain.UseCase
	// ArxivFetcher is the fetcher supplied via TestEnvOpts, re-exposed so
	// tests can read Invocations/Queries without keeping a separate handle.
	// Nil when the arxiv route is not wired.
	ArxivFetcher paper.Fetcher
	// PaperRepo is the repository — either the harness-built real one or
	// the caller-injected fake — that ultimately backs every /api/papers
	// and /api/arxiv/fetch call. Exposed so tests can assert persisted
	// state (real repo) or recorded invocations (injected fake) directly.
	PaperRepo paper.Repository

	// ExtractionRepo is the GORM-backed extraction.Repository wired by the
	// harness when Opts.Extractor is non-nil. Tests can read it to assert
	// persisted row state directly without going through HTTP.
	ExtractionRepo extraction.Repository
	// ExtractionUseCase is the wired use case backing the extraction
	// routes; exposed so tests can drive Submit / Get without HTTP if a
	// case calls for it (the hermetic suite uses HTTP exclusively).
	ExtractionUseCase extraction.UseCase

	Close func()
}

// SetupTestEnv builds an in-memory test server with the standard middleware
// stack and /api group. Passing a TestEnvOpts{ArxivFetcher: ...} additionally
// wires the arxiv fetch route under the same authenticated /api group, so
// tests exercise the real auth path (requirement 1.2). The /api/papers read
// endpoints are always wired; pass TestEnvOpts.PaperRepo to substitute a
// failing fake (requirement 5.5).
func SetupTestEnv(t *testing.T, opts ...TestEnvOpts) *TestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var o TestEnvOpts
	if len(opts) > 0 {
		o = opts[0]
	}

	dir := t.TempDir()
	db, err := persistence.OpenSQLite(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Silence gorm in tests — production keeps its Warn-level logger.
	db.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	logger := observability.NewLogger("test")
	clock := shared.SystemClock{}

	// Build a real repository over the harness's SQLite DB by default. A
	// caller-supplied PaperRepo wins verbatim — failure-injection tests rely
	// on the injected instance reaching both routers unchanged.
	repo := o.PaperRepo
	if repo == nil {
		repo = paperrepo.NewRepository(db)
	}

	srcRepo := sourcerepo.NewRepository(db)
	uc := application.NewSourceUseCase(srcRepo, clock)
	sourceCtrl := controller.NewSourceController(uc)

	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Logger(logger), middleware.Recovery(logger), middleware.ErrorEnvelope())
	api := engine.Group("/api", middleware.APIToken(TestToken))

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, common.Data(gin.H{"status": "ok"}))
	})
	g := api.Group("/sources")
	g.POST("", sourceCtrl.Create)
	g.GET("", sourceCtrl.List)
	g.GET("/:id", sourceCtrl.Get)
	g.PATCH("/:id", sourceCtrl.Update)
	g.DELETE("/:id", sourceCtrl.Delete)

	// Deps is assembled once and reused for both routers so the same repo
	// instance backs the catalogue read endpoints and the arxiv fetch+persist
	// orchestrator — exactly the production wiring shape from bootstrap.
	deps := route.Deps{
		Group:  api,
		DB:     db,
		Logger: logger,
		Clock:  clock,
		Arxiv: route.ArxivConfig{
			Fetcher: o.ArxivFetcher,
			Query:   o.ArxivQuery,
		},
		Paper: route.PaperConfig{Repo: repo},
	}

	route.PaperRouter(deps)
	if o.ArxivFetcher != nil {
		route.ArxivRouter(deps)
	}

	// Extraction wiring is opt-in via TestEnvOpts.Extractor so existing
	// tests retain their old surface area. When the fake is supplied we
	// compose the extraction stack manually (mirroring bootstrap.NewApp)
	// with the fake substituted at the Extractor seam — bootstrap stays
	// unchanged. The worker is started under a harness-owned context that
	// the Close hook cancels so each test's worker goroutine exits before
	// the test server shuts down.
	var (
		extractionRepo    extraction.Repository
		extractionUseCase extraction.UseCase
		extractionWorker  *appextraction.Worker
	)
	// workerStop, when non-nil, cancels the worker context and blocks on
	// Worker.Stop. Wired into the Close hook below.
	var workerStop func()
	if o.Extractor != nil {
		maxWords := o.ExtractionMaxWords
		if maxWords == 0 {
			maxWords = 50000
		}
		jobExpiry := o.ExtractionJobExpiry
		if jobExpiry == 0 {
			jobExpiry = time.Hour
		}
		signalBuffer := o.ExtractionSignalBuffer
		if signalBuffer == 0 {
			signalBuffer = 10
		}

		extractionRepo = extractionrepo.NewRepository(db)
		notifier := appextraction.NewChannelNotifier(signalBuffer)
		extractionUseCase = appextraction.NewExtractionUseCase(
			extractionRepo,
			o.Extractor,
			logger,
			clock,
			notifier,
			maxWords,
		)
		extractionWorker = appextraction.NewWorker(
			extractionRepo,
			extractionUseCase,
			logger,
			clock,
			notifier.C(),
			jobExpiry,
		)
		deps.Extraction = route.ExtractionConfig{
			Repo:    extractionRepo,
			UseCase: extractionUseCase,
			Worker:  extractionWorker,
		}
		route.ExtractionRouter(deps)

		workerCtx, cancel := context.WithCancel(context.Background())
		extractionWorker.Start(workerCtx)
		workerStop = func() {
			cancel()
			extractionWorker.Stop()
		}
	}

	srv := httptest.NewServer(engine)
	closeFn := srv.Close
	if workerStop != nil {
		// Stop the worker before tearing down the HTTP server so an
		// in-flight Process call observes its own ctx cancellation rather
		// than a half-closed handle (Critical Issue 1 resolution mirrored
		// at the test-harness level).
		closeFn = func() {
			workerStop()
			srv.Close()
		}
	}

	return &TestEnv{
		Server:            srv,
		SourceUC:          uc,
		ArxivFetcher:      o.ArxivFetcher,
		PaperRepo:         repo,
		ExtractionRepo:    extractionRepo,
		ExtractionUseCase: extractionUseCase,
		Close:             closeFn,
	}
}
