// Package paper is the GORM-backed persistence adapter for paper.Repository.
// It owns the on-disk shape of a stored Entry (the Paper struct), the JSON
// encoding for slice fields that SQLite cannot store natively, and the
// ToDomain/FromDomain conversion that keeps domain code free of GORM types.
package paper

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// Paper is the on-disk row shape. Composite uniqueness on (Source, SourceID)
// is the storage-level invariant that makes dedupe race-safe; see the
// uniqueIndex tags below and the // DEDUPE: marker in repo.go.
type Paper struct {
	ID string `gorm:"type:text;primaryKey"`
	// DEDUPE: Source and SourceID share uniqueIndex idx_papers_source_source_id.
	// AutoMigrate creates this composite index, and concurrent INSERTs that
	// would violate it surface as gorm.ErrDuplicatedKey at the driver level —
	// that's the race-safe enforcement of paper R4.1 / R4.2.
	Source          string    `gorm:"type:text;not null;uniqueIndex:idx_papers_source_source_id;index"`
	SourceID        string    `gorm:"type:text;not null;uniqueIndex:idx_papers_source_source_id"`
	Version         string    `gorm:"type:text"`
	Title           string    `gorm:"type:text;not null"`
	Authors         string    `gorm:"type:text;not null"` // JSON-encoded []string; SQLite has no native array type.
	Abstract        string    `gorm:"type:text;not null"`
	PrimaryCategory string    `gorm:"type:text;not null"`
	Categories      string    `gorm:"type:text;not null"` // JSON-encoded []string; SQLite has no native array type.
	SubmittedAt time.Time `gorm:"not null;index"` // Indexed so List's ORDER BY submitted_at DESC stays cheap.
	// UpdatedAt holds the upstream "last updated" timestamp from the source
	// catalogue, NOT a GORM-managed mtime. We only ever Create (never Update)
	// rows, and the value is non-zero from FromDomain, so GORM's
	// autoUpdateTime logic — which only fills zero values — leaves it intact.
	UpdatedAt time.Time `gorm:"not null"`
	PDFURL    string    `gorm:"type:text"`
	AbsURL    string    `gorm:"type:text"`
	CreatedAt time.Time
}

// TableName is fixed regardless of the package name to keep the migration
// invariant stable across refactors.
func (Paper) TableName() string { return "papers" }

// FromDomain converts a domain Entry into the persistence row, allocating a
// fresh UUID for the primary key. The slice fields (Authors, Categories) are
// JSON-encoded because SQLite cannot store arrays natively. The error returns
// from json.Marshal are unreachable for the current ([]string) field types but
// are kept so the function can absorb future field additions without changing
// its signature.
func FromDomain(e *domain.Entry) (Paper, error) {
	authors, err := json.Marshal(e.Authors)
	if err != nil {
		return Paper{}, fmt.Errorf("marshal authors: %w", err)
	}
	cats, err := json.Marshal(e.Categories)
	if err != nil {
		return Paper{}, fmt.Errorf("marshal categories: %w", err)
	}
	return Paper{
		ID:              uuid.NewString(),
		Source:          e.Source,
		SourceID:        e.SourceID,
		Version:         e.Version,
		Title:           e.Title,
		Authors:         string(authors),
		Abstract:        e.Abstract,
		PrimaryCategory: e.PrimaryCategory,
		Categories:      string(cats),
		SubmittedAt:     e.SubmittedAt,
		UpdatedAt:       e.UpdatedAt,
		PDFURL:          e.PDFURL,
		AbsURL:          e.AbsURL,
	}, nil
}

// ToDomain rehydrates a persistence row into a domain Entry. JSON unmarshal
// errors here mean the row in storage is malformed (likely manual edit or
// corruption) — surfacing as a regular error lets the repository wrap it as
// paper.ErrCatalogueUnavailable just like any other read-side failure.
func (m Paper) ToDomain() (*domain.Entry, error) {
	var authors []string
	if err := json.Unmarshal([]byte(m.Authors), &authors); err != nil {
		return nil, fmt.Errorf("unmarshal authors: %w", err)
	}
	var cats []string
	if err := json.Unmarshal([]byte(m.Categories), &cats); err != nil {
		return nil, fmt.Errorf("unmarshal categories: %w", err)
	}
	return &domain.Entry{
		Source:          m.Source,
		SourceID:        m.SourceID,
		Version:         m.Version,
		Title:           m.Title,
		Authors:         authors,
		Abstract:        m.Abstract,
		PrimaryCategory: m.PrimaryCategory,
		Categories:      cats,
		SubmittedAt:     m.SubmittedAt,
		UpdatedAt:       m.UpdatedAt,
		PDFURL:          m.PDFURL,
		AbsURL:          m.AbsURL,
	}, nil
}
