package user

import (
	"time"

	"github.com/google/uuid"
)

// SessionResponse is the wire-format view of the authenticated user. No
// credential material is included.
type SessionResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginResponse is the JSON body returned by POST /auth/login.
type LoginResponse struct {
	AccessToken string          `json:"access_token"`
	ExpiresAt   time.Time       `json:"expires_at"`
	User        SessionResponse `json:"user"`
}

// LoginResult is the use-case return for Login. The controller maps it to
// LoginResponse on the wire.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      *User
}
