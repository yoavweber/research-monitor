// Package extraction is the GORM-backed persistence adapter for
// extraction.Repository. It owns the on-disk shape of an extraction row
// (the Extraction struct), the JSON encoding for the request_payload column,
// and the ToDomain / FromDomain conversion that keeps domain code free of
// GORM types. The repository implementation lives in repo.go (added later).
package extraction

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// Extraction is the on-disk row shape. Composite uniqueness on
// (SourceType, SourceID) is the storage-level invariant that enforces the
// "at most one artifact per (source_type, source_id)" rule (Requirement 6.3),
// race-safe via gorm.ErrDuplicatedKey at the driver level (TranslateError on).
//
// The composite index on (Status, CreatedAt) backs PeekNextPending's
// ORDER-BY-created_at-LIMIT-1 scan over pending rows.
type Extraction struct {
	ID string `gorm:"type:text;primaryKey"`
	// DEDUPE: SourceType and SourceID share uniqueIndex
	// idx_extractions_source_source_id. AutoMigrate creates this composite
	// index, and concurrent Upserts that would violate it surface as
	// gorm.ErrDuplicatedKey at the driver level — that's the race-safe
	// enforcement of Requirement 6.3.
	SourceType string `gorm:"type:text;not null;uniqueIndex:idx_extractions_source_source_id;index"`
	SourceID   string `gorm:"type:text;not null;uniqueIndex:idx_extractions_source_source_id"`
	// Status is the leading column of idx_extractions_status_created_at so
	// PeekNextPending's WHERE status='pending' ORDER BY created_at scan stays
	// cheap.
	Status              string    `gorm:"type:text;not null;index:idx_extractions_status_created_at,priority:1"`
	RequestPayload      string    `gorm:"column:request_payload;type:text;not null"` // JSON-encoded domain.RequestPayload.
	BodyMarkdown        string    `gorm:"column:body_markdown;type:text;not null;default:''"`
	MetadataContentType string    `gorm:"column:metadata_content_type;type:text;not null;default:''"`
	MetadataWordCount   int       `gorm:"column:metadata_word_count;type:integer;not null;default:0"`
	Title               string    `gorm:"type:text;not null;default:''"`
	FailureReason       string    `gorm:"column:failure_reason;type:text;not null;default:''"`
	FailureMessage      string    `gorm:"column:failure_message;type:text;not null;default:''"`
	CreatedAt           time.Time `gorm:"not null;index:idx_extractions_status_created_at,priority:2"`
	UpdatedAt           time.Time `gorm:"not null"`
}

// TableName is fixed regardless of the package name to keep the migration
// invariant stable across refactors.
func (Extraction) TableName() string { return "extractions" }

// FromDomain converts a domain Extraction into the persistence row. A fresh
// UUID is allocated only when the input ID is empty, so update paths through
// the repository preserve the row's identity. RequestPayload is JSON-encoded
// because it is a struct-shaped value the row stores in a single TEXT column.
// The error return covers JSON marshal failure — unreachable for the current
// RequestPayload shape but kept so the function can absorb future field
// additions without changing its signature (mirrors paper.FromDomain).
func FromDomain(e *domain.Extraction) (Extraction, error) {
	payload, err := json.Marshal(e.RequestPayload)
	if err != nil {
		return Extraction{}, fmt.Errorf("marshal request_payload: %w", err)
	}

	id := e.ID
	if id == "" {
		id = uuid.NewString()
	}

	row := Extraction{
		ID:             id,
		SourceType:     e.SourceType,
		SourceID:       e.SourceID,
		Status:         string(e.Status),
		RequestPayload: string(payload),
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
	if e.Artifact != nil {
		row.Title = e.Artifact.Title
		row.BodyMarkdown = e.Artifact.BodyMarkdown
		row.MetadataContentType = e.Artifact.Metadata.ContentType
		row.MetadataWordCount = e.Artifact.Metadata.WordCount
	}
	if e.Failure != nil {
		row.FailureReason = string(e.Failure.Reason)
		row.FailureMessage = e.Failure.Message
	}
	return row, nil
}

// ToDomain rehydrates a persistence row into a domain Extraction. JSON
// unmarshal errors here mean the row in storage is malformed (likely manual
// edit or corruption); we wrap as ErrCatalogueUnavailable so callers see one
// consistent sentinel for read-side failures (mirrors paper.ToDomain).
//
// Artifact is populated only when status == done; Failure only when status ==
// failed. The aggregate's invariant — Artifact non-nil iff status == done,
// Failure non-nil iff status == failed — is preserved by this asymmetric
// hydration.
func (m Extraction) ToDomain() (*domain.Extraction, error) {
	var payload domain.RequestPayload
	if err := json.Unmarshal([]byte(m.RequestPayload), &payload); err != nil {
		return nil, fmt.Errorf("%w: unmarshal request_payload: %v", domain.ErrCatalogueUnavailable, err)
	}

	status := domain.JobStatus(m.Status)
	out := &domain.Extraction{
		ID:             m.ID,
		SourceType:     m.SourceType,
		SourceID:       m.SourceID,
		Status:         status,
		RequestPayload: payload,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	if status == domain.JobStatusDone {
		out.Artifact = &domain.Artifact{
			Title:        m.Title,
			BodyMarkdown: m.BodyMarkdown,
			Metadata: domain.Metadata{
				ContentType: m.MetadataContentType,
				WordCount:   m.MetadataWordCount,
			},
		}
	}
	if status == domain.JobStatusFailed {
		out.Failure = &domain.Failure{
			Reason:  domain.FailureReason(m.FailureReason),
			Message: m.FailureMessage,
		}
	}
	return out, nil
}
