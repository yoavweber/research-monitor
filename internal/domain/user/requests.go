package user

import (
	"net/http"
	"net/mail"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// bcryptInputMaxBytes is the bcrypt input ceiling. Inputs longer than this are
// silently truncated by the library, which would weaken the hash, so the
// validation layer rejects them explicitly.
const bcryptInputMaxBytes = 72

// newPasswordMinBytes is the minimum length policy for a *new* password. The
// minimum is not enforced on submitted login passwords so legacy accounts can
// still authenticate and fail with invalid_credentials at the use-case layer.
const newPasswordMinBytes = 12

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (r LoginRequest) Validate() error {
	if r.Email == "" || r.Password == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "email and password are required", nil).
			WithReason(ReasonValidationFailed)
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return shared.NewHTTPError(http.StatusBadRequest, "email is not a valid address", nil).
			WithReason(ReasonValidationFailed)
	}
	if len(r.Password) > bcryptInputMaxBytes {
		return shared.NewHTTPError(http.StatusBadRequest, "password exceeds 72 bytes", nil).
			WithReason(ReasonPasswordTooLong)
	}
	return nil
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

func (r ChangePasswordRequest) Validate() error {
	if r.CurrentPassword == "" || r.NewPassword == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "current and new passwords are required", nil).
			WithReason(ReasonValidationFailed)
	}
	if len(r.CurrentPassword) > bcryptInputMaxBytes {
		return shared.NewHTTPError(http.StatusBadRequest, "current password exceeds 72 bytes", nil).
			WithReason(ReasonPasswordTooLong)
	}
	if len(r.NewPassword) > bcryptInputMaxBytes {
		return shared.NewHTTPError(http.StatusBadRequest, "new password exceeds 72 bytes", nil).
			WithReason(ReasonPasswordTooLong)
	}
	if len(r.NewPassword) < newPasswordMinBytes {
		return shared.NewHTTPError(http.StatusBadRequest, "new password must be at least 12 bytes", nil).
			WithReason(ReasonPasswordPolicyViolation)
	}
	return nil
}
