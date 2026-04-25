//go:build integration

package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// arxivCategories mirrors the configured categories a realistic deployment
// would feed the use case. The exact values don't matter to the controller;
// the tests only assert that whatever is handed to SetupTestEnv round-trips
// unmodified into the fake's recorded Query.
var arxivCategories = []string{"cs.LG", "q-fin.ST"}

// arxivQuery is the canonical paper.Query used across the success-path tests.
// Defined once so the "fake records exactly the harness-configured query"
// assertion has a single reference value.
func arxivQuery() paper.Query {
	return paper.Query{Categories: arxivCategories, MaxResults: 100}
}

// doAuthenticatedFetch issues GET /api/arxiv/fetch with the valid test token.
// Extracted so each scenario stays focused on its assertions.
func doAuthenticatedFetch(t *testing.T, env *setup.TestEnv) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/arxiv/fetch", nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// TestArxivIntegration_Happy drives the full /api/arxiv/fetch path against the
// fake fetcher. It asserts the wire shape (envelope + entries + count), that
// the fake was invoked exactly once, that every entry carries source="arxiv"
// and is_new=true on first call (R5.2, R5.3), that an immediate second call
// flips is_new to false on every entry via composite-key dedupe (R5.4), and
// that the persisted catalogue is observable through GET /api/papers (R5.1).
// Requirements covered: 1.1, 1.3, 5.1, 5.2, 5.3, 5.4, 5.7.
func TestArxivIntegration_Happy(t *testing.T) {
	t.Parallel()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 4, 2, 10, 0, 0, 0, time.UTC)
	// Source="arxiv" is stamped on every fixture entry because the production
	// arxiv parser does the same (R5.2). The fake passes entries through
	// verbatim, so without the explicit stamp the read-back via
	// /api/papers/arxiv/<id> below would 404 — composite-key lookup needs a
	// non-empty Source.
	fake := &mocks.PaperFetcher{
		Entries: []paper.Entry{
			{
				Source:          "arxiv",
				SourceID:        "2404.12345",
				Version:         "v1",
				Title:           "Fake Paper One",
				Authors:         []string{"Alice", "Bob"},
				Abstract:        "Abstract one",
				PrimaryCategory: "cs.LG",
				Categories:      []string{"cs.LG", "q-fin.ST"},
				SubmittedAt:     submitted,
				UpdatedAt:       updated,
				PDFURL:          "https://arxiv.org/pdf/2404.12345v1",
				AbsURL:          "https://arxiv.org/abs/2404.12345v1",
			},
			{
				Source:      "arxiv",
				SourceID:    "2404.67890",
				Title:       "Fake Paper Two",
				Authors:     []string{"Carol"},
				Categories:  []string{"cs.LG"},
				SubmittedAt: submitted,
				UpdatedAt:   updated,
			},
		},
	}
	query := arxivQuery()

	// Real PaperRepo is wired by default (no PaperRepo override): the fetch
	// path persists, and the same SQLite catalogue backs /api/papers reads.
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   query,
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Entries []struct {
				Source   string `json:"source"`
				SourceID string `json:"source_id"`
				Version  string `json:"version,omitempty"`
				Title    string `json:"title"`
				IsNew    bool   `json:"is_new"`
			} `json:"entries"`
			Count     int       `json:"count"`
			FetchedAt time.Time `json:"fetched_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got, want := len(body.Data.Entries), 2; got != want {
		t.Fatalf("len(entries) = %d want %d", got, want)
	}
	if body.Data.Count != 2 {
		t.Errorf("data.count = %d want 2", body.Data.Count)
	}
	if body.Data.Entries[0].SourceID != "2404.12345" {
		t.Errorf("entries[0].source_id = %q want %q", body.Data.Entries[0].SourceID, "2404.12345")
	}
	if body.Data.Entries[0].Title != "Fake Paper One" {
		t.Errorf("entries[0].title = %q want %q", body.Data.Entries[0].Title, "Fake Paper One")
	}
	if body.Data.FetchedAt.IsZero() {
		t.Error("data.fetched_at is zero; controller must stamp a time")
	}

	// R5.2 + R5.3: every entry surfaces source="arxiv" and is_new=true on the
	// first call (catalogue starts empty). Looping rather than indexing keeps
	// the assertion robust if more fixtures are added later.
	for i, e := range body.Data.Entries {
		if e.Source != "arxiv" {
			t.Errorf("entries[%d].source = %q want %q", i, e.Source, "arxiv")
		}
		if !e.IsNew {
			t.Errorf("entries[%d].is_new = false want true (first call)", i)
		}
	}

	if fake.Invocations != 1 {
		t.Fatalf("fake.Invocations = %d want 1", fake.Invocations)
	}
	if len(fake.Queries) != 1 {
		t.Fatalf("fake.Queries len = %d want 1", len(fake.Queries))
	}
	if !reflect.DeepEqual(fake.Queries[0], query) {
		t.Fatalf("fake.Queries[0] = %+v want %+v", fake.Queries[0], query)
	}

	// R5.1: persistence side effect — the first entry is now retrievable via
	// the read endpoint, proving the fetch path actually wrote to the same
	// catalogue /api/papers serves from.
	getResp := doAuthenticatedGet(t, env.Server.URL+"/api/papers/arxiv/2404.12345")
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/papers/arxiv/2404.12345 status = %d want 200 (R5.1)", getResp.StatusCode)
	}
	var stored struct {
		Data struct {
			Source   string `json:"source"`
			SourceID string `json:"source_id"`
			Title    string `json:"title"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&stored); err != nil {
		t.Fatalf("decode stored: %v", err)
	}
	if stored.Data.Source != "arxiv" || stored.Data.SourceID != "2404.12345" {
		t.Errorf("stored entry composite key = (%q, %q) want (%q, %q)",
			stored.Data.Source, stored.Data.SourceID, "arxiv", "2404.12345")
	}
	if stored.Data.Title != "Fake Paper One" {
		t.Errorf("stored entry title = %q want %q", stored.Data.Title, "Fake Paper One")
	}

	// R5.4: an immediate second call against the same upstream fixture must
	// dedupe via the (source, source_id) unique index. Every entry comes back
	// with is_new=false; the wire shape is otherwise unchanged.
	resp2 := doAuthenticatedFetch(t, env)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second call status = %d want 200", resp2.StatusCode)
	}
	var body2 struct {
		Data struct {
			Entries []struct {
				Source   string `json:"source"`
				SourceID string `json:"source_id"`
				IsNew    bool   `json:"is_new"`
			} `json:"entries"`
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&body2); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if body2.Data.Count != 2 {
		t.Errorf("second data.count = %d want 2", body2.Data.Count)
	}
	for i, e := range body2.Data.Entries {
		if e.Source != "arxiv" {
			t.Errorf("second entries[%d].source = %q want %q", i, e.Source, "arxiv")
		}
		if e.IsNew {
			t.Errorf("second entries[%d].is_new = true want false (R5.4 dedupe)", i)
		}
	}
	if fake.Invocations != 2 {
		t.Errorf("fake.Invocations after second call = %d want 2", fake.Invocations)
	}
}

// TestArxivIntegration_500_SaveFailure covers requirement 5.5: when the
// repository fails to persist (paper.ErrCatalogueUnavailable), the fetch
// endpoint surfaces a 500 with the standard error envelope and the use case
// short-circuits without producing a partial slice. The fake is configured
// to fail every Save, but with multiple fixture entries we additionally
// assert exactly one Save attempt was recorded — proof the loop aborted on
// the first failure rather than continuing through the batch.
func TestArxivIntegration_500_SaveFailure(t *testing.T) {
	t.Parallel()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	fake := &mocks.PaperFetcher{
		Entries: []paper.Entry{
			{
				Source:      "arxiv",
				SourceID:    "2404.12345",
				Title:       "Fake Paper One",
				Authors:     []string{"Alice"},
				Categories:  []string{"cs.LG"},
				SubmittedAt: submitted,
				UpdatedAt:   submitted,
			},
			{
				Source:      "arxiv",
				SourceID:    "2404.67890",
				Title:       "Fake Paper Two",
				Authors:     []string{"Bob"},
				Categories:  []string{"cs.LG"},
				SubmittedAt: submitted,
				UpdatedAt:   submitted,
			},
		},
	}
	// SaveDefaultErr makes every Save invocation return ErrCatalogueUnavailable.
	// Combined with two fixture entries, the "exactly one Save call" assertion
	// below proves the orchestrator short-circuits on the first failure.
	repo := &mocks.PaperRepo{SaveDefaultErr: paper.ErrCatalogueUnavailable}

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
		PaperRepo:    repo,
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d want 500", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, http.StatusInternalServerError)

	if fake.Invocations != 1 {
		t.Errorf("fake.Invocations = %d want 1 (fetcher must run before save fails)", fake.Invocations)
	}
	// R5.5: the orchestrator must abort on the first save failure — never
	// emit a partial slice. With two fixture entries and a save that always
	// fails, the only legitimate count is 1.
	if got := len(repo.SaveCalls); got != 1 {
		t.Errorf("repo.SaveCalls = %d want 1 (R5.5 short-circuit, no partial slice)", got)
	}
}

// TestArxivIntegration_EmptyFeed verifies that a source returning zero entries
// yields a 200 with "entries":[] (non-null, zero-length) and count 0, matching
// requirement 1.5. The raw-bytes assertion catches any regression where the
// controller accidentally marshals nil instead of an empty slice.
func TestArxivIntegration_EmptyFeed(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Entries: []paper.Entry{}}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(raw), `"entries":[]`) {
		t.Fatalf("body missing `\"entries\":[]`; got: %s", string(raw))
	}

	var body struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Count != 0 {
		t.Errorf("data.count = %d want 0", body.Data.Count)
	}
	if fake.Invocations != 1 {
		t.Errorf("fake.Invocations = %d want 1", fake.Invocations)
	}
}

// TestArxivIntegration_401_MissingToken covers the "no X-API-Token header"
// branch of requirement 1.2. Auth must short-circuit before the fetcher is
// invoked; the assertion on fake.Invocations protects that guarantee.
func TestArxivIntegration_401_MissingToken(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Entries: []paper.Entry{}}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	resp, err := http.Get(env.Server.URL + "/api/arxiv/fetch")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
	if fake.Invocations != 0 {
		t.Fatalf("fake.Invocations = %d want 0 (auth must short-circuit)", fake.Invocations)
	}
}

// TestArxivIntegration_401_InvalidToken covers the "wrong X-API-Token" branch
// of requirement 1.2. Same safety property as the missing-token case: the
// fetcher must not be contacted.
func TestArxivIntegration_401_InvalidToken(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Entries: []paper.Entry{}}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/arxiv/fetch", nil)
	req.Header.Set("X-API-Token", "wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
	if fake.Invocations != 0 {
		t.Fatalf("fake.Invocations = %d want 0 (auth must short-circuit)", fake.Invocations)
	}
}

// assertErrorEnvelope decodes the standard { "error": { "code": N, "message": "..." } }
// envelope rendered by the ErrorEnvelope middleware from *shared.HTTPError
// sentinels, and verifies the shape. code arrives as float64 after JSON decode.
func assertErrorEnvelope(t *testing.T, resp *http.Response, wantCode int) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type: %#v", body)
	}
	gotCode, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("body.error.code missing or wrong type: %#v", errObj["code"])
	}
	if int(gotCode) != wantCode {
		t.Errorf("body.error.code = %d want %d", int(gotCode), wantCode)
	}
	msg, ok := errObj["message"].(string)
	if !ok || msg == "" {
		t.Errorf("body.error.message missing or empty: %#v", errObj["message"])
	}
}

// TestArxivIntegration_502_BadStatus covers requirement 4.1: an upstream
// non-success status from the paper source surfaces as a 502 with the
// standard error envelope. The fetcher MUST have been invoked (this is a
// downstream failure, not an auth short-circuit).
func TestArxivIntegration_502_BadStatus(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Error: paper.ErrUpstreamBadStatus}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d want 502", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, http.StatusBadGateway)
	if fake.Invocations != 1 {
		t.Errorf("fake.Invocations = %d want 1", fake.Invocations)
	}
}

// TestArxivIntegration_502_Malformed covers requirement 4.2: a malformed
// upstream body surfaces as a 502 with the standard error envelope.
func TestArxivIntegration_502_Malformed(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Error: paper.ErrUpstreamMalformed}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d want 502", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, http.StatusBadGateway)
	if fake.Invocations != 1 {
		t.Errorf("fake.Invocations = %d want 1", fake.Invocations)
	}
}

// TestArxivIntegration_504_Unavailable covers requirement 4.3: upstream
// network unavailability / timeout surfaces as a 504 with the standard error
// envelope. Distinct from the 502 cases so operators can tell transport
// failures apart from bad responses without reading logs.
func TestArxivIntegration_504_Unavailable(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{Error: paper.ErrUpstreamUnavailable}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   arxivQuery(),
	})
	defer env.Close()

	resp := doAuthenticatedFetch(t, env)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d want 504", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, http.StatusGatewayTimeout)
	if fake.Invocations != 1 {
		t.Errorf("fake.Invocations = %d want 1", fake.Invocations)
	}
}
