//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// TestAuthSession covers Requirement 3: GET /auth/session returns the
// authenticated user's identity, leaks no credential material, and is
// gated by the same JWTAuth middleware as /api/*.
func TestAuthSession(t *testing.T) {
	t.Parallel()

	t.Run("GET /auth/session with a valid bearer returns id, email, and created_at", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req := setup.AuthorizedRequest(t, env, http.MethodGet, "/auth/session", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d want 200", resp.StatusCode)
		}
		var body struct {
			Data user.SessionResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Data.ID == uuid.Nil {
			t.Error("data.id is the zero UUID")
		}
		if body.Data.Email != setup.TestUserEmail {
			t.Errorf("data.email = %q want %q", body.Data.Email, setup.TestUserEmail)
		}
		if body.Data.CreatedAt.IsZero() {
			t.Error("data.created_at is zero")
		}
	})

	t.Run("the response body does not contain a password_hash field at any nesting level", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req := setup.AuthorizedRequest(t, env, http.MethodGet, "/auth/session", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		var raw map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatalf("decode: %v", err)
		}
		assertNoCredentialKey(t, raw, "body")
	})

	t.Run("GET /auth/session without a token returns 401 with the same reason codes as the /api/* guard", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp, err := http.Get(env.Server.URL + "/auth/session")
		if err != nil {
			t.Fatalf("get: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != "credentials_missing" {
			t.Errorf("reason = %q want %q (same taxonomy as the /api/* guard)", got.reason(), "credentials_missing")
		}
	})
}

// TestAuthChangePassword covers Requirement 4: POST /auth/change-password
// rotates the stored hash on success, enforces the current-password check
// and the new-password policy, and — because tokens are stateless with no
// server-side revocation — leaves tokens issued before the change valid
// until they naturally expire (Req 4.7).
func TestAuthChangePassword(t *testing.T) {
	t.Parallel()

	t.Run("correct current and valid new returns 204 and a subsequent login with the new password succeeds", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		const newPassword = "a-brand-new-valid-password-1"
		req := setup.AuthorizedRequest(t, env, http.MethodPost, "/auth/change-password", user.ChangePasswordRequest{
			CurrentPassword: setup.TestUserPassword,
			NewPassword:     newPassword,
		})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d want 204", resp.StatusCode)
		}

		loginResp := postLogin(t, env, setup.TestUserEmail, newPassword)
		defer loginResp.Body.Close()
		if loginResp.StatusCode != http.StatusOK {
			t.Fatalf("login with new password: status = %d want 200", loginResp.StatusCode)
		}
	})

	t.Run("correct current and a new password equal to the current returns 400 with reason=password-unchanged", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req := setup.AuthorizedRequest(t, env, http.MethodPost, "/auth/change-password", user.ChangePasswordRequest{
			CurrentPassword: setup.TestUserPassword,
			NewPassword:     setup.TestUserPassword,
		})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonPasswordUnchanged {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonPasswordUnchanged)
		}
	})

	t.Run("correct current and a weak new password returns 400 with reason=password-policy-violation", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req := setup.AuthorizedRequest(t, env, http.MethodPost, "/auth/change-password", user.ChangePasswordRequest{
			CurrentPassword: setup.TestUserPassword,
			NewPassword:     "tooshort1",
		})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonPasswordPolicyViolation {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonPasswordPolicyViolation)
		}
	})

	t.Run("an incorrect current password returns 400 with reason=current-password-incorrect", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req := setup.AuthorizedRequest(t, env, http.MethodPost, "/auth/change-password", user.ChangePasswordRequest{
			CurrentPassword: "definitely-not-the-current-password",
			NewPassword:     "a-perfectly-valid-new-password-1",
		})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonCurrentPasswordIncorrect {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonCurrentPasswordIncorrect)
		}
	})

	t.Run("an access token issued before a password change still passes the /api/* guard until its TTL elapses", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		oldToken := setup.LoginAsTestUser(t, env)

		changeReq, err := newBearerRequest(oldToken, http.MethodPost, env.Server.URL+"/auth/change-password",
			user.ChangePasswordRequest{
				CurrentPassword: setup.TestUserPassword,
				NewPassword:     "a-different-valid-password-2",
			})
		if err != nil {
			t.Fatalf("build change-password request: %v", err)
		}
		changeResp, err := http.DefaultClient.Do(changeReq)
		if err != nil {
			t.Fatalf("change-password request: %v", err)
		}
		defer changeResp.Body.Close()
		if changeResp.StatusCode != http.StatusNoContent {
			t.Fatalf("change-password status = %d want 204", changeResp.StatusCode)
		}

		// Req 4.7: stateless tokens, no server-side revocation — the token
		// minted before the change must still authenticate.
		guardReq, err := newBearerRequest(oldToken, http.MethodGet, env.Server.URL+"/api/sources", nil)
		if err != nil {
			t.Fatalf("build guard request: %v", err)
		}
		guardResp, err := http.DefaultClient.Do(guardReq)
		if err != nil {
			t.Fatalf("guard request: %v", err)
		}
		defer guardResp.Body.Close()
		if guardResp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("pre-change token rejected after password change; status = 401 (stateless tokens must survive rotation until TTL expiry)")
		}
	})
}

// newBearerRequest builds a request carrying an explicit bearer token,
// bypassing AuthorizedRequest's lazy-login-and-cache so the caller controls
// exactly which token is attached (needed to prove a specific pre-change
// token instance keeps working after rotation).
func newBearerRequest(token, method, url string, body any) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		req, err = http.NewRequest(method, url, bytes.NewReader(encoded))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}
