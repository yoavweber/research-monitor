// Package analyzer is the GORM-backed persistence adapter for
// analyzer.Repository. From/ToDomain keep GORM types out of the domain
// package.
package analyzer

import (
	"time"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

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

func (Analysis) TableName() string { return "analyses" }

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
