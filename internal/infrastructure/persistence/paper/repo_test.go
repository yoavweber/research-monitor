package paper_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	paperrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
)

// newTestDB opens a fresh on-disk SQLite database in a temp dir with the
// same TranslateError flag the production helper sets, so unique-violation
// surfaces as gorm.ErrDuplicatedKey just like production.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "papers_test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newEntry returns a populated domain.Entry with deterministic timestamps so
// round-trip assertions are not flaky on monotonic-clock differences.
func newEntry(source, sourceID string) domain.Entry {
	submitted := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	updated := submitted.Add(2 * time.Hour)
	return domain.Entry{
		Source:          source,
		SourceID:        sourceID,
		Version:         "v1",
		Title:           "Sample paper",
		Authors:         []string{"Alice", "Bob"},
		Abstract:        "An abstract.",
		PrimaryCategory: "cs.AI",
		Categories:      []string{"cs.AI", "cs.LG"},
		SubmittedAt:     submitted,
		UpdatedAt:       updated,
		PDFURL:          "https://arxiv.org/pdf/2404.12345",
		AbsURL:          "https://arxiv.org/abs/2404.12345",
	}
}

func TestRepository_Save_NewRow(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))
	ctx := context.Background()

	isNew, err := repo.Save(ctx, newEntry("arxiv", "2404.12345"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !isNew {
		t.Fatalf("isNew = false, want true on first insert")
	}
}

func TestRepository_Save_DedupeOnRepeat(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))
	ctx := context.Background()
	e := newEntry("arxiv", "2404.12345")

	if _, err := repo.Save(ctx, e); err != nil {
		t.Fatalf("first save: %v", err)
	}
	isNew, err := repo.Save(ctx, e)
	if err != nil {
		t.Fatalf("second save returned error, want nil: %v", err)
	}
	if isNew {
		t.Fatalf("isNew = true on duplicate, want false")
	}

	// Confirm only one row landed in storage.
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(got))
	}
}

func TestRepository_Save_SameSourceIDDifferentSourcePersistsBoth(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))
	ctx := context.Background()

	a := newEntry("arxiv", "2404.12345")
	b := newEntry("biorxiv", "2404.12345")

	for _, e := range []domain.Entry{a, b} {
		isNew, err := repo.Save(ctx, e)
		if err != nil {
			t.Fatalf("save %s: %v", e.Source, err)
		}
		if !isNew {
			t.Fatalf("isNew = false on first insert for %s", e.Source)
		}
	}

	gotA, err := repo.FindByKey(ctx, "arxiv", "2404.12345")
	if err != nil {
		t.Fatalf("find arxiv: %v", err)
	}
	if gotA.Source != "arxiv" {
		t.Errorf("arxiv hit returned Source=%q", gotA.Source)
	}
	gotB, err := repo.FindByKey(ctx, "biorxiv", "2404.12345")
	if err != nil {
		t.Fatalf("find biorxiv: %v", err)
	}
	if gotB.Source != "biorxiv" {
		t.Errorf("biorxiv hit returned Source=%q", gotB.Source)
	}
}

func TestRepository_FindByKey_Miss(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))

	got, err := repo.FindByKey(context.Background(), "arxiv", "missing")
	if got != nil {
		t.Errorf("entry = %+v, want nil", got)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_OrdersBySubmittedAtDesc(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))
	ctx := context.Background()

	older := newEntry("arxiv", "older")
	older.SubmittedAt = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	newer := newEntry("arxiv", "newer")
	newer.SubmittedAt = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)

	// Insert older first to confirm order is governed by SubmittedAt, not
	// insertion order.
	if _, err := repo.Save(ctx, older); err != nil {
		t.Fatalf("save older: %v", err)
	}
	if _, err := repo.Save(ctx, newer); err != nil {
		t.Fatalf("save newer: %v", err)
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SourceID != "newer" || got[1].SourceID != "older" {
		t.Errorf("order = [%s, %s], want [newer, older]", got[0].SourceID, got[1].SourceID)
	}
}

func TestRepository_List_EmptyReturnsNonNilSlice(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Errorf("list returned nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestRepository_RoundTrip_PreservesAuthorsAndCategories(t *testing.T) {
	t.Parallel()
	repo := paperrepo.NewRepository(newTestDB(t))
	ctx := context.Background()

	want := newEntry("arxiv", "2404.99999")
	want.Authors = []string{"Alice", "Bob with, comma", `Carol "quoted"`}
	want.Categories = []string{"cs.AI", "cs.LG", "stat.ML"}
	if _, err := repo.Save(ctx, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.FindByKey(ctx, want.Source, want.SourceID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !reflect.DeepEqual(got.Authors, want.Authors) {
		t.Errorf("authors = %#v, want %#v", got.Authors, want.Authors)
	}
	if !reflect.DeepEqual(got.Categories, want.Categories) {
		t.Errorf("categories = %#v, want %#v", got.Categories, want.Categories)
	}
	// Spot-check a few scalar fields too — round-trip should be lossless on
	// every domain field, not just the JSON-encoded ones.
	if got.Title != want.Title || got.Abstract != want.Abstract || got.PrimaryCategory != want.PrimaryCategory {
		t.Errorf("scalar fields drifted: got %+v, want %+v", got, want)
	}
	if !got.SubmittedAt.Equal(want.SubmittedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("timestamps drifted: got submitted=%v updated=%v, want submitted=%v updated=%v",
			got.SubmittedAt, got.UpdatedAt, want.SubmittedAt, want.UpdatedAt)
	}
}

func TestRepository_SentinelTranslation_OnDBFailure(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	repo := paperrepo.NewRepository(db)
	ctx := context.Background()

	// Closing the underlying *sql.DB makes every subsequent driver call fail
	// with "sql: database is closed" — a non-dedupe DB error path. The
	// repository must wrap each variant with paper.ErrCatalogueUnavailable.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := repo.Save(ctx, newEntry("arxiv", "2404.12345")); !errors.Is(err, domain.ErrCatalogueUnavailable) {
		t.Errorf("Save err = %v, want wrapping ErrCatalogueUnavailable", err)
	}
	if _, err := repo.FindByKey(ctx, "arxiv", "2404.12345"); !errors.Is(err, domain.ErrCatalogueUnavailable) {
		t.Errorf("FindByKey err = %v, want wrapping ErrCatalogueUnavailable", err)
	}
	if _, err := repo.List(ctx); !errors.Is(err, domain.ErrCatalogueUnavailable) {
		t.Errorf("List err = %v, want wrapping ErrCatalogueUnavailable", err)
	}
}

func TestAutoMigrate_CreatesCompositeIndex(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	type indexRow struct {
		Seq    int
		Name   string
		Unique int
		Origin string
		Partial int
	}
	var rows []indexRow
	if err := db.Raw("PRAGMA index_list('papers')").Scan(&rows).Error; err != nil {
		t.Fatalf("pragma: %v", err)
	}
	var found bool
	for _, r := range rows {
		if r.Name == "idx_papers_source_source_id" {
			found = true
			if r.Unique != 1 {
				t.Errorf("idx_papers_source_source_id unique = %d, want 1", r.Unique)
			}
		}
	}
	if !found {
		t.Errorf("composite index idx_papers_source_source_id not present, got rows=%+v", rows)
	}
}
