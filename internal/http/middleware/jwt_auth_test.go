package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// frozenClock is a deterministic shared.Clock for time-sensitive subtests.
// A test-local clock is acceptable here (vs. shared tests/mocks/) because
// each subtest owns its own instant and must advance time independently.
type frozenClock struct{ t time.Time }

func (c *frozenClock) Now() time.Time { return c.t }

// jwtAuthTestSecret is a 32-byte HS256 secret used across the middleware
// subtests. The same constant is reused for the verifier so signature checks
// only fail when a test explicitly creates a mismatched signer.
const jwtAuthTestSecret = "a-32-byte-secret-for-hs256-tests!"

// newJWTService builds a real JWT service with the supplied clock and TTL.
// Tests construct one per subtest so each owns its own time source.
func newJWTService(t *testing.T, secret string, ttl time.Duration, clk shared.Clock) *auth.JWTTokenService {
	t.Helper()
	return auth.NewJWTTokenService(auth.JWTConfig{
		Secret: []byte(secret),
		TTL:    ttl,
		Clock:  clk,
	})
}

// newJWTAuthEngine builds a minimal Gin engine that mounts the production
// ErrorEnvelope middleware ahead of JWTAuth, so subtests assert against the
// real JSON envelope shape rather than gin.Context.Errors. A `capturedUserID`
// pointer is set on the success path so the success subtest can confirm the
// downstream handler observed the parsed UUID.
func newJWTAuthEngine(validator shared.TokenValidator, capturedUserID *uuid.UUID) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorEnvelope())
	r.GET("/protected", middleware.JWTAuth(validator), func(c *gin.Context) {
		v, ok := c.Get("user_id")
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		id, ok := v.(uuid.UUID)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		if capturedUserID != nil {
			*capturedUserID = id
		}
		c.Status(http.StatusOK)
	})
	return r
}

// requestWithHeader builds a GET /protected request. If headerValue is the
// empty sentinel "__omit__" the Authorization header is left off entirely; any
// other value (including "") is set verbatim so subtests can drive the
// missing-vs-empty distinction.
func requestWithHeader(headerValue string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if headerValue != "__omit__" {
		req.Header.Set("Authorization", headerValue)
	}
	return req
}

func decodeJWTAuthEnvelope(t *testing.T, body []byte) common.Envelope {
	t.Helper()
	var env common.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, body)
	}
	return env
}

func assertEnvelopeReason(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantReason string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, wantStatus, w.Body.String())
	}
	env := decodeJWTAuthEnvelope(t, w.Body.Bytes())
	if env.Error == nil {
		t.Fatalf("envelope.Error is nil; body=%s", w.Body.String())
	}
	if env.Error.Code != wantStatus {
		t.Fatalf("envelope.Error.Code = %d, want %d", env.Error.Code, wantStatus)
	}
	gotReason, ok := env.Error.Details["reason"].(string)
	if !ok {
		t.Fatalf("envelope.Error.Details.reason missing or not a string: %v", env.Error.Details)
	}
	if gotReason != wantReason {
		t.Fatalf("envelope.Error.Details.reason = %q, want %q", gotReason, wantReason)
	}
}

func TestJWTAuth(t *testing.T) {
	t.Parallel()

	t.Run("accepts a valid bearer token and sets user_id on the request context", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		userID := uuid.New()
		token, _, err := svc.Issue(userID.String())
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		var captured uuid.UUID
		engine := newJWTAuthEngine(svc, &captured)
		req := requestWithHeader("Bearer " + token)
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if captured != userID {
			t.Errorf("captured user_id = %v, want %v", captured, userID)
		}
	})

	t.Run("rejects a request without an Authorization header with reason credentials_missing", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("__omit__")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "credentials_missing")
	})

	t.Run("rejects a request whose Authorization header does not start with Bearer with reason credentials_malformed", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("Basic dXNlcjpwYXNz")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "credentials_malformed")
	})

	t.Run("rejects a request with an empty token after Bearer with reason credentials_malformed", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("Bearer ")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "credentials_malformed")
	})

	t.Run("rejects a token with an invalid signature with reason invalid_access_token", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		signer := newJWTService(t, "secret-A-padded-to-32-bytes-aaaa", time.Hour, clk)
		verifier := newJWTService(t, "secret-B-padded-to-32-bytes-bbbb", time.Hour, clk)
		token, _, err := signer.Issue(uuid.New().String())
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		engine := newJWTAuthEngine(verifier, nil)
		req := requestWithHeader("Bearer " + token)
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "invalid_access_token")
	})

	t.Run("rejects an expired token with reason expired_access_token", func(t *testing.T) {
		t.Parallel()
		start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		clk := &frozenClock{t: start}
		svc := newJWTService(t, jwtAuthTestSecret, time.Minute, clk)
		token, _, err := svc.Issue(uuid.New().String())
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		// Advance the clock past the token's expiry without changing the validator
		// instance — the JWT service reads the clock at verification time.
		clk.t = start.Add(2 * time.Minute)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("Bearer " + token)
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "expired_access_token")
	})

	t.Run("rejects a structurally malformed token with reason malformed_access_token", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("Bearer not-a-real-jwt")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "malformed_access_token")
	})

	t.Run("rejects a token whose subject is not a UUID with reason invalid_access_token", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		// Mint a token directly with a non-UUID subject. JWTTokenService.Issue
		// does not enforce UUID shape, so we bypass it via the library to drive
		// the defensive parse failure in the middleware.
		claims := jwt.RegisteredClaims{
			Subject:   "not-a-uuid",
			IssuedAt:  jwt.NewNumericDate(clk.t),
			ExpiresAt: jwt.NewNumericDate(clk.t.Add(time.Hour)),
		}
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtAuthTestSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		svc := newJWTService(t, jwtAuthTestSecret, time.Hour, clk)
		engine := newJWTAuthEngine(svc, nil)
		req := requestWithHeader("Bearer " + signed)
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		assertEnvelopeReason(t, w, http.StatusUnauthorized, "invalid_access_token")
	})
}
