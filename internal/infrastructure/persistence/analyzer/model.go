// Package analyzer is the GORM-backed persistence adapter for
// analyzer.Repository. It owns the on-disk shape of an analyses row and the
// ToDomain / FromDomain conversion that keeps the domain package free of
// GORM types.
package analyzer

import (
	"time"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

// Analysis is the on-disk row shape. Identity is ExtractionID — the same
// string the extraction's persisted UUID uses — and there is no separate id
// column. The upsert contract guarantees one row per ExtractionID.
type Analysis struct {
	ExtractionID         string    `gorm:"column:extraction_id;type:text;primaryKey"`
	ShortSummary         string    `gorm:"column:short_summary;type:text;not null;default:''"`
	LongSummary          string    `gorm:"column:long_summary;type:text;not null;default:''"`
	ThesisAngleFlag      bool      `gorm:"column:thesis_angle_flag;type:integer;not null;default:0"`
	ThesisAngleRationale string    `gorm:"column:thesis_angle_rationale;type:text;not null;default:''"`
	Model                string    `gorm:"column:model;type:text;not null;default:''"`
	PromptVersion        string    `gorm:"column:prompt_version;type:text;not null;default:''"`
	CreatedAt            time.Time `gorm:"column:created_at;not null"`
	UpdatedAt            time.Time `gorm:"column:updated_at;not null"`
}

// TableName pins the GORM table name so it stays stable across package renames.
func (Analysis) TableName() string { return "analyses" }

// FromDomain converts a domain Analysis into the persistence row.
func FromDomain(a domain.Analysis) Analysis {
	return Analysis{
		ExtractionID:         a.ExtractionID,
		ShortSummary:         a.ShortSummary,
		LongSummary:          a.LongSummary,
		ThesisAngleFlag:      a.ThesisAngleFlag,
		ThesisAngleRationale: a.ThesisAngleRationale,
		Model:                a.Model,
		PromptVersion:        a.PromptVersion,
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
	}
}

// ToDomain rehydrates a persistence row into a domain Analysis.
func (m Analysis) ToDomain() domain.Analysis {
	return domain.Analysis{
		ExtractionID:         m.ExtractionID,
		ShortSummary:         m.ShortSummary,
		LongSummary:          m.LongSummary,
		ThesisAngleFlag:      m.ThesisAngleFlag,
		ThesisAngleRationale: m.ThesisAngleRationale,
		Model:                m.Model,
		PromptVersion:        m.PromptVersion,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}
