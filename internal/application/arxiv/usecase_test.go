package arxiv

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// --- Test doubles ----------------------------------------------------------
//
// We use hand-rolled fakes (not a real persistence.Repository against an
// in-memory SQLite) for two reasons:
//
//   1. Layering: application-level tests must depend only on domain ports.
//      Importing infrastructure/persistence here would invert the dependency
//      rule (see CLAUDE.md → "Dependency rule (inward only)").
//   2. Failure injection: the orchestrator's job is to relay paper.* sentinels
//      verbatim and abort the loop on the first save failure. Driving a real
//      DB into ErrCatalogueUnavailable mid-loop is awkward and slow; a fake
//      lets each row return an exact, deterministic outcome.
//
// The persistence layer has its own dedicated tests against a real SQLite
// instance (internal/infrastructure/persistence/paper).

type fakePaperFetcher struct {
	capturedQuery paper.Query
	invocations   int
	returnEntries []paper.Entry
	returnErr     error
}

func (f *fakePaperFetcher) Fetch(_ context.Context, q paper.Query) ([]paper.Entry, error) {
	f.invocations++
	f.capturedQuery = q
	return f.returnEntries, f.returnErr
}

// saveResult is the canned outcome fakeRepo will return on a single Save call.
type saveResult struct {
	isNew bool
	err   error
}

type fakeRepo struct {
	results     []saveResult // consumed in order, one per Save call
	savedKeys   []string     // SourceID of each saved entry, in call order
	invocations int
}

func (r *fakeRepo) Save(_ context.Context, e paper.Entry) (bool, error) {
	idx := r.invocations
	r.invocations++
	r.savedKeys = append(r.savedKeys, e.SourceID)
	if idx >= len(r.results) {
		// Default to "new insert" so cases that don't care about per-call
		// outcomes can leave results empty.
		return true, nil
	}
	res := r.results[idx]
	return res.isNew, res.err
}

// FindByKey/List are unused by arxivUseCase; panic to surface accidental coupling.
func (r *fakeRepo) FindByKey(_ context.Context, _, _ string) (*paper.Entry, error) {
	panic("FindByKey not used by arxivUseCase")
}

func (r *fakeRepo) List(_ context.Context) ([]paper.Entry, error) {
	panic("List not used by arxivUseCase")
}

type logRecord struct {
	level string
	msg   string
	args  map[string]any
}

type recordingLogger struct {
	records []logRecord
}

func (l *recordingLogger) record(level, msg string, args []any) {
	m := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		m[k] = args[i+1]
	}
	l.records = append(l.records, logRecord{level: level, msg: msg, args: m})
}

func (l *recordingLogger) InfoContext(_ context.Context, msg string, args ...any) {
	l.record("Info", msg, args)
}
func (l *recordingLogger) WarnContext(_ context.Context, msg string, args ...any) {
	l.record("Warn", msg, args)
}
func (l *recordingLogger) ErrorContext(_ context.Context, msg string, args ...any) {
	l.record("Error", msg, args)
}
func (l *recordingLogger) DebugContext(_ context.Context, msg string, args ...any) {
	l.record("Debug", msg, args)
}
func (l *recordingLogger) With(_ ...any) shared.Logger { return l }

func (l *recordingLogger) recordsAt(level string) []logRecord {
	var out []logRecord
	for _, r := range l.records {
		if r.level == level {
			out = append(out, r)
		}
	}
	return out
}

// --- Assertion helpers -----------------------------------------------------

// expectedLog describes a single structured-log record we expect to see.
// argSubset is checked as a subset of the recorded args (so callers don't need
// to enumerate every key).
type expectedLog struct {
	level     string
	msg       string
	argSubset map[string]any
}

func assertSingleLog(t *testing.T, logs []logRecord, want expectedLog) {
	t.Helper()
	if len(logs) != 1 {
		t.Fatalf("%s log count=%d, want 1; records=%v", want.level, len(logs), logs)
	}
	got := logs[0]
	if got.msg != want.msg {
		t.Fatalf("%s msg=%q, want %q", want.level, got.msg, want.msg)
	}
	for k, v := range want.argSubset {
		if !reflect.DeepEqual(got.args[k], v) {
			t.Fatalf("%s arg %q=%v, want %v", want.level, k, got.args[k], v)
		}
	}
}

func assertNoLogAt(t *testing.T, logs []logRecord, level string) {
	t.Helper()
	if len(logs) != 0 {
		t.Fatalf("expected zero %s logs, got %d: %v", level, len(logs), logs)
	}
}

// --- The table -------------------------------------------------------------

func TestArxivUseCase_Fetch(t *testing.T) {
	t.Parallel()

	entriesABC := []paper.Entry{
		{SourceID: "a"},
		{SourceID: "b"},
		{SourceID: "c"},
	}

	cases := []struct {
		name string

		// arrange
		fetchEntries []paper.Entry
		fetchErr     error
		saveResults  []saveResult

		// expectations
		wantErrSentinel error // nil → no error expected
		wantResults     []Result
		wantSaveCount   int
		wantSavedKeys   []string
		wantInfo        *expectedLog
		wantWarn        *expectedLog
	}{
		{
			name:         "happy_all_new",
			fetchEntries: entriesABC,
			// saveResults empty → fakeRepo defaults isNew=true,err=nil for every call
			wantResults: []Result{
				{Entry: entriesABC[0], IsNew: true},
				{Entry: entriesABC[1], IsNew: true},
				{Entry: entriesABC[2], IsNew: true},
			},
			wantSaveCount: 3,
			wantSavedKeys: []string{"a", "b", "c"},
			wantInfo: &expectedLog{
				level: "Info",
				msg:   "paper.fetch.ok",
				argSubset: map[string]any{
					"source":  "arxiv",
					"new":     3,
					"skipped": 0,
				},
			},
		},
		{
			name:         "mixed_new_and_skipped_preserves_order",
			fetchEntries: entriesABC,
			// new + skipped + new — skip in the middle to verify per-call mapping
			saveResults: []saveResult{
				{isNew: true},
				{isNew: false},
				{isNew: true},
			},
			wantResults: []Result{
				{Entry: entriesABC[0], IsNew: true},
				{Entry: entriesABC[1], IsNew: false},
				{Entry: entriesABC[2], IsNew: true},
			},
			wantSaveCount: 3,
			wantSavedKeys: []string{"a", "b", "c"},
			wantInfo: &expectedLog{
				level: "Info",
				msg:   "paper.fetch.ok",
				argSubset: map[string]any{
					"new":     2,
					"skipped": 1,
				},
			},
		},
		{
			name:            "fetcher_error_skips_repo_and_warns",
			fetchErr:        paper.ErrUpstreamBadStatus,
			wantErrSentinel: paper.ErrUpstreamBadStatus,
			wantSaveCount:   0,
			wantWarn: &expectedLog{
				level: "Warn",
				msg:   "paper.fetch.failed",
				argSubset: map[string]any{
					"category": "bad_status",
				},
			},
		},
		{
			name:         "save_failure_aborts_loop_no_partial_slice",
			fetchEntries: entriesABC,
			saveResults: []saveResult{
				{isNew: true},
				{isNew: false, err: paper.ErrCatalogueUnavailable},
				{isNew: true}, // unreachable
			},
			wantErrSentinel: paper.ErrCatalogueUnavailable,
			wantSaveCount:   2,
			wantSavedKeys:   []string{"a", "b"},
			// no Info on the failure path (R5.5)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// arrange
			fetcher := &fakePaperFetcher{returnEntries: tc.fetchEntries, returnErr: tc.fetchErr}
			repo := &fakeRepo{results: tc.saveResults}
			log := &recordingLogger{}
			uc := NewArxivUseCase(fetcher, repo, log, newQuery())

			// act
			got, err := uc.Fetch(context.Background())

			// assert: error
			if tc.wantErrSentinel != nil {
				if !errors.Is(err, tc.wantErrSentinel) {
					t.Fatalf("err=%v, want errors.Is(%v)", err, tc.wantErrSentinel)
				}
				if got != nil {
					t.Fatalf("results=%v on error path, want nil (R5.5)", got)
				}
			} else {
				if err != nil {
					t.Fatalf("Fetch returned unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got, tc.wantResults) {
					t.Fatalf("results=%v, want %v", got, tc.wantResults)
				}
			}

			// assert: repo invocations + saved keys
			if repo.invocations != tc.wantSaveCount {
				t.Fatalf("repo.Save invocations=%d, want %d", repo.invocations, tc.wantSaveCount)
			}
			if tc.wantSavedKeys != nil && !reflect.DeepEqual(repo.savedKeys, tc.wantSavedKeys) {
				t.Fatalf("repo.savedKeys=%v, want %v", repo.savedKeys, tc.wantSavedKeys)
			}

			// assert: log records
			if tc.wantInfo != nil {
				assertSingleLog(t, log.recordsAt("Info"), *tc.wantInfo)
			} else {
				assertNoLogAt(t, log.recordsAt("Info"), "Info")
			}
			if tc.wantWarn != nil {
				assertSingleLog(t, log.recordsAt("Warn"), *tc.wantWarn)
			}
		})
	}
}

// newQuery is the standard query used across tests unless overridden.
func newQuery() paper.Query {
	return paper.Query{
		Categories: []string{"cs.LG", "q-fin.ST"},
		MaxResults: 100,
	}
}
