//go:build manual

package manual_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/extraction/mineru"
)

// TestMineruAdapter runs the real MinerU CLI against a sample arXiv paper.
// It's slow (multi-minute cold start) and gated behind the `manual` build
// tag, so it only runs when you invoke it by hand.
//
// Each run overwrites testdata/amm_arbitrage_with_fees.md; the git diff of
// that file across runs is how we eyeball MinerU drift between versions.
// Assertions check structural shape only (math, headings, references) —
// byte-exact output isn't stable enough to assert against.
func TestMineruAdapter(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Fixture is shared with the integration-tagged e2e test; reference it in
	// place rather than duplicating the ~860KB PDF in tests/manual/testdata.
	pdfPath := filepath.Join(wd, "..", "integration", "testdata", "amm_arbitrage_with_fees.pdf")
	if _, statErr := os.Stat(pdfPath); statErr != nil {
		// Distinguish "fixture moved" from "MinerU broken" — skip rather than
		// fail so the operator gets an unambiguous signal.
		if errors.Is(statErr, os.ErrNotExist) {
			t.Skipf("fixture missing at %s; restore it before running this test", pdfPath)
		}
		t.Fatalf("stat fixture %s: %v", pdfPath, statErr)
	}

	mineruPath := os.Getenv("MINERU_PATH")
	if mineruPath == "" {
		mineruPath = "mineru"
	}

	// Match the e2e test deadline: cold-start model loading can dominate the
	// first invocation, so 5 minutes is the realistic upper bound.
	const callTimeout = 5 * time.Minute
	var ext extraction.Extractor = mineru.NewMineruExtractor(mineruPath, callTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	output, err := ext.Extract(ctx, extraction.ExtractInput{
		PDFPath:    pdfPath,
		SourceType: "paper",
		SourceID:   "amm-arbitrage-fees",
	})
	if err != nil {
		t.Fatalf("MinerU adapter Extract returned error: %v (markdown length=%d)", err, len(output.Markdown))
	}

	// Persist the markdown beside the PDF fixture so PR diffs surface MinerU
	// output drift. Overwrite on every run; the operator commits intentional
	// shifts. MkdirAll handles a fresh checkout where testdata/ has not been
	// created yet.
	outputDir := filepath.Join(wd, "testdata")
	if mkErr := os.MkdirAll(outputDir, 0o755); mkErr != nil {
		t.Fatalf("create %s: %v", outputDir, mkErr)
	}
	outputPath := filepath.Join(outputDir, "amm_arbitrage_with_fees.md")
	if writeErr := os.WriteFile(outputPath, []byte(output.Markdown), 0o644); writeErr != nil {
		t.Fatalf("write markdown to %s: %v", outputPath, writeErr)
	}
	t.Logf("wrote markdown to %s (length=%d)", outputPath, len(output.Markdown))

	// Adapter-level assertions: MinerU emits the artifacts the design's
	// normalizer rules need to operate on. Each assertion uses t.Errorf so a
	// single run surfaces every gap. Sample-verified against the AMM paper
	// fixture in Task 2.3; figure-caption / reference-section assertions live
	// in the e2e test (Task 5.2) where the normalized body is observable.
	body, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read back %s: %v", outputPath, readErr)
	}
	markdown := string(body)

	if markdown == "" {
		t.Errorf("expected non-empty markdown body, got empty string")
	}

	mathPattern := regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$]+\$`)
	if !mathPattern.MatchString(markdown) {
		t.Errorf("expected at least one $...$ inline or $$...$$ display math span, found none")
	}

	if !strings.Contains(markdown, "# ") {
		t.Errorf("expected at least one '# ' level-1 heading marker, found none")
	}

	if !regexp.MustCompile(`(?im)^#+\s*(references|bibliography|works cited)\s*$`).MatchString(markdown) {
		t.Errorf("expected a references/bibliography/works-cited heading for the normalizer to truncate")
	}
}
