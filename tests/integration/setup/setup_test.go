//go:build integration

package setup_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// TestSetupTestEnv_ZeroOpts_PapersListEmpty proves the harness wires the
// /api/papers read endpoint without any opts: the default-built real repo
// over the temp SQLite DB starts empty, so List must yield 200 + an empty
// (non-null) entries slice. Catches regressions where someone gates
// PaperRouter behind an opt.
func TestSetupTestEnv_ZeroOpts_PapersListEmpty(t *testing.T) {
	t.Parallel()

	env := setup.SetupTestEnv(t)
	defer env.Close()

	if env.PaperRepo == nil {
		t.Fatal("env.PaperRepo is nil; harness must expose the default-built repo")
	}

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/papers", nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// Raw substring assertion: encoding/json marshals a nil slice as null,
	// and the read endpoint contract is an empty array. Catching that here
	// is the same regression guard the arxiv tests use.
	if !strings.Contains(string(raw), `"papers":[]`) {
		t.Fatalf("body missing `\"papers\":[]`; got: %s", string(raw))
	}
}

// TestSetupTestEnv_InjectedFailingRepo_ArxivFetch500 proves the injection
// point reaches the arxiv use case: a fake repo whose Save always returns
// paper.ErrCatalogueUnavailable must surface as a 500 on /api/arxiv/fetch
// (the use case translates persistence failure into the catalogue sentinel).
// This is the harness-level proof of R5.5 — failure-injection works without
// the test having to reach into the use case directly.
func TestSetupTestEnv_InjectedFailingRepo_ArxivFetch500(t *testing.T) {
	t.Parallel()

	failing := &mocks.PaperRepo{SaveDefaultErr: paper.ErrCatalogueUnavailable}
	fetcher := &mocks.PaperFetcher{
		Entries: []paper.Entry{
			{
				SourceID:    "2404.99999",
				Title:       "Persistence Failure Probe",
				Authors:     []string{"Probe"},
				Categories:  []string{"cs.LG"},
				SubmittedAt: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fetcher,
		ArxivQuery:   paper.Query{Categories: []string{"cs.LG"}, MaxResults: 100},
		PaperRepo:    failing,
	})
	defer env.Close()

	if env.PaperRepo != failing {
		t.Fatal("env.PaperRepo did not preserve the injected fake")
	}

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/arxiv/fetch", nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d want 500", resp.StatusCode)
	}

	// Decode the standard envelope so the test fails loudly if the error
	// bubbles up some other shape — the wire contract is part of R5.5.
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body.error missing or wrong type: %#v", body)
	}
	if code, _ := errObj["code"].(float64); int(code) != http.StatusInternalServerError {
		t.Errorf("body.error.code = %v want 500", errObj["code"])
	}

	if len(failing.SaveCalls) == 0 {
		t.Error("failing.SaveCalls is empty; arxiv use case did not reach the injected repo")
	}
}
