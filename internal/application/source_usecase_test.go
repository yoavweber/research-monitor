package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoavweber/defi-monitor-backend/internal/application"
	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
)

// fakeRepo is a hand-rolled in-memory Repository for unit tests.
type fakeRepo struct {
	byID  map[string]*domain.Source
	byURL map[string]*domain.Source
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]*domain.Source{}, byURL: map[string]*domain.Source{}}
}

func (r *fakeRepo) Save(_ context.Context, s *domain.Source) error {
	r.byID[s.ID] = s
	r.byURL[s.URL] = s
	return nil
}
func (r *fakeRepo) FindByID(_ context.Context, id string) (*domain.Source, error) {
	s, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}
func (r *fakeRepo) FindByURL(_ context.Context, url string) (*domain.Source, error) {
	s, ok := r.byURL[url]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}
func (r *fakeRepo) List(_ context.Context) ([]domain.Source, error) {
	out := make([]domain.Source, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, *s)
	}
	return out, nil
}
func (r *fakeRepo) Delete(_ context.Context, id string) error {
	s, ok := r.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.byID, id)
	delete(r.byURL, s.URL)
	return nil
}

// fixedClock returns a fixed time for deterministic tests.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestSourceUseCase_Create(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	uc := application.NewSourceUseCase(repo, fixedClock{t: time.Unix(1000, 0).UTC()})

	got, err := uc.Create(context.Background(), domain.CreateRequest{
		Name: "Test", Kind: domain.KindRSS, URL: "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == "" {
		t.Error("ID not assigned")
	}
	if !got.IsActive {
		t.Error("IsActive should default true")
	}
}

func TestSourceUseCase_Create_DuplicateURL(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	uc := application.NewSourceUseCase(repo, fixedClock{t: time.Unix(1000, 0).UTC()})
	req := domain.CreateRequest{Name: "A", Kind: domain.KindRSS, URL: "https://example.com/feed.xml"}

	if _, err := uc.Create(context.Background(), req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := uc.Create(context.Background(), req)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("err = %v want ErrConflict", err)
	}
}

func TestSourceUseCase_Create_ValidationFails(t *testing.T) {
	t.Parallel()
	uc := application.NewSourceUseCase(newFakeRepo(), fixedClock{})
	_, err := uc.Create(context.Background(), domain.CreateRequest{Name: "x", Kind: "bogus", URL: "https://x"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
