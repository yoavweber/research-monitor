//go:build integration

package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	authinfra "github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// TestAuthAPIGuard covers Requirements 2.1-2.5: every /api/* route rejects a
// request whose bearer credential is missing, malformed, invalid, or
// expired, and forwards a request carrying a genuinely valid one. GET
// /api/sources stands in as the representative protected endpoint — the
// guard is mounted once on the shared /api group, so any route proves it.
func TestAuthAPIGuard(t *testing.T) {
	t.Parallel()

	t.Run("request without Authorization header returns 401 with reason=credentials_missing", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp, err := http.Get(env.Server.URL + "/api/sources")
		if err != nil {
			t.Fatalf("get: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != "credentials_missing" {
			t.Errorf("reason = %q want %q", got.reason(), "credentials_missing")
		}
	})

	t.Run("request with a non-Bearer scheme returns 401 with reason=credentials_malformed", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/sources", nil)
		req.Header.Set("Authorization", "Token some-opaque-value")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != "credentials_malformed" {
			t.Errorf("reason = %q want %q", got.reason(), "credentials_malformed")
		}
	})

	t.Run("request with a token whose signature does not verify returns 401 with reason=invalid_access_token", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		// Signed with a different key than the harness's JWTAuth guard —
		// same claim shape, wrong signature.
		foreignSigner := authinfra.NewJWTTokenService(authinfra.JWTConfig{
			Secret: []byte("a-completely-different-signing-key-32b-plus"),
			TTL:    time.Hour,
			Clock:  shared.SystemClock{},
		})
		foreignToken, _, err := foreignSigner.Issue("00000000-0000-0000-0000-000000000001")
		if err != nil {
			t.Fatalf("issue foreign token: %v", err)
		}

		req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/sources", nil)
		req.Header.Set("Authorization", "Bearer "+foreignToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != "invalid_access_token" {
			t.Errorf("reason = %q want %q", got.reason(), "invalid_access_token")
		}
	})

	t.Run("request with an expired access token returns 401 with reason=expired_access_token", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		token := setup.LoginAsTestUser(t, env)

		// Advance the harness's shared clock (same clock the token was
		// signed and will be verified against) well past the default 24h
		// TTL — deterministic, no sleeping past a real expiry.
		env.AuthClock.Set(time.Now().Add(25 * time.Hour))

		req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/sources", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != "expired_access_token" {
			t.Errorf("reason = %q want %q", got.reason(), "expired_access_token")
		}
	})

	t.Run("request with a valid access token returns a non-401 status from the downstream handler", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp := doAuthenticatedGet(t, env, "/api/sources")
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("status = 401 with a valid token; body should have reached the source controller")
		}
	})
}
