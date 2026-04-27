package arxiv_test

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

	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
	paperrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// newTestDB mirrors the helper in internal/infrastructure/persistence/paper/repo_test.go.
// Steering (testing.md) prefers real over fake for paper.Repository so the
// tests catch schema and migration bugs a hand-rolled fake would mask. We
// rely on the same TranslateError flag production uses so unique-violation
// surfaces as gorm.ErrDuplicatedKey just like in production.
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

// newEntry returns a minimally valid domain.Entry. The repository requires a
// non-zero SubmittedAt for List ordering and a non-empty (Source, SourceID)
// for the dedupe composite key, so we fill those plus a unique title per id.
func newEntry(sourceID string) paper.Entry {
	submitted := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	return paper.Entry{
		Source:          paper.SourceArxiv,
		SourceID:        sourceID,
		Version:         "v1",
		Title:           "Paper " + sourceID,
		Authors:         []string{"Alice"},
		Abstract:        "An abstract.",
		PrimaryCategory: "cs.LG",
		Categories:      []string{"cs.LG"},
		SubmittedAt:     submitted,
		UpdatedAt:       submitted.Add(time.Hour),
	}
}

func newQuery() paper.Query {
	return paper.Query{
		Categories: []string{"cs.LG", "q-fin.ST"},
		MaxResults: 100,
	}
}

func TestArxivUseCase_Fetch(t *testing.T) {
	t.Parallel()

	t.Run("returns all entries as new on an empty catalogue", func(t *testing.T) {
		t.Parallel()
		entries := []paper.Entry{newEntry("a"), newEntry("b"), newEntry("c")}
		fetcher := &mocks.PaperFetcher{Entries: entries}
		repo := paperrepo.NewRepository(newTestDB(t))
		log := &mocks.RecordingLogger{}
		uc := arxivapp.NewArxivUseCase(fetcher, repo, log, newQuery())

		got, err := uc.Fetch(context.Background())

		if err != nil {
			t.Fatalf("Fetch err = %v, want nil", err)
		}
		want := []arxivapp.Result{
			{Entry: entries[0], IsNew: true},
			{Entry: entries[1], IsNew: true},
			{Entry: entries[2], IsNew: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("results = %v, want %v", got, want)
		}
		assertSingleLog(t, log.RecordsAt("Info"), "paper.fetch.ok", map[string]any{
			"source": paper.SourceArxiv, "new": 3, "skipped": 0,
		})
	})

	t.Run("preserves fetcher order on mixed new and dedupe-skipped", func(t *testing.T) {
		t.Parallel()
		entries := []paper.Entry{newEntry("a"), newEntry("b"), newEntry("c")}
		db := newTestDB(t)
		repo := paperrepo.NewRepository(db)
		// Pre-seed entry "b" so its second Save is a real composite-key dedupe
		// against the running SQLite — this is the steering-doc point: a real
		// repo exercises the actual unique-index path, not a fake's flag.
		if _, err := repo.Save(context.Background(), entries[1]); err != nil {
			t.Fatalf("seed: %v", err)
		}
		fetcher := &mocks.PaperFetcher{Entries: entries}
		log := &mocks.RecordingLogger{}
		uc := arxivapp.NewArxivUseCase(fetcher, repo, log, newQuery())

		got, err := uc.Fetch(context.Background())

		if err != nil {
			t.Fatalf("Fetch err = %v, want nil", err)
		}
		wantFlags := []bool{true, false, true}
		if len(got) != len(wantFlags) {
			t.Fatalf("len(results) = %d, want %d", len(got), len(wantFlags))
		}
		for i, r := range got {
			if r.Entry.SourceID != entries[i].SourceID {
				t.Fatalf("results[%d].SourceID = %q, want %q (order must match fetcher)", i, r.Entry.SourceID, entries[i].SourceID)
			}
			if r.IsNew != wantFlags[i] {
				t.Fatalf("results[%d].IsNew = %v, want %v", i, r.IsNew, wantFlags[i])
			}
		}
		assertSingleLog(t, log.RecordsAt("Info"), "paper.fetch.ok", map[string]any{
			"new": 2, "skipped": 1,
		})
	})

	t.Run("skips repo and warns when fetcher returns a sentinel", func(t *testing.T) {
		t.Parallel()
		fetcher := &mocks.PaperFetcher{Error: paper.ErrUpstreamBadStatus}
		repo := paperrepo.NewRepository(newTestDB(t))
		log := &mocks.RecordingLogger{}
		uc := arxivapp.NewArxivUseCase(fetcher, repo, log, newQuery())

		got, err := uc.Fetch(context.Background())

		if !errors.Is(err, paper.ErrUpstreamBadStatus) {
			t.Fatalf("err = %v, want errors.Is(ErrUpstreamBadStatus)", err)
		}
		if got != nil {
			t.Fatalf("results = %v, want nil on fetcher error", got)
		}
		// repo wasn't reached: List against the empty DB still returns []
		stored, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(stored) != 0 {
			t.Fatalf("repo persisted %d entries on fetcher-error path, want 0", len(stored))
		}
		if len(log.RecordsAt("Info")) != 0 {
			t.Fatal("unexpected Info log on fetcher-error path")
		}
		assertSingleLog(t, log.RecordsAt("Warn"), "paper.fetch.failed", map[string]any{
			"category": "bad_status",
		})
	})

	t.Run("aborts loop and returns no partial slice on save failure", func(t *testing.T) {
		t.Parallel()
		entries := []paper.Entry{newEntry("a"), newEntry("b"), newEntry("c")}
		fetcher := &mocks.PaperFetcher{Entries: entries}
		// Step 3 of testing.md ("contract violation the real impl cannot
		// produce"): we need Save #1 to succeed and Save #2 to return
		// ErrCatalogueUnavailable so the third entry is never reached. A
		// real SQLite repo can't selectively fail mid-loop without contrived
		// setup (e.g. closing the DB between rows), so the canonical
		// mocks.PaperRepo with a SaveResults queue is the right tool here.
		repo := &mocks.PaperRepo{SaveResults: []mocks.PaperRepoSaveResult{
			{IsNew: true},
			{IsNew: false, Err: paper.ErrCatalogueUnavailable},
			{IsNew: true}, // unreachable; if Save #3 fires the test fails on count
		}}
		log := &mocks.RecordingLogger{}
		uc := arxivapp.NewArxivUseCase(fetcher, repo, log, newQuery())

		got, err := uc.Fetch(context.Background())

		if !errors.Is(err, paper.ErrCatalogueUnavailable) {
			t.Fatalf("err = %v, want errors.Is(ErrCatalogueUnavailable)", err)
		}
		if got != nil {
			t.Fatalf("results = %v, want nil on save failure (R5.5)", got)
		}
		if len(repo.SaveCalls) != 2 {
			t.Fatalf("repo.Save calls = %d, want 2 (loop must abort after first failure)", len(repo.SaveCalls))
		}
		gotKeys := []string{repo.SaveCalls[0].SourceID, repo.SaveCalls[1].SourceID}
		if !reflect.DeepEqual(gotKeys, []string{"a", "b"}) {
			t.Fatalf("saved keys = %v, want [a b]", gotKeys)
		}
		if len(log.RecordsAt("Info")) != 0 {
			t.Fatal("unexpected Info log on save-failure path")
		}
	})
}

// assertSingleLog asserts exactly one record was emitted at this level, with
// the given msg and an args superset of want.
func assertSingleLog(t *testing.T, got []mocks.LogRecord, msg string, want map[string]any) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("log count = %d, want 1; records = %v", len(got), got)
	}
	if got[0].Msg != msg {
		t.Fatalf("log msg = %q, want %q", got[0].Msg, msg)
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[0].Args[k], v) {
			t.Fatalf("log arg %q = %v, want %v", k, got[0].Args[k], v)
		}
	}
}
