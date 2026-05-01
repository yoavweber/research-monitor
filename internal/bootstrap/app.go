package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

	// Swagger UI is mounted outside the /api group so it is not gated by the
	// APIToken middleware, and only in non-prod environments — production must
	// not expose the schema or the UI.
	if env.AppEnv != "prod" {
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

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

	// Extraction composition: extractor → repository → notifier → use case
	// → worker. The Notifier owns the buffered wake channel so the use case
	// only sees the publish port and the worker only sees the receive side.
	// Buffer size comes from EXTRACTION_SIGNAL_BUFFER and bounds the number
	// of in-flight pickup signals; the worker drains every pending row on
	// each wake, so a full buffer is harmless — Notify drops the extra.
	mineruExtractor := mineruadapter.NewMineruExtractor(env.MineruPath, env.MineruTimeout)
	extractionRepo := extractionrepo.NewRepository(db)
	extractionNotifier := appextraction.NewChannelNotifier(env.ExtractionSignalBuffer)
	extractionUseCase := appextraction.NewExtractionUseCase(
		extractionRepo,
		mineruExtractor,
		logger,
		shared.SystemClock{},
		extractionNotifier,
		env.ExtractionMaxWords,
	)
	extractionWorker := appextraction.NewWorker(
		extractionRepo,
		extractionUseCase,
		logger,
		shared.SystemClock{},
		extractionNotifier.C(),
		env.ExtractionJobExpiry,
	)

	// Recover running rows from a prior process exit BEFORE the worker
	// starts and BEFORE the first HTTP request is served. Fail-fast: a
	// partially-recovered catalogue could re-run an extraction whose state
	// we lost track of.
	recovered, err := extractionRepo.RecoverRunningOnStartup(ctx)
	if err != nil {
		return nil, fmt.Errorf("extraction recover on startup: %w", err)
	}
	if recovered > 0 {
		logger.InfoContext(ctx, "extraction.recovery.flipped", "count", recovered)
	}

	// One wake is enough: the worker drains the entire queue per signal,
	// re-checking the DB after every Process. Self-signal kicks the
	// goroutine into its first drain pass.
	extractionNotifier.Notify(ctx)

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
