package source

import (
	"time"

	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
)

type Source struct {
	ID            string     `gorm:"type:text;primaryKey"`
	Name          string     `gorm:"type:text;not null"`
	Kind          string     `gorm:"type:text;not null;index"`
	URL           string     `gorm:"type:text;not null;uniqueIndex"`
	IsActive      bool       `gorm:"not null;default:true"`
	LastFetchedAt *time.Time `gorm:"index"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Source) TableName() string { return "sources" }

func FromDomain(s *domain.Source) Source {
	return Source{
		ID:            s.ID,
		Name:          s.Name,
		Kind:          string(s.Kind),
		URL:           s.URL,
		IsActive:      s.IsActive,
		LastFetchedAt: s.LastFetchedAt,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func (m Source) ToDomain() *domain.Source {
	return &domain.Source{
		ID:            m.ID,
		Name:          m.Name,
		Kind:          domain.Kind(m.Kind),
		URL:           m.URL,
		IsActive:      m.IsActive,
		LastFetchedAt: m.LastFetchedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}
