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

	// Durations parse post-Unmarshal so a malformed value fails fast with the
	// offending env-var name; viper would otherwise silently coerce to zero.
	ExtractionMaxWords     int           `mapstructure:"EXTRACTION_MAX_WORDS"`
	ExtractionSignalBuffer int           `mapstructure:"EXTRACTION_SIGNAL_BUFFER"`
	ExtractionJobExpiryRaw string        `mapstructure:"EXTRACTION_JOB_EXPIRY"`
	ExtractionJobExpiry    time.Duration `mapstructure:"-"`
	MineruPath             string        `mapstructure:"MINERU_PATH"`
	MineruTimeoutRaw       string        `mapstructure:"MINERU_TIMEOUT"`
	MineruTimeout          time.Duration `mapstructure:"-"`
	PDFStoreRoot           string        `mapstructure:"PDF_STORE_ROOT"`
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
	v.SetDefault("PDF_STORE_ROOT", "data/pdfs")

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
		"PDF_STORE_ROOT",
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

	if err := requirePositiveInt("EXTRACTION_MAX_WORDS", env.ExtractionMaxWords); err != nil {
		return nil, err
	}
	if err := requirePositiveInt("EXTRACTION_SIGNAL_BUFFER", env.ExtractionSignalBuffer); err != nil {
		return nil, err
	}

	jobExpiry, err := parsePositiveDuration("EXTRACTION_JOB_EXPIRY", env.ExtractionJobExpiryRaw)
	if err != nil {
		return nil, err
	}
	env.ExtractionJobExpiry = jobExpiry

	// Viper substitutes the MINERU_PATH default when the env var is unset,
	// but a literal `MINERU_PATH=` from the operator is a misconfiguration
	// we want to refuse rather than silently paper over with the default.
	if rawPath, present := os.LookupEnv("MINERU_PATH"); present && rawPath == "" {
		return nil, fmt.Errorf("MINERU_PATH is required and must be a non-empty executable name or path")
	}
	if strings.TrimSpace(env.MineruPath) == "" {
		return nil, fmt.Errorf("MINERU_PATH is required and must be a non-empty executable name or path")
	}

	mineruTimeout, err := parsePositiveDuration("MINERU_TIMEOUT", env.MineruTimeoutRaw)
	if err != nil {
		return nil, err
	}
	env.MineruTimeout = mineruTimeout

	if err := validatePDFStoreRoot(env.PDFStoreRoot); err != nil {
		return nil, err
	}

	return &env, nil
}

// validatePDFStoreRoot rejects obviously-broken values for PDF_STORE_ROOT
// before NewApp tries to construct the rest of the App. The store
// constructor owns directory creation and the writability probe; here we
// only fail-fast on inputs that would never produce a usable store.
func validatePDFStoreRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("PDF_STORE_ROOT is required and must be a non-empty path")
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("PDF_STORE_ROOT %q cannot be inspected: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("PDF_STORE_ROOT %q exists but is not a directory", root)
	}
	return nil
}

func requirePositiveInt(name string, v int) error {
	if v <= 0 {
		return fmt.Errorf("%s must be positive (got %d)", name, v)
	}
	return nil
}

func parsePositiveDuration(name, raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid Go duration (got %q): %w", name, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration (got %s)", name, d)
	}
	return d, nil
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
