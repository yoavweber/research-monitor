package bootstrap

import (
	"reflect"
	"strings"
	"testing"
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
