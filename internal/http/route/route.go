package route

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// ArxivConfig is the feature-scoped sub-bundle passed through route.Deps to
// wire the arXiv fetch endpoint. Bootstrap assembles it once at startup;
// ArxivRouter reads it to construct the use case and controller locally.
type ArxivConfig struct {
	Fetcher paper.Fetcher
	Query   paper.Query
}

// PaperConfig is the feature-scoped sub-bundle for the source-neutral
// /api/papers read endpoints. The persisted paper.Repository is the only
// dependency; both the catalogue handlers and the arXiv use case share it,
// so bootstrap constructs the repo once and Deps hands it to whichever
// router needs it.
type PaperConfig struct {
	Repo paper.Repository
}

// ExtractionConfig is the feature-scoped sub-bundle for the document-extraction
// aggregate. Bootstrap assembles it once at startup; ExtractionRouter reads it
// to register the controller. The Worker handle is exposed here so route-level
// smoke tests can inspect it; production callers use it for graceful shutdown.
type ExtractionConfig struct {
	Repo    extraction.Repository
	UseCase extraction.UseCase
	Worker  *appextraction.Worker
}

// PDFConfig is the feature-scoped sub-bundle for the pdf-storage aggregate.
// Bootstrap constructs the local store once at startup; no current router
// consumes it. The follow-on document-extraction integration will read
// PDFConfig.Store to materialize PDFs before invoking the extractor, which
// is why the surface is wired now — adding it later would force a second
// bootstrap edit purely to thread one field through Deps.
type PDFConfig struct {
	Store pdf.Store
}

// Deps are the shared dependencies passed to every per-resource router.
// Per-resource routers construct their own repo → usecase → controller chains from these.
type Deps struct {
	Group      *gin.RouterGroup
	DB         *gorm.DB
	Logger     shared.Logger
	Clock      shared.Clock
	Arxiv      ArxivConfig
	Paper      PaperConfig
	PDF        PDFConfig
	Extraction ExtractionConfig
}

func Setup(d Deps) {
	HealthRouter(d)
	SourceRouter(d)
	ArxivRouter(d)
	PaperRouter(d)
	ExtractionRouter(d)
}
