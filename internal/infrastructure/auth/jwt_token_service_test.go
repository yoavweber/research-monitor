package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
)

// frozenClock is a deterministic shared.Clock for time-sensitive subtests.
// A test-local clock is acceptable here (vs. shared tests/mocks/) because
// each subtest owns its own instant and must advance time independently.
type frozenClock struct{ t time.Time }

func (c *frozenClock) Now() time.Time { return c.t }

func newJWTService(t *testing.T, secret string, ttl time.Duration, clk shared.Clock) *auth.JWTTokenService {
	t.Helper()
	return auth.NewJWTTokenService(auth.JWTConfig{
		Secret: []byte(secret),
		TTL:    ttl,
		Clock:  clk,
	})
}

func TestJWTTokenService_Issue(t *testing.T) {
	t.Run("Issue then Verify round-trips the supplied subject", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, "a-32-byte-secret-for-hs256-tests!", time.Hour, clk)
		subject := "user-123"

		token, expiresAt, err := svc.Issue(subject)

		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if token == "" {
			t.Fatal("Issue returned empty token")
		}
		wantExp := clk.t.Add(time.Hour)
		if !expiresAt.Equal(wantExp) {
			t.Errorf("expiresAt = %v, want %v", expiresAt, wantExp)
		}

		got, err := svc.Verify(token)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if got != subject {
			t.Errorf("subject = %q, want %q", got, subject)
		}
	})

	t.Run("Issue rejects an empty subject", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, "a-32-byte-secret-for-hs256-tests!", time.Hour, clk)

		_, _, err := svc.Issue("")

		if err == nil {
			t.Fatal("Issue with empty subject returned nil error")
		}
	})
}

func TestJWTTokenService_Verify(t *testing.T) {
	t.Run("rejects a token whose signature does not match the key", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		signer := newJWTService(t, "secret-A-padded-to-32-bytes-aaaa", time.Hour, clk)
		verifier := newJWTService(t, "secret-B-padded-to-32-bytes-bbbb", time.Hour, clk)

		token, _, err := signer.Issue("user-123")
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}

		_, err = verifier.Verify(token)

		if !errors.Is(err, shared.ErrTokenSignatureInvalid) {
			t.Errorf("err = %v, want ErrTokenSignatureInvalid", err)
		}
	})

	t.Run("rejects an expired token", func(t *testing.T) {
		t.Parallel()
		start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		clk := &frozenClock{t: start}
		svc := newJWTService(t, "a-32-byte-secret-for-hs256-tests!", time.Minute, clk)

		token, _, err := svc.Issue("user-123")
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		clk.t = start.Add(2 * time.Minute)

		_, err = svc.Verify(token)

		if !errors.Is(err, shared.ErrTokenExpired) {
			t.Errorf("err = %v, want ErrTokenExpired", err)
		}
	})

	t.Run("rejects a token whose signature has been tampered with", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		secret := "a-32-byte-secret-for-hs256-tests!"
		svc := newJWTService(t, secret, time.Hour, clk)
		claims := jwt.RegisteredClaims{
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(clk.t),
			ExpiresAt: jwt.NewNumericDate(clk.t.Add(time.Hour)),
		}
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		// Flip the final character of the signature segment so the token is still
		// structurally well-formed (three base64url segments) and parses cleanly,
		// but the signature no longer matches the payload. This exercises the
		// signature-verification failure path, not the malformed-input path.
		last := signed[len(signed)-1]
		flipped := byte('A')
		if last == 'A' {
			flipped = 'B'
		}
		tampered := signed[:len(signed)-1] + string(flipped)

		_, err = svc.Verify(tampered)

		if !errors.Is(err, shared.ErrTokenSignatureInvalid) {
			t.Errorf("err = %v, want ErrTokenSignatureInvalid", err)
		}
	})

	t.Run("rejects a structurally malformed token", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, "a-32-byte-secret-for-hs256-tests!", time.Hour, clk)

		_, err := svc.Verify("not-a-real-jwt")

		if !errors.Is(err, shared.ErrTokenMalformed) {
			t.Errorf("err = %v, want ErrTokenMalformed", err)
		}
	})

	t.Run("rejects a token signed with a non-HS256 algorithm", func(t *testing.T) {
		t.Parallel()
		clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		svc := newJWTService(t, "a-32-byte-secret-for-hs256-tests!", time.Hour, clk)
		claims := jwt.RegisteredClaims{
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(clk.t),
			ExpiresAt: jwt.NewNumericDate(clk.t.Add(time.Hour)),
		}
		noneToken, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
			SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("sign with alg=none: %v", err)
		}

		_, err = svc.Verify(noneToken)

		if !errors.Is(err, shared.ErrTokenSignatureInvalid) {
			t.Errorf("err = %v, want ErrTokenSignatureInvalid", err)
		}
	})
}
