package source_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	sourcerepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return db
}

func newSource() *domain.Source {
	now := time.Now().UTC()
	return &domain.Source{
		ID:        uuid.NewString(),
		Name:      "Test",
		Kind:      domain.KindRSS,
		URL:       "https://example.com/feed.xml",
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestRepository_SaveAndFindByID(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	repo := sourcerepo.NewRepository(db)
	ctx := context.Background()

	s := newSource()
	if err := repo.Save(ctx, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.FindByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.URL != s.URL {
		t.Errorf("url = %q want %q", got.URL, s.URL)
	}
}

func TestRepository_FindByID_NotFound(t *testing.T) {
	t.Parallel()
	repo := sourcerepo.NewRepository(newTestDB(t))
	_, err := repo.FindByID(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepository_List(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	repo := sourcerepo.NewRepository(db)
	ctx := context.Background()
	s1, s2 := newSource(), newSource()
	s2.URL = "https://other.example.com/feed.xml"
	_ = repo.Save(ctx, s1)
	_ = repo.Save(ctx, s2)

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("len = %d want 2", len(all))
	}
}

func TestRepository_Delete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	repo := sourcerepo.NewRepository(db)
	ctx := context.Background()
	s := newSource()
	_ = repo.Save(ctx, s)

	if err := repo.Delete(ctx, s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, s.ID); err == nil {
		t.Error("expected not-found after delete")
	}
}
