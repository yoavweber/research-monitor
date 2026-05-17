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

// LoginResponse is the JSON body returned by POST /auth/login. The refresh
// token is delivered via Set-Cookie, not in this body.
type LoginResponse struct {
	AccessToken string          `json:"access_token"`
	ExpiresAt   time.Time       `json:"expires_at"`
	User        SessionResponse `json:"user"`
}

// RefreshResponse is the JSON body returned by POST /auth/refresh. Refresh
// tokens are not rotated, so no new cookie is issued.
type RefreshResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// LoginResult is the use-case return for Login. The controller maps the
// access token, expiry, and user into LoginResponse, and sets the refresh
// token in a cookie.
type LoginResult struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             *User
}

// RefreshResult is the use-case return for Refresh. Only an access token is
// issued; the refresh token is not rotated.
type RefreshResult struct {
	AccessToken string
	ExpiresAt   time.Time
}
