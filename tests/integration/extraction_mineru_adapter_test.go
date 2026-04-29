//go:build mineru

package integration_test

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

// TestMineruAdapter exercises the real MinerU CLI against the committed DeFi
// paper fixture. It is opt-in via the `mineru` build tag because the CLI is a
// heavy external dependency (model weights, multi-minute cold start) that the
// default `task test` run cannot assume is installed.
//
// The test deliberately does not call t.Parallel(): MinerU is resource-heavy
// (CPU, memory, model files on disk) and concurrent invocations conflict.
//
// The markdown body is logged unconditionally before any assertion so the
// operator can inspect what MinerU actually produces. Assertions verify the
// adapter passes through MinerU's raw output (the normalizer in Task 3.1 is
// what strips references / images / figure captions; the adapter is a
// pass-through). Post-normalize behavior is the e2e test's responsibility.
func TestMineruAdapter(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	pdfPath := filepath.Join(wd, "testdata", "amm_arbitrage_with_fees.pdf")
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

	// Log the markdown BEFORE any assertion so the operator sees the output
	// even when assertions fail. This is the headline observable for Task 2.3.
	t.Logf("=== MinerU Markdown Output ===\n%s\n=== End Output ===", output.Markdown)

	if err != nil {
		t.Fatalf("MinerU adapter Extract returned error: %v (markdown length=%d)", err, len(output.Markdown))
	}

	// Adapter-level assertions: MinerU emits the artifacts the design's
	// normalizer rules need to operate on. Each assertion uses t.Errorf so a
	// single run surfaces every gap. Sample-verified against the AMM paper
	// fixture in Task 2.3; figure-caption / reference-section assertions live
	// in the e2e test (Task 5.2) where the normalized body is observable.
	if output.Markdown == "" {
		t.Errorf("expected non-empty markdown body, got empty string")
	}

	mathPattern := regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$]+\$`)
	if !mathPattern.MatchString(output.Markdown) {
		t.Errorf("expected at least one $...$ inline or $$...$$ display math span, found none")
	}

	if !strings.Contains(output.Markdown, "# ") {
		t.Errorf("expected at least one '# ' level-1 heading marker, found none")
	}

	if !regexp.MustCompile(`(?im)^#+\s*(references|bibliography|works cited)\s*$`).MatchString(output.Markdown) {
		t.Errorf("expected a references/bibliography/works-cited heading for the normalizer to truncate")
	}
}
