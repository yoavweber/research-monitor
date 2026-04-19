package source

import (
	"context"
	"errors"

	"gorm.io/gorm"

	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
)

type repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) domain.Repository {
	return &repository{db: db}
}

func (r *repository) Save(ctx context.Context, s *domain.Source) error {
	m := FromDomain(s)
	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *repository) FindByID(ctx context.Context, id string) (*domain.Source, error) {
	var m Source
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

func (r *repository) FindByURL(ctx context.Context, url string) (*domain.Source, error) {
	var m Source
	if err := r.db.WithContext(ctx).First(&m, "url = ?", url).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

func (r *repository) List(ctx context.Context) ([]domain.Source, error) {
	var rows []Source
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Source, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToDomain())
	}
	return out, nil
}

func (r *repository) Delete(ctx context.Context, id string) error {
	tx := r.db.WithContext(ctx).Delete(&Source{}, "id = ?", id)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
