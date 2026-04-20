//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/tests/integration/setup"
	"github.com/yoavweber/defi-monitor-backend/tests/mocks"
)

// TestSetupTestEnv_WiresArxivRoute verifies the integration harness now
// accepts a fake paper.Fetcher and a fixed paper.Query, and mounts the arxiv
// route under the same /api group protected by the APIToken middleware. The
// fake's invocation counter is the observable signal that the wiring works.
func TestSetupTestEnv_WiresArxivRoute(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{
		Entries: []paper.Entry{{SourceID: "2404.12345", Title: "Fake Paper"}},
	}
	query := paper.Query{Categories: []string{"cs.LG"}, MaxResults: 5}

	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   query,
	})
	defer env.Close()

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/arxiv/fetch", nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}

	// Body must decode as an envelope; we only care that the request landed
	// in the fake, so we don't assert the full shape here.
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if fake.Invocations != 1 {
		t.Fatalf("fake.Invocations = %d want 1", fake.Invocations)
	}
	if len(fake.Queries) != 1 {
		t.Fatalf("fake.Queries len = %d want 1", len(fake.Queries))
	}
	gotQ := fake.Queries[0]
	if len(gotQ.Categories) != 1 || gotQ.Categories[0] != "cs.LG" || gotQ.MaxResults != 5 {
		t.Fatalf("fake.Queries[0] = %+v want {[cs.LG] 5}", gotQ)
	}
}

// TestSetupTestEnv_ArxivUnauthorized confirms the harness mounts the arxiv
// route under the authenticated /api group: a missing token short-circuits
// with 401 and the fetcher is never invoked.
func TestSetupTestEnv_ArxivUnauthorized(t *testing.T) {
	t.Parallel()

	fake := &mocks.PaperFetcher{}
	env := setup.SetupTestEnv(t, setup.TestEnvOpts{
		ArxivFetcher: fake,
		ArxivQuery:   paper.Query{Categories: []string{"cs.LG"}, MaxResults: 1},
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
