package paper

import (
	"errors"
	"net/http"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Upstream-failure sentinels. Each value is a *shared.HTTPError so the
// existing ErrorEnvelope middleware renders the correct status code directly
// from the error, following the source.ErrNotFound pattern.
var (
	ErrUpstreamBadStatus   = shared.NewHTTPError(http.StatusBadGateway, "paper source returned non-success status", nil)
	ErrUpstreamMalformed   = shared.NewHTTPError(http.StatusBadGateway, "paper source returned malformed response", nil)
	ErrUpstreamUnavailable = shared.NewHTTPError(http.StatusGatewayTimeout, "paper source unavailable", nil)
)

var (
	ErrNotFound             = shared.NewHTTPError(http.StatusNotFound, "paper not found", nil)
	ErrCatalogueUnavailable = shared.NewHTTPError(http.StatusInternalServerError, "paper catalogue unavailable", nil)
)

// ErrInvalidID signals that a paper.ID failed value-object validation —
// empty Source/SourceID, or path-traversal characters in any identity
// component. Construction does not enforce validity; callers invoke
// (ID).Validate before using the ID to address artifacts.
var ErrInvalidID = errors.New("paper: invalid id")
