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

// logRecord captures a single structured log call for assertions.
type logRecord struct {
	level string
	msg   string
	args  map[string]any
}

// recordingLogger is an inline fake implementing shared.Logger. It records
// every call into records so tests can assert level, message, and args.
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

// filterLevel returns only the records emitted at the given level.
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

func TestArxivUseCase_Success_ReturnsEntriesAndLogsInfo(t *testing.T) {
	t.Parallel()

	entries := []paper.Entry{{SourceID: "a"}, {SourceID: "b"}}
	fake := &fakePaperFetcher{returnEntries: entries}
	log := &recordingLogger{}
	q := newQuery()

	uc := NewArxivUseCase(fake, log, q)

	got, err := uc.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Fatalf("Fetch returned entries=%v, want %v", got, entries)
	}
	if fake.invocations != 1 {
		t.Fatalf("fetcher invocations=%d, want 1", fake.invocations)
	}
	if !reflect.DeepEqual(fake.capturedQuery, q) {
		t.Fatalf("captured query=%v, want %v", fake.capturedQuery, q)
	}

	infos := log.filterLevel("Info")
	if len(infos) != 1 {
		t.Fatalf("Info log count=%d, want 1; records=%v", len(infos), log.records)
	}
	if infos[0].msg != "paper.fetch.ok" {
		t.Fatalf("Info msg=%q, want %q", infos[0].msg, "paper.fetch.ok")
	}
	if infos[0].args["source"] != "arxiv" {
		t.Fatalf("Info source arg=%v, want arxiv", infos[0].args["source"])
	}
	if infos[0].args["count"] != 2 {
		t.Fatalf("Info count arg=%v, want 2", infos[0].args["count"])
	}
	if cats, ok := infos[0].args["categories"].([]string); !ok || !reflect.DeepEqual(cats, q.Categories) {
		t.Fatalf("Info categories arg=%v, want %v", infos[0].args["categories"], q.Categories)
	}
}

func TestArxivUseCase_EmptyEntries_IsSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnEntries: []paper.Entry{}}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	got, err := uc.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if got == nil {
		t.Fatalf("Fetch returned nil slice, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("Fetch returned %d entries, want 0", len(got))
	}

	infos := log.filterLevel("Info")
	if len(infos) != 1 {
		t.Fatalf("Info log count=%d, want 1", len(infos))
	}
	if infos[0].args["count"] != 0 {
		t.Fatalf("Info count arg=%v, want 0", infos[0].args["count"])
	}
}

func TestArxivUseCase_BadStatus_RelaysSentinelAndLogsWarn(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnErr: paper.ErrUpstreamBadStatus}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	got, err := uc.Fetch(context.Background())
	if got != nil {
		t.Fatalf("Fetch returned entries=%v on error, want nil", got)
	}
	if !errors.Is(err, paper.ErrUpstreamBadStatus) {
		t.Fatalf("Fetch err=%v, want Is(ErrUpstreamBadStatus)", err)
	}

	if len(log.filterLevel("Info")) != 0 {
		t.Fatalf("unexpected Info log on error path")
	}
	warns := log.filterLevel("Warn")
	if len(warns) != 1 {
		t.Fatalf("Warn log count=%d, want 1", len(warns))
	}
	if warns[0].msg != "paper.fetch.failed" {
		t.Fatalf("Warn msg=%q, want %q", warns[0].msg, "paper.fetch.failed")
	}
	if warns[0].args["source"] != "arxiv" {
		t.Fatalf("Warn source arg=%v, want arxiv", warns[0].args["source"])
	}
	if warns[0].args["category"] != "bad_status" {
		t.Fatalf("Warn category arg=%v, want bad_status", warns[0].args["category"])
	}
}

func TestArxivUseCase_Malformed_RelaysSentinelAndLogsWarn(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnErr: paper.ErrUpstreamMalformed}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	got, err := uc.Fetch(context.Background())
	if got != nil {
		t.Fatalf("Fetch returned entries=%v on error, want nil", got)
	}
	if !errors.Is(err, paper.ErrUpstreamMalformed) {
		t.Fatalf("Fetch err=%v, want Is(ErrUpstreamMalformed)", err)
	}

	warns := log.filterLevel("Warn")
	if len(warns) != 1 {
		t.Fatalf("Warn log count=%d, want 1", len(warns))
	}
	if warns[0].args["category"] != "malformed" {
		t.Fatalf("Warn category arg=%v, want malformed", warns[0].args["category"])
	}
}

func TestArxivUseCase_Unavailable_RelaysSentinelAndLogsWarn(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnErr: paper.ErrUpstreamUnavailable}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	got, err := uc.Fetch(context.Background())
	if got != nil {
		t.Fatalf("Fetch returned entries=%v on error, want nil", got)
	}
	if !errors.Is(err, paper.ErrUpstreamUnavailable) {
		t.Fatalf("Fetch err=%v, want Is(ErrUpstreamUnavailable)", err)
	}

	warns := log.filterLevel("Warn")
	if len(warns) != 1 {
		t.Fatalf("Warn log count=%d, want 1", len(warns))
	}
	if warns[0].args["category"] != "unavailable" {
		t.Fatalf("Warn category arg=%v, want unavailable", warns[0].args["category"])
	}
}

func TestArxivUseCase_UnknownError_ReliedUnchanged(t *testing.T) {
	t.Parallel()

	weird := errors.New("weird")
	fake := &fakePaperFetcher{returnErr: weird}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	got, err := uc.Fetch(context.Background())
	if got != nil {
		t.Fatalf("Fetch returned entries=%v on error, want nil", got)
	}
	if err != weird {
		t.Fatalf("Fetch err=%v, want %v (verbatim)", err, weird)
	}

	warns := log.filterLevel("Warn")
	if len(warns) != 1 {
		t.Fatalf("Warn log count=%d, want 1", len(warns))
	}
	if warns[0].args["category"] != "unknown" {
		t.Fatalf("Warn category arg=%v, want unknown", warns[0].args["category"])
	}
}

func TestArxivUseCase_PassesQueryToFetcher(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnEntries: []paper.Entry{}}
	log := &recordingLogger{}
	q := paper.Query{Categories: []string{"cs.LG"}, MaxResults: 50}

	uc := NewArxivUseCase(fake, log, q)
	if _, err := uc.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(fake.capturedQuery, q) {
		t.Fatalf("captured query=%v, want %v", fake.capturedQuery, q)
	}
}

func TestArxivUseCase_SingleFetcherCall(t *testing.T) {
	t.Parallel()

	fake := &fakePaperFetcher{returnEntries: []paper.Entry{}}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())
	if _, err := uc.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if fake.invocations != 1 {
		t.Fatalf("fetcher invocations=%d, want 1", fake.invocations)
	}
}

func TestArxivUseCase_NeverReturnsEntriesOnError(t *testing.T) {
	t.Parallel()

	// Even if the fake were to return non-empty entries alongside an error,
	// the use case must return nil. Our fake follows the contract and returns
	// nil entries with a non-nil err; assert the use case surface is nil.
	fake := &fakePaperFetcher{returnErr: paper.ErrUpstreamBadStatus}
	log := &recordingLogger{}

	uc := NewArxivUseCase(fake, log, newQuery())

	entries, err := uc.Fetch(context.Background())
	if err == nil {
		t.Fatalf("expected non-nil error")
	}
	if entries != nil {
		t.Fatalf("entries=%v on error path, want nil", entries)
	}
}
