//go:build integration

package setup

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/application"
	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/infrastructure/observability"
	persistence "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence"
	sourcerepo "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence/source"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/common"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/controller"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/middleware"
)

const TestToken = "test-token"

type TestEnv struct {
	Server   *httptest.Server
	SourceUC domain.UseCase
	Close    func()
}

func SetupTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	db, err := persistence.OpenSQLite(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	logger := observability.NewLogger("test")
	clock := shared.SystemClock{}

	repo := sourcerepo.NewRepository(db)
	uc := application.NewSourceUseCase(repo, clock)
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

	srv := httptest.NewServer(engine)
	return &TestEnv{
		Server:   srv,
		SourceUC: uc,
		Close:    srv.Close,
	}
}
