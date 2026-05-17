// Package user holds the GORM persistence model for the user aggregate.
// Consumers import this package as `userpersist` to disambiguate from the
// domain package at internal/domain/user.
package user

import (
	"time"

	"github.com/google/uuid"

	domainuser "github.com/yoavweber/research-monitor/backend/internal/domain/user"
)

// Model is the GORM-tagged row for the users table. The schema is the
// physical data model from the user-auth design: text primary key, unique
// email, mandatory bcrypt hash, and timestamps managed by GORM.
type Model struct {
	ID           string    `gorm:"type:text;primaryKey"`
	Email        string    `gorm:"type:text;not null;uniqueIndex"`
	PasswordHash string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

// TableName pins the SQL table name so a future rename of the Go type does
// not silently rewrite the schema.
func (Model) TableName() string { return "users" }

// FromDomain serializes a domain user into a persistence row. The UUID is
// stored as its canonical text form so the row remains portable across
// SQLite, Postgres, and any future store.
func FromDomain(u *domainuser.User) Model {
	return Model{
		ID:           u.ID.String(),
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// ToDomain parses the stored row into the domain aggregate. A malformed ID is
// surfaced as an error rather than panicking so callers can map storage
// corruption to a credentials failure without leaking the underlying cause.
func (m Model) ToDomain() (*domainuser.User, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, err
	}
	return &domainuser.User{
		ID:           id,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}, nil
}
