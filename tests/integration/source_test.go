//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

func TestSources_CreateThenList(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	body, _ := json.Marshal(map[string]any{
		"name": "Uniswap", "kind": "rss", "url": "https://uniswap.org/blog/rss.xml",
	})
	resp := doAuthenticatedPost(t, env.Server.URL+"/api/sources", string(body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d want 201", resp.StatusCode)
	}

	list := doAuthenticatedGet(t, env.Server.URL+"/api/sources")
	defer list.Body.Close()

	var listBody struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listBody.Data) != 1 {
		t.Errorf("len=%d want 1", len(listBody.Data))
	}
}

// security
func TestSources_Unauthorized(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()
	resp, _ := http.Get(env.Server.URL + "/api/sources")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d want 401", resp.StatusCode)
	}
}
