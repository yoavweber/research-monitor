package paper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domainpaper "github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRepo is an inline fake for paper.Repository scoped to the read paths
// this controller exercises. Save is unused by the controller but must be
// present to satisfy the interface; calling it from a test is a programmer
// error and the helper panics so we notice immediately.
type fakeRepo struct {
	findByKeyEntry *domainpaper.Entry
	findByKeyErr   error
	findByKeyCalls int
	lastSource     string
	lastSourceID   string

	listEntries []domainpaper.Entry
	listErr     error
	listCalls   int
}

func (f *fakeRepo) Save(_ context.Context, _ domainpaper.Entry) (bool, error) {
	panic("fakeRepo.Save must not be called by the read-only controller paths")
}

func (f *fakeRepo) FindByKey(_ context.Context, source, sourceID string) (*domainpaper.Entry, error) {
	f.findByKeyCalls++
	f.lastSource = source
	f.lastSourceID = sourceID
	return f.findByKeyEntry, f.findByKeyErr
}

func (f *fakeRepo) List(_ context.Context) ([]domainpaper.Entry, error) {
	f.listCalls++
	return f.listEntries, f.listErr
}

// fixedClock implements shared.Clock with a pre-set time. The controller does
// not consult it today, but injecting it keeps the constructor symmetric with
// the arxiv controller.
type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

// newEngine wires an in-memory Gin engine with the error envelope middleware
// so sentinel-translation tests see the same rendering as production.
func newEngine(ctrl *PaperController) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.ErrorEnvelope())
	engine.GET("/api/papers/:source/:source_id", ctrl.Get)
	engine.GET("/api/papers", ctrl.List)
	return engine
}

func sampleEntry() domainpaper.Entry {
	submitted := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 10, 2, 11, 0, 0, 0, time.UTC)
	return domainpaper.Entry{
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

func newController(repo domainpaper.Repository) *PaperController {
	return NewPaperController(repo, fixedClock{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)})
}

func TestPaperController_Get_Hit_ReturnsAllTwelveFields(t *testing.T) {
	t.Parallel()

	entry := sampleEntry()
	repo := &fakeRepo{findByKeyEntry: &entry}

	req := httptest.NewRequest(http.MethodGet, "/api/papers/arxiv/2404.12345", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	if repo.findByKeyCalls != 1 {
		t.Fatalf("FindByKey calls=%d, want 1", repo.findByKeyCalls)
	}
	if repo.lastSource != "arxiv" || repo.lastSourceID != "2404.12345" {
		t.Fatalf("FindByKey called with (%q, %q), want (arxiv, 2404.12345)", repo.lastSource, repo.lastSourceID)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not JSON: %v; raw=%s", err, w.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing or wrong type; body=%v", body)
	}

	// All 12 wire fields must be present and carry the expected values. We
	// drive this via a table so an accidental field rename surfaces as a
	// localised failure, not a wall of unrelated assertions.
	wantStrings := map[string]string{
		"source":           "arxiv",
		"source_id":        "2404.12345",
		"version":          "v1",
		"title":            "A sample paper",
		"abstract":         "an abstract",
		"primary_category": "cs.LG",
		"pdf_url":          "http://arxiv.org/pdf/2404.12345v1",
		"abs_url":          "http://arxiv.org/abs/2404.12345v1",
	}
	for k, want := range wantStrings {
		if got, _ := data[k].(string); got != want {
			t.Fatalf("data[%q]=%v, want %q", k, data[k], want)
		}
	}

	authors, ok := data["authors"].([]any)
	if !ok || len(authors) != 2 || authors[0] != "A. Author" || authors[1] != "B. Author" {
		t.Fatalf("data.authors=%v, want [A. Author B. Author]", data["authors"])
	}
	cats, ok := data["categories"].([]any)
	if !ok || len(cats) != 2 || cats[0] != "cs.LG" || cats[1] != "stat.ML" {
		t.Fatalf("data.categories=%v", data["categories"])
	}

	for _, k := range []string{"submitted_at", "updated_at"} {
		raw, ok := data[k].(string)
		if !ok {
			t.Fatalf("data[%q] missing or not string; data=%v", k, data)
		}
		if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
			t.Fatalf("data[%q] not RFC3339: %v (raw=%q)", k, err, raw)
		}
	}
}

func TestPaperController_Get_NotFound_Returns404Envelope(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{findByKeyErr: domainpaper.ErrNotFound}

	req := httptest.NewRequest(http.MethodGet, "/api/papers/arxiv/missing", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusNotFound)
}

func TestPaperController_Get_CatalogueUnavailable_Returns500Envelope(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{findByKeyErr: domainpaper.ErrCatalogueUnavailable}

	req := httptest.NewRequest(http.MethodGet, "/api/papers/arxiv/anything", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusInternalServerError)
}

func TestPaperController_List_TwoEntries_PreservesRepoOrder(t *testing.T) {
	t.Parallel()

	first := sampleEntry()
	second := sampleEntry()
	second.SourceID = "2410.99999"
	second.Title = "Second paper"

	repo := &fakeRepo{listEntries: []domainpaper.Entry{first, second}}

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	if repo.listCalls != 1 {
		t.Fatalf("List calls=%d, want 1", repo.listCalls)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data missing or wrong type; body=%v", body)
	}

	if got, _ := data["count"].(float64); int(got) != 2 {
		t.Fatalf("data.count=%v, want 2", data["count"])
	}
	papers, ok := data["papers"].([]any)
	if !ok || len(papers) != 2 {
		t.Fatalf("data.papers length=%d, want 2; raw=%v", len(papers), data["papers"])
	}

	firstWire := papers[0].(map[string]any)
	secondWire := papers[1].(map[string]any)
	if firstWire["source_id"] != "2404.12345" {
		t.Fatalf("papers[0].source_id=%v, want 2404.12345 (repo order must be preserved)", firstWire["source_id"])
	}
	if secondWire["source_id"] != "2410.99999" {
		t.Fatalf("papers[1].source_id=%v, want 2410.99999 (repo order must be preserved)", secondWire["source_id"])
	}
}

func TestPaperController_List_Empty_Returns_NonNull_EmptyArray(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{listEntries: []domainpaper.Entry{}}

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Raw-substring check: the wire must contain "papers":[] not "papers":null.
	// JSON map decoding hides the null-vs-empty distinction, hence the string
	// match — this is the explicit guard against a nil-vs-empty regression.
	raw := w.Body.String()
	if !strings.Contains(raw, `"papers":[]`) {
		t.Fatalf(`response body must contain "papers":[] (non-null empty array); body=%s`, raw)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data := body["data"].(map[string]any)
	if got, _ := data["count"].(float64); int(got) != 0 {
		t.Fatalf("data.count=%v, want 0", data["count"])
	}
}

func TestPaperController_List_CatalogueUnavailable_Returns500Envelope(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{listErr: domainpaper.ErrCatalogueUnavailable}

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()
	newEngine(newController(repo)).ServeHTTP(w, req)

	assertErrorEnvelope(t, w, http.StatusInternalServerError)
}

// assertErrorEnvelope verifies the response is a well-formed error envelope at
// the expected status code and that no `data` field leaked through alongside it.
func assertErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

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
	if _, present := body["data"]; present {
		t.Fatalf("error response must not carry data; body=%v", body)
	}
}
