package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
)

type sourceUseCase struct {
	repo  domain.Repository
	clock shared.Clock
}

func NewSourceUseCase(repo domain.Repository, clock shared.Clock) domain.UseCase {
	return &sourceUseCase{repo: repo, clock: clock}
}

func (uc *sourceUseCase) Create(ctx context.Context, req domain.CreateRequest) (*domain.Source, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := uc.repo.FindByURL(ctx, req.URL); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	now := uc.clock.Now()
	s := &domain.Source{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Kind:      req.Kind,
		URL:       req.URL,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.repo.Save(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (uc *sourceUseCase) Get(ctx context.Context, id string) (*domain.Source, error) {
	return uc.repo.FindByID(ctx, id)
}

func (uc *sourceUseCase) List(ctx context.Context) ([]domain.Source, error) {
	return uc.repo.List(ctx)
}

func (uc *sourceUseCase) Update(ctx context.Context, id string, req domain.UpdateRequest) (*domain.Source, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	s, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		s.Name = *req.Name
	}
	if req.IsActive != nil {
		s.IsActive = *req.IsActive
	}
	s.UpdatedAt = uc.clock.Now()
	if err := uc.repo.Save(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (uc *sourceUseCase) Delete(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
}
