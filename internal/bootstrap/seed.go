package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/domain/source"
	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
	sourcepersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
)

// Password policy bounds duplicated from internal/domain/user/requests.go,
// where the equivalents (newPasswordMinBytes, bcryptInputMaxBytes) are
// unexported. Both call sites — the CLI seed path and the HTTP change-password
// validator — must agree, so any future loosening or tightening must be
// applied in both places.
const (
	seedPasswordMinBytes = 12
	seedPasswordMaxBytes = 72
)

// SeedSources populates the sources table from seedSources.
//
// Idempotent: source.UseCase.Create returns source.ErrConflict on an already-
// existing URL, which we treat as a successful skip. Any other error aborts
// the seed and is returned verbatim so a deploy that fails mid-seed leaves a
// loud signal in the logs.
func SeedSources(ctx context.Context, db *gorm.DB, clock shared.Clock, logger shared.Logger) error {
	uc := application.NewSourceUseCase(sourcepersist.NewRepository(db), clock)

	var created, skipped int
	for _, req := range seedSources {
		s, err := uc.Create(ctx, req)
		switch {
		case err == nil:
			logger.InfoContext(ctx, "seed.source.created",
				"name", s.Name, "id", s.ID, "url", s.URL)
			created++
		case errors.Is(err, source.ErrConflict):
			logger.InfoContext(ctx, "seed.source.skipped",
				"name", req.Name, "url", req.URL, "reason", "already exists")
			skipped++
		default:
			return fmt.Errorf("seed source %q: %w", req.Name, err)
		}
	}

	logger.InfoContext(ctx, "seed.complete", "created", created, "skipped", skipped)
	return nil
}

// SeedUser provisions a single operator account from the CLI. Idempotent: an
// existing row with the same email yields nil and a skip log line so re-runs
// from a deploy pipeline are safe. The plaintext password is never logged or
// returned in any error message — callers may surface returned errors to
// stderr without leaking the credential. Repository Save does NOT stamp
// timestamps, so this helper assigns CreatedAt/UpdatedAt itself.
func SeedUser(
	ctx context.Context,
	db *gorm.DB,
	hasher shared.PasswordHasher,
	log shared.Logger,
	email, plain string,
) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("seed user: %q is not a valid email address: %w", email, err)
	}
	if len(plain) < seedPasswordMinBytes {
		return fmt.Errorf("seed user: password must be at least %d bytes", seedPasswordMinBytes)
	}
	if len(plain) > seedPasswordMaxBytes {
		return fmt.Errorf("seed user: password must be at most %d bytes", seedPasswordMaxBytes)
	}

	hash, err := hasher.Hash(plain)
	if err != nil {
		return fmt.Errorf("seed user: hash: %w", err)
	}

	repo := userpersist.NewRepository(db)
	now := time.Now().UTC()
	u := &user.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	switch err := repo.Save(ctx, u); {
	case err == nil:
		log.InfoContext(ctx, "seed.user.created", "email", email, "user_id", u.ID)
		return nil
	case errors.Is(err, user.ErrEmailExists):
		log.InfoContext(ctx, "seed.user.skipped", "email", email, "user_id", u.ID, "reason", "already exists")
		return nil
	default:
		return fmt.Errorf("seed user: save: %w", err)
	}
}
