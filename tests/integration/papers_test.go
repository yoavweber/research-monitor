//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

// doAuthenticatedGet issues a GET against the test server with the canonical
// X-API-Token. Extracted so each scenario stays focused on its assertions and
// the auth header is set in exactly one place (mirrors arxiv_test.go style).
func doAuthenticatedGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// seedEntry constructs a fully-populated paper.Entry. Tests that need to seed
// the real repo via env.PaperRepo.Save call this so the resulting row carries
// every field the wire DTO advertises — that's the only way the 12-field
// assertion in TestPapers_Get_AllFields can prove no field is dropped on the
// persistence round-trip.
func seedEntry(source, sourceID string, submitted time.Time) paper.Entry {
	return paper.Entry{
		Source:          source,
		SourceID:        sourceID,
		Version:         "v1",
		Title:           "Title for " + sourceID,
		Authors:         []string{"Alice", "Bob"},
		Abstract:        "Abstract for " + sourceID,
		PrimaryCategory: "cs.LG",
		Categories:      []string{"cs.LG", "q-fin.ST"},
		SubmittedAt:     submitted,
		UpdatedAt:       submitted.Add(24 * time.Hour),
		PDFURL:          "https://arxiv.org/pdf/" + sourceID + "v1",
		AbsURL:          "https://arxiv.org/abs/" + sourceID + "v1",
	}
}

// paperWire mirrors the controller's PaperResponse (12 fields). Defined
// inline here, not imported, so the test fails loudly if the wire contract
// drifts — the JSON tags are part of the public API per the design doc.
type paperWire struct {
	Source          string    `json:"source"`
	SourceID        string    `json:"source_id"`
	Version         string    `json:"version,omitempty"`
	Title           string    `json:"title"`
	Authors         []string  `json:"authors"`
	Abstract        string    `json:"abstract"`
	PrimaryCategory string    `json:"primary_category"`
	Categories      []string  `json:"categories"`
	SubmittedAt     time.Time `json:"submitted_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	PDFURL          string    `json:"pdf_url"`
	AbsURL          string    `json:"abs_url"`
}

// TestPapers_Get_401_MissingToken covers requirement 2.1 (auth on Get):
// the APIToken middleware MUST short-circuit before the repo is touched.
func TestPapers_Get_401_MissingToken(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	resp, err := http.Get(env.Server.URL + "/api/papers/arxiv/2404.12345")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
}

// TestPapers_Get_401_InvalidToken covers requirement 2.1 again: a wrong
// token is rejected with 401, indistinguishable from a missing one.
func TestPapers_Get_401_InvalidToken(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/papers/arxiv/2404.12345", nil)
	req.Header.Set("X-API-Token", "wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
}

// TestPapers_List_401_MissingToken covers requirement 3.1 (auth on List).
func TestPapers_List_401_MissingToken(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	resp, err := http.Get(env.Server.URL + "/api/papers")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
}

// TestPapers_List_401_InvalidToken covers requirement 3.1 (auth on List).
func TestPapers_List_401_InvalidToken(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/papers", nil)
	req.Header.Set("X-API-Token", "wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
}

// TestPapers_Get_404 covers requirement 2.4: a missing (source, source_id)
// composite key materialises as a 404 with the standard error envelope —
// never as a 500 or a 200 with empty body.
func TestPapers_Get_404(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/papers/arxiv/nonexistent")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d want 404", resp.StatusCode)
	}
	assertErrorEnvelope(t, resp, http.StatusNotFound)
}

// TestPapers_List_Empty covers requirement 3.3: with nothing persisted, the
// list endpoint MUST return 200 with a non-null empty papers array and
// count=0. The raw-bytes assertion catches a regression where Go marshals
// nil as null instead of [].
func TestPapers_List_Empty(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/papers")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(raw), `"papers":[]`) {
		t.Fatalf("body missing `\"papers\":[]`; got: %s", string(raw))
	}
	if !strings.Contains(string(raw), `"count":0`) {
		t.Fatalf("body missing `\"count\":0`; got: %s", string(raw))
	}
}

// TestPapers_Get_AllFields seeds a single entry directly through the real
// repository exposed by the harness, then asserts that GET returns 200 and
// every one of the 12 wire fields round-trips intact (R2.2, R2.3). The
// matrix below is the contract — adding a field to PaperResponse without
// updating this test should make the test fail to compile.
func TestPapers_Get_AllFields(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	entry := seedEntry("arxiv", "2404.12345", submitted)

	// Save through the harness-exposed real repo: the read endpoint observes
	// the same row the persistence round-trip produced.
	if _, err := env.PaperRepo.Save(context.Background(), entry); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/papers/arxiv/2404.12345")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	var body struct {
		Data paperWire `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	got := body.Data
	if got.Source != entry.Source {
		t.Errorf("source = %q want %q", got.Source, entry.Source)
	}
	if got.SourceID != entry.SourceID {
		t.Errorf("source_id = %q want %q", got.SourceID, entry.SourceID)
	}
	if got.Version != entry.Version {
		t.Errorf("version = %q want %q", got.Version, entry.Version)
	}
	if got.Title != entry.Title {
		t.Errorf("title = %q want %q", got.Title, entry.Title)
	}
	if !equalStrings(got.Authors, entry.Authors) {
		t.Errorf("authors = %v want %v", got.Authors, entry.Authors)
	}
	if got.Abstract != entry.Abstract {
		t.Errorf("abstract = %q want %q", got.Abstract, entry.Abstract)
	}
	if got.PrimaryCategory != entry.PrimaryCategory {
		t.Errorf("primary_category = %q want %q", got.PrimaryCategory, entry.PrimaryCategory)
	}
	if !equalStrings(got.Categories, entry.Categories) {
		t.Errorf("categories = %v want %v", got.Categories, entry.Categories)
	}
	if !got.SubmittedAt.Equal(entry.SubmittedAt) {
		t.Errorf("submitted_at = %v want %v", got.SubmittedAt, entry.SubmittedAt)
	}
	if !got.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("updated_at = %v want %v", got.UpdatedAt, entry.UpdatedAt)
	}
	if got.PDFURL != entry.PDFURL {
		t.Errorf("pdf_url = %q want %q", got.PDFURL, entry.PDFURL)
	}
	if got.AbsURL != entry.AbsURL {
		t.Errorf("abs_url = %q want %q", got.AbsURL, entry.AbsURL)
	}
}

// TestPapers_List_AfterSeed covers requirements 3.3 / 3.4: a single seeded
// entry must surface in the list with count=1 and the same wire fields the
// single-paper Get returns. Pairs with TestPapers_Get_AllFields to lock the
// list/get DTO consistency.
func TestPapers_List_AfterSeed(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	entry := seedEntry("arxiv", "2404.12345", submitted)
	if _, err := env.PaperRepo.Save(context.Background(), entry); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/papers")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Papers []paperWire `json:"papers"`
			Count  int         `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Count != 1 {
		t.Fatalf("count = %d want 1", body.Data.Count)
	}
	if len(body.Data.Papers) != 1 {
		t.Fatalf("len(papers) = %d want 1", len(body.Data.Papers))
	}
	if body.Data.Papers[0].SourceID != entry.SourceID {
		t.Errorf("papers[0].source_id = %q want %q", body.Data.Papers[0].SourceID, entry.SourceID)
	}
}

// TestPapers_List_OrderedNewestFirst covers requirement 3.2: list ordering
// is descending by SubmittedAt regardless of insertion order. We seed the
// older row first so a naive insertion-order ORDER BY would produce the
// wrong sequence and fail this assertion.
func TestPapers_List_OrderedNewestFirst(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	older := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	// Seed older first; if List doesn't sort by submitted_at DESC the
	// resulting body would have older at index 0 and the test would fail.
	if _, err := env.PaperRepo.Save(context.Background(), seedEntry("arxiv", "older.id", older)); err != nil {
		t.Fatalf("seed older: %v", err)
	}
	if _, err := env.PaperRepo.Save(context.Background(), seedEntry("arxiv", "newer.id", newer)); err != nil {
		t.Fatalf("seed newer: %v", err)
	}

	resp := doAuthenticatedGet(t, env.Server.URL+"/api/papers")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Papers []paperWire `json:"papers"`
			Count  int         `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Count != 2 {
		t.Fatalf("count = %d want 2", body.Data.Count)
	}
	// R3.2: newest-first.
	if body.Data.Papers[0].SourceID != "newer.id" {
		t.Errorf("papers[0].source_id = %q want %q (R3.2 newest-first)",
			body.Data.Papers[0].SourceID, "newer.id")
	}
	if body.Data.Papers[1].SourceID != "older.id" {
		t.Errorf("papers[1].source_id = %q want %q (R3.2 newest-first)",
			body.Data.Papers[1].SourceID, "older.id")
	}
}

// TestPapers_CompositeKey_DistinctSources covers requirement 1.3: SourceID
// alone is NOT unique — uniqueness is enforced over the (Source, SourceID)
// pair. Two entries sharing a SourceID but coming from different Sources
// must coexist in the catalogue and each be retrievable by its own
// composite key.
func TestPapers_CompositeKey_DistinctSources(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	submitted := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	sharedID := "1234.56789"

	arxivEntry := seedEntry("arxiv", sharedID, submitted)
	otherEntry := seedEntry("biorxiv", sharedID, submitted.Add(time.Hour))

	if _, err := env.PaperRepo.Save(context.Background(), arxivEntry); err != nil {
		t.Fatalf("seed arxiv: %v", err)
	}
	if _, err := env.PaperRepo.Save(context.Background(), otherEntry); err != nil {
		t.Fatalf("seed biorxiv: %v", err)
	}

	// R1.3: List exposes both rows — no collapse on shared SourceID.
	listResp := doAuthenticatedGet(t, env.Server.URL+"/api/papers")
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d want 200", listResp.StatusCode)
	}
	var listBody struct {
		Data struct {
			Papers []paperWire `json:"papers"`
			Count  int         `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listBody.Data.Count != 2 {
		t.Fatalf("count = %d want 2 (R1.3: distinct (source,source_id) pairs coexist)", listBody.Data.Count)
	}

	// R1.3: each row is independently retrievable by its full composite key.
	for _, want := range []struct {
		source, id string
	}{
		{"arxiv", sharedID},
		{"biorxiv", sharedID},
	} {
		url := env.Server.URL + "/api/papers/" + want.source + "/" + want.id
		resp := doAuthenticatedGet(t, url)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("get %s/%s status = %d want 200", want.source, want.id, resp.StatusCode)
		}
		var body struct {
			Data paperWire `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatalf("decode get %s: %v", want.source, err)
		}
		resp.Body.Close()
		if body.Data.Source != want.source {
			t.Errorf("source = %q want %q", body.Data.Source, want.source)
		}
		if body.Data.SourceID != want.id {
			t.Errorf("source_id = %q want %q", body.Data.SourceID, want.id)
		}
	}
}

// equalStrings is a local helper because importing reflect just for one
// slice comparison adds noise; behaviourally we only care about same
// length and same element ordering, which the repo preserves verbatim.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
