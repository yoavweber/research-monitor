package paper

import (
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
