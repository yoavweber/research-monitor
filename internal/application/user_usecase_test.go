package application_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	userdomain "github.com/yoavweber/research-monitor/backend/internal/domain/user"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// frozenClock is a test-local Clock whose instant is mutable so subtests can
// advance time when verifying TTL behavior. Inlining is justified because each
// subtest owns its own clock and the production type does not need to control
// time deterministically.
type frozenClock struct{ t time.Time }

func (c *frozenClock) Now() time.Time { return c.t }

// testHarness bundles the collaborators a Login/ChangePassword test needs.
// Built with real adapters per testing.md: bcrypt at cost 4, JWT signer with
// a 32-byte test secret, in-memory SQLite repo. The Logger is the only
// recording double — slog's wire format is opaque so structured-field
// assertions go through tests/mocks.RecordingLogger.
type testHarness struct {
	uc     userdomain.UseCase
	repo   userdomain.Repository
	hasher shared.PasswordHasher
	signer *auth.JWTTokenService
	clock  *frozenClock
	log    *mocks.RecordingLogger
	jwtTTL time.Duration
}

const (
	testJWTSecret = "a-32-byte-secret-for-hs256-tests!"
	testJWTTTL    = time.Hour
	testBcrypt    = 4
)

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	clk := &frozenClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	signer := auth.NewJWTTokenService(auth.JWTConfig{
		Secret: []byte(testJWTSecret),
		TTL:    testJWTTTL,
		Clock:  clk,
	})
	hasher := auth.NewBcryptHasher(testBcrypt)
	repo := userpersist.NewRepository(testdb.New(t))
	log := &mocks.RecordingLogger{}
	uc := application.NewUserUseCase(repo, hasher, signer, clk, log)
	return &testHarness{
		uc:     uc,
		repo:   repo,
		hasher: hasher,
		signer: signer,
		clock:  clk,
		log:    log,
		jwtTTL: testJWTTTL,
	}
}

func seedUser(t *testing.T, h *testHarness, email, plain string) *userdomain.User {
	t.Helper()
	hash, err := h.hasher.Hash(plain)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := h.clock.Now()
	u := &userdomain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := h.repo.Save(context.Background(), u); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	return u
}

func TestUserUseCase_Login(t *testing.T) {
	t.Parallel()

	t.Run("with an unknown email returns ErrInvalidCredentials and emits a warn log with reason invalid_credentials", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		req := userdomain.LoginRequest{Email: "ghost@example.com", Password: "any-password"}

		_, err := h.uc.Login(context.Background(), req)

		if !errors.Is(err, userdomain.ErrInvalidCredentials) {
			t.Fatalf("err = %v, want ErrInvalidCredentials", err)
		}
		warns := h.log.RecordsAt("Warn")
		if len(warns) != 1 {
			t.Fatalf("warn records = %d, want 1; records=%v", len(warns), h.log.Records)
		}
		if warns[0].Args["reason"] != userdomain.ReasonInvalidCredentials {
			t.Errorf("reason = %v, want %q", warns[0].Args["reason"], userdomain.ReasonInvalidCredentials)
		}
		if warns[0].Args["email"] != req.Email {
			t.Errorf("logged email = %v, want %q", warns[0].Args["email"], req.Email)
		}
		assertNoCredentialMaterial(t, warns[0].Args, req.Password)
	})

	t.Run("with a known email but wrong password returns ErrInvalidCredentials identical to the unknown-email path and emits the same warn log", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		u := seedUser(t, h, "alice@example.com", "Password123abc!")
		req := userdomain.LoginRequest{Email: u.Email, Password: "different-password"}

		_, err := h.uc.Login(context.Background(), req)

		if !errors.Is(err, userdomain.ErrInvalidCredentials) {
			t.Fatalf("err = %v, want ErrInvalidCredentials", err)
		}
		warns := h.log.RecordsAt("Warn")
		if len(warns) != 1 {
			t.Fatalf("warn records = %d, want 1; records=%v", len(warns), h.log.Records)
		}
		if warns[0].Args["reason"] != userdomain.ReasonInvalidCredentials {
			t.Errorf("reason = %v, want %q", warns[0].Args["reason"], userdomain.ReasonInvalidCredentials)
		}
		if warns[0].Args["email"] != req.Email {
			t.Errorf("logged email = %v, want %q", warns[0].Args["email"], req.Email)
		}
		assertNoCredentialMaterial(t, warns[0].Args, req.Password)
	})

	t.Run("with correct credentials returns a token whose Verify round-trips the user id and emits an auth.login.ok info log without credential material", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		plain := "Password123abc!"
		u := seedUser(t, h, "alice@example.com", plain)
		req := userdomain.LoginRequest{Email: u.Email, Password: plain}

		result, err := h.uc.Login(context.Background(), req)

		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if result.Token == "" {
			t.Fatal("Token empty")
		}
		wantExp := h.clock.Now().Add(h.jwtTTL)
		if !result.ExpiresAt.Equal(wantExp) {
			t.Errorf("ExpiresAt = %v, want %v", result.ExpiresAt, wantExp)
		}
		if result.User == nil || result.User.ID != u.ID {
			t.Errorf("result.User = %+v, want id %v", result.User, u.ID)
		}
		sub, err := h.signer.Verify(result.Token)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if sub != u.ID.String() {
			t.Errorf("subject = %q, want %q", sub, u.ID.String())
		}
		infos := h.log.RecordsAt("Info")
		if len(infos) != 1 {
			t.Fatalf("info records = %d, want 1; records=%v", len(infos), h.log.Records)
		}
		if infos[0].Msg != "auth.login.ok" {
			t.Errorf("msg = %q, want %q", infos[0].Msg, "auth.login.ok")
		}
		if infos[0].Args["user_id"] != u.ID.String() {
			t.Errorf("user_id = %v, want %q", infos[0].Args["user_id"], u.ID.String())
		}
		assertNoCredentialMaterial(t, infos[0].Args, plain, result.Token)
	})

	t.Run("with a syntactically invalid email returns the DTO validation error", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		req := userdomain.LoginRequest{Email: "not-an-email", Password: "any-password"}

		_, err := h.uc.Login(context.Background(), req)

		he := shared.AsHTTPError(err)
		if he == nil {
			t.Fatalf("err = %v, want *shared.HTTPError", err)
		}
		if he.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want %d", he.Code, http.StatusBadRequest)
		}
		if he.Reason != userdomain.ReasonValidationFailed {
			t.Errorf("reason = %q, want %q", he.Reason, userdomain.ReasonValidationFailed)
		}
	})
}

func TestUserUseCase_ChangePassword(t *testing.T) {
	t.Parallel()

	t.Run("rotates the hash and a subsequent Login with the new password succeeds", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		oldPlain := "Password123abc!"
		newPlain := "BrandNewPass99X"
		u := seedUser(t, h, "alice@example.com", oldPlain)

		err := h.uc.ChangePassword(context.Background(), u.ID, userdomain.ChangePasswordRequest{
			CurrentPassword: oldPlain,
			NewPassword:     newPlain,
		})

		if err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}
		if _, err := h.uc.Login(context.Background(), userdomain.LoginRequest{Email: u.Email, Password: newPlain}); err != nil {
			t.Fatalf("Login after rotation: %v", err)
		}
		if _, err := h.uc.Login(context.Background(), userdomain.LoginRequest{Email: u.Email, Password: oldPlain}); !errors.Is(err, userdomain.ErrInvalidCredentials) {
			t.Errorf("old-password Login err = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("with wrong current password returns ErrCurrentPasswordIncorrect", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		u := seedUser(t, h, "alice@example.com", "Password123abc!")

		err := h.uc.ChangePassword(context.Background(), u.ID, userdomain.ChangePasswordRequest{
			CurrentPassword: "wrong-current",
			NewPassword:     "BrandNewPass99X",
		})

		if !errors.Is(err, userdomain.ErrCurrentPasswordIncorrect) {
			t.Errorf("err = %v, want ErrCurrentPasswordIncorrect", err)
		}
	})

	t.Run("where new equals current returns ErrPasswordUnchanged", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		plain := "Password123abc!"
		u := seedUser(t, h, "alice@example.com", plain)

		err := h.uc.ChangePassword(context.Background(), u.ID, userdomain.ChangePasswordRequest{
			CurrentPassword: plain,
			NewPassword:     plain,
		})

		if !errors.Is(err, userdomain.ErrPasswordUnchanged) {
			t.Errorf("err = %v, want ErrPasswordUnchanged", err)
		}
	})

	t.Run("with a weak new password returns the DTO's password-policy-violation HTTPError", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		u := seedUser(t, h, "alice@example.com", "Password123abc!")

		err := h.uc.ChangePassword(context.Background(), u.ID, userdomain.ChangePasswordRequest{
			CurrentPassword: "Password123abc!",
			NewPassword:     "short",
		})

		he := shared.AsHTTPError(err)
		if he == nil {
			t.Fatalf("err = %v, want *shared.HTTPError", err)
		}
		if he.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want %d", he.Code, http.StatusBadRequest)
		}
		if he.Reason != userdomain.ReasonPasswordPolicyViolation {
			t.Errorf("reason = %q, want %q", he.Reason, userdomain.ReasonPasswordPolicyViolation)
		}
	})
}

func TestUserUseCase_Session(t *testing.T) {
	t.Parallel()

	t.Run("returns the user for a known id and ErrNotFound for an unknown id", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		u := seedUser(t, h, "alice@example.com", "Password123abc!")

		got, err := h.uc.Session(context.Background(), u.ID)
		if err != nil {
			t.Fatalf("Session known: %v", err)
		}
		if got.ID != u.ID || got.Email != u.Email {
			t.Errorf("Session got = %+v, want id %v email %q", got, u.ID, u.Email)
		}

		_, err = h.uc.Session(context.Background(), uuid.New())

		if !errors.Is(err, userdomain.ErrNotFound) {
			t.Errorf("Session unknown err = %v, want ErrNotFound", err)
		}
	})
}

// TestUserUseCase_TokenSurvivesPasswordChange covers Requirement 4.7: a token
// issued before a password change must remain valid until its normal TTL
// elapses, because the JWT model is stateless and there is no server-side
// revocation.
func TestUserUseCase_TokenSurvivesPasswordChange(t *testing.T) {
	t.Parallel()

	t.Run("a token issued before a password change still validates until its TTL elapses", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		oldPlain := "Password123abc!"
		newPlain := "BrandNewPass99X"
		u := seedUser(t, h, "alice@example.com", oldPlain)
		t0 := h.clock.Now()

		result, err := h.uc.Login(context.Background(), userdomain.LoginRequest{Email: u.Email, Password: oldPlain})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}

		h.clock.t = t0.Add(time.Second)
		if err := h.uc.ChangePassword(context.Background(), u.ID, userdomain.ChangePasswordRequest{
			CurrentPassword: oldPlain,
			NewPassword:     newPlain,
		}); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}

		// Advance clock to within TTL window and verify the prior token still parses.
		h.clock.t = t0.Add(h.jwtTTL / 2)
		sub, err := h.signer.Verify(result.Token)

		if err != nil {
			t.Fatalf("Verify after change: %v", err)
		}
		if sub != u.ID.String() {
			t.Errorf("subject = %q, want %q", sub, u.ID.String())
		}
	})
}

// assertNoCredentialMaterial fails if any logged value contains a forbidden
// substring — a plaintext password or an issued token. Substring search (not
// equality) so a log line that interpolated the password into a larger
// string would still be caught.
func assertNoCredentialMaterial(t *testing.T, args map[string]any, forbidden ...string) {
	t.Helper()
	for k, v := range args {
		s, ok := v.(string)
		if !ok {
			continue
		}
		for _, f := range forbidden {
			if f != "" && strings.Contains(s, f) {
				t.Errorf("log field %q contains credential material %q (value=%q)", k, f, s)
			}
		}
	}
}
