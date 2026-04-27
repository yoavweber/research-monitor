package paper

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// repository is the GORM-backed paper.Repository implementation. The struct
// is unexported so callers depend on the domain port; only NewRepository is
// public and it returns the interface, not the struct.
type repository struct{ db *gorm.DB }

// NewRepository builds a paper.Repository over the given GORM handle.
//
// The handle MUST be opened with gorm.Config{TranslateError: true} (the
// project's bootstrap helper enables this) so that driver-level unique
// constraint violations surface as gorm.ErrDuplicatedKey. Without that flag
// the dedupe path in Save would silently fall through to the catastrophic
// branch and report duplicates as catalogue failures.
func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

// Save inserts a new row or reports a dedupe skip on composite-key collision.
// First-seen wins per paper R1.2 / R1.6: a second Save with the same
// (Source, SourceID) is NOT an error — it returns (false, nil), which the
// arxiv use case surfaces as is_new=false on its response.
func (r *repository) Save(ctx context.Context, e domain.Entry) (bool, error) {
	m, err := FromDomain(&e)
	if err != nil {
		// JSON marshal failure on a domain slice is a programmer error in the
		// caller, not a storage condition. Wrap it in the catalogue sentinel
		// so callers see one error type from this port.
		return false, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
	switch err := r.db.WithContext(ctx).Create(&m).Error; {
	case err == nil:
		return true, nil
	case errors.Is(err, gorm.ErrDuplicatedKey):
		// DEDUPE: the composite unique index idx_papers_source_source_id
		// rejected an insert that collided with an existing (Source, SourceID).
		// This is the storage-level race-safe enforcement (R4.1, R4.2);
		// returning (false, nil) is the normal "skipped" outcome, never an
		// error.
		return false, nil
	default:
		// Wrap with the catalogue sentinel so errors.Is(err, ErrCatalogueUnavailable)
		// holds, while %v keeps the underlying driver message in logs for
		// operators (R5.5).
		return false, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// FindByKey returns the row keyed by the (source, sourceID) composite or a
// typed sentinel. gorm.ErrRecordNotFound becomes paper.ErrNotFound (404);
// every other DB-side failure becomes paper.ErrCatalogueUnavailable (500).
// Raw GORM errors never leak past this method.
func (r *repository) FindByKey(ctx context.Context, source, sourceID string) (*domain.Entry, error) {
	var m Paper
	err := r.db.WithContext(ctx).
		Where("source = ? AND source_id = ?", source, sourceID).
		First(&m).Error
	switch {
	case err == nil:
		entry, convErr := m.ToDomain()
		if convErr != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, convErr)
		}
		return entry, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, domain.ErrNotFound
	default:
		return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// List returns every persisted entry newest-first by SubmittedAt. An empty
// catalogue yields a non-nil empty slice (R3.3); only DB-side failures
// surface as paper.ErrCatalogueUnavailable.
func (r *repository) List(ctx context.Context) ([]domain.Entry, error) {
	var rows []Paper
	if err := r.db.WithContext(ctx).Order("submitted_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
	out := make([]domain.Entry, 0, len(rows))
	for i := range rows {
		entry, err := rows[i].ToDomain()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
		}
		out = append(out, *entry)
	}
	return out, nil
}
