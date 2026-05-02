package analyzer

import (
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Reason discriminator strings, surfaced on the wire under
// error.details.reason via the shared HTTPError envelope. The two 502 modes
// share an HTTP code, so callers must use these strings to tell them apart.
const (
	reasonExtractionNotFound = "extraction_not_found"
	reasonExtractionNotReady = "extraction_not_ready"
	reasonLLMUpstream        = "llm_upstream"
	reasonLLMMalformed       = "llm_malformed_response"
	reasonAnalysisNotFound   = "analysis_not_found"
)

// Sentinel errors. Each wraps *shared.HTTPError so the existing ErrorEnvelope
// middleware translates them into the standard wire envelope without any
// analyzer-local HTTP plumbing. Callers identify them with errors.Is.

// ErrExtractionNotFound is returned by Analyze when the requested
// extraction id does not exist. Maps to HTTP 404.
var ErrExtractionNotFound = shared.NewHTTPError(
	http.StatusNotFound,
	"extraction not found",
	nil,
).WithReason(reasonExtractionNotFound)

// ErrExtractionNotReady is returned by Analyze when the extraction exists
// but its status is not "done", so its body markdown is not yet available.
// Maps to HTTP 409.
var ErrExtractionNotReady = shared.NewHTTPError(
	http.StatusConflict,
	"extraction not in done status",
	nil,
).WithReason(reasonExtractionNotReady)

// ErrLLMUpstream is returned when shared.LLMClient.Complete returns a
// transport-level error for any of the three calls. Maps to HTTP 502 with
// reason "llm_upstream". The underlying transport error is wrapped in Err.
var ErrLLMUpstream = shared.NewHTTPError(
	http.StatusBadGateway,
	"llm upstream failed",
	nil,
).WithReason(reasonLLMUpstream)

// ErrAnalyzerMalformedResponse is returned when the thesis-angle LLM
// response cannot be decoded as the required JSON envelope. Maps to HTTP
// 502 with reason "llm_malformed_response".
var ErrAnalyzerMalformedResponse = shared.NewHTTPError(
	http.StatusBadGateway,
	"llm response did not satisfy thesis envelope",
	nil,
).WithReason(reasonLLMMalformed)

// ErrAnalysisNotFound is returned by Get when no analysis row exists for
// the requested extraction id. Maps to HTTP 404.
var ErrAnalysisNotFound = shared.NewHTTPError(
	http.StatusNotFound,
	"analysis not found",
	nil,
).WithReason(reasonAnalysisNotFound)

// ErrCatalogueUnavailable wraps any non-not-found persistence failure. Maps
// to HTTP 500. No reason field — operators triage via logs, and the wire
// shape stays minimal.
var ErrCatalogueUnavailable = shared.NewHTTPError(
	http.StatusInternalServerError,
	"analysis storage unavailable",
	nil,
)
