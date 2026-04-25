//go:build integration

package setup

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
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
	Close     func()
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

	srv := httptest.NewServer(engine)
	return &TestEnv{
		Server:       srv,
		SourceUC:     uc,
		ArxivFetcher: o.ArxivFetcher,
		PaperRepo:    repo,
		Close:        srv.Close,
	}
}
