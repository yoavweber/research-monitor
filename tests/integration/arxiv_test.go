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

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/tests/integration/setup"
	"github.com/yoavweber/defi-monitor-backend/tests/mocks"
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
// the fake was invoked exactly once, and that the Query the fake observed is
// exactly the one the harness was configured with (requirements 1.1, 1.3).
func TestArxivIntegration_Happy(t *testing.T) {
	t.Parallel()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 4, 2, 10, 0, 0, 0, time.UTC)
	fake := &mocks.PaperFetcher{
		Entries: []paper.Entry{
			{
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
				SourceID string `json:"source_id"`
				Version  string `json:"version,omitempty"`
				Title    string `json:"title"`
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

	if fake.Invocations != 1 {
		t.Fatalf("fake.Invocations = %d want 1", fake.Invocations)
	}
	if len(fake.Queries) != 1 {
		t.Fatalf("fake.Queries len = %d want 1", len(fake.Queries))
	}
	if !reflect.DeepEqual(fake.Queries[0], query) {
		t.Fatalf("fake.Queries[0] = %+v want %+v", fake.Queries[0], query)
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
