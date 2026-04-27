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
	fetcher := &liveArxivFetcher{
		client: httpclient.NewByteFetcher(15*time.Second, "research-monitor-manual-test"),
	}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fetcher,
		ArxivQuery:   paper.Query{MaxResults: 2}, // ignored by liveArxivFetcher; required by harness contract
	})
	t.Cleanup(env.Close)

	// Step 1 — first fetch persists both entries.
	first := doFetch(t, env)
	if len(first.Data.Entries) != 2 {
		t.Fatalf("first fetch returned %d entries, want 2", len(first.Data.Entries))
	}
	for i, e := range first.Data.Entries {
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

	// Step 2 — list endpoint must surface both persisted papers.
	listed := doListPapers(t, env)
	if listed.Data.Count != 2 {
		t.Fatalf("/api/papers count=%d, want 2", listed.Data.Count)
	}
	gotIDs := []string{listed.Data.Papers[0].SourceID, listed.Data.Papers[1].SourceID}
	wantIDs := []string{expected[0].SourceID, expected[1].SourceID}
	slices.Sort(gotIDs)
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("/api/papers IDs = %v, want (any order) %v", gotIDs, wantIDs)
	}
	for _, p := range listed.Data.Papers {
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

	// Step 3 — single-paper read-back returns the full record for the first
	// pinned entry.
	one := doGetPaper(t, env, expected[0].SourceID)
	if one.Data.SourceID != expected[0].SourceID {
		t.Errorf("Get source_id=%q, want %q", one.Data.SourceID, expected[0].SourceID)
	}
	if one.Data.Title != expected[0].Title {
		t.Errorf("Get title=%q, want %q", one.Data.Title, expected[0].Title)
	}

	// Step 4 — second fetch hits dedupe; both entries come back with
	// is_new=false. Proves the composite-unique-index works against real
	// persisted rows, end-to-end.
	second := doFetch(t, env)
	if len(second.Data.Entries) != 2 {
		t.Fatalf("second fetch returned %d entries, want 2", len(second.Data.Entries))
	}
	for i, e := range second.Data.Entries {
		if e.IsNew {
			t.Errorf("entries[%d].is_new = true on second fetch, want false (dedupe)", i)
		}
		if e.SourceID != expected[i].SourceID || e.Title != expected[i].Title {
			t.Errorf("entries[%d] mismatched on second fetch: source_id=%q title=%q, want %q / %q",
				i, e.SourceID, e.Title, expected[i].SourceID, expected[i].Title)
		}
	}
}

// --- response shapes (mirror the production wire contract) ---

type entryResponse struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	IsNew    bool   `json:"is_new"`
}

type fetchResponse struct {
	Data struct {
		Entries []entryResponse `json:"entries"`
	} `json:"data"`
}

type paperResponse struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
}

type listResponse struct {
	Data struct {
		Papers []paperResponse `json:"papers"`
		Count  int             `json:"count"`
	} `json:"data"`
}

type getResponse struct {
	Data paperResponse `json:"data"`
}

func doFetch(t *testing.T, env *setup.TestEnv) fetchResponse {
	t.Helper()
	var out fetchResponse
	doAuthenticatedJSON(t, env, "/api/arxiv/fetch", &out)
	return out
}

func doListPapers(t *testing.T, env *setup.TestEnv) listResponse {
	t.Helper()
	var out listResponse
	doAuthenticatedJSON(t, env, "/api/papers", &out)
	return out
}

func doGetPaper(t *testing.T, env *setup.TestEnv, sourceID string) getResponse {
	t.Helper()
	var out getResponse
	doAuthenticatedJSON(t, env, "/api/papers/"+paper.SourceArxiv+"/"+sourceID, &out)
	return out
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
