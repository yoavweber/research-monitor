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
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	arxivctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/arxiv"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
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

func TestLiveArxiv_FullPath(t *testing.T) {
	// Not parallel: a single live network roundtrip is easier to reason about
	// (and to debug from logs) when nothing else is racing.
	t.Logf("setup: building test server, hitting %s", liveQueryURL)
	fetcher := &liveArxivFetcher{
		client: httpclient.NewByteFetcher(15*time.Second, "research-monitor-manual-test"),
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fetcher,
		ArxivQuery:   paper.Query{MaxResults: 2}, // ignored by liveArxivFetcher; required by harness contract
	})
	t.Cleanup(env.Close)
	t.Logf("setup: server ready at %s", env.Server.URL)

	t.Logf("step 1/4: GET /api/arxiv/fetch — first fetch should hit arxiv.org and persist both entries")
	first := doFetch(t, env)
	t.Logf("step 1/4: got %d entries; is_new=[%t,%t]; ids=[%s,%s]",
		len(first.Entries),
		first.Entries[0].IsNew, lastIsNew(first.Entries),
		first.Entries[0].SourceID, lastSourceID(first.Entries))
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

	t.Logf("step 2/4: GET /api/papers — list endpoint must surface both persisted papers")
	listed := doListPapers(t, env)
	t.Logf("step 2/4: list returned count=%d", listed.Count)
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

	t.Logf("step 3/4: GET /api/papers/arxiv/%s — single-paper read-back", expected[0].SourceID)
	one := doGetPaper(t, env, expected[0].SourceID)
	t.Logf("step 3/4: got source_id=%q title=%q", one.SourceID, one.Title)
	if one.SourceID != expected[0].SourceID {
		t.Errorf("Get source_id=%q, want %q", one.SourceID, expected[0].SourceID)
	}
	if one.Title != expected[0].Title {
		t.Errorf("Get title=%q, want %q", one.Title, expected[0].Title)
	}

	t.Logf("step 4/4: GET /api/arxiv/fetch — second fetch should hit dedupe (is_new=false on both)")
	second := doFetch(t, env)
	t.Logf("step 4/4: got %d entries; is_new=[%t,%t]",
		len(second.Entries),
		second.Entries[0].IsNew, lastIsNew(second.Entries))
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
}

// lastIsNew / lastSourceID return the last entry's field or a sentinel when
// the slice is shorter than expected — used only for narration, never for
// assertions, so a soft fallback keeps the log line readable on partial data.
func lastIsNew(entries []arxivctrl.EntryResponse) bool {
	if len(entries) < 2 {
		return false
	}
	return entries[1].IsNew
}

func lastSourceID(entries []arxivctrl.EntryResponse) string {
	if len(entries) < 2 {
		return "<missing>"
	}
	return entries[1].SourceID
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

func doAuthenticatedJSON(t *testing.T, env *setup.TestEnv, path string, out any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
	if err != nil {
		t.Fatalf("new request %s: %v", path, err)
	}
	req.Header.Set(middleware.APITokenHeader, setup.TestToken)
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
