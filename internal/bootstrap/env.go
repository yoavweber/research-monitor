package bootstrap

import (
	"fmt"
	"os"
	"strings"
	"time"

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

	// Extraction service config (requirements 4.4, 5.2, 6.6). Durations are
	// parsed post-Unmarshal from raw strings so we can fail-fast at startup
	// with an env-var-named error rather than letting viper silently coerce
	// invalid input to zero.
	ExtractionMaxWords     int           `mapstructure:"EXTRACTION_MAX_WORDS"`
	ExtractionSignalBuffer int           `mapstructure:"EXTRACTION_SIGNAL_BUFFER"`
	ExtractionJobExpiryRaw string        `mapstructure:"EXTRACTION_JOB_EXPIRY"`
	ExtractionJobExpiry    time.Duration `mapstructure:"-"` // populated post-Unmarshal
	MineruPath             string        `mapstructure:"MINERU_PATH"`
	MineruTimeoutRaw       string        `mapstructure:"MINERU_TIMEOUT"`
	MineruTimeout          time.Duration `mapstructure:"-"` // populated post-Unmarshal
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
	v.SetDefault("EXTRACTION_MAX_WORDS", 50000)
	v.SetDefault("EXTRACTION_SIGNAL_BUFFER", 10)
	v.SetDefault("EXTRACTION_JOB_EXPIRY", "1h")
	v.SetDefault("MINERU_PATH", "mineru")
	v.SetDefault("MINERU_TIMEOUT", "10m")

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
		"EXTRACTION_MAX_WORDS",
		"EXTRACTION_SIGNAL_BUFFER",
		"EXTRACTION_JOB_EXPIRY",
		"MINERU_PATH",
		"MINERU_TIMEOUT",
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

	// Extraction config validation (requirements 4.4, 5.2, 6.6). Each rejection
	// returns a wrapped error before we hand back any *Env, so a misconfigured
	// process never observes a half-built config.
	if env.ExtractionMaxWords <= 0 {
		return nil, fmt.Errorf("EXTRACTION_MAX_WORDS must be positive (got %d)", env.ExtractionMaxWords)
	}
	if env.ExtractionSignalBuffer <= 0 {
		return nil, fmt.Errorf("EXTRACTION_SIGNAL_BUFFER must be positive (got %d)", env.ExtractionSignalBuffer)
	}

	jobExpiry, err := time.ParseDuration(env.ExtractionJobExpiryRaw)
	if err != nil {
		return nil, fmt.Errorf("EXTRACTION_JOB_EXPIRY must be a valid Go duration (got %q): %w", env.ExtractionJobExpiryRaw, err)
	}
	if jobExpiry <= 0 {
		return nil, fmt.Errorf("EXTRACTION_JOB_EXPIRY must be a positive duration (got %s)", jobExpiry)
	}
	env.ExtractionJobExpiry = jobExpiry

	// Reject empty MINERU_PATH. Viper falls back to its default when the env
	// var is unset, so we must check the raw env explicitly: an operator who
	// writes `MINERU_PATH=` is signalling a misconfiguration we refuse rather
	// than silently papering over with the `mineru` default.
	if rawPath, present := os.LookupEnv("MINERU_PATH"); present && rawPath == "" {
		return nil, fmt.Errorf("MINERU_PATH is required and must be a non-empty executable name or path")
	}
	if strings.TrimSpace(env.MineruPath) == "" {
		return nil, fmt.Errorf("MINERU_PATH is required and must be a non-empty executable name or path")
	}

	mineruTimeout, err := time.ParseDuration(env.MineruTimeoutRaw)
	if err != nil {
		return nil, fmt.Errorf("MINERU_TIMEOUT must be a valid Go duration (got %q): %w", env.MineruTimeoutRaw, err)
	}
	if mineruTimeout <= 0 {
		return nil, fmt.Errorf("MINERU_TIMEOUT must be a positive duration (got %s)", mineruTimeout)
	}
	env.MineruTimeout = mineruTimeout

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
