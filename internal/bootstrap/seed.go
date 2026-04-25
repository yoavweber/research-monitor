package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/domain/source"
	sourcepersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
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
