package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	domainuser "github.com/yoavweber/research-monitor/backend/internal/domain/user"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
)

func TestModel_ToDomain(t *testing.T) {
	t.Parallel()

	t.Run("parses the string ID back into a uuid.UUID", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		now := time.Now().UTC()
		m := userpersist.Model{
			ID:           id.String(),
			Email:        "alice@example.com",
			PasswordHash: "$2a$12$abcdefghijklmnopqrstuv",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		got, err := m.ToDomain()

		if err != nil {
			t.Fatalf("ToDomain: %v", err)
		}
		if got.ID != id {
			t.Errorf("ID = %v want %v", got.ID, id)
		}
		if got.Email != m.Email {
			t.Errorf("Email = %q want %q", got.Email, m.Email)
		}
		if got.PasswordHash != m.PasswordHash {
			t.Errorf("PasswordHash = %q want %q", got.PasswordHash, m.PasswordHash)
		}
		if !got.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v want %v", got.CreatedAt, now)
		}
		if !got.UpdatedAt.Equal(now) {
			t.Errorf("UpdatedAt = %v want %v", got.UpdatedAt, now)
		}
	})

	t.Run("returns nil and an error when the stored ID is malformed", func(t *testing.T) {
		t.Parallel()
		m := userpersist.Model{
			ID:           "not-a-uuid",
			Email:        "alice@example.com",
			PasswordHash: "hash",
		}

		got, err := m.ToDomain()

		if err == nil {
			t.Fatal("expected error for malformed ID, got nil")
		}
		if got != nil {
			t.Errorf("expected nil user on error, got %+v", got)
		}
	})
}

func TestModel_FromDomain(t *testing.T) {
	t.Parallel()

	t.Run("serializes a domain user into a persistence model with matching fields", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		now := time.Now().UTC()
		u := &domainuser.User{
			ID:           id,
			Email:        "bob@example.com",
			PasswordHash: "$2a$12$zyxwvutsrqponmlkjihgf",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		got := userpersist.FromDomain(u)

		if got.ID != id.String() {
			t.Errorf("ID = %q want %q", got.ID, id.String())
		}
		if got.Email != u.Email {
			t.Errorf("Email = %q want %q", got.Email, u.Email)
		}
		if got.PasswordHash != u.PasswordHash {
			t.Errorf("PasswordHash = %q want %q", got.PasswordHash, u.PasswordHash)
		}
		if !got.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v want %v", got.CreatedAt, now)
		}
		if !got.UpdatedAt.Equal(now) {
			t.Errorf("UpdatedAt = %v want %v", got.UpdatedAt, now)
		}
	})
}
