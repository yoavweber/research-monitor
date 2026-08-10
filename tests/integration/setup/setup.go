//go:build integration || manual || mineru

package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	appanalyzer "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	apparxiv "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	apppdfdownload "github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	analyzerdomain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
	userdomain "github.com/yoavweber/research-monitor/backend/internal/domain/user"
	authinfra "github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
	llmstub "github.com/yoavweber/research-monitor/backend/internal/infrastructure/llm/stub"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	pdflocal "github.com/yoavweber/research-monitor/backend/internal/infrastructure/pdf/local"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	analyzerrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/analyzer"
	extractionrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	paperrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	sourcerepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	"github.com/yoavweber/research-monitor/backend/internal/http/controller"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

const TestToken = "test-token"

// testBcryptCost keeps SeedTestUser fast — production uses cost 12, but
// every integration test that logs in would otherwise pay that cost on
// every run. Mirrors the cost used by bcrypt_hasher_test.go.
const testBcryptCost = 4

// TestUserEmail and TestUserPassword are the deterministic credentials
// SeedTestUser provisions and LoginAsTestUser authenticates with.
const (
	TestUserEmail    = "integration-test@example.com"
	TestUserPassword = "integration-test-password-1"
)

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

	// WireAnalyzer toggles the llm-analyzer slice. When true, the harness
	// constructs the analyzer repository over the shared SQLite DB and the
	// production fake LLMClient, builds the use case, and registers
	// /api/analyses on the same /api group. The analyzer reads the
	// extraction repo for body markdown, so meaningful analyzer tests
	// should also seed the extractions table directly via TestEnv.DB.
	WireAnalyzer bool

	// WirePDFDownload toggles the PDF-download slice. When true, the
	// harness constructs an in-process pdfdownload.Registry (rooted at a
	// temp dir for the underlying pdf.Store) and wires it as both
	// ArxivConfig.Scheduler and DownloadConfig.Reader, mounts the
	// status + SSE routes, and registers shutdown into Close. The arxiv
	// route is also wired automatically when WirePDFDownload is true
	// (the download flow has no purpose without the trigger), so callers
	// should also supply ArxivFetcher.
	WirePDFDownload bool

	// PDFDownloadRetention overrides the registry's retention window.
	// Zero defaults to 5 minutes to mirror bootstrap.
	PDFDownloadRetention time.Duration

	// PDFDownloadSubscriberBuf overrides the per-subscriber channel
	// capacity. Zero defaults to 32 to mirror bootstrap.
	PDFDownloadSubscriberBuf int

	// AuthJWTTTL overrides the harness's JWT lifetime. Zero defaults to 24h
	// to mirror bootstrap. Tests that need an already-expired token instead
	// advance TestEnv.AuthClock past whatever TTL is in effect, so this
	// exists for completeness rather than being the primary expiry lever.
	AuthJWTTTL time.Duration
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

	// AnalyzerRepo is the analyzer.Repository wired by the harness when
	// Opts.WireAnalyzer is true. Exposed so tests can read persisted state
	// directly without going through HTTP.
	AnalyzerRepo analyzerdomain.Repository
	// AnalyzerUseCase is the wired analyzer use case backing the
	// /api/analyses routes when Opts.WireAnalyzer is true.
	AnalyzerUseCase analyzerdomain.UseCase
	// DB is the shared SQLite handle, exposed so tests can seed dependent
	// rows (e.g. extractions whose body the analyzer reads) without
	// re-opening the database.
	DB *gorm.DB

	// PDFDownloadRegistry is exposed for retention/eviction tests so
	// task 5.3 can invoke Sweep directly. Nil when WirePDFDownload is
	// false.
	PDFDownloadRegistry *apppdfdownload.Registry

	// PDFStoreRoot is the on-disk root used by the PDF store. Empty when
	// WirePDFDownload is false. Exposed so live manual tests can verify
	// and explicitly delete downloaded artifacts under the canonical
	// `<root>/<source>/<key>.pdf` layout.
	PDFStoreRoot string

	// AuthClock is the movable clock backing the harness's JWT signer and
	// validator. Tests exercising expiry (task 5.2) advance it past a
	// token's exp claim to simulate the passage of time deterministically.
	AuthClock *mocks.MovableClock

	// authToken caches the token obtained by the first AuthorizedRequest
	// call in a test, so repeated calls don't re-login. Set via
	// LoginAsTestUser; access only through AuthorizedRequest.
	authToken string

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
	rootGroup := engine.Group("/")

	// Auth is wired unconditionally (not opt-in) so SeedTestUser,
	// LoginAsTestUser, and AuthorizedRequest work in every test without each
	// call site remembering to ask for it. The clock is a MovableClock (not
	// SystemClock) so task 5.2's expired-token scenario can advance time
	// deterministically instead of sleeping past a real TTL.
	authClock := mocks.NewMovableClock(time.Now())
	jwtTTL := o.AuthJWTTTL
	if jwtTTL == 0 {
		jwtTTL = 24 * time.Hour
	}
	jwtService := authinfra.NewJWTTokenService(authinfra.JWTConfig{
		Secret: []byte(testJWTSecret),
		TTL:    jwtTTL,
		Clock:  authClock,
	})
	hasher := authinfra.NewBcryptHasher(testBcryptCost)

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, common.Data(gin.H{"status": "ok"}))
	})
	g := api.Group("/sources")
	g.POST("", sourceCtrl.Create)
	g.GET("", sourceCtrl.List)
	g.GET("/:id", sourceCtrl.Get)
	g.PATCH("/:id", sourceCtrl.Update)
	g.DELETE("/:id", sourceCtrl.Delete)

	// PDF-download wiring is opt-in via TestEnvOpts.WirePDFDownload. The
	// harness mirrors bootstrap.NewApp: a real on-disk pdf.Store rooted at
	// a temp dir plus the real byte fetcher, so the worker performs an
	// actual HTTP GET against whichever test httptest.Server the caller
	// stands up. One Registry value satisfies both ArxivConfig.Scheduler
	// (write side) and DownloadConfig.Reader (read side); ShutdownFunc is
	// chained into closeFn so in-flight downloads drain before the HTTP
	// server tears down.
	//
	// When WirePDFDownload is false but the arxiv route is wired, the
	// harness injects a recording PaperPDFScheduler fake so the arxiv use
	// case's Scheduler.Schedule call does not blow up on a nil
	// interface. The fake records the requests but performs no I/O —
	// pre-existing arxiv-only tests stay hermetic.
	var (
		pdfDownloadRegistry *apppdfdownload.Registry
		pdfDownloadShutdown apppdfdownload.ShutdownFunc
		arxivScheduler      apparxiv.DownloadScheduler
		downloadReader      paperctrl.PDFDownloadReader
		pdfStoreRoot        string
	)
	if o.WirePDFDownload {
		retention := o.PDFDownloadRetention
		if retention == 0 {
			retention = 5 * time.Minute
		}
		subscriberBuf := o.PDFDownloadSubscriberBuf
		if subscriberBuf == 0 {
			subscriberBuf = 32
		}
		// Per-test root keeps file writes isolated. The byte fetcher's
		// timeout is generous; integration tests serve from an in-process
		// httptest.Server so transport latency is negligible.
		pdfStoreRoot = filepath.Join(dir, "pdfstore")
		byteFetcher := httpclient.NewByteFetcher(15*time.Second, "research-monitor-test/1.0")
		store, err := pdflocal.NewStore(pdfStoreRoot, byteFetcher, logger)
		if err != nil {
			t.Fatalf("pdf local store: %v", err)
		}
		pdfDownloadRegistry, pdfDownloadShutdown = apppdfdownload.NewRegistry(
			store,
			logger,
			clock,
			apppdfdownload.Options{
				Retention:        retention,
				SubscriberBuffer: subscriberBuf,
			},
		)
		arxivScheduler = pdfDownloadRegistry
		downloadReader = pdfDownloadRegistry
	} else if o.ArxivFetcher != nil {
		// Arxiv use case unconditionally calls Scheduler.Schedule after a
		// successful persist; a nil scheduler panics. The recording fake
		// satisfies the port without performing any I/O.
		arxivScheduler = mocks.NewPDFDownloadScheduler()
	}

	// Deps is assembled once and reused for both routers so the same repo
	// instance backs the catalogue read endpoints and the arxiv fetch+persist
	// orchestrator — exactly the production wiring shape from bootstrap.
	deps := route.Deps{
		Group:     api,
		RootGroup: rootGroup,
		DB:        db,
		Logger:    logger,
		Clock:     clock,
		Hasher:    hasher,
		Signer:    jwtService,
		Validator: jwtService,
		JWTTTL:    jwtTTL,
		Arxiv: route.ArxivConfig{
			Fetcher:   o.ArxivFetcher,
			Query:     o.ArxivQuery,
			Scheduler: arxivScheduler,
		},
		Paper:    route.PaperConfig{Repo: repo},
		Download: route.DownloadConfig{Reader: downloadReader},
	}

	route.AuthRouter(deps)
	route.PaperRouter(deps)
	if o.ArxivFetcher != nil {
		route.ArxivRouter(deps)
	}
	if o.WirePDFDownload {
		route.PDFDownloadRouter(deps)
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

	// Analyzer wiring is opt-in via TestEnvOpts.WireAnalyzer. The harness
	// constructs the analyzer repo over the shared DB and the production
	// fake LLMClient, mirroring bootstrap. The analyzer needs an
	// extraction repo to read body markdown from, so we lazily build one
	// here when WireAnalyzer is true and the extraction wiring path above
	// did not already construct it.
	var (
		analyzerRepo    analyzerdomain.Repository
		analyzerUseCase analyzerdomain.UseCase
	)
	if o.WireAnalyzer {
		extRepoForAnalyzer := extractionRepo
		if extRepoForAnalyzer == nil {
			extRepoForAnalyzer = extractionrepo.NewRepository(db)
		}
		analyzerRepo = analyzerrepo.NewRepository(db)
		analyzerUseCase = appanalyzer.NewAnalyzerUseCase(
			analyzerRepo,
			extRepoForAnalyzer,
			llmstub.New(),
			logger,
			clock,
		)
		deps.Analyzer = route.AnalyzerConfig{UseCase: analyzerUseCase}
		route.AnalyzerRouter(deps)
	}

	srv := httptest.NewServer(engine)
	// closeFn chains optional shutdown hooks (extraction worker, pdfdownload
	// registry) ahead of the HTTP server teardown so background goroutines
	// owned by the harness observe their own cancellation rather than racing
	// the server close.
	closeFn := func() {
		if workerStop != nil {
			workerStop()
		}
		if pdfDownloadShutdown != nil {
			// Bound the drain by a short context: v1 workers ignore ctx
			// (R3.5), so cancellation cannot interrupt them; the deadline
			// only caps the test's wait. Integration scenarios complete
			// within milliseconds so the bound is comfortable.
			drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = pdfDownloadShutdown(drainCtx)
			cancel()
		}
		srv.Close()
	}

	return &TestEnv{
		Server:              srv,
		SourceUC:            uc,
		ArxivFetcher:        o.ArxivFetcher,
		PaperRepo:           repo,
		ExtractionRepo:      extractionRepo,
		ExtractionUseCase:   extractionUseCase,
		AnalyzerRepo:        analyzerRepo,
		AnalyzerUseCase:     analyzerUseCase,
		DB:                  db,
		PDFDownloadRegistry: pdfDownloadRegistry,
		PDFStoreRoot:        pdfStoreRoot,
		AuthClock:           authClock,
		Close:               closeFn,
	}
}

// testJWTSecret is a fixed, non-secret signing key used only by the
// integration-test harness. It satisfies the ≥32-byte minimum bootstrap
// enforces on AUTH_JWT_SECRET in production.
const testJWTSecret = "integration-test-harness-signing-key-not-secret"

// SeedTestUser inserts the deterministic test user directly through the
// user repository, bypassing HTTP, for tests that only need a valid
// principal to exist and don't care about the login round trip. Idempotent:
// a second call in the same test env is a no-op rather than a failure, so
// callers (including LoginAsTestUser) don't need to track whether seeding
// already happened.
func SeedTestUser(t *testing.T, env *TestEnv) {
	t.Helper()

	hash, err := authinfra.NewBcryptHasher(testBcryptCost).Hash(TestUserPassword)
	if err != nil {
		t.Fatalf("hash test user password: %v", err)
	}

	repo := userpersist.NewRepository(env.DB)
	now := time.Now().UTC()
	u := &userdomain.User{
		ID:           uuid.New(),
		Email:        TestUserEmail,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = repo.Save(context.Background(), u)
	if err == nil || errors.Is(err, userdomain.ErrEmailExists) {
		return
	}
	t.Fatalf("seed test user: %v", err)
}

// LoginAsTestUser seeds the deterministic test user (idempotent) and logs in
// through the real POST /auth/login endpoint, returning the issued access
// token. Exercising the real endpoint — rather than minting a token directly
// — means tests get the same signature/claims path production traffic does.
func LoginAsTestUser(t *testing.T, env *TestEnv) string {
	t.Helper()

	SeedTestUser(t, env)

	body, err := json.Marshal(userdomain.LoginRequest{
		Email:    TestUserEmail,
		Password: TestUserPassword,
	})
	if err != nil {
		t.Fatalf("marshal login request: %v", err)
	}

	resp, err := http.Post(env.Server.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope struct {
		Data userdomain.LoginResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return envelope.Data.AccessToken
}

// AuthorizedRequest builds an *http.Request against the test server carrying
// a valid bearer token, logging in on first use and caching the token on
// env for the rest of the test. body, when non-nil, is JSON-encoded and set
// as the request body with a matching Content-Type.
func AuthorizedRequest(t *testing.T, env *TestEnv, method, path string, body any) *http.Request {
	t.Helper()

	if env.authToken == "" {
		env.authToken = LoginAsTestUser(t, env)
	}

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, env.Server.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", env.authToken))
	return req
}
