package analyzer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

// repository is the GORM-backed analyzer.Repository implementation. It is
// unexported so callers depend on the domain port; only NewRepository is
// public and it returns the interface, not the struct.
type repository struct{ db *gorm.DB }

// NewRepository builds an analyzer.Repository over the given GORM handle.
//
// The handle MUST be opened with gorm.Config{TranslateError: true} so that
// driver-level unique-constraint violations surface as gorm.ErrDuplicatedKey
// — the upsert path's conflict branch depends on this.
func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

// Upsert inserts the row, or — on a primary-key conflict — overwrites the
// existing row's content while preserving its CreatedAt. Mirrors the
// extraction repository's pattern (transaction + duplicated-key catch +
// explicit UPDATE via map[string]any so zero-values land). No
// clause.OnConflict — the project precedent is the manual path.
func (r *repository) Upsert(ctx context.Context, a domain.Analysis) (domain.Analysis, error) {
	row := FromDomain(a)

	switch err := r.db.WithContext(ctx).Create(&row).Error; {
	case err == nil:
		return row.ToDomain(), nil
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return r.overwriteOnConflict(ctx, a)
	default:
		return domain.Analysis{}, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// overwriteOnConflict reads the existing row's CreatedAt and atomically
// rewrites the content fields plus UpdatedAt inside a single transaction so
// concurrent reruns converge on exactly one row per ExtractionID.
func (r *repository) overwriteOnConflict(ctx context.Context, a domain.Analysis) (domain.Analysis, error) {
	var (
		preservedCreated time.Time
		updatedAt        = a.UpdatedAt
	)

	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Analysis
		if err := tx.
			Select("created_at").
			Where("extraction_id = ?", a.ExtractionID).
			First(&existing).Error; err != nil {
			return err
		}
		preservedCreated = existing.CreatedAt

		// Updates(map) so explicit zero-values land. The boolean is special:
		// GORM's Updates skips zero values for struct paths, but maps include
		// every key, so flag = false is persisted faithfully.
		return tx.Model(&Analysis{}).
			Where("extraction_id = ?", a.ExtractionID).
			Updates(map[string]any{
				"short_summary":          a.ShortSummary,
				"long_summary":           a.LongSummary,
				"thesis_angle_flag":      a.ThesisAngleFlag,
				"thesis_angle_rationale": a.ThesisAngleRationale,
				"model":                  a.Model,
				"prompt_version":         a.PromptVersion,
				"updated_at":             updatedAt,
			}).Error
	})
	if txErr != nil {
		return domain.Analysis{}, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, txErr)
	}

	a.CreatedAt = preservedCreated
	a.UpdatedAt = updatedAt
	return a, nil
}

// FindByID returns the row keyed by ExtractionID, or ErrAnalysisNotFound.
// Any other storage failure wraps ErrCatalogueUnavailable.
func (r *repository) FindByID(ctx context.Context, extractionID string) (*domain.Analysis, error) {
	var row Analysis
	err := r.db.WithContext(ctx).
		Where("extraction_id = ?", extractionID).
		First(&row).Error
	switch {
	case err == nil:
		out := row.ToDomain()
		return &out, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, domain.ErrAnalysisNotFound
	default:
		return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}
