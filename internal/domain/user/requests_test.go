package user_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
)

func TestLoginRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("accepts a valid email and any non-empty password under 72 bytes", func(t *testing.T) {
		t.Parallel()
		req := user.LoginRequest{Email: "op@example.com", Password: "short"}

		err := req.Validate()

		if err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("rejects an empty email with reason validation_failed", func(t *testing.T) {
		t.Parallel()
		req := user.LoginRequest{Email: "", Password: "any-password"}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonValidationFailed)
	})

	t.Run("rejects an empty password with reason validation_failed", func(t *testing.T) {
		t.Parallel()
		req := user.LoginRequest{Email: "op@example.com", Password: ""}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonValidationFailed)
	})

	t.Run("rejects a syntactically invalid email with reason validation_failed", func(t *testing.T) {
		t.Parallel()
		req := user.LoginRequest{Email: "not-an-email", Password: "any-password"}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonValidationFailed)
	})

	t.Run("rejects a password longer than 72 bytes with reason password_too_long", func(t *testing.T) {
		t.Parallel()
		req := user.LoginRequest{Email: "op@example.com", Password: strings.Repeat("a", 73)}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonPasswordTooLong)
	})
}

func TestChangePasswordRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("accepts a 12+ byte new password", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: "old-password",
			NewPassword:     "new-password-strong",
		}

		err := req.Validate()

		if err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("rejects an empty current password with reason validation_failed", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: "",
			NewPassword:     "new-password-strong",
		}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonValidationFailed)
	})

	t.Run("rejects an empty new password with reason validation_failed", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: "old-password",
			NewPassword:     "",
		}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonValidationFailed)
	})

	t.Run("rejects a new password under 12 bytes with reason password-policy-violation", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: "old-password",
			NewPassword:     "short",
		}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonPasswordPolicyViolation)
	})

	t.Run("rejects a new password over 72 bytes with reason password_too_long", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: "old-password",
			NewPassword:     strings.Repeat("a", 73),
		}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonPasswordTooLong)
	})

	t.Run("rejects a current password over 72 bytes with reason password_too_long", func(t *testing.T) {
		t.Parallel()
		req := user.ChangePasswordRequest{
			CurrentPassword: strings.Repeat("a", 73),
			NewPassword:     "new-password-strong",
		}

		err := req.Validate()

		assertHTTPError(t, err, http.StatusBadRequest, user.ReasonPasswordTooLong)
	})
}

func assertHTTPError(t *testing.T, err error, wantCode int, wantReason string) {
	t.Helper()

	he := shared.AsHTTPError(err)
	if he == nil {
		t.Fatalf("Validate() = %v, want *shared.HTTPError with code %d reason %q", err, wantCode, wantReason)
	}
	if he.Code != wantCode {
		t.Errorf("HTTPError.Code = %d, want %d", he.Code, wantCode)
	}
	if he.Reason != wantReason {
		t.Errorf("HTTPError.Reason = %q, want %q", he.Reason, wantReason)
	}
}
