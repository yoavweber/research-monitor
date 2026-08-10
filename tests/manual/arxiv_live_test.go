//go:build manual

// Package manual hosts manually-run, network-dependent end-to-end tests. Files
// in this package are excluded from CI: they only compile under the `manual`
// build tag and must be invoked explicitly:
//
//	go test -tags=manual -count=1 -v ./tests/manual/...
//
// Each test in this package hits a real third-party service (arxiv.org for
// arxiv_live_test.go) and is allowed to fail loudly if the service is
// unreachable — it is a manual sanity check, not a CI gate.
package manual_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	arxivctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/arxiv"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// liveQueryURL is the exact arxiv endpoint the live fetcher hits. Pinning the
// query (abstract-text "defi", first month of 2024, ascending order, two
// results) yields a stable response: arxiv papers are immutable once
// submitted, so the same SourceID + Title pair must come back on every run.
//
// Note: arxiv's `all:` field does not combine reliably with `submittedDate:`
// filters; using `abs:` (abstract-only search) is what makes the date range
// take effect. The window was widened from "first week of January" to the
// whole month because the narrower window had zero matching papers.
const liveQueryURL = "https://export.arxiv.org/api/query" +
	"?search_query=abs:defi+AND+submittedDate:[202401010000+TO+202401312359]" +
	"&sortBy=submittedDate&sortOrder=ascending" +
	"&max_results=2"

// expected lists the two papers the live query returns in order. Captured
// once via the curl command in /Users/yoavweber/.claude/plans/radiant-floating-pebble.md;
// arxiv guarantees these IDs and titles never change post-submission.
var expected = []struct {
	SourceID string
	Title    string
}{
	{
		SourceID: "2401.04521",
		Title:    "Proof of Efficient Liquidity: A Staking Mechanism for Capital Efficient Liquidity",
	},
	{
		SourceID: "2401.06044",
		Title:    "Safeguarding DeFi Smart Contracts against Oracle Deviations",
	},
}

// liveArxivFetcher implements paper.Fetcher by issuing a real HTTP GET against
// arxiv.org with a fixed (keyword + date range + max_results) query the
// production paper.Query shape does not express. The incoming Query is
// ignored on purpose: this test pins the wire request, not the configured
// query path.
//
// MIGRATION: when paper.Query grows Keyword and SubmittedDateRange fields,
// delete this type and inject the production arxivFetcher with a fully-
// specified Query — the test will then exercise the production URL builder
// too, and the live path will have no test-only seam left.
type liveArxivFetcher struct {
	client shared
}

// shared is the minimal slice of the byte fetcher's interface we need; spelled
// inline so we don't reach into internal packages from a test.
type shared interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

func (l *liveArxivFetcher) Fetch(ctx context.Context, _ paper.Query) ([]paper.Entry, error) {
	body, err := l.client.Fetch(ctx, liveQueryURL)
	if err != nil {
		return nil, err
	}
	return arxiv.ParseFeed(body)
}

func TestLiveArxiv(t *testing.T) {
	// Not parallel: a single live network roundtrip is easier to reason about
	// (and to debug from logs) when nothing else is racing. Subtests below
	// share env + DB state and run sequentially in order.
	logStep(t, "SETUP", "build test server + live fetcher pinned to %s", liveQueryURL)
	fetcher := &liveArxivFetcher{
		client: httpclient.NewByteFetcher(15*time.Second, "research-monitor-manual-test"),
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher:    fetcher,
		ArxivQuery:      paper.Query{MaxResults: 2}, // ignored by liveArxivFetcher; required by harness contract
		WirePDFDownload: true,
	})
	t.Cleanup(env.Close)
	logResult(t, "server up at %s, pdfstore=%s", env.Server.URL, env.PDFStoreRoot)

	// jobID is captured by the first subtest and consumed by the PDF
	// subtest. Subtests run sequentially (no t.Parallel) so the closure
	// hand-off is safe.
	var jobID string

	t.Run("fetches 2 pinned papers and persists them as new", func(t *testing.T) {
		logStep(t, "FETCH", "GET /api/arxiv/fetch (expect 2 new, fresh DB)")
		first := doFetch(t, env)
		logResult(t, "got %d entries  %s", len(first.Entries), summariseEntries(first.Entries))
		if len(first.Entries) != 2 {
			t.Fatalf("first fetch returned %d entries, want 2", len(first.Entries))
		}
		for i, e := range first.Entries {
			if e.Source != paper.SourceArxiv {
				t.Errorf("entries[%d].source = %q, want %q", i, e.Source, paper.SourceArxiv)
			}
			if !e.IsNew {
				t.Errorf("entries[%d].is_new = false, want true on first fetch (fresh DB)", i)
			}
			if e.SourceID != expected[i].SourceID {
				t.Errorf("entries[%d].source_id = %q, want %q (arxiv changed pinned IDs?)", i, e.SourceID, expected[i].SourceID)
			}
			if e.Title != expected[i].Title {
				t.Errorf("entries[%d].title = %q, want %q (arxiv changed pinned title?)", i, e.Title, expected[i].Title)
			}
		}
		if first.Job == nil || first.Job.JobID == "" {
			t.Fatalf("fetch response missing job.job_id (WirePDFDownload not active?)")
		}
		jobID = first.Job.JobID
		logResult(t, "captured job_id=%s for download verification", jobID)
	})

	t.Run("downloads the pinned PDFs, verifies bytes on disk, and removes them", func(t *testing.T) {
		if jobID == "" {
			t.Fatal("previous subtest did not capture a job_id")
		}
		logStep(t, "DOWNLOAD", "poll GET /api/arxiv/downloads/%s until completed=true (60s deadline)", jobID)

		deadline := time.Now().Add(60 * time.Second)
		var snap paperctrl.DownloadJobSnapshotDTO
		for {
			snap = doDownloadStatus(t, env, jobID)
			if snap.Completed {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("download job %s did not complete within 60s: %+v", jobID, snap)
			}
			time.Sleep(250 * time.Millisecond)
		}
		logResult(t, "completed: total=%d succeeded=%d failed=%d", snap.Total, snap.Succeeded, snap.Failed)
		if snap.Succeeded != 2 || snap.Failed != 0 {
			t.Fatalf("status: succeeded=%d failed=%d total=%d; want 2/0/2  (entries=%+v)",
				snap.Succeeded, snap.Failed, snap.Total, snap.Entries)
		}

		logStep(t, "VERIFY", "stat each artifact under %s", env.PDFStoreRoot)
		paths := make([]string, 0, len(snap.Entries))
		for i, e := range snap.Entries {
			if e.Status != "success" {
				t.Errorf("entries[%d] status=%q want success", i, e.Status)
				continue
			}
			// Compute the canonical on-disk path the same way the production
			// store does: PDFArtifactKey owns the (source_id+version) rule;
			// localStore lays out files as <root>/<source>/<key>.pdf.
			id := paper.ID{Source: e.PaperID.Source, SourceID: e.PaperID.SourceID, Version: e.PaperID.Version}
			path := filepath.Join(env.PDFStoreRoot, id.Source, pdfdownload.PDFArtifactKey(id)+".pdf")
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("stat %s: %v", path, err)
				continue
			}
			if info.Size() == 0 {
				t.Errorf("entries[%d] file %s is empty (0 bytes)", i, path)
			}
			if int(info.Size()) != e.Bytes {
				t.Errorf("entries[%d] disk size=%d, status.bytes=%d (mismatch)", i, info.Size(), e.Bytes)
			}
			logResult(t, "%s  %d bytes ok", path, info.Size())
			paths = append(paths, path)
		}

		logStep(t, "DELETE", "remove %d files and confirm they are gone", len(paths))
		for _, path := range paths {
			if err := os.Remove(path); err != nil {
				t.Errorf("remove %s: %v", path, err)
				continue
			}
			if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("post-delete stat %s: err=%v, want fs.ErrNotExist", path, err)
			}
		}
		logResult(t, "removed %d files, all gone", len(paths))
	})

	t.Run("lists both persisted papers via /api/papers", func(t *testing.T) {
		logStep(t, "LIST", "GET /api/papers (must surface both persisted papers)")
		listed := doListPapers(t, env)
		logResult(t, "got count=%d", listed.Count)
		if listed.Count != 2 {
			t.Fatalf("/api/papers count=%d, want 2", listed.Count)
		}
		gotIDs := []string{listed.Papers[0].SourceID, listed.Papers[1].SourceID}
		wantIDs := []string{expected[0].SourceID, expected[1].SourceID}
		slices.Sort(gotIDs)
		slices.Sort(wantIDs)
		if !slices.Equal(gotIDs, wantIDs) {
			t.Errorf("/api/papers IDs = %v, want (any order) %v", gotIDs, wantIDs)
		}
		for _, p := range listed.Papers {
			var match bool
			for _, exp := range expected {
				if p.SourceID == exp.SourceID && p.Title == exp.Title {
					match = true
					break
				}
			}
			if !match {
				t.Errorf("/api/papers contains unexpected (source_id=%q, title=%q)", p.SourceID, p.Title)
			}
		}
	})

	t.Run("reads a single paper by source id", func(t *testing.T) {
		logStep(t, "READ", "GET /api/papers/arxiv/%s", expected[0].SourceID)
		one := doGetPaper(t, env, expected[0].SourceID)
		logResult(t, "%s  %q", one.SourceID, one.Title)
		if one.SourceID != expected[0].SourceID {
			t.Errorf("Get source_id=%q, want %q", one.SourceID, expected[0].SourceID)
		}
		if one.Title != expected[0].Title {
			t.Errorf("Get title=%q, want %q", one.Title, expected[0].Title)
		}
	})

	t.Run("deduplicates papers on re-fetch (is_new=false)", func(t *testing.T) {
		logStep(t, "DEDUPE", "second fetch — GET /api/arxiv/fetch (expect is_new=false on both)")
		second := doFetch(t, env)
		logResult(t, "got %d entries  %s", len(second.Entries), summariseEntries(second.Entries))
		if len(second.Entries) != 2 {
			t.Fatalf("second fetch returned %d entries, want 2", len(second.Entries))
		}
		for i, e := range second.Entries {
			if e.IsNew {
				t.Errorf("entries[%d].is_new = true on second fetch, want false (dedupe)", i)
			}
			if e.SourceID != expected[i].SourceID || e.Title != expected[i].Title {
				t.Errorf("entries[%d] mismatched on second fetch: source_id=%q title=%q, want %q / %q",
					i, e.SourceID, e.Title, expected[i].SourceID, expected[i].Title)
			}
		}
		// Dedupe-time download re-scheduling is intentionally not asserted:
		// any job started here drains via env.Close.
	})
}

// logStep prints a high-contrast section header so the four phases of the
// test are skimmable in -v output. The leading newline gives breathing room
// after the slog records emitted by the previous step.
func logStep(t *testing.T, label, format string, args ...any) {
	t.Helper()
	t.Log("")
	t.Logf("──── %s ──── %s", label, fmtMsg(format, args...))
}

// logResult prints a follow-up line attributed to the step that just ran.
func logResult(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("       └─ %s", fmtMsg(format, args...))
}

func fmtMsg(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// summariseEntries renders entries as "id1(new) id2(dup)" — short and
// scannable, no truncation surprises since the live query is pinned to 2.
func summariseEntries(entries []arxivctrl.EntryResponse) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		state := "dup"
		if e.IsNew {
			state = "new"
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", e.SourceID, state))
	}
	return strings.Join(parts, " ")
}

// envelope wraps the production controller response in the common {"data": ...}
// shell so json.Decode lands the typed payload directly. Type-parameterized so
// the same wrapper works for every endpoint we hit.
type envelope[T any] struct {
	Data T `json:"data"`
}

func doFetch(t *testing.T, env *setup.TestEnv) arxivctrl.FetchResponse {
	t.Helper()
	var out envelope[arxivctrl.FetchResponse]
	doAuthenticatedJSON(t, env, "/api/arxiv/fetch", &out)
	return out.Data
}

func doListPapers(t *testing.T, env *setup.TestEnv) paperctrl.PaperListResponse {
	t.Helper()
	var out envelope[paperctrl.PaperListResponse]
	doAuthenticatedJSON(t, env, "/api/papers", &out)
	return out.Data
}

func doGetPaper(t *testing.T, env *setup.TestEnv, sourceID string) paperctrl.PaperResponse {
	t.Helper()
	var out envelope[paperctrl.PaperResponse]
	doAuthenticatedJSON(t, env, "/api/papers/"+paper.SourceArxiv+"/"+sourceID, &out)
	return out.Data
}

func doDownloadStatus(t *testing.T, env *setup.TestEnv, jobID string) paperctrl.DownloadJobSnapshotDTO {
	t.Helper()
	var out envelope[paperctrl.DownloadJobSnapshotDTO]
	doAuthenticatedJSON(t, env, "/api/arxiv/downloads/"+jobID, &out)
	return out.Data
}

func doAuthenticatedJSON(t *testing.T, env *setup.TestEnv, path string, out any) {
	t.Helper()
	req := setup.AuthorizedRequest(t, env, http.MethodGet, path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: status=%d, body=%s", path, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("%s decode: %v", path, err)
	}
}
