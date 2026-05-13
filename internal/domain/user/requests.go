package user

import (
	"net/http"
	"net/mail"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// LoginRequest is the JSON body of POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Validate enforces the inbound contract for login submissions: non-empty
// fields, a syntactically valid email, and the bcrypt 72-byte input ceiling
// (Requirement 9.5). The 12-byte minimum password policy is intentionally NOT
// enforced here so that a user with a legacy short password can still attempt
// login and fail with invalid_credentials rather than validation_failed
// (Requirement 1.6).
func (r LoginRequest) Validate() error {
	if r.Email == "" || r.Password == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "email and password are required", nil).
			WithReason("validation_failed")
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return shared.NewHTTPError(http.StatusBadRequest, "email is not a valid address", nil).
			WithReason("validation_failed")
	}
	if len(r.Password) > 72 {
		return shared.NewHTTPError(http.StatusBadRequest, "password exceeds 72 bytes", nil).
			WithReason("password_too_long")
	}
	return nil
}

// ChangePasswordRequest is the JSON body of POST /auth/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// Validate enforces the inbound contract for password rotation: non-empty
// fields, the 72-byte ceiling on every password input (Requirement 9.5), and
// the 12-byte minimum on the new password (Requirement 6.5). The current
// password has no minimum because a user with a legacy short password must
// still be able to rotate it.
func (r ChangePasswordRequest) Validate() error {
	if r.CurrentPassword == "" || r.NewPassword == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "current and new passwords are required", nil).
			WithReason("validation_failed")
	}
	if len(r.CurrentPassword) > 72 {
		return shared.NewHTTPError(http.StatusBadRequest, "current password exceeds 72 bytes", nil).
			WithReason("password_too_long")
	}
	if len(r.NewPassword) > 72 {
		return shared.NewHTTPError(http.StatusBadRequest, "new password exceeds 72 bytes", nil).
			WithReason("password_too_long")
	}
	if len(r.NewPassword) < 12 {
		return shared.NewHTTPError(http.StatusBadRequest, "new password must be at least 12 bytes", nil).
			WithReason("password-policy-violation")
	}
	return nil
}
