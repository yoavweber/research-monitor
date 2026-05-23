package bootstrap_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/bootstrap"
	domainuser "github.com/yoavweber/research-monitor/backend/internal/domain/user"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// TestSeedUser exercises the cmd/seed-facing helper end-to-end against a real
// SQLite DB and a real bcrypt hasher at the test cost. The behaviors here are
// the same ones the CLI dispatch in cmd/seed/main.go relies on, so a regression
// in this helper is observable in operator workflow without an integration
// test.
func TestSeedUser(t *testing.T) {
	t.Parallel()

	const validPassword = "PasswordThatIsLongEnough!"

	t.Run("creates a user when the email is new", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		err := bootstrap.SeedUser(ctx, db, hasher, log, "alice@example.com", validPassword)

		if err != nil {
			t.Fatalf("SeedUser: %v", err)
		}
		got, ferr := repo.FindByEmail(ctx, "alice@example.com")
		if ferr != nil {
			t.Fatalf("FindByEmail: %v", ferr)
		}
		if got.Email != "alice@example.com" {
			t.Errorf("Email = %q want alice@example.com", got.Email)
		}
		if got.PasswordHash == "" {
			t.Error("PasswordHash is empty — Hash result not persisted")
		}
		if got.PasswordHash == validPassword {
			t.Error("PasswordHash equals the plaintext — password was not hashed")
		}
		if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
			t.Error("timestamps are zero — SeedUser must stamp CreatedAt/UpdatedAt before Save")
		}
		infos := log.RecordsAt("Info")
		var sawCreated bool
		for _, r := range infos {
			if r.Msg == "seed.user.created" {
				sawCreated = true
			}
		}
		if !sawCreated {
			t.Errorf("expected an Info log with msg %q, got %+v", "seed.user.created", infos)
		}
	})

	t.Run("is idempotent on a duplicate email (returns nil, leaves the row unchanged)", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		if err := bootstrap.SeedUser(ctx, db, hasher, log, "dup@example.com", validPassword); err != nil {
			t.Fatalf("first SeedUser: %v", err)
		}
		first, err := repo.FindByEmail(ctx, "dup@example.com")
		if err != nil {
			t.Fatalf("FindByEmail after first seed: %v", err)
		}

		err = bootstrap.SeedUser(ctx, db, hasher, log, "dup@example.com", validPassword)

		if err != nil {
			t.Fatalf("second SeedUser: %v want nil for idempotent skip", err)
		}
		second, ferr := repo.FindByEmail(ctx, "dup@example.com")
		if ferr != nil {
			t.Fatalf("FindByEmail after second seed: %v", ferr)
		}
		if second.ID != first.ID {
			t.Errorf("ID changed across re-seed: first=%v second=%v", first.ID, second.ID)
		}
		if second.PasswordHash != first.PasswordHash {
			t.Errorf("PasswordHash changed across re-seed: first=%q second=%q", first.PasswordHash, second.PasswordHash)
		}
		if !second.CreatedAt.Equal(first.CreatedAt) {
			t.Errorf("CreatedAt changed across re-seed: first=%v second=%v", first.CreatedAt, second.CreatedAt)
		}
		var sawSkipped bool
		for _, r := range log.RecordsAt("Info") {
			if r.Msg == "seed.user.skipped" {
				sawSkipped = true
			}
		}
		if !sawSkipped {
			t.Errorf("expected an Info log with msg %q on the duplicate path", "seed.user.skipped")
		}
	})

	t.Run("rejects a syntactically invalid email", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		err := bootstrap.SeedUser(ctx, db, hasher, log, "not-an-email", validPassword)

		if err == nil {
			t.Fatal("SeedUser err = nil want a validation error for the malformed email")
		}
		if !strings.Contains(err.Error(), "not-an-email") {
			t.Errorf("err message %q must name the offending email", err.Error())
		}
		// No row written.
		_, ferr := repo.FindByEmail(ctx, "not-an-email")
		if !errors.Is(ferr, domainuser.ErrNotFound) {
			t.Errorf("expected no row after invalid-email path, FindByEmail err = %v", ferr)
		}
	})

	t.Run("rejects a password shorter than 12 bytes", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		err := bootstrap.SeedUser(ctx, db, hasher, log, "short@example.com", "tooShort")

		if err == nil {
			t.Fatal("SeedUser err = nil want a password-policy error for a 12-byte-minimum violation")
		}
		_, ferr := repo.FindByEmail(ctx, "short@example.com")
		if !errors.Is(ferr, domainuser.ErrNotFound) {
			t.Errorf("expected no row after short-password path, FindByEmail err = %v", ferr)
		}
	})

	t.Run("rejects a password longer than 72 bytes", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		repo := userpersist.NewRepository(db)
		ctx := context.Background()
		// 73 bytes — one over bcrypt's input ceiling.
		tooLong := strings.Repeat("a", 73)

		err := bootstrap.SeedUser(ctx, db, hasher, log, "long@example.com", tooLong)

		if err == nil {
			t.Fatal("SeedUser err = nil want a password-too-long error for a 72-byte ceiling violation")
		}
		_, ferr := repo.FindByEmail(ctx, "long@example.com")
		if !errors.Is(ferr, domainuser.ErrNotFound) {
			t.Errorf("expected no row after long-password path, FindByEmail err = %v", ferr)
		}
	})

	t.Run("never echoes the plaintext password through the logger", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		hasher := auth.NewBcryptHasher(4)
		log := &mocks.RecordingLogger{}
		ctx := context.Background()
		// A distinctive sentinel so a substring match is unambiguous.
		secret := "Sentinel-Pa55phrase-A!Z9"

		if err := bootstrap.SeedUser(ctx, db, hasher, log, "leak@example.com", secret); err != nil {
			t.Fatalf("SeedUser: %v", err)
		}
		// Re-run to exercise the duplicate-skip log path as well.
		if err := bootstrap.SeedUser(ctx, db, hasher, log, "leak@example.com", secret); err != nil {
			t.Fatalf("SeedUser second call: %v", err)
		}

		assertNoSecret(t, log, secret)
	})
}

// assertNoSecret walks every recorded log entry and fails the test if the
// plaintext appears in any message, key, or value. The check is intentionally
// blunt — a single sighting is a credential leak (Req 8.5).
func assertNoSecret(t *testing.T, log *mocks.RecordingLogger, secret string) {
	t.Helper()
	for _, r := range log.Records {
		if strings.Contains(r.Msg, secret) {
			t.Errorf("plaintext password leaked into log message: level=%s msg=%q", r.Level, r.Msg)
		}
		for k, v := range r.Args {
			if strings.Contains(k, secret) {
				t.Errorf("plaintext password leaked into log key: level=%s msg=%q key=%q", r.Level, r.Msg, k)
			}
			if s, ok := v.(string); ok && strings.Contains(s, secret) {
				t.Errorf("plaintext password leaked into log value: level=%s msg=%q key=%q value=%q", r.Level, r.Msg, k, s)
			}
		}
	}
}
