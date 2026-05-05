package analyzer

import (
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

const (
	reasonExtractionNotFound = "extraction_not_found"
	reasonExtractionNotReady = "extraction_not_ready"
	reasonLLMUpstream        = "llm_upstream"
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
