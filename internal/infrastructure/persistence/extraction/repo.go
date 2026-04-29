package extraction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// repository is the GORM-backed extraction.Repository implementation. The
// struct is unexported so callers depend on the domain port; only
// NewRepository is public and it returns the interface, not the struct.
type repository struct{ db *gorm.DB }

// NewRepository builds an extraction.Repository over the given GORM handle.
//
// The handle MUST be opened with gorm.Config{TranslateError: true} (the
// project's bootstrap helper enables this) so that driver-level unique
// constraint violations surface as gorm.ErrDuplicatedKey. Without that flag
// the conflict path in Upsert would silently fall through to the catastrophic
// branch and report duplicates as catalogue failures.
func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

// Upsert inserts a new pending row for payload, or — if a row with the same
// (SourceType, SourceID) already exists — overwrites it: status reset to
// pending, body / failure cleared, request_payload replaced, created_at
// refreshed so the new request gets a full job_expiry window.
//
// The original row id is preserved on overwrite. priorStatus is non-nil iff
// a row was overwritten.
//
// Why time.Now() lives here rather than coming from a shared.Clock: this
// repository is the single writer of created_at, and v1 has no test that
// injects a clock at this layer (use-case-layer expiry tests inject the
// clock themselves). The repo is a sink for the timestamp.
func (r *repository) Upsert(ctx context.Context, payload domain.RequestPayload) (string, *domain.PriorState, error) {
	now := time.Now().UTC()

	fresh := domain.Extraction{
		SourceType:     payload.SourceType,
		SourceID:       payload.SourceID,
		Status:         domain.JobStatusPending,
		RequestPayload: payload,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	row, err := FromDomain(&fresh)
	if err != nil {
		// JSON marshal failure on a fixed-shape RequestPayload is a programmer
		// error in the caller, not a storage condition. Wrap with the
		// catalogue sentinel so callers see a consistent error type.
		return "", nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}

	switch err := r.db.WithContext(ctx).Create(&row).Error; {
	case err == nil:
		return row.ID, nil, nil
	case errors.Is(err, gorm.ErrDuplicatedKey):
		// CONFLICT path: a row with the same (SourceType, SourceID) already
		// exists. We capture its prior state and overwrite it within a single
		// transaction so the prior-status read and the overwrite are atomic
		// from the caller's point of view.
		return r.overwriteOnConflict(ctx, payload, now)
	default:
		return "", nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// overwriteOnConflict reads the prior state for the (SourceType, SourceID)
// composite key and atomically resets the row to a fresh pending state inside
// a single transaction. The original id is preserved.
func (r *repository) overwriteOnConflict(ctx context.Context, payload domain.RequestPayload, now time.Time) (string, *domain.PriorState, error) {
	freshRow, marshalErr := FromDomain(&domain.Extraction{
		SourceType:     payload.SourceType,
		SourceID:       payload.SourceID,
		Status:         domain.JobStatusPending,
		RequestPayload: payload,
	})
	if marshalErr != nil {
		return "", nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, marshalErr)
	}

	var (
		priorID    string
		priorState domain.PriorState
	)
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Extraction
		if err := tx.
			Select("id", "status", "failure_reason").
			Where("source_type = ? AND source_id = ?", payload.SourceType, payload.SourceID).
			First(&existing).Error; err != nil {
			return err
		}
		priorID = existing.ID
		priorState = domain.PriorState{
			Status:        existing.Status,
			FailureReason: existing.FailureReason,
		}

		// Overwrite resets status / artifact / failure and refreshes
		// created_at so the new request gets a full job_expiry window. Using
		// Updates(map) so explicit zero-values land in the row.
		return tx.Model(&Extraction{}).
			Where("source_type = ? AND source_id = ?", payload.SourceType, payload.SourceID).
			Updates(map[string]any{
				"status":                domain.JobStatusPending,
				"body_markdown":         "",
				"metadata_content_type": "",
				"metadata_word_count":   0,
				"title":                 "",
				"failure_reason":        domain.FailureReason(""),
				"failure_message":       "",
				"request_payload":       freshRow.RequestPayload,
				"created_at":            now,
				"updated_at":            now,
			}).Error
	})
	if txErr != nil {
		return "", nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, txErr)
	}
	return priorID, &priorState, nil
}

// FindByID returns the extraction row keyed by id or a typed sentinel.
// gorm.ErrRecordNotFound becomes domain.ErrNotFound (404); JSON unmarshal
// failure on the persisted request_payload becomes ErrCatalogueUnavailable
// so callers see one consistent error for read-side failures.
func (r *repository) FindByID(ctx context.Context, id string) (*domain.Extraction, error) {
	var row Extraction
	switch err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; {
	case err == nil:
		out, convErr := row.ToDomain()
		if convErr != nil {
			// ToDomain already wraps with ErrCatalogueUnavailable, but we
			// re-wrap here so the operation name lands in the chain for logs.
			return nil, fmt.Errorf("find_by_id: %w", convErr)
		}
		return out, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, domain.ErrNotFound
	default:
		return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// PeekNextPending returns the oldest pending row (oldest by created_at)
// without transitioning its status. ok=false signals an empty queue, which is
// not an error condition.
func (r *repository) PeekNextPending(ctx context.Context) (*domain.Extraction, bool, error) {
	var row Extraction
	switch err := r.db.WithContext(ctx).
		Where("status = ?", domain.JobStatusPending).
		Order("created_at ASC").
		First(&row).Error; {
	case err == nil:
		out, convErr := row.ToDomain()
		if convErr != nil {
			return nil, false, fmt.Errorf("peek_next_pending: %w", convErr)
		}
		return out, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
}

// ClaimPending atomically transitions a row from pending to running. The
// status='pending' predicate in the WHERE clause makes this race-safe: a
// second concurrent claimer observes RowsAffected==0 and gets
// ErrInvalidTransition rather than silently re-running.
func (r *repository) ClaimPending(ctx context.Context, id string) error {
	now := time.Now().UTC()
	tx := r.db.WithContext(ctx).
		Model(&Extraction{}).
		Where("id = ? AND status = ?", id, domain.JobStatusPending).
		Updates(map[string]any{
			"status":     domain.JobStatusRunning,
			"updated_at": now,
		})
	if tx.Error != nil {
		return fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("%w: row %s is not in pending", domain.ErrInvalidTransition, id)
	}
	return nil
}

// MarkDone writes the artifact and transitions running -> done. The
// status='running' predicate enforces the precondition: a zero-rows-affected
// UPDATE means the row is no longer in running (already done / failed /
// pending) and surfaces as ErrInvalidTransition.
func (r *repository) MarkDone(ctx context.Context, id string, artifact domain.Artifact) error {
	now := time.Now().UTC()
	tx := r.db.WithContext(ctx).
		Model(&Extraction{}).
		Where("id = ? AND status = ?", id, domain.JobStatusRunning).
		Updates(map[string]any{
			"status":                domain.JobStatusDone,
			"body_markdown":         artifact.BodyMarkdown,
			"metadata_content_type": artifact.Metadata.ContentType,
			"metadata_word_count":   artifact.Metadata.WordCount,
			"title":                 artifact.Title,
			"updated_at":            now,
		})
	if tx.Error != nil {
		return fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("%w: row %s is not in running (mark_done)", domain.ErrInvalidTransition, id)
	}
	return nil
}

// MarkFailed transitions a row to failed with the given reason and message.
// The allowed prior status is reason-conditional:
//   - FailureReasonExpired: prior status MUST be pending. The expiry
//     predicate is evaluated at pickup, before the worker claims, so an
//     expired row is failed straight from pending without ever going running.
//   - any other reason: prior status MUST be running. Extractor-side and
//     normalization-side failures only happen during the running phase.
func (r *repository) MarkFailed(ctx context.Context, id string, reason domain.FailureReason, message string) error {
	expectedPrior := domain.JobStatusRunning
	if reason == domain.FailureReasonExpired {
		expectedPrior = domain.JobStatusPending
	}
	now := time.Now().UTC()

	tx := r.db.WithContext(ctx).
		Model(&Extraction{}).
		Where("id = ? AND status = ?", id, expectedPrior).
		Updates(map[string]any{
			"status":          domain.JobStatusFailed,
			"failure_reason":  reason,
			"failure_message": message,
			"updated_at":      now,
		})
	if tx.Error != nil {
		return fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf(
			"%w: row %s is not in %s (mark_failed reason=%s)",
			domain.ErrInvalidTransition, id, expectedPrior, reason,
		)
	}
	return nil
}

// RecoverRunningOnStartup transitions every running row to
// failed: process_restart. Called from bootstrap before the worker goroutine
// launches and before any HTTP request is served. The operation is idempotent
// — a second consecutive call observes recovered == 0.
func (r *repository) RecoverRunningOnStartup(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	tx := r.db.WithContext(ctx).
		Model(&Extraction{}).
		Where("status = ?", domain.JobStatusRunning).
		Updates(map[string]any{
			"status":          domain.JobStatusFailed,
			"failure_reason":  domain.FailureReasonProcessRestart,
			"failure_message": "process exited while extraction was in flight",
			"updated_at":      now,
		})
	if tx.Error != nil {
		return 0, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, tx.Error)
	}
	return int(tx.RowsAffected), nil
}

// ListPendingIDs returns the ids of every pending row, oldest-first by
// created_at. An empty queue yields a non-nil empty slice so callers can
// `for _, id := range ids` without nil checks; only DB-side failures surface
// as ErrCatalogueUnavailable.
func (r *repository) ListPendingIDs(ctx context.Context) ([]string, error) {
	ids := make([]string, 0)
	if err := r.db.WithContext(ctx).
		Model(&Extraction{}).
		Where("status = ?", domain.JobStatusPending).
		Order("created_at ASC").
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrCatalogueUnavailable, err)
	}
	return ids, nil
}
