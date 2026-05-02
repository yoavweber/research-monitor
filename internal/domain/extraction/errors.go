package extraction

import (
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Caller-facing sentinels surfaced through the HTTP boundary.
var (
	ErrInvalidRequest        = shared.NewHTTPError(http.StatusBadRequest, "invalid extraction request", nil)
	ErrUnsupportedSourceType = shared.NewHTTPError(http.StatusBadRequest, "unsupported source_type", nil)
	ErrNotFound              = shared.NewHTTPError(http.StatusNotFound, "extraction not found", nil)
)

// Internal sentinels. The use case is expected to intercept ErrScannedPDF /
// ErrParseFailed / ErrExtractorFailure and persist a `failed: <reason>` row;
// the 500 tag is the fallback rendering when one of these leaks past the use
// case unintentionally.
var (
	ErrCatalogueUnavailable = shared.NewHTTPError(http.StatusInternalServerError, "extraction catalogue unavailable", nil)
	ErrInvalidTransition    = shared.NewHTTPError(http.StatusInternalServerError, "invalid extraction status transition", nil)
	ErrScannedPDF           = shared.NewHTTPError(http.StatusInternalServerError, "extractor reported no extractable text", nil)
	ErrParseFailed          = shared.NewHTTPError(http.StatusInternalServerError, "extractor reported a parse failure", nil)
	ErrExtractorFailure     = shared.NewHTTPError(http.StatusInternalServerError, "extractor failed", nil)
)
