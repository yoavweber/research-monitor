//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// errorEnvelope mirrors the standard { "error": { "code", "message",
// "details": { "reason" } } } shape rendered by the ErrorEnvelope
// middleware. Decoded generically (Details as map[string]any) so a missing
// reason key surfaces as a zero value rather than a decode error.
type errorEnvelope struct {
	Error struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

// decodeErrorEnvelope decodes and closes resp.Body. Callers read
// resp.StatusCode beforehand — it survives body consumption.
func decodeErrorEnvelope(t *testing.T, resp *http.Response) errorEnvelope {
	t.Helper()
	defer resp.Body.Close()
	var body errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return body
}

func (e errorEnvelope) reason() string {
	r, _ := e.Error.Details["reason"].(string)
	return r
}

// postLogin issues POST /auth/login with the given credentials. Unlike
// AuthorizedRequest, login is deliberately unauthenticated.
func postLogin(t *testing.T, env *setup.TestEnv, email, password string) *http.Response {
	t.Helper()
	body, err := json.Marshal(user.LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("marshal login request: %v", err)
	}
	resp, err := http.Post(env.Server.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	return resp
}

// assertNoCredentialKey walks a decoded JSON value (map[string]any /
// []any / scalars) and fails the test if any object key contains
// "password" or "hash" (case-insensitive), anywhere in the structure. Used
// to prove a response body never leaks credential material even if a
// future field is added carelessly.
func assertNoCredentialKey(t *testing.T, v any, path string) {
	t.Helper()
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "password") || strings.Contains(lower, "hash") {
				t.Errorf("response body contains credential-shaped key %q at %s", k, path+"."+k)
			}
			assertNoCredentialKey(t, child, path+"."+k)
		}
	case []any:
		for _, child := range val {
			assertNoCredentialKey(t, child, path+"[]")
		}
	}
}

func TestAuthLogin(t *testing.T) {
	t.Parallel()

	t.Run("login with correct credentials returns 200 with access token and user payload", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)
		setup.SeedTestUser(t, env)

		resp := postLogin(t, env, setup.TestUserEmail, setup.TestUserPassword)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d want 200", resp.StatusCode)
		}

		var raw map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatalf("decode: %v", err)
		}
		assertNoCredentialKey(t, raw, "body")

		data, ok := raw["data"].(map[string]any)
		if !ok {
			t.Fatalf("body.data missing or wrong type: %#v", raw["data"])
		}
		token, _ := data["access_token"].(string)
		if token == "" {
			t.Error("data.access_token is empty")
		}
		if _, ok := data["expires_at"]; !ok {
			t.Error("data.expires_at missing")
		}
		userObj, ok := data["user"].(map[string]any)
		if !ok {
			t.Fatalf("data.user missing or wrong type: %#v", data["user"])
		}
		if id, _ := userObj["id"].(string); id == "" {
			t.Error("data.user.id is empty")
		}
		if email, _ := userObj["email"].(string); email != setup.TestUserEmail {
			t.Errorf("data.user.email = %q want %q", email, setup.TestUserEmail)
		}
	})

	t.Run("login with unknown email and login with wrong password return identical 401 envelopes with reason=invalid_credentials", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)
		setup.SeedTestUser(t, env)

		unknownResp := postLogin(t, env, "nobody-registered@example.com", "irrelevant-password-1")
		wrongResp := postLogin(t, env, setup.TestUserEmail, "definitely-the-wrong-password")

		if unknownResp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unknown email status = %d want 401", unknownResp.StatusCode)
		}
		if wrongResp.StatusCode != http.StatusUnauthorized {
			t.Errorf("wrong password status = %d want 401", wrongResp.StatusCode)
		}

		unknownEnv := decodeErrorEnvelope(t, unknownResp)
		wrongEnv := decodeErrorEnvelope(t, wrongResp)

		if unknownEnv.reason() != user.ReasonInvalidCredentials {
			t.Errorf("unknown email reason = %q want %q", unknownEnv.reason(), user.ReasonInvalidCredentials)
		}
		if wrongEnv.reason() != user.ReasonInvalidCredentials {
			t.Errorf("wrong password reason = %q want %q", wrongEnv.reason(), user.ReasonInvalidCredentials)
		}
		if unknownEnv.Error.Message != wrongEnv.Error.Message {
			t.Errorf("envelopes must be identical (no unknown-email vs wrong-password signal): unknown=%q wrong=%q",
				unknownEnv.Error.Message, wrongEnv.Error.Message)
		}
	})

	t.Run("login with a syntactically invalid email returns 400 with reason=validation_failed", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp := postLogin(t, env, "not-an-email-address", "some-password-123")

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonValidationFailed {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonValidationFailed)
		}
	})

	t.Run("login with a password longer than 72 bytes returns 400 with reason=password_too_long", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp := postLogin(t, env, "someone@example.com", strings.Repeat("a", 73))

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonPasswordTooLong {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonPasswordTooLong)
		}
	})

	t.Run("login with an empty body returns 400 with reason=validation_failed", func(t *testing.T) {
		t.Parallel()
		env := setup.SetupTestEnv(t)
		t.Cleanup(env.Close)

		resp, err := http.Post(env.Server.URL+"/auth/login", "application/json", bytes.NewReader(nil))
		if err != nil {
			t.Fatalf("post login: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d want 400", resp.StatusCode)
		}
		got := decodeErrorEnvelope(t, resp)
		if got.reason() != user.ReasonValidationFailed {
			t.Errorf("reason = %q want %q", got.reason(), user.ReasonValidationFailed)
		}
	})
}
