// Package mineru is the v1 implementation of extraction.Extractor. It shells
// out to the MinerU CLI's `pipeline` backend, captures combined output, and
// translates exit-time signals into the four-category extractor error
// taxonomy defined in domain/extraction.
package mineru

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// mineruExtractor invokes the MinerU CLI as a subprocess against a single
// PDF. The `-b pipeline` backend is mandatory in v1 (per design Decision:
// MinerU 3.x with the pipeline backend) and is hard-coded into every
// invocation rather than exposed via configuration. State is limited to the
// CLI binary path and the per-call timeout; both are captured at construction
// time so concurrent Extract calls share no mutable state.
type mineruExtractor struct {
	path    string
	timeout time.Duration
}

// NewMineruExtractor returns an extraction.Extractor backed by the MinerU CLI
// at path. timeout is applied per Extract call as a context deadline so a
// hanging subprocess cannot stall the worker indefinitely. The interface
// return type prevents callers from depending on the concrete struct.
func NewMineruExtractor(path string, timeout time.Duration) extraction.Extractor {
	return &mineruExtractor{path: path, timeout: timeout}
}

// Extract runs `mineru -b pipeline -p <pdf> -o <tmpdir>` against in.PDFPath,
// reads the produced markdown bundle, and returns it. Every failure exits
// through one of four typed channels: ctx.Err() on cancellation, ErrScannedPDF
// on no-text input, ErrParseFailed on corrupt input, or ErrExtractorFailure
// as the catch-all. The temp directory is removed on every return path
// including panics.
func (m *mineruExtractor) Extract(ctx context.Context, in extraction.ExtractInput) (extraction.ExtractOutput, error) {
	// Fresh per-call workspace: the bundle name MinerU writes is content-derived,
	// so isolation prevents cross-call collisions and simplifies discovery.
	tmpDir, err := os.MkdirTemp("", "mineru-*")
	if err != nil {
		return extraction.ExtractOutput{}, fmt.Errorf("%w: %v", extraction.ErrExtractorFailure, err)
	}
	defer os.RemoveAll(tmpDir)

	// Per-call timeout derives from the caller's ctx so caller cancellation
	// still wins; CommandContext propagates ctx.Done() to a SIGKILL on the
	// child process.
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// `-b pipeline` is mandatory and precedes `-p` per the CLI contract in
	// design.md (Technology Stack row).
	cmd := exec.CommandContext(ctx, m.path, "-b", "pipeline", "-p", in.PDFPath, "-o", tmpDir)
	combined, runErr := cmd.CombinedOutput()

	// Cancellation is a distinct outcome from a tool failure; the use case
	// must observe ctx.Err() unwrapped to leave the row in running for
	// process-restart recovery.
	if ctx.Err() != nil {
		return extraction.ExtractOutput{}, ctx.Err()
	}

	if runErr != nil {
		return extraction.ExtractOutput{}, classifyRunError(runErr, combined)
	}

	mdPath, err := findBundleMarkdown(tmpDir)
	if err != nil {
		return extraction.ExtractOutput{}, fmt.Errorf("%w: %v", extraction.ErrExtractorFailure, err)
	}

	body, err := os.ReadFile(mdPath)
	if err != nil {
		return extraction.ExtractOutput{}, fmt.Errorf("%w: %v", extraction.ErrExtractorFailure, err)
	}

	// TitleHint is informational; the pipeline backend does not surface a
	// dedicated title field, so the use case's Normalize-derived title wins.
	return extraction.ExtractOutput{Markdown: string(body)}, nil
}

// classifyRunError maps a non-zero MinerU exit (or a launch failure such as
// ENOENT on the binary) to the typed extraction error taxonomy. Classification
// is by case-insensitive substring match against the combined output so the
// adapter is resilient to MinerU phrasing variations across patch versions.
// The order matters: scanned-PDF and parse-failure markers are checked before
// the catch-all so a structurally invalid file is not flattened into a
// generic infrastructure failure.
func classifyRunError(runErr error, combined []byte) error {
	lowered := strings.ToLower(string(combined))
	firstLine := firstNonEmptyLine(combined)

	scannedMarkers := []string{"no extractable text", "image-only", "image only", "scanned"}
	for _, marker := range scannedMarkers {
		if strings.Contains(lowered, marker) {
			return fmt.Errorf("%w: %s", extraction.ErrScannedPDF, firstLine)
		}
	}

	parseMarkers := []string{"parse error", "corrupt", "unable to parse", "invalid pdf"}
	for _, marker := range parseMarkers {
		if strings.Contains(lowered, marker) {
			return fmt.Errorf("%w: %s", extraction.ErrParseFailed, firstLine)
		}
	}

	// Catch-all: covers exec.LookPath ENOENT (binary missing), non-zero exits
	// without recognised markers, and any other run-time failure.
	return fmt.Errorf("%w: %v", extraction.ErrExtractorFailure, runErr)
}

// findBundleMarkdown locates the single `.md` file MinerU's pipeline backend
// writes into the per-call output directory. The bundle subdirectory's name
// is derived from the input filename and may vary across MinerU versions, so
// a recursive walk is the most robust discovery path.
func findBundleMarkdown(tmpDir string) (string, error) {
	var found string
	walkErr := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if found == "" {
		return "", fmt.Errorf("no markdown file produced in bundle")
	}
	return found, nil
}

// firstNonEmptyLine returns the first non-empty line of combined output, used
// as the human-readable suffix when wrapping a sentinel error. An empty input
// yields the empty string so the caller's format string still renders cleanly.
func firstNonEmptyLine(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
