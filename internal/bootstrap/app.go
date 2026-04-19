package bootstrap

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
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

	api := engine.Group("/api", middleware.APIToken(env.APIToken))
	route.Setup(route.Deps{
		Group:  api,
		DB:     db,
		Logger: logger,
		Clock:  shared.SystemClock{},
	})

	return &App{Env: env, DB: db, Engine: engine, Logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", a.Env.HTTPPort)
	a.Logger.InfoContext(ctx, "http.listen", "addr", addr, "env", a.Env.AppEnv)
	return a.Engine.Run(addr)
}
