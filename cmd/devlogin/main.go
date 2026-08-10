// Command devlogin is a local-development convenience: it logs in against
// an already-running instance of the API and caches the returned JWT on
// disk so curl/Postman/etc. don't require a manual login round trip on
// every session. It deliberately does NOT call bootstrap.LoadEnv — this
// tool only needs the port the server is listening on, and depending on
// the full env validation (AUTH_JWT_SECRET, SQLITE_PATH, MINERU_PATH, ...)
// would make it fail for reasons unrelated to logging in.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/spf13/viper"
)

// tokenFile is where the access token is cached, relative to the directory
// devlogin is run from (the backend module root, per Taskfile convention).
// Listed in .gitignore — never commit a live token.
const tokenFile = ".dev-token"

const usage = `usage: devlogin <email> <password>

Logs in against the locally running API (task run) and writes the
returned access token to .dev-token. Use it in shell scripts / curl as:

    curl -H "Authorization: Bearer $(cat .dev-token)" http://localhost:8080/api/sources

Re-run devlogin once the token expires (24h by default).
`

func main() {
	if len(os.Args) != 3 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	email, password := os.Args[1], os.Args[2]

	port := httpPort()

	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		log.Fatalf("marshal login request: %v", err)
	}

	url := fmt.Sprintf("http://localhost:%s/auth/login", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("login request to %s (is `task run` running?): %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		log.Fatalf("login failed: status=%d body=%s", resp.StatusCode, raw)
	}

	var out struct {
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresAt   string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Fatalf("decode login response: %v", err)
	}
	if out.Data.AccessToken == "" {
		log.Fatalf("login response had no access_token")
	}

	if err := os.WriteFile(tokenFile, []byte(out.Data.AccessToken), 0o600); err != nil {
		log.Fatalf("write %s: %v", tokenFile, err)
	}
	fmt.Printf("token written to %s (expires %s)\n", tokenFile, out.Data.ExpiresAt)
}

// httpPort resolves HTTP_PORT the same way bootstrap.LoadEnv does (.env
// file, overridable by a real environment variable) without importing
// bootstrap itself — this tool must not fail on unrelated required env
// vars (AUTH_JWT_SECRET, SQLITE_PATH, ...) it has no use for. Defaults to
// 8080, matching env.go's default.
func httpPort() string {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // .env is optional; ignore missing
	v.AutomaticEnv()
	if p := v.GetString("HTTP_PORT"); p != "" {
		return p
	}
	return "8080"
}
