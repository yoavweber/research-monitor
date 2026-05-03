package analyzer_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/analyzer"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

func sampleAnalysis(id string, when time.Time) domain.Analysis {
	return domain.Analysis{
		ExtractionID:         id,
		ShortSummary:         "short",
		LongSummary:          "long",
		ThesisAngleFlag:      true,
		ThesisAngleRationale: "promising",
		Model:                "fake-model",
		PromptVersion:        "short.v1+long.v1+thesis.v1",
		CreatedAt:            when,
		UpdatedAt:            when,
	}
}

func TestRepository_Upsert_InsertPath_ReturnsRow(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := analyzer.NewRepository(db)
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	a := sampleAnalysis("ex-1", now)

	got, err := repo.Upsert(context.Background(), a)
	if err != nil {
		t.Fatalf("Upsert err = %v, want nil", err)
	}

	if got.ExtractionID != "ex-1" || got.ShortSummary != "short" {
		t.Errorf("got = %+v, want core fields preserved", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}

	stored, err := repo.FindByID(context.Background(), "ex-1")
	if err != nil {
		t.Fatalf("FindByID after insert err = %v", err)
	}
	if stored.ShortSummary != "short" {
		t.Errorf("stored = %+v, want the inserted row", stored)
	}
}

func TestRepository_Upsert_OverwritePath_PreservesCreatedAtAdvancesUpdatedAt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := analyzer.NewRepository(db)
	created := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	updated := created.Add(2 * time.Hour)

	first := sampleAnalysis("ex-overwrite", created)
	if _, err := repo.Upsert(context.Background(), first); err != nil {
		t.Fatalf("first Upsert err = %v", err)
	}

	second := sampleAnalysis("ex-overwrite", updated)
	second.ShortSummary = "updated short"
	second.LongSummary = "updated long"
	second.ThesisAngleFlag = false
	second.ThesisAngleRationale = "less promising"
	second.Model = "fake-model-v2"
	second.CreatedAt = updated // use case sets both to now; repo must override

	got, err := repo.Upsert(context.Background(), second)
	if err != nil {
		t.Fatalf("second Upsert err = %v", err)
	}

	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want preserved value %v", got.CreatedAt, created)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, updated)
	}
	if got.ShortSummary != "updated short" || got.ThesisAngleFlag != false {
		t.Errorf("content not overwritten: %+v", got)
	}

	stored, err := repo.FindByID(context.Background(), "ex-overwrite")
	if err != nil {
		t.Fatalf("FindByID after overwrite err = %v", err)
	}
	if stored.ThesisAngleFlag != false || stored.ShortSummary != "updated short" {
		t.Errorf("stored row not overwritten: %+v", stored)
	}
	if !stored.CreatedAt.Equal(created) {
		t.Errorf("stored CreatedAt = %v, want preserved %v", stored.CreatedAt, created)
	}
}

func TestRepository_Upsert_ConcurrentReruns_LeaveOneRow(t *testing.T) {
	t.Parallel()

	// Production opens SQLite with _journal_mode=WAL plus a default busy
	// timeout so brief writer-on-writer contention serializes instead of
	// failing fast with "database is locked". The shared testdb helper
	// uses a vanilla file DB which surfaces the lock to concurrent writers
	// before the upsert's conflict branch ever runs. Use the production
	// open path here so the test exercises the repository's race-safe
	// contract under realistic pragmas.
	path := filepath.Join(t.TempDir(), "race.db")
	db, err := persistence.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := db.Exec("PRAGMA busy_timeout = 30000").Error; err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}

	repo := analyzer.NewRepository(db)
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)

	// Requirement 3.5 specifies "two concurrent goroutines"; SQLite serializes
	// writers under WAL+busy_timeout so the property exercised here is
	// "exactly one row remains, no spurious storage error to the caller."
	const N = 2
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			a := sampleAnalysis("ex-race", now.Add(time.Duration(i)*time.Second))
			a.ShortSummary = "writer-" + string(rune('A'+i))
			if _, err := repo.Upsert(context.Background(), a); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Upsert err = %v, want all to succeed", err)
	}

	var count int64
	if err := db.Table("analyses").Where("extraction_id = ?", "ex-race").Count(&count).Error; err != nil {
		t.Fatalf("count err = %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want exactly 1 after concurrent reruns", count)
	}
}

func TestRepository_FindByID_Miss_ReturnsAnalysisNotFound(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := analyzer.NewRepository(db)

	_, err := repo.FindByID(context.Background(), "nope")

	if !errors.Is(err, domain.ErrAnalysisNotFound) {
		t.Fatalf("err = %v, want ErrAnalysisNotFound", err)
	}
}

func TestRepository_FindByID_DBClosed_ReturnsCatalogueUnavailable(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := analyzer.NewRepository(db)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = repo.FindByID(context.Background(), "ex-x")

	if err == nil {
		t.Fatal("FindByID against closed DB returned nil err")
	}
	if !errors.Is(err, domain.ErrCatalogueUnavailable) {
		t.Errorf("err = %v, want wrapping ErrCatalogueUnavailable", err)
	}
	// Sanity: ensure the wrapped sentinel still surfaces a 500.
	if he := shared.AsHTTPError(err); he == nil || he.Code != 500 {
		t.Errorf("AsHTTPError code = %v, want 500", he)
	}
}
