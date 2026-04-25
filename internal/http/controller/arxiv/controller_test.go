package arxiv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeOutcomeFetcher is an inline fake for arxivapp.OutcomeFetcher. It records
// invocations and returns the configured outcomes/error. Kept inline (not
// shared) on purpose — task 7.2 introduces the shared mocks file.
type fakeOutcomeFetcher struct {
	returnOutcomes []arxivapp.FetchedEntry
	returnErr      error
	invocations    int
}

func (f *fakeOutcomeFetcher) FetchWithOutcomes(_ context.Context) ([]arxivapp.FetchedEntry, error) {
	f.invocations++
	return f.returnOutcomes, f.returnErr
}

// fixedClock implements shared.Clock with a pre-set time so tests can assert
// the exact fetched_at stamped onto the response.
type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

// newEngine wires an in-memory Gin engine with the error envelope middleware so
// sentinel-translation tests see the same rendering as production.
func newEngine(ctrl *ArxivController) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	engine.GET("/api/arxiv/fetch", ctrl.Fetch)
	return engine
}

func sampleEntry() paper.Entry {
	submitted := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 10, 2, 11, 0, 0, 0, time.UTC)
	return paper.Entry{
		Source:          "arxiv",
		SourceID:        "2404.12345",
		Version:         "v1",
		Title:           "A sample paper",
		Authors:         []string{"A. Author", "B. Author"},
		Abstract:        "an abstract",
		PrimaryCategory: "cs.LG",
		Categories:      []string{"cs.LG", "stat.ML"},
		SubmittedAt:     submitted,
		UpdatedAt:       updated,
		PDFURL:          "http://arxiv.org/pdf/2404.12345v1",
		AbsURL:          "http://arxiv.org/abs/2404.12345v1",
	}
}

func TestArxivController_Success(t *testing.T) {
	t.Parallel()

	entry := sampleEntry()
	uc := &fakeOutcomeFetcher{
		returnOutcomes: []arxivapp.FetchedEntry{{Entry: entry, IsNew: true}},
	}
	clock := fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)}
	ctrl := NewArxivController(uc, clock)

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body not JSON: %v; raw=%s", err, w.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing or wrong type; body=%v", body)
	}

	gotCount, ok := data["count"].(float64)
	if !ok || int(gotCount) != 1 {
		t.Fatalf("data.count=%v, want 1", data["count"])
	}

	// fetched_at — decode as time.Time via JSON round-trip
	fetchedAtRaw, ok := data["fetched_at"].(string)
	if !ok {
		t.Fatalf("data.fetched_at missing or not string; data=%v", data)
	}
	gotFetchedAt, err := time.Parse(time.RFC3339Nano, fetchedAtRaw)
	if err != nil {
		t.Fatalf("data.fetched_at not RFC3339: %v (raw=%q)", err, fetchedAtRaw)
	}
	if !gotFetchedAt.Equal(clock.now) {
		t.Fatalf("data.fetched_at=%v, want %v", gotFetchedAt, clock.now)
	}

	entries, ok := data["entries"].([]any)
	if !ok {
		t.Fatalf("data.entries missing or not array; data=%v", data)
	}
	if len(entries) != 1 {
		t.Fatalf("data.entries length=%d, want 1", len(entries))
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("data.entries[0] wrong type: %T", entries[0])
	}
	if first["source"] != "arxiv" {
		t.Fatalf("entries[0].source=%v, want arxiv", first["source"])
	}
	if first["source_id"] != "2404.12345" {
		t.Fatalf("entries[0].source_id=%v, want 2404.12345", first["source_id"])
	}
	if first["version"] != "v1" {
		t.Fatalf("entries[0].version=%v, want v1", first["version"])
	}
	if first["title"] != "A sample paper" {
		t.Fatalf("entries[0].title=%v, want A sample paper", first["title"])
	}
	if first["primary_category"] != "cs.LG" {
		t.Fatalf("entries[0].primary_category=%v, want cs.LG", first["primary_category"])
	}
	if first["pdf_url"] != "http://arxiv.org/pdf/2404.12345v1" {
		t.Fatalf("entries[0].pdf_url=%v", first["pdf_url"])
	}
	if first["abs_url"] != "http://arxiv.org/abs/2404.12345v1" {
		t.Fatalf("entries[0].abs_url=%v", first["abs_url"])
	}
	if isNew, ok := first["is_new"].(bool); !ok || !isNew {
		t.Fatalf("entries[0].is_new=%v, want true", first["is_new"])
	}

	authors, ok := first["authors"].([]any)
	if !ok || len(authors) != 2 || authors[0] != "A. Author" || authors[1] != "B. Author" {
		t.Fatalf("entries[0].authors=%v, want [A. Author B. Author]", first["authors"])
	}
	cats, ok := first["categories"].([]any)
	if !ok || len(cats) != 2 || cats[0] != "cs.LG" || cats[1] != "stat.ML" {
		t.Fatalf("entries[0].categories=%v", first["categories"])
	}
}

// TestArxivController_IsNewMix verifies that a mixed batch (one new, one
// duplicate) round-trips through the wire envelope with per-entry is_new and
// source preserved in fetcher order (R5.3, R5.7).
func TestArxivController_IsNewMix(t *testing.T) {
	t.Parallel()

	first := sampleEntry()
	second := sampleEntry()
	second.SourceID = "2404.99999"
	second.Version = "v2"
	second.PDFURL = "http://arxiv.org/pdf/2404.99999v2"
	second.AbsURL = "http://arxiv.org/abs/2404.99999v2"

	uc := &fakeOutcomeFetcher{
		returnOutcomes: []arxivapp.FetchedEntry{
			{Entry: first, IsNew: true},
			{Entry: second, IsNew: false},
		},
	}
	clock := fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)}
	ctrl := NewArxivController(uc, clock)

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body not JSON: %v; raw=%s", err, w.Body.String())
	}
	data := body["data"].(map[string]any)
	entries := data["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries length=%d, want 2", len(entries))
	}

	e0 := entries[0].(map[string]any)
	e1 := entries[1].(map[string]any)
	if e0["source_id"] != "2404.12345" || e0["is_new"] != true {
		t.Fatalf("entries[0]: source_id=%v is_new=%v, want 2404.12345/true", e0["source_id"], e0["is_new"])
	}
	if e1["source_id"] != "2404.99999" || e1["is_new"] != false {
		t.Fatalf("entries[1]: source_id=%v is_new=%v, want 2404.99999/false", e1["source_id"], e1["is_new"])
	}
	if e0["source"] != "arxiv" || e1["source"] != "arxiv" {
		t.Fatalf("source: e0=%v e1=%v, want both arxiv", e0["source"], e1["source"])
	}
}

func TestArxivController_Empty_Returns_NonNull_EmptyArray(t *testing.T) {
	t.Parallel()

	uc := &fakeOutcomeFetcher{returnOutcomes: []arxivapp.FetchedEntry{}}
	clock := fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)}
	ctrl := NewArxivController(uc, clock)

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Raw body check: the wire must contain "entries":[] not "entries":null.
	// This is critical for requirement 1.5.
	raw := w.Body.String()
	if !containsJSONToken(raw, `"entries":[]`) {
		t.Fatalf("response body must contain \"entries\":[] (non-null empty array); body=%s", raw)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data := body["data"].(map[string]any)
	if gotCount, _ := data["count"].(float64); int(gotCount) != 0 {
		t.Fatalf("data.count=%v, want 0", data["count"])
	}
	entries, ok := data["entries"].([]any)
	if !ok {
		t.Fatalf("data.entries missing or null (must be []): %v", data["entries"])
	}
	if len(entries) != 0 {
		t.Fatalf("data.entries length=%d, want 0", len(entries))
	}
}

// containsJSONToken is a light substring helper; used for the critical empty-array
// wire-shape check where map decoding hides the null-vs-empty distinction.
func containsJSONToken(haystack, needle string) bool {
	return len(needle) > 0 && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestArxivController_BadStatus_Returns502(t *testing.T) {
	t.Parallel()
	assertSentinelEnvelope(t, paper.ErrUpstreamBadStatus, http.StatusBadGateway)
}

func TestArxivController_Malformed_Returns502(t *testing.T) {
	t.Parallel()
	assertSentinelEnvelope(t, paper.ErrUpstreamMalformed, http.StatusBadGateway)
}

func TestArxivController_Unavailable_Returns504(t *testing.T) {
	t.Parallel()
	assertSentinelEnvelope(t, paper.ErrUpstreamUnavailable, http.StatusGatewayTimeout)
}

// TestArxivController_CatalogueUnavailable_Returns500 covers the persistence
// failure path (R5.5): the use case relays paper.ErrCatalogueUnavailable, the
// ErrorEnvelope middleware renders it as a 500 envelope.
func TestArxivController_CatalogueUnavailable_Returns500(t *testing.T) {
	t.Parallel()
	assertSentinelEnvelope(t, paper.ErrCatalogueUnavailable, http.StatusInternalServerError)
}

// assertSentinelEnvelope runs a request that the fake use case fails with the
// provided sentinel and asserts the response envelope carries the expected
// status and error-envelope shape.
func assertSentinelEnvelope(t *testing.T, sentinel error, wantStatus int) {
	t.Helper()

	uc := &fakeOutcomeFetcher{returnErr: sentinel}
	clock := fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)}
	ctrl := NewArxivController(uc, clock)

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != wantStatus {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, wantStatus, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	errEnv, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type; body=%v", body)
	}
	if code, _ := errEnv["code"].(float64); int(code) != wantStatus {
		t.Fatalf("error.code=%v, want %d", errEnv["code"], wantStatus)
	}
	if msg, _ := errEnv["message"].(string); msg == "" {
		t.Fatalf("error.message empty")
	}
	// The envelope must NOT carry data when the controller short-circuits on error.
	if _, present := body["data"]; present {
		t.Fatalf("error response must not carry data; body=%v", body)
	}

	// Sanity: the use case was invoked exactly once (controller did not retry).
	if uc.invocations != 1 {
		t.Fatalf("use case invocations=%d, want 1", uc.invocations)
	}

	// The controller must pass the sentinel to c.Error (errorEnvelope relies
	// on *shared.HTTPError). Unwrap via shared.AsHTTPError on the sentinel
	// itself and verify it resolves to the wantStatus — this is a sanity check
	// on the fixture wiring, not a behavior the controller adds.
	if he := shared.AsHTTPError(sentinel); he == nil || he.Code != wantStatus {
		t.Fatalf("test fixture: sentinel does not resolve to an HTTPError with code %d", wantStatus)
	}
}

func TestArxivController_UseCaseInvokedOnce(t *testing.T) {
	t.Parallel()

	uc := &fakeOutcomeFetcher{returnOutcomes: []arxivapp.FetchedEntry{}}
	clock := fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)}
	ctrl := NewArxivController(uc, clock)

	req := httptest.NewRequest(http.MethodGet, "/api/arxiv/fetch", nil)
	w := httptest.NewRecorder()
	newEngine(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	if uc.invocations != 1 {
		t.Fatalf("use case invocations=%d, want 1", uc.invocations)
	}
}
