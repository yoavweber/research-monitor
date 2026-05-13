package user

import (
	"time"

	"github.com/google/uuid"
)

// User is the operator entity authenticated by the auth module. The aggregate's
// only persistent invariant is the uniqueness of Email, enforced at the
// persistence layer.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
