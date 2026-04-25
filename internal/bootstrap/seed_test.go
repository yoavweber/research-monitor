package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	sourcepersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
)

// noopLogger satisfies shared.Logger without any side effect — sufficient for
// the seed test since it asserts on DB state, not log output.
type noopLogger struct{}

func (noopLogger) InfoContext(context.Context, string, ...any)  {}
func (noopLogger) WarnContext(context.Context, string, ...any)  {}
func (noopLogger) ErrorContext(context.Context, string, ...any) {}
func (noopLogger) DebugContext(context.Context, string, ...any) {}
func (noopLogger) With(...any) shared.Logger                    { return noopLogger{} }

func newSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestSeedSources_Idempotent verifies that SeedSources creates a row on the
// first call and treats every subsequent call as a no-op skip — the property
// that makes it safe to run on every prod deploy.
func TestSeedSources_Idempotent(t *testing.T) {
	db := newSeedTestDB(t)
	repo := sourcepersist.NewRepository(db)
	ctx := context.Background()
	logger := noopLogger{}

	if err := SeedSources(ctx, db, shared.SystemClock{}, logger); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	first, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list after first seed: %v", err)
	}
	if got, want := len(first), len(seedSources); got != want {
		t.Fatalf("after first seed: got %d sources, want %d", got, want)
	}

	if err := SeedSources(ctx, db, shared.SystemClock{}, logger); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	second, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list after second seed: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("seed not idempotent: row count changed from %d to %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID {
		t.Fatalf("seed re-created the row: id changed from %q to %q", first[0].ID, second[0].ID)
	}
}

// TestSeedSources_PersistsArxiv asserts the canonical arxiv row is materialised
// with the expected wire fields after seeding a fresh DB.
func TestSeedSources_PersistsArxiv(t *testing.T) {
	db := newSeedTestDB(t)
	repo := sourcepersist.NewRepository(db)
	ctx := context.Background()

	if err := SeedSources(ctx, db, shared.SystemClock{}, noopLogger{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.FindByURL(ctx, "https://export.arxiv.org/api/query")
	if err != nil {
		t.Fatalf("FindByURL: %v", err)
	}
	if got.Name != "arXiv" {
		t.Errorf("Name: got %q, want %q", got.Name, "arXiv")
	}
	if string(got.Kind) != "api" {
		t.Errorf("Kind: got %q, want %q", got.Kind, "api")
	}
	if !got.IsActive {
		t.Errorf("IsActive: got false, want true")
	}
	if got.ID == "" {
		t.Errorf("ID is empty — use case did not assign UUID")
	}
}
