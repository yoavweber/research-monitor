package user

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/user"
)

// repository is the GORM-backed implementation of domain.Repository. It is
// unexported so callers depend on the interface; the constructor returns the
// interface, not the concrete type.
type repository struct{ db *gorm.DB }

// NewRepository wires a GORM connection to the user.Repository port. The DB
// is expected to have TranslateError enabled so unique-constraint violations
// surface as gorm.ErrDuplicatedKey rather than driver-specific strings.
func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

// Save inserts a new user row. A duplicate email is mapped to the domain
// sentinel so the use-case layer can keep its error contract independent of
// the persistence driver.
func (r *repository) Save(ctx context.Context, u *domain.User) error {
	m := FromDomain(u)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.ErrEmailExists
		}
		return err
	}
	return nil
}

// FindByEmail looks up a user by email. ToDomain errors are returned
// unwrapped so the use-case can fold them into invalid_credentials without
// learning persistence details.
func (r *repository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var m Model
	if err := r.db.WithContext(ctx).First(&m, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain()
}

// FindByID looks up a user by primary key.
func (r *repository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var m Model
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain()
}

// UpdatePasswordHash rotates only the password_hash column and refreshes
// updated_at. A full-record Save would also rewrite email and created_at,
// which a hash rotation must not touch.
func (r *repository) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	tx := r.db.WithContext(ctx).
		Model(&Model{}).
		Where("id = ?", id.String()).
		Updates(map[string]any{
			"password_hash": hash,
			"updated_at":    time.Now().UTC(),
		})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
