package auth_test

import (
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
)

const testCost = 4

func TestBcryptHasher_Hash(t *testing.T) {
	t.Run("returns error for empty password before calling bcrypt", func(t *testing.T) {
		t.Parallel()
		h := auth.NewBcryptHasher(testCost)

		_, err := h.Hash("")

		if err == nil {
			t.Fatal("expected error for empty password, got nil")
		}
	})
}

func TestBcryptHasher_Verify(t *testing.T) {
	t.Run("round-trip hash and verify returns nil", func(t *testing.T) {
		t.Parallel()
		h := auth.NewBcryptHasher(testCost)
		plain := "correct horse battery staple"

		hash, err := h.Hash(plain)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}

		if err := h.Verify(plain, hash); err != nil {
			t.Errorf("verify: %v, want nil", err)
		}
	})

	t.Run("returns ErrHashMismatch on wrong password", func(t *testing.T) {
		t.Parallel()
		h := auth.NewBcryptHasher(testCost)
		hash, err := h.Hash("correct horse battery staple")
		if err != nil {
			t.Fatalf("hash: %v", err)
		}

		err = h.Verify("wrong password", hash)

		if !errors.Is(err, shared.ErrHashMismatch) {
			t.Errorf("err = %v, want ErrHashMismatch", err)
		}
	})
}
