package extraction

import (
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Request-shape sentinels. Surfaced through the HTTP boundary as 4xx so callers
// can distinguish bad input from server-side failure without parsing strings.
var (
	ErrInvalidRequest        = shared.NewHTTPError(http.StatusBadRequest, "invalid extraction request", nil)
	ErrUnsupportedSourceType = shared.NewHTTPError(http.StatusBadRequest, "unsupported source_type", nil)
	ErrNotFound              = shared.NewHTTPError(http.StatusNotFound, "extraction not found", nil)
)

// Internal failure sentinels. These never originate from caller input; they
// signal storage/extractor problems and map to terminal extraction states.
// They are tagged 500 so the ErrorEnvelope middleware renders them as opaque
// server errors when they leak past the use-case layer, but the use case is
// expected to catch ErrScannedPDF / ErrParseFailed / ErrExtractorFailure first
// and persist the corresponding `failed: <reason>` row.
var (
	ErrCatalogueUnavailable = shared.NewHTTPError(http.StatusInternalServerError, "extraction catalogue unavailable", nil)
	ErrInvalidTransition    = shared.NewHTTPError(http.StatusInternalServerError, "invalid extraction status transition", nil)
	ErrScannedPDF           = shared.NewHTTPError(http.StatusInternalServerError, "extractor reported no extractable text", nil)
	ErrParseFailed          = shared.NewHTTPError(http.StatusInternalServerError, "extractor reported a parse failure", nil)
	ErrExtractorFailure     = shared.NewHTTPError(http.StatusInternalServerError, "extractor failed", nil)
)
