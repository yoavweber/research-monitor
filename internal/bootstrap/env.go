package bootstrap

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Env struct {
	AppEnv             string   `mapstructure:"APP_ENV"`
	HTTPPort           int      `mapstructure:"HTTP_PORT"`
	APIToken           string   `mapstructure:"API_TOKEN"`
	SQLitePath         string   `mapstructure:"SQLITE_PATH"`
	AnthropicAPIKey    string   `mapstructure:"ANTHROPIC_API_KEY"`
	AnthropicModel     string   `mapstructure:"ANTHROPIC_MODEL"`
	ArxivBaseURL       string   `mapstructure:"ARXIV_BASE_URL"`
	ArxivCategoriesRaw string   `mapstructure:"ARXIV_CATEGORIES"`
	ArxivCategories    []string `mapstructure:"-"` // populated post-Unmarshal
	ArxivMaxResults    int      `mapstructure:"ARXIV_MAX_RESULTS"`
}

func LoadEnv() (*Env, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // .env is optional; ignore missing
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// defaults
	v.SetDefault("APP_ENV", "dev")
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("SQLITE_PATH", "./data/app.db")
	v.SetDefault("ANTHROPIC_MODEL", "claude-haiku-4-5-20251001")
	v.SetDefault("ARXIV_BASE_URL", "https://export.arxiv.org/api/query")

	// BindEnv forces each struct-tagged key into AllSettings so Unmarshal
	// observes it even when no .env file and no default exists. Without this,
	// viper's AutomaticEnv only surfaces values via explicit Get calls.
	for _, key := range []string{
		"APP_ENV",
		"HTTP_PORT",
		"API_TOKEN",
		"SQLITE_PATH",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_MODEL",
		"ARXIV_BASE_URL",
		"ARXIV_CATEGORIES",
		"ARXIV_MAX_RESULTS",
	} {
		_ = v.BindEnv(key)
	}

	var env Env
	if err := v.Unmarshal(&env); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}

	if env.APIToken == "" {
		return nil, fmt.Errorf("API_TOKEN is required")
	}
	if env.SQLitePath == "" {
		return nil, fmt.Errorf("SQLITE_PATH is required")
	}

	// Parse and validate the arxiv category CSV. Tolerate extra whitespace and
	// trailing commas, but reject configurations that leave us with zero
	// usable categories (requirement 2.2).
	env.ArxivCategories = parseCategories(env.ArxivCategoriesRaw)
	if len(env.ArxivCategories) == 0 {
		return nil, fmt.Errorf("ARXIV_CATEGORIES is required and must contain at least one non-empty category")
	}

	// arXiv's API caps a single query at 30000 results; anything outside
	// [1, 30000] is a misconfiguration we must refuse at startup (requirement 3.3).
	if env.ArxivMaxResults < 1 || env.ArxivMaxResults > 30000 {
		return nil, fmt.Errorf("ARXIV_MAX_RESULTS must be between 1 and 30000 (got %d)", env.ArxivMaxResults)
	}

	return &env, nil
}

func parseCategories(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
