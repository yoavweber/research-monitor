package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// bcryptHasher implements shared.PasswordHasher using golang.org/x/crypto/bcrypt.
// Cost is supplied at construction so production wiring (cost 12) and tests
// (cost 4) share the same implementation.
type bcryptHasher struct {
	cost int
}

// NewBcryptHasher returns a PasswordHasher that hashes with the given bcrypt cost.
func NewBcryptHasher(cost int) shared.PasswordHasher {
	return &bcryptHasher{cost: cost}
}

// Hash refuses an empty plaintext before invoking bcrypt — an empty password
// is a validation error, not a hashing error, and the caller should never
// reach the crypto layer with one.
func (h *bcryptHasher) Hash(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("bcrypt hasher: plain password is empty")
	}
	out, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hasher: generate: %w", err)
	}
	return string(out), nil
}

// Verify maps bcrypt's mismatch sentinel to shared.ErrHashMismatch so the
// use-case can distinguish "wrong password" from "hash unreadable" via
// errors.Is. Other errors (malformed hash, unsupported cost) are returned as
// the underlying error so the use-case can fold them into invalid-credentials.
func (h *bcryptHasher) Verify(plain, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return shared.ErrHashMismatch
	}
	return err
}
