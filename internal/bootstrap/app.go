package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	arxivinfra "github.com/yoavweber/research-monitor/backend/internal/infrastructure/arxiv"
	mineruadapter "github.com/yoavweber/research-monitor/backend/internal/infrastructure/extraction/mineru"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	extractionrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	paperpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
)

type App struct {
	Env    *Env
	DB     *gorm.DB
	Engine *gin.Engine
	Logger shared.Logger

	// extractionWorker is the long-lived background drainer for pending
	// extraction rows. Stored on App so Shutdown can call Stop() before the
	// DB handle closes; never accessed by HTTP handlers (those go through
	// route.Deps.Extraction.UseCase).
	extractionWorker *appextraction.Worker
}

func NewApp(ctx context.Context, env *Env) (*App, error) {
	logger := observability.NewLogger(env.AppEnv)

	db, err := persistence.OpenSQLite(env.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if env.AppEnv == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.ErrorEnvelope(),
	)

	// The byte fetcher owns the long-lived *http.Client (connection pooling)
	// and the User-Agent arXiv sees on every outbound call. Contact URL in
	// the UA is a courtesy for arXiv's operators per their API etiquette.
	byteFetcher := httpclient.NewByteFetcher(
		15*time.Second,
		"defi-monitor/1.0 (+https://github.com/yoavweber/research-monitor/backend)",
	)
	arxivFetcher := arxivinfra.NewArxivFetcher(env.ArxivBaseURL, byteFetcher)
	// Query is assembled once at startup so every request against this
	// process sees the same validated category list and max_results.
	query := paper.Query{
		Categories: env.ArxivCategories,
		MaxResults: env.ArxivMaxResults,
	}

	// One persisted catalogue is shared by every router: the arxiv use case
	// (fetch+persist) and the source-neutral /api/papers reads must observe
	// the same rows, so the repo is constructed once here and threaded in.
	paperRepo := paperpersist.NewRepository(db)

	// Extraction composition: extractor → repository → wake channel → use
	// case → worker. The wake channel is owned here at bootstrap so the
	// send-side (use case) and receive-side (worker) observe the same buffer.
	// Buffer size comes from EXTRACTION_SIGNAL_BUFFER and bounds the number
	// of in-flight pickup signals; the worker drains every pending row on
	// each wake, so a full buffer is harmless — extra signals are dropped
	// non-blockingly by both Submit and the on-startup self-signal below.
	mineruExtractor := mineruadapter.NewMineruExtractor(env.MineruPath, env.MineruTimeout)
	extractionRepo := extractionrepo.NewRepository(db)
	wakeCh := make(chan struct{}, env.ExtractionSignalBuffer)
	extractionUseCase := appextraction.NewExtractionUseCase(
		extractionRepo,
		mineruExtractor,
		logger,
		shared.SystemClock{},
		wakeCh,
		env.ExtractionMaxWords,
	)
	extractionWorker := appextraction.NewWorker(
		extractionRepo,
		extractionUseCase,
		logger,
		shared.SystemClock{},
		wakeCh,
		env.ExtractionJobExpiry,
	)

	// Startup recovery flip: every row left in `running` from a prior
	// process exit is transitioned to `failed: process_restart` BEFORE the
	// worker starts and BEFORE any HTTP request is served. A failure here
	// is fail-fast — Requirement 6.6 — because a partially-recovered
	// catalogue could re-run an extraction whose state we lost track of.
	recovered, err := extractionRepo.RecoverRunningOnStartup(ctx)
	if err != nil {
		return nil, fmt.Errorf("extraction recover on startup: %w", err)
	}
	if recovered > 0 {
		logger.InfoContext(ctx, "extraction.recovery.flipped", "count", recovered)
	}

	// Self-signal one wake per pre-existing pending row so the worker
	// drains the queue without operator action. Non-blocking sends mirror
	// the use-case-side semantics: the channel is empty and the buffer is
	// sized for the typical pending count, so all sends should land — but
	// the non-blocking pattern protects against a misconfigured tiny buffer.
	pendingIDs, err := extractionRepo.ListPendingIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("extraction list pending on startup: %w", err)
	}
	for range pendingIDs {
		select {
		case wakeCh <- struct{}{}:
		default:
		}
	}

	// Worker starts AFTER recovery + self-signal so the goroutine's first
	// drain pass picks up the seeded backlog. The worker holds the receive
	// end of wakeCh; cancellation of ctx is the sole exit signal.
	extractionWorker.Start(ctx)

	api := engine.Group("/api", middleware.APIToken(env.APIToken))
	route.Setup(route.Deps{
		Group:  api,
		DB:     db,
		Logger: logger,
		Clock:  shared.SystemClock{},
		Arxiv: route.ArxivConfig{
			Fetcher: arxivFetcher,
			Query:   query,
		},
		Paper: route.PaperConfig{Repo: paperRepo},
		Extraction: route.ExtractionConfig{
			Repo:    extractionRepo,
			UseCase: extractionUseCase,
			Worker:  extractionWorker,
		},
	})

	return &App{
		Env:              env,
		DB:               db,
		Engine:           engine,
		Logger:           logger,
		extractionWorker: extractionWorker,
	}, nil
}

// Shutdown drains the extraction worker and closes the DB connection.
// Bootstrap callers (cmd/api/main.go) invoke this on SIGTERM / interrupt
// AFTER cancelling the app context they passed to NewApp; cancelling that
// context is what signals the worker goroutine to exit. Shutdown blocks on
// Worker.Stop() until the goroutine returns, then closes the DB so an
// in-flight Process call observes either a usable connection or its own
// ctx cancellation — never a half-closed handle.
//
// In-flight extractions whose ctx is cancelled here return without writing;
// those rows stay in `running` and the next process boot's
// RecoverRunningOnStartup flips them to failed: process_restart.
func (a *App) Shutdown(ctx context.Context) error {
	if a.extractionWorker != nil {
		a.extractionWorker.Stop()
	}
	if a.DB != nil {
		sqlDB, err := a.DB.DB()
		if err != nil {
			return fmt.Errorf("get sql.DB: %w", err)
		}
		return sqlDB.Close()
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", a.Env.HTTPPort)
	a.Logger.InfoContext(ctx, "http.listen", "addr", addr, "env", a.Env.AppEnv)
	return a.Engine.Run(addr)
}
