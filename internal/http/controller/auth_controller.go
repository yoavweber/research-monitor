package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

// AuthController handles the three endpoints of the auth surface. It depends
// only on the user use-case port — no cookie config and no Origin allow-list
// because the single-token model dropped both.
type AuthController struct {
	uc user.UseCase
}

// NewAuthController wires the auth controller. The constructor returns a
// concrete pointer (not the port) following the convention used by sibling
// controllers in this package.
func NewAuthController(uc user.UseCase) *AuthController {
	return &AuthController{uc: uc}
}

// Login godoc
//
// @Summary      Authenticate with email and password
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        credentials  body      user.LoginRequest  true  "Login credentials"
// @Success      200          {object}  LoginEnvelope         "Issued access token and session"
// @Failure      400          {object}  common.ErrorEnvelope  "Invalid JSON body or validation error"
// @Failure      401          {object}  common.ErrorEnvelope  "Invalid credentials"
// @Router       /auth/login [post]
func (ctrl *AuthController) Login(c *gin.Context) {
	var req user.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortAuth(c, http.StatusBadRequest, "invalid json body", user.ReasonValidationFailed, err)
		return
	}

	ctx := application.WithRequestID(c.Request.Context(), c.GetString(middleware.RequestIDKey))

	result, err := ctrl.uc.Login(ctx, req)
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			abortAuth(c, http.StatusUnauthorized, "invalid credentials", user.ReasonInvalidCredentials, err)
			return
		}
		// Validation HTTPErrors from req.Validate() and any other
		// pre-typed *shared.HTTPError flow through unchanged; anything
		// else falls back to the envelope's internal-error path.
		_ = c.Error(err)
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, common.Data(user.LoginResponse{
		AccessToken: result.Token,
		ExpiresAt:   result.ExpiresAt,
		User: user.SessionResponse{
			ID:        result.User.ID,
			Email:     result.User.Email,
			CreatedAt: result.User.CreatedAt,
		},
	}))
}

// Session godoc
//
// @Summary      Return the currently authenticated user
// @Tags         Auth
// @Produce      json
// @Success      200  {object}  SessionEnvelope       "Authenticated session"
// @Failure      401  {object}  common.ErrorEnvelope  "Missing or invalid access token"
// @Security     BearerAuth
// @Router       /auth/session [get]
func (ctrl *AuthController) Session(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ctx := application.WithRequestID(c.Request.Context(), c.GetString(middleware.RequestIDKey))

	u, err := ctrl.uc.Session(ctx, userID)
	if err != nil {
		// The user existed when the token was minted but has since been
		// removed. Surface as an auth failure with the shared
		// invalid_credentials taxonomy rather than a 404 so the client
		// reacts the same as it would to any other token-time failure.
		if errors.Is(err, user.ErrNotFound) {
			abortAuth(c, http.StatusUnauthorized, "invalid credentials", user.ReasonInvalidCredentials, err)
			return
		}
		_ = c.Error(err)
		c.Abort()
		return
	}

	// SessionResponse intentionally omits PasswordHash; build it explicitly
	// from named fields so a future addition to user.User cannot leak via a
	// wider struct copy.
	c.JSON(http.StatusOK, common.Data(user.SessionResponse{
		ID:        u.ID,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}))
}

// ChangePassword godoc
//
// @Summary      Rotate the authenticated user's password
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        change  body  user.ChangePasswordRequest  true  "Current and new password"
// @Success      204     "Password updated"
// @Failure      400     {object}  common.ErrorEnvelope  "Validation error, incorrect current password, weak new password, or unchanged password"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid access token"
// @Security     BearerAuth
// @Router       /auth/change-password [post]
func (ctrl *AuthController) ChangePassword(c *gin.Context) {
	var req user.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortAuth(c, http.StatusBadRequest, "invalid json body", user.ReasonValidationFailed, err)
		return
	}

	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ctx := application.WithRequestID(c.Request.Context(), c.GetString(middleware.RequestIDKey))

	err := ctrl.uc.ChangePassword(ctx, userID, req)
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}

	switch {
	case errors.Is(err, user.ErrCurrentPasswordIncorrect):
		abortAuth(c, http.StatusBadRequest, "current password incorrect", user.ReasonCurrentPasswordIncorrect, err)
	case errors.Is(err, user.ErrPasswordPolicyViolation):
		abortAuth(c, http.StatusBadRequest, "password policy violation", user.ReasonPasswordPolicyViolation, err)
	case errors.Is(err, user.ErrPasswordUnchanged):
		abortAuth(c, http.StatusBadRequest, "new password equals current password", user.ReasonPasswordUnchanged, err)
	default:
		// Anything already shaped as *shared.HTTPError (e.g. from
		// req.Validate()) flows through with its reason intact; an
		// untyped error renders as 500 via the envelope middleware.
		_ = c.Error(err)
		c.Abort()
	}
}

// abortAuth records an *shared.HTTPError on the Gin context for the
// ErrorEnvelope middleware to render and stops the handler chain. Mirrors the
// abortWith helper in middleware/jwt_auth.go so the controller's failure shape
// stays identical to the middleware's.
func abortAuth(c *gin.Context, status int, message, reason string, cause error) {
	_ = c.Error(shared.NewHTTPError(status, message, cause).WithReason(reason))
	c.Abort()
}

// requireUserID extracts the authenticated user id placed on the Gin context
// by middleware.JWTAuth. A missing or wrong-typed value is a programmer error
// — the middleware always runs first on the routes that mount this controller
// — so the handler bails with 500 rather than coercing the request to 401.
func requireUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		_ = c.Error(shared.NewHTTPError(http.StatusInternalServerError, "user_id missing from context", nil))
		c.Abort()
		return uuid.Nil, false
	}
	id, ok := raw.(uuid.UUID)
	if !ok {
		_ = c.Error(shared.NewHTTPError(http.StatusInternalServerError, "user_id has unexpected type", nil))
		c.Abort()
		return uuid.Nil, false
	}
	return id, true
}
