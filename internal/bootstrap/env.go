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

	// PDF-download orchestration. Retention is the wall-clock window that
	// completed download jobs remain readable via the status / SSE
	// endpoints before the registry evicts them. SubscriberBuffer is the
	// per-subscriber channel capacity used by the registry's non-blocking
	// fan-out; an SSE client that fails to drain past this depth is
	// dropped rather than blocking the worker.
	PDFDownloadRetentionRaw  string        `mapstructure:"PDF_DOWNLOAD_RETENTION"`
	PDFDownloadRetention     time.Duration `mapstructure:"-"`
	PDFDownloadSubscriberBuf int           `mapstructure:"PDF_DOWNLOAD_SUBSCRIBER_BUFFER"`

	// "anthropic" is reserved but rejected at startup until that adapter
	// ships, so analyzer requests can never silently no-op.
	LLMProvider string `mapstructure:"LLM_PROVIDER"`

	// Auth bootstrap. Secrets are required and must be ≥ 32 bytes and
	// byte-distinct from each other so a forged refresh token cannot pass
	// access-token verification (and vice versa). CookieInsecure flips the
	// refresh cookie's Secure flag off for local non-TLS dev only.
	AccessSecret   string        `mapstructure:"AUTH_ACCESS_SECRET"`
	RefreshSecret  string        `mapstructure:"AUTH_REFRESH_SECRET"`
	AccessTTL      time.Duration `mapstructure:"AUTH_ACCESS_TTL"`
	RefreshTTL     time.Duration `mapstructure:"AUTH_REFRESH_TTL"`
	CookieInsecure bool          `mapstructure:"AUTH_COOKIE_INSECURE"`
	RefreshOrigin  string        `mapstructure:"AUTH_REFRESH_ORIGIN"`
}

// authSecretMinBytes is the minimum byte length we accept for either
// HMAC signing key. 32 bytes matches the HS256 block size and rules out
// the "joined two short literals" misconfiguration that would weaken
// the keyed MAC below the recommended security margin.
const authSecretMinBytes = 32

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
	v.SetDefault("PDF_DOWNLOAD_RETENTION", "5m")
	v.SetDefault("PDF_DOWNLOAD_SUBSCRIBER_BUFFER", 32)
	v.SetDefault("LLM_PROVIDER", "fake")
	v.SetDefault("AUTH_ACCESS_TTL", "15m")
	v.SetDefault("AUTH_REFRESH_TTL", "24h")
	v.SetDefault("AUTH_COOKIE_INSECURE", false)

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
		"PDF_DOWNLOAD_RETENTION",
		"PDF_DOWNLOAD_SUBSCRIBER_BUFFER",
		"LLM_PROVIDER",
		"AUTH_ACCESS_SECRET",
		"AUTH_REFRESH_SECRET",
		"AUTH_ACCESS_TTL",
		"AUTH_REFRESH_TTL",
		"AUTH_COOKIE_INSECURE",
		"AUTH_REFRESH_ORIGIN",
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

	pdfDownloadRetention, err := parsePositiveDuration("PDF_DOWNLOAD_RETENTION", env.PDFDownloadRetentionRaw)
	if err != nil {
		return nil, err
	}
	env.PDFDownloadRetention = pdfDownloadRetention

	if err := requirePositiveInt("PDF_DOWNLOAD_SUBSCRIBER_BUFFER", env.PDFDownloadSubscriberBuf); err != nil {
		return nil, err
	}

	switch env.LLMProvider {
	case "fake":
	case "anthropic":
		return nil, fmt.Errorf("LLM_PROVIDER=anthropic is reserved but not implemented yet; use \"fake\" until the Anthropic adapter ships")
	default:
		return nil, fmt.Errorf("LLM_PROVIDER must be one of [fake, anthropic] (got %q)", env.LLMProvider)
	}

	if err := validateAuthSecrets(env.AccessSecret, env.RefreshSecret); err != nil {
		return nil, err
	}
	if env.RefreshOrigin == "" {
		return nil, fmt.Errorf("AUTH_REFRESH_ORIGIN is required and must match the Origin/Referer the browser will send to /auth/refresh")
	}

	return &env, nil
}

// validateAuthSecrets enforces the three startup rules on the JWT signing
// keys: both must be present, both must be at least 32 bytes, and the two
// must be byte-distinct so a refresh token forged with one key cannot be
// accepted by the other verifier. Each error names the offending variable
// so an operator can fix the misconfiguration without reading the source.
func validateAuthSecrets(access, refresh string) error {
	if access == "" {
		return fmt.Errorf("AUTH_ACCESS_SECRET is required and must be a HMAC key of at least %d bytes", authSecretMinBytes)
	}
	if refresh == "" {
		return fmt.Errorf("AUTH_REFRESH_SECRET is required and must be a HMAC key of at least %d bytes", authSecretMinBytes)
	}
	if len(access) < authSecretMinBytes {
		return fmt.Errorf("AUTH_ACCESS_SECRET is shorter than the %d-byte minimum (got %d bytes)", authSecretMinBytes, len(access))
	}
	if len(refresh) < authSecretMinBytes {
		return fmt.Errorf("AUTH_REFRESH_SECRET is shorter than the %d-byte minimum (got %d bytes)", authSecretMinBytes, len(refresh))
	}
	if access == refresh {
		return fmt.Errorf("AUTH_ACCESS_SECRET and AUTH_REFRESH_SECRET must be distinct so a forged refresh token cannot pass access verification (and vice versa)")
	}
	return nil
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
