package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository — persistence port. Implemented in
// infrastructure/persistence/user/repo.go.
type Repository interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	Save(ctx context.Context, u *User) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
}

// UseCase — application port. Implemented in application/user_usecase.go.
// There is no Logout method: tokens are stateless and there is no
// server-side state to clear, so logout reduces to the client discarding the
// JWT.
type UseCase interface {
	Login(ctx context.Context, req LoginRequest) (LoginResult, error)
	Session(ctx context.Context, userID uuid.UUID) (*User, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) error
}
