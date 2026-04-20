package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	arxivinfra "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/arxiv"
	httpinfra "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/http"
	"github.com/yoavweber/defi-monitor-backend/internal/infrastructure/observability"
	"github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/middleware"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/route"
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

	// The byte fetcher owns the long-lived *http.Client (connection pooling)
	// and the User-Agent arXiv sees on every outbound call. Contact URL in
	// the UA is a courtesy for arXiv's operators per their API etiquette.
	byteFetcher := httpinfra.NewByteFetcher(
		15*time.Second,
		"defi-monitor/1.0 (+https://github.com/yoavweber/defi-monitor-backend)",
	)
	arxivFetcher := arxivinfra.NewArxivFetcher(env.ArxivBaseURL, byteFetcher)
	// Query is assembled once at startup so every request against this
	// process sees the same validated category list and max_results.
	query := paper.Query{
		Categories: env.ArxivCategories,
		MaxResults: env.ArxivMaxResults,
	}

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
	})

	return &App{Env: env, DB: db, Engine: engine, Logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", a.Env.HTTPPort)
	a.Logger.InfoContext(ctx, "http.listen", "addr", addr, "env", a.Env.AppEnv)
	return a.Engine.Run(addr)
}
