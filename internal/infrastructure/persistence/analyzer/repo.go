package analyzer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

type repository struct{ db *gorm.DB }

// NewRepository builds an analyzer.Repository over the given GORM handle.
//
// The handle MUST be opened with gorm.Config{TranslateError: true} so
// driver-level unique-constraint violations surface as
// gorm.ErrDuplicatedKey — the upsert's conflict branch depends on this.
func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

// Upsert mirrors the extraction repo's pattern: insert, catch
// duplicated-key, then UPDATE via map[string]any so zero-values land. No
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

		// Updates(map) so explicit zero-values (e.g. flag=false) land;
		// Updates(struct) would skip them.
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
