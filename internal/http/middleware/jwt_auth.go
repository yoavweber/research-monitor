package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Reason codes emitted by the JWT auth middleware. Kept local to the
// middleware file: a future task (auth controller) may centralize these
// alongside the other auth reason taxonomy if the controller needs to
// reference them. Until then duplication is cheaper than premature sharing.
const (
	reasonCredentialsMissing   = "credentials_missing"
	reasonCredentialsMalformed = "credentials_malformed"
	reasonInvalidAccessToken   = "invalid_access_token"
	reasonExpiredAccessToken   = "expired_access_token"
	reasonMalformedAccessToken = "malformed_access_token"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// JWTAuth verifies a bearer token on each protected request and stores the
// authenticated user id under "user_id" on the Gin context. Verification is
// signature-and-expiry only — the middleware does not consult any database or
// remote service. Errors are emitted through the standard error envelope:
// c.Error(*shared.HTTPError) so the ErrorEnvelope middleware mounted upstream
// renders a uniform JSON body with error.details.reason set to one of the
// stable reason codes declared above.
func JWTAuth(validator shared.TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader(authorizationHeader)
		if header == "" {
			abortWith(c, "missing authorization header", reasonCredentialsMissing, nil)
			return
		}
		if !strings.HasPrefix(header, bearerPrefix) {
			abortWith(c, "malformed authorization header", reasonCredentialsMalformed, nil)
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)
		if token == "" {
			abortWith(c, "malformed authorization header", reasonCredentialsMalformed, nil)
			return
		}

		subject, err := validator.Verify(token)
		if err != nil {
			abortWith(c, "invalid access token", reasonForVerifyError(err), err)
			return
		}

		// Defensive: Issue rejects empty subjects, but a token minted outside
		// the production path (or one carrying a subject in the wrong shape)
		// could still parse. Treat any non-UUID subject as an invalid token
		// rather than panicking downstream when a handler reads user_id.
		userID, err := uuid.Parse(subject)
		if err != nil {
			abortWith(c, "invalid access token", reasonInvalidAccessToken, err)
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// reasonForVerifyError collapses TokenValidator errors onto the public reason
// codes. Any unrecognized error defaults to invalid_access_token so a future
// adapter that introduces a new failure mode degrades safely rather than
// leaking through as a non-auth status.
func reasonForVerifyError(err error) string {
	switch {
	case errors.Is(err, shared.ErrTokenExpired):
		return reasonExpiredAccessToken
	case errors.Is(err, shared.ErrTokenMalformed):
		return reasonMalformedAccessToken
	case errors.Is(err, shared.ErrTokenSignatureInvalid):
		return reasonInvalidAccessToken
	default:
		return reasonInvalidAccessToken
	}
}

// abortWith records an *shared.HTTPError on the Gin context for the
// ErrorEnvelope middleware to render and stops the handler chain. The cause
// is preserved on the HTTPError so logs further up the stack can include it,
// while the public envelope only exposes the reason string.
func abortWith(c *gin.Context, message, reason string, cause error) {
	_ = c.Error(shared.NewHTTPError(http.StatusUnauthorized, message, cause).WithReason(reason))
	c.Abort()
}
