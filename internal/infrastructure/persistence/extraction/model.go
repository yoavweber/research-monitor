// Package extraction is the GORM-backed persistence adapter for
// extraction.Repository. It owns the on-disk shape of an extraction row,
// the JSON encoding for the request_payload column, and the ToDomain /
// FromDomain conversion that keeps domain code free of GORM types.
package extraction

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// Extraction is the on-disk row shape. The composite uniqueIndex on
// (SourceType, SourceID) is what enforces the one-artifact-per-paper rule
// race-safely: gorm.ErrDuplicatedKey surfaces from the driver when a parallel
// insert would violate it. The (Status, CreatedAt) composite index backs
// PeekNextPending's ORDER-BY-created_at-LIMIT-1 scan over pending rows.
type Extraction struct {
	ID                  string               `gorm:"type:text;primaryKey"`
	SourceType          string               `gorm:"type:text;not null;uniqueIndex:idx_extractions_source_source_id;index"`
	SourceID            string               `gorm:"type:text;not null;uniqueIndex:idx_extractions_source_source_id"`
	Status              domain.JobStatus     `gorm:"type:text;not null;index:idx_extractions_status_created_at,priority:1"`
	RequestPayload      string               `gorm:"column:request_payload;type:text;not null"`
	BodyMarkdown        string               `gorm:"column:body_markdown;type:text;not null;default:''"`
	MetadataContentType string               `gorm:"column:metadata_content_type;type:text;not null;default:''"`
	MetadataWordCount   int                  `gorm:"column:metadata_word_count;type:integer;not null;default:0"`
	Title               string               `gorm:"type:text;not null;default:''"`
	FailureReason       domain.FailureReason `gorm:"column:failure_reason;type:text;not null;default:''"`
	FailureMessage      string               `gorm:"column:failure_message;type:text;not null;default:''"`
	CreatedAt           time.Time            `gorm:"not null;index:idx_extractions_status_created_at,priority:2"`
	UpdatedAt           time.Time            `gorm:"not null"`
}

// TableName pins the GORM table name so it stays stable across package renames.
func (Extraction) TableName() string { return "extractions" }

// FromDomain converts a domain Extraction into the persistence row. A fresh
// UUID is allocated only when the input ID is empty, so update paths through
// the repository preserve the row's identity. The error return covers a
// future RequestPayload shape that adds a field whose marshaler can fail —
// today the marshal is infallible for the fixed three-string struct.
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
		Status:         e.Status,
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
		row.FailureReason = e.Failure.Reason
		row.FailureMessage = e.Failure.Message
	}
	return row, nil
}

// ToDomain rehydrates a persistence row into a domain Extraction. A malformed
// RequestPayload (manual edit, corruption) is wrapped as
// ErrCatalogueUnavailable so callers see one consistent sentinel for
// read-side failures. Artifact is populated only when status == done;
// Failure only when status == failed.
func (m Extraction) ToDomain() (*domain.Extraction, error) {
	var payload domain.RequestPayload
	if err := json.Unmarshal([]byte(m.RequestPayload), &payload); err != nil {
		return nil, fmt.Errorf("%w: unmarshal request_payload: %v", domain.ErrCatalogueUnavailable, err)
	}

	out := &domain.Extraction{
		ID:             m.ID,
		SourceType:     m.SourceType,
		SourceID:       m.SourceID,
		Status:         m.Status,
		RequestPayload: payload,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	if m.Status == domain.JobStatusDone {
		out.Artifact = &domain.Artifact{
			Title:        m.Title,
			BodyMarkdown: m.BodyMarkdown,
			Metadata: domain.Metadata{
				ContentType: m.MetadataContentType,
				WordCount:   m.MetadataWordCount,
			},
		}
	}
	if m.Status == domain.JobStatusFailed {
		out.Failure = &domain.Failure{
			Reason:  m.FailureReason,
			Message: m.FailureMessage,
		}
	}
	return out, nil
}
