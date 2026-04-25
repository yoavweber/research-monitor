package arxiv

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// fakePaperFetcher is an inline fake implementing paper.Fetcher. It records
// the Query it receives and the number of Fetch invocations, and returns the
// canned entries/error configured by the test.
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

// saveResult is a single canned outcome the fakeRepo will return on the
// matching Save invocation.
type saveResult struct {
	isNew bool
	err   error
}

// fakeRepo is an inline fake satisfying paper.Repository for the persist path.
// FindByKey and List are unused by arxivUseCase and are not exercised here;
// they panic to surface accidental coupling.
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
		// Default to "new insert" so tests that don't care about per-call
		// outcomes can leave results empty.
		return true, nil
	}
	res := r.results[idx]
	return res.isNew, res.err
}

func (r *fakeRepo) FindByKey(_ context.Context, _, _ string) (*paper.Entry, error) {
	panic("FindByKey not used by arxivUseCase")
}

func (r *fakeRepo) List(_ context.Context) ([]paper.Entry, error) {
	panic("List not used by arxivUseCase")
}

// logRecord captures a single structured log call for assertions.
type logRecord struct {
	level string
	msg   string
	args  map[string]any
}

// recordingLogger is an inline fake implementing shared.Logger.
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

func (l *recordingLogger) filterLevel(level string) []logRecord {
	var out []logRecord
	for _, r := range l.records {
		if r.level == level {
			out = append(out, r)
		}
	}
	return out
}

// newQuery is the standard query used across tests unless overridden.
func newQuery() paper.Query {
	return paper.Query{
		Categories: []string{"cs.LG", "q-fin.ST"},
		MaxResults: 100,
	}
}

func TestArxivUseCase_Happy_AllNew(t *testing.T) {
	t.Parallel()

	entries := []paper.Entry{
		{SourceID: "a"},
		{SourceID: "b"},
		{SourceID: "c"},
	}
	fetcher := &fakePaperFetcher{returnEntries: entries}
	repo := &fakeRepo{} // default isNew=true,err=nil for every call
	log := &recordingLogger{}

	uc := NewArxivUseCase(fetcher, repo, log, newQuery())

	got, err := uc.FetchWithOutcomes(context.Background())
	if err != nil {
		t.Fatalf("FetchWithOutcomes returned unexpected error: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("outcomes len=%d, want %d", len(got), len(entries))
	}
	for i, fe := range got {
		if !reflect.DeepEqual(fe.Entry, entries[i]) {
			t.Fatalf("outcomes[%d].Entry=%v, want %v (order must match fetcher)", i, fe.Entry, entries[i])
		}
		if !fe.IsNew {
			t.Fatalf("outcomes[%d].IsNew=false, want true", i)
		}
	}
	if repo.invocations != 3 {
		t.Fatalf("repo.Save invocations=%d, want 3", repo.invocations)
	}

	infos := log.filterLevel("Info")
	if len(infos) != 1 {
		t.Fatalf("Info log count=%d, want 1; records=%v", len(infos), log.records)
	}
	rec := infos[0]
	if rec.msg != "paper.fetch.ok" {
		t.Fatalf("Info msg=%q, want paper.fetch.ok", rec.msg)
	}
	if rec.args["new"] != 3 {
		t.Fatalf("Info new=%v, want 3", rec.args["new"])
	}
	if rec.args["skipped"] != 0 {
		t.Fatalf("Info skipped=%v, want 0", rec.args["skipped"])
	}
	if rec.args["source"] != "arxiv" {
		t.Fatalf("Info source=%v, want arxiv", rec.args["source"])
	}
}

func TestArxivUseCase_MixedOutcomes_CountsMatch(t *testing.T) {
	t.Parallel()

	entries := []paper.Entry{
		{SourceID: "a"},
		{SourceID: "b"},
		{SourceID: "c"},
	}
	fetcher := &fakePaperFetcher{returnEntries: entries}
	// 2 new + 1 skipped, with the skip in the middle to verify per-call mapping.
	repo := &fakeRepo{results: []saveResult{
		{isNew: true},
		{isNew: false},
		{isNew: true},
	}}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fetcher, repo, log, newQuery())

	got, err := uc.FetchWithOutcomes(context.Background())
	if err != nil {
		t.Fatalf("FetchWithOutcomes returned unexpected error: %v", err)
	}
	wantFlags := []bool{true, false, true}
	if len(got) != len(wantFlags) {
		t.Fatalf("outcomes len=%d, want %d", len(got), len(wantFlags))
	}
	for i, fe := range got {
		if fe.Entry.SourceID != entries[i].SourceID {
			t.Fatalf("outcomes[%d].Entry.SourceID=%q, want %q", i, fe.Entry.SourceID, entries[i].SourceID)
		}
		if fe.IsNew != wantFlags[i] {
			t.Fatalf("outcomes[%d].IsNew=%v, want %v", i, fe.IsNew, wantFlags[i])
		}
	}

	infos := log.filterLevel("Info")
	if len(infos) != 1 {
		t.Fatalf("Info log count=%d, want 1", len(infos))
	}
	if infos[0].args["new"] != 2 {
		t.Fatalf("Info new=%v, want 2", infos[0].args["new"])
	}
	if infos[0].args["skipped"] != 1 {
		t.Fatalf("Info skipped=%v, want 1", infos[0].args["skipped"])
	}
}

func TestArxivUseCase_FetcherError_RepoNeverCalled(t *testing.T) {
	t.Parallel()

	fetcher := &fakePaperFetcher{returnErr: paper.ErrUpstreamBadStatus}
	repo := &fakeRepo{}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fetcher, repo, log, newQuery())

	got, err := uc.FetchWithOutcomes(context.Background())
	if got != nil {
		t.Fatalf("outcomes=%v on fetcher error, want nil", got)
	}
	if !errors.Is(err, paper.ErrUpstreamBadStatus) {
		t.Fatalf("err=%v, want Is(ErrUpstreamBadStatus)", err)
	}
	if repo.invocations != 0 {
		t.Fatalf("repo.Save invocations=%d on fetcher error, want 0", repo.invocations)
	}
	if len(log.filterLevel("Info")) != 0 {
		t.Fatalf("unexpected Info log on fetcher-error path")
	}
	warns := log.filterLevel("Warn")
	if len(warns) != 1 || warns[0].msg != "paper.fetch.failed" {
		t.Fatalf("Warn log records=%v, want exactly one paper.fetch.failed", warns)
	}
	if warns[0].args["category"] != "bad_status" {
		t.Fatalf("Warn category=%v, want bad_status", warns[0].args["category"])
	}
}

func TestArxivUseCase_SaveFailureMidLoop_NoPartialOutcomes(t *testing.T) {
	t.Parallel()

	entries := []paper.Entry{
		{SourceID: "a"},
		{SourceID: "b"},
		{SourceID: "c"},
	}
	fetcher := &fakePaperFetcher{returnEntries: entries}
	// First save succeeds, second fails — third must never be attempted.
	repo := &fakeRepo{results: []saveResult{
		{isNew: true},
		{isNew: false, err: paper.ErrCatalogueUnavailable},
		{isNew: true}, // unreachable
	}}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fetcher, repo, log, newQuery())

	got, err := uc.FetchWithOutcomes(context.Background())
	if got != nil {
		t.Fatalf("outcomes=%v on save failure, want nil (no partial slice, R5.5)", got)
	}
	if !errors.Is(err, paper.ErrCatalogueUnavailable) {
		t.Fatalf("err=%v, want Is(ErrCatalogueUnavailable)", err)
	}
	if repo.invocations != 2 {
		t.Fatalf("repo.Save invocations=%d, want 2 (loop must abort after failure)", repo.invocations)
	}
	if len(repo.savedKeys) != 2 || repo.savedKeys[0] != "a" || repo.savedKeys[1] != "b" {
		t.Fatalf("repo.savedKeys=%v, want [a b]", repo.savedKeys)
	}
	// No paper.fetch.ok aggregate log on the failure path.
	if len(log.filterLevel("Info")) != 0 {
		t.Fatalf("unexpected Info log on save-failure path")
	}
}
