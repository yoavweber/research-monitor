package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	arxivinfra "github.com/yoavweber/research-monitor/backend/internal/infrastructure/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	paperpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/http/route"
)

type App struct {
	Env    *Env
	DB     *gorm.DB
	Engine *gin.Engine
	Logger shared.Logger
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
	})

	return &App{Env: env, DB: db, Engine: engine, Logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", a.Env.HTTPPort)
	a.Logger.InfoContext(ctx, "http.listen", "addr", addr, "env", a.Env.AppEnv)
	return a.Engine.Run(addr)
}
