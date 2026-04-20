//go:build integration

package setup

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/application"
	arxivapp "github.com/yoavweber/defi-monitor-backend/internal/application/arxiv"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/infrastructure/observability"
	persistence "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence"
	sourcerepo "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence/source"
	"github.com/yoavweber/defi-monitor-backend/internal/http/common"
	arxivctrl "github.com/yoavweber/defi-monitor-backend/internal/http/controller/arxiv"
	"github.com/yoavweber/defi-monitor-backend/internal/http/controller"
	"github.com/yoavweber/defi-monitor-backend/internal/http/middleware"
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
}

type TestEnv struct {
	Server   *httptest.Server
	SourceUC domain.UseCase
	// ArxivFetcher is the fetcher supplied via TestEnvOpts, re-exposed so
	// tests can read Invocations/Queries without keeping a separate handle.
	// Nil when the arxiv route is not wired.
	ArxivFetcher paper.Fetcher
	Close        func()
}

// SetupTestEnv builds an in-memory test server with the standard middleware
// stack and /api group. Passing a TestEnvOpts{ArxivFetcher: ...} additionally
// wires the arxiv fetch route under the same authenticated /api group, so
// tests exercise the real auth path (requirement 1.2).
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

	if o.ArxivFetcher != nil {
		arxivUC := arxivapp.NewArxivUseCase(o.ArxivFetcher, logger, o.ArxivQuery)
		ctrl := arxivctrl.NewArxivController(arxivUC, clock)
		a := api.Group("/arxiv")
		a.GET("/fetch", ctrl.Fetch)
	}

	srv := httptest.NewServer(engine)
	return &TestEnv{
		Server:       srv,
		SourceUC:     uc,
		ArxivFetcher: o.ArxivFetcher,
		Close:        srv.Close,
	}
}
