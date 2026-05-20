package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	userdomain "github.com/yoavweber/research-monitor/backend/internal/domain/user"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

func newUser(t *testing.T, email string) *userdomain.User {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &userdomain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "$2a$12$abcdefghijklmnopqrstuv",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestRepository(t *testing.T) {
	t.Parallel()

	t.Run("Save persists a new user and the row round-trips through FindByID and FindByEmail", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()
		u := newUser(t, "alice@example.com")

		if err := repo.Save(ctx, u); err != nil {
			t.Fatalf("Save: %v", err)
		}

		gotByID, err := repo.FindByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if gotByID.ID != u.ID {
			t.Errorf("FindByID ID = %v want %v", gotByID.ID, u.ID)
		}
		if gotByID.Email != u.Email {
			t.Errorf("FindByID Email = %q want %q", gotByID.Email, u.Email)
		}
		if gotByID.PasswordHash != u.PasswordHash {
			t.Errorf("FindByID PasswordHash = %q want %q", gotByID.PasswordHash, u.PasswordHash)
		}

		gotByEmail, err := repo.FindByEmail(ctx, u.Email)
		if err != nil {
			t.Fatalf("FindByEmail: %v", err)
		}
		if gotByEmail.ID != u.ID {
			t.Errorf("FindByEmail ID = %v want %v", gotByEmail.ID, u.ID)
		}
		if gotByEmail.Email != u.Email {
			t.Errorf("FindByEmail Email = %q want %q", gotByEmail.Email, u.Email)
		}
	})

	t.Run("Save returns ErrEmailExists when another row already uses that email", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()
		first := newUser(t, "dup@example.com")
		second := newUser(t, "dup@example.com")
		if err := repo.Save(ctx, first); err != nil {
			t.Fatalf("seed Save: %v", err)
		}

		err := repo.Save(ctx, second)

		if !errors.Is(err, userdomain.ErrEmailExists) {
			t.Fatalf("Save duplicate err = %v want ErrEmailExists", err)
		}
	})

	t.Run("FindByEmail returns ErrNotFound for an unknown email", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		_, err := repo.FindByEmail(ctx, "ghost@example.com")

		if !errors.Is(err, userdomain.ErrNotFound) {
			t.Fatalf("FindByEmail err = %v want ErrNotFound", err)
		}
	})

	t.Run("FindByID returns ErrNotFound for an unknown id", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		_, err := repo.FindByID(ctx, uuid.New())

		if !errors.Is(err, userdomain.ErrNotFound) {
			t.Fatalf("FindByID err = %v want ErrNotFound", err)
		}
	})

	t.Run("UpdatePasswordHash replaces only the hash and bumps updated_at", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()
		u := newUser(t, "rotator@example.com")
		if err := repo.Save(ctx, u); err != nil {
			t.Fatalf("seed Save: %v", err)
		}
		newHash := "$2a$12$ZZZZZZZZZZZZZZZZZZZZZZ"

		// Sleep is avoided; instead we read back UpdatedAt and assert it is
		// at least as recent as the pre-update timestamp.
		preUpdate := time.Now().UTC()

		if err := repo.UpdatePasswordHash(ctx, u.ID, newHash); err != nil {
			t.Fatalf("UpdatePasswordHash: %v", err)
		}

		got, err := repo.FindByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("FindByID after update: %v", err)
		}
		if got.PasswordHash != newHash {
			t.Errorf("PasswordHash = %q want %q", got.PasswordHash, newHash)
		}
		if got.Email != u.Email {
			t.Errorf("Email mutated: got %q want %q", got.Email, u.Email)
		}
		if !got.CreatedAt.Equal(u.CreatedAt) {
			t.Errorf("CreatedAt mutated: got %v want %v", got.CreatedAt, u.CreatedAt)
		}
		if got.UpdatedAt.Before(preUpdate) {
			t.Errorf("UpdatedAt = %v not bumped (preUpdate %v)", got.UpdatedAt, preUpdate)
		}
	})

	t.Run("UpdatePasswordHash returns ErrNotFound for an unknown id", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)
		repo := userpersist.NewRepository(db)
		ctx := context.Background()

		err := repo.UpdatePasswordHash(ctx, uuid.New(), "$2a$12$does-not-matter")

		if !errors.Is(err, userdomain.ErrNotFound) {
			t.Fatalf("UpdatePasswordHash err = %v want ErrNotFound", err)
		}
	})
}
