//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yoavweber/research-monitor/backend/tests/integration/setup"
)

func postJSON(t *testing.T, url, token string, body any) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func TestSources_CreateThenList(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()

	resp := postJSON(t, env.Server.URL+"/api/sources", setup.TestToken, map[string]any{
		"name": "Uniswap", "kind": "rss", "url": "https://uniswap.org/blog/rss.xml",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d want 201", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, env.Server.URL+"/api/sources", nil)
	req.Header.Set("X-API-Token", setup.TestToken)
	list, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer list.Body.Close()

	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 {
		t.Errorf("len=%d want 1", len(body.Data))
	}
}

func TestSources_Unauthorized(t *testing.T) {
	t.Parallel()
	env := setup.SetupTestEnv(t)
	defer env.Close()
	resp, _ := http.Get(env.Server.URL + "/api/sources")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d want 401", resp.StatusCode)
	}
}
