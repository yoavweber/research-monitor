package bootstrap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These tests all mutate process-level environment via t.Setenv, which Go's
// testing package refuses to combine with t.Parallel (env is process-global).
// They therefore run serially within this file; other packages still parallelise.

// setRequiredEnv wires the pre-existing required env vars (API_TOKEN, SQLITE_PATH)
// so tests can focus on the arxiv-specific fields. Each caller must still set the
// ARXIV_* values it wants to exercise.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("API_TOKEN", "test-token")
	t.Setenv("SQLITE_PATH", "./data/test.db")
	// Provide a baseline arxiv config so tests that target the new
	// extraction block don't trip the unrelated arxiv validators first.
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "10")
}

func TestLoadEnv_HappyPath(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_BASE_URL", "https://example.test/api/query")
	t.Setenv("ARXIV_CATEGORIES", "cs.LG,q-fin.ST")
	t.Setenv("ARXIV_MAX_RESULTS", "100")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	want := []string{"cs.LG", "q-fin.ST"}
	if !reflect.DeepEqual(env.ArxivCategories, want) {
		t.Errorf("ArxivCategories = %#v, want %#v", env.ArxivCategories, want)
	}
	if env.ArxivMaxResults != 100 {
		t.Errorf("ArxivMaxResults = %d, want 100", env.ArxivMaxResults)
	}
	if env.ArxivBaseURL != "https://example.test/api/query" {
		t.Errorf("ArxivBaseURL = %q, want the value from env", env.ArxivBaseURL)
	}
}

func TestLoadEnv_DefaultArxivBaseURL(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_BASE_URL", "")
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "50")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	if env.ArxivBaseURL != "https://export.arxiv.org/api/query" {
		t.Errorf("ArxivBaseURL = %q, want the default", env.ArxivBaseURL)
	}
}

func TestLoadEnv_CategoriesTrimWhitespaceAndDropEmpties(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", " cs.LG , q-fin.ST , ")
	t.Setenv("ARXIV_MAX_RESULTS", "10")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	want := []string{"cs.LG", "q-fin.ST"}
	if !reflect.DeepEqual(env.ArxivCategories, want) {
		t.Errorf("ArxivCategories = %#v, want %#v", env.ArxivCategories, want)
	}
}

func TestLoadEnv_CategoriesEmptyRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "")
	t.Setenv("ARXIV_MAX_RESULTS", "10")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for empty ARXIV_CATEGORIES")
	}
	if !strings.Contains(err.Error(), "ARXIV_CATEGORIES") {
		t.Errorf("error %q does not mention ARXIV_CATEGORIES", err.Error())
	}
}

func TestLoadEnv_CategoriesWhitespaceOnlyRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "  ,  ,")
	t.Setenv("ARXIV_MAX_RESULTS", "10")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for whitespace-only ARXIV_CATEGORIES")
	}
	if !strings.Contains(err.Error(), "ARXIV_CATEGORIES") {
		t.Errorf("error %q does not mention ARXIV_CATEGORIES", err.Error())
	}
}

func TestLoadEnv_MaxResultsZeroRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "0")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for ARXIV_MAX_RESULTS=0")
	}
	if !strings.Contains(err.Error(), "ARXIV_MAX_RESULTS") {
		t.Errorf("error %q does not mention ARXIV_MAX_RESULTS", err.Error())
	}
}

func TestLoadEnv_MaxResultsNegativeRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "-1")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for ARXIV_MAX_RESULTS=-1")
	}
	if !strings.Contains(err.Error(), "ARXIV_MAX_RESULTS") {
		t.Errorf("error %q does not mention ARXIV_MAX_RESULTS", err.Error())
	}
}

func TestLoadEnv_MaxResultsOverLimitRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "30001")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for ARXIV_MAX_RESULTS=30001")
	}
	if !strings.Contains(err.Error(), "ARXIV_MAX_RESULTS") {
		t.Errorf("error %q does not mention ARXIV_MAX_RESULTS", err.Error())
	}
}

func TestLoadEnv_MaxResultsNonNumericRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("ARXIV_CATEGORIES", "cs.LG")
	t.Setenv("ARXIV_MAX_RESULTS", "abc")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for non-numeric ARXIV_MAX_RESULTS")
	}
}

// --- extraction config block ---------------------------------------------
//
// Covers requirements 4.4 (operator-configurable max_words gate), 5.2
// (operator-configurable job_expiry, default 1h), and 6.6 (fail-fast startup
// when configuration cannot be initialised). The contract mirrors the existing
// arxiv block: defaults applied when env is absent, explicit values picked up,
// and rejection paths return a wrapped error without leaking a partially-built
// Env.

func TestLoadEnv_ExtractionDefaults(t *testing.T) {

	setRequiredEnv(t)

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	if env.ExtractionMaxWords != 50000 {
		t.Errorf("ExtractionMaxWords = %d, want 50000", env.ExtractionMaxWords)
	}
	if env.ExtractionSignalBuffer != 10 {
		t.Errorf("ExtractionSignalBuffer = %d, want 10", env.ExtractionSignalBuffer)
	}
	if env.ExtractionJobExpiry != time.Hour {
		t.Errorf("ExtractionJobExpiry = %s, want 1h", env.ExtractionJobExpiry)
	}
	if env.MineruPath != "mineru" {
		t.Errorf("MineruPath = %q, want \"mineru\"", env.MineruPath)
	}
	if env.MineruTimeout != 10*time.Minute {
		t.Errorf("MineruTimeout = %s, want 10m", env.MineruTimeout)
	}
}

func TestLoadEnv_ExtractionExplicitValues(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_MAX_WORDS", "12345")
	t.Setenv("EXTRACTION_SIGNAL_BUFFER", "32")
	t.Setenv("EXTRACTION_JOB_EXPIRY", "2h30m")
	t.Setenv("MINERU_PATH", "/usr/local/bin/mineru")
	t.Setenv("MINERU_TIMEOUT", "5m")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	if env.ExtractionMaxWords != 12345 {
		t.Errorf("ExtractionMaxWords = %d, want 12345", env.ExtractionMaxWords)
	}
	if env.ExtractionSignalBuffer != 32 {
		t.Errorf("ExtractionSignalBuffer = %d, want 32", env.ExtractionSignalBuffer)
	}
	if env.ExtractionJobExpiry != 2*time.Hour+30*time.Minute {
		t.Errorf("ExtractionJobExpiry = %s, want 2h30m", env.ExtractionJobExpiry)
	}
	if env.MineruPath != "/usr/local/bin/mineru" {
		t.Errorf("MineruPath = %q, want %q", env.MineruPath, "/usr/local/bin/mineru")
	}
	if env.MineruTimeout != 5*time.Minute {
		t.Errorf("MineruTimeout = %s, want 5m", env.MineruTimeout)
	}
}

func TestLoadEnv_ExtractionMaxWordsZeroRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_MAX_WORDS", "0")

	envPtr, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_MAX_WORDS=0")
	}
	if envPtr != nil {
		t.Errorf("LoadEnv returned non-nil Env on rejection: %+v", envPtr)
	}
	if !strings.Contains(err.Error(), "EXTRACTION_MAX_WORDS") {
		t.Errorf("error %q does not mention EXTRACTION_MAX_WORDS", err.Error())
	}
}

func TestLoadEnv_ExtractionMaxWordsNegativeRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_MAX_WORDS", "-1")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_MAX_WORDS=-1")
	}
	if !strings.Contains(err.Error(), "EXTRACTION_MAX_WORDS") {
		t.Errorf("error %q does not mention EXTRACTION_MAX_WORDS", err.Error())
	}
}

func TestLoadEnv_ExtractionSignalBufferZeroRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_SIGNAL_BUFFER", "0")

	envPtr, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_SIGNAL_BUFFER=0")
	}
	if envPtr != nil {
		t.Errorf("LoadEnv returned non-nil Env on rejection: %+v", envPtr)
	}
	if !strings.Contains(err.Error(), "EXTRACTION_SIGNAL_BUFFER") {
		t.Errorf("error %q does not mention EXTRACTION_SIGNAL_BUFFER", err.Error())
	}
}

func TestLoadEnv_ExtractionSignalBufferNegativeRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_SIGNAL_BUFFER", "-3")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_SIGNAL_BUFFER=-3")
	}
	if !strings.Contains(err.Error(), "EXTRACTION_SIGNAL_BUFFER") {
		t.Errorf("error %q does not mention EXTRACTION_SIGNAL_BUFFER", err.Error())
	}
}

func TestLoadEnv_ExtractionJobExpiryZeroRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_JOB_EXPIRY", "0s")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_JOB_EXPIRY=0s")
	}
	if !strings.Contains(err.Error(), "EXTRACTION_JOB_EXPIRY") {
		t.Errorf("error %q does not mention EXTRACTION_JOB_EXPIRY", err.Error())
	}
}

func TestLoadEnv_ExtractionJobExpiryNegativeRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_JOB_EXPIRY", "-5m")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for EXTRACTION_JOB_EXPIRY=-5m")
	}
	if !strings.Contains(err.Error(), "EXTRACTION_JOB_EXPIRY") {
		t.Errorf("error %q does not mention EXTRACTION_JOB_EXPIRY", err.Error())
	}
}

func TestLoadEnv_ExtractionJobExpiryMalformedRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("EXTRACTION_JOB_EXPIRY", "not-a-duration")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for malformed EXTRACTION_JOB_EXPIRY")
	}
	if !strings.Contains(err.Error(), "EXTRACTION_JOB_EXPIRY") {
		t.Errorf("error %q does not mention EXTRACTION_JOB_EXPIRY", err.Error())
	}
}

func TestLoadEnv_MineruTimeoutZeroRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("MINERU_TIMEOUT", "0s")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for MINERU_TIMEOUT=0s")
	}
	if !strings.Contains(err.Error(), "MINERU_TIMEOUT") {
		t.Errorf("error %q does not mention MINERU_TIMEOUT", err.Error())
	}
}

func TestLoadEnv_MineruTimeoutNegativeRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("MINERU_TIMEOUT", "-1s")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for MINERU_TIMEOUT=-1s")
	}
	if !strings.Contains(err.Error(), "MINERU_TIMEOUT") {
		t.Errorf("error %q does not mention MINERU_TIMEOUT", err.Error())
	}
}

func TestLoadEnv_MineruTimeoutMalformedRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("MINERU_TIMEOUT", "ten-minutes")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for malformed MINERU_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "MINERU_TIMEOUT") {
		t.Errorf("error %q does not mention MINERU_TIMEOUT", err.Error())
	}
}

func TestLoadEnv_MineruPathEmptyRejected(t *testing.T) {

	setRequiredEnv(t)
	t.Setenv("MINERU_PATH", "")

	_, err := LoadEnv()
	if err == nil {
		t.Fatal("LoadEnv returned nil error for empty MINERU_PATH")
	}
	if !strings.Contains(err.Error(), "MINERU_PATH") {
		t.Errorf("error %q does not mention MINERU_PATH", err.Error())
	}
}

// --- pdf store root config block -----------------------------------------
//
// Env-side validation only inspects the path with Stat. Directory creation
// and the writability probe live in pdflocal.NewStore, so a missing path is
// accepted here and created lazily later, and an unwritable existing
// directory is caught downstream rather than re-checked twice on every boot.

func TestLoadEnv_PDFStoreRootDefault(t *testing.T) {

	t.Run("unset variable resolves to data/pdfs default", func(t *testing.T) {

		setRequiredEnv(t)

		env, err := LoadEnv()

		if err != nil {
			t.Fatalf("LoadEnv returned error: %v", err)
		}
		if env.PDFStoreRoot != "data/pdfs" {
			t.Errorf("PDFStoreRoot = %q, want %q", env.PDFStoreRoot, "data/pdfs")
		}
	})
}

func TestLoadEnv_PDFStoreRootExistingWritableDirectory(t *testing.T) {

	t.Run("existing writable directory is accepted", func(t *testing.T) {

		setRequiredEnv(t)
		dir := t.TempDir()
		t.Setenv("PDF_STORE_ROOT", dir)

		env, err := LoadEnv()

		if err != nil {
			t.Fatalf("LoadEnv returned error: %v", err)
		}
		if env.PDFStoreRoot != dir {
			t.Errorf("PDFStoreRoot = %q, want %q", env.PDFStoreRoot, dir)
		}
	})
}

func TestLoadEnv_PDFStoreRootMissingPathAccepted(t *testing.T) {

	t.Run("path that does not exist is accepted because the store creates it lazily", func(t *testing.T) {

		setRequiredEnv(t)
		missing := filepath.Join(t.TempDir(), "not-yet-created")
		t.Setenv("PDF_STORE_ROOT", missing)

		env, err := LoadEnv()

		if err != nil {
			t.Fatalf("LoadEnv returned error: %v", err)
		}
		if env.PDFStoreRoot != missing {
			t.Errorf("PDFStoreRoot = %q, want %q", env.PDFStoreRoot, missing)
		}
	})
}

func TestLoadEnv_PDFStoreRootRegularFileRejected(t *testing.T) {

	t.Run("regular file at the configured path fails fast and names PDF_STORE_ROOT", func(t *testing.T) {

		setRequiredEnv(t)
		regularFile := filepath.Join(t.TempDir(), "not-a-dir.pdf")
		if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed regular file: %v", err)
		}
		t.Setenv("PDF_STORE_ROOT", regularFile)

		_, err := LoadEnv()

		if err == nil {
			t.Fatal("LoadEnv returned nil error for PDF_STORE_ROOT pointing at a regular file")
		}
		if !strings.Contains(err.Error(), "PDF_STORE_ROOT") {
			t.Errorf("error %q does not mention PDF_STORE_ROOT", err.Error())
		}
	})
}

