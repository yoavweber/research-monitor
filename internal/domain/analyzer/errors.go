package analyzer

import (
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Reason discriminator strings surface under error.details.reason on the
// wire. The two 502 modes share an HTTP code, so callers must use these
// strings to tell them apart.
const (
	reasonExtractionNotFound = "extraction_not_found"
	reasonExtractionNotReady = "extraction_not_ready"
	reasonLLMUpstream        = "llm_upstream"
	reasonLLMMalformed       = "llm_malformed_response"
	reasonAnalysisNotFound   = "analysis_not_found"
)

var (
	ErrExtractionNotFound = shared.NewHTTPError(
		http.StatusNotFound, "extraction not found", nil,
	).WithReason(reasonExtractionNotFound)

	ErrExtractionNotReady = shared.NewHTTPError(
		http.StatusConflict, "extraction not in done status", nil,
	).WithReason(reasonExtractionNotReady)

	ErrLLMUpstream = shared.NewHTTPError(
		http.StatusBadGateway, "llm upstream failed", nil,
	).WithReason(reasonLLMUpstream)

	ErrAnalyzerMalformedResponse = shared.NewHTTPError(
		http.StatusBadGateway, "llm response did not satisfy thesis envelope", nil,
	).WithReason(reasonLLMMalformed)

	ErrAnalysisNotFound = shared.NewHTTPError(
		http.StatusNotFound, "analysis not found", nil,
	).WithReason(reasonAnalysisNotFound)

	ErrCatalogueUnavailable = shared.NewHTTPError(
		http.StatusInternalServerError, "analysis storage unavailable", nil,
	)

	ErrInvalidRequest = shared.NewHTTPError(
		http.StatusBadRequest, "invalid analyzer request", nil,
	)
)
