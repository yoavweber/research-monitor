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
// Per the design's "MinerU verification tests" section, the markdown body is
// logged unconditionally before any assertion runs so the operator can inspect
// what MinerU actually produces and ratchet the assertions in Task 2.3. The
// initial assertions below are expected to fail until the normalizer contract
// is locked.
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

	// Initial assertions — expected to fail until Task 2.3 ratchets them.
	// Use t.Errorf (not t.Fatalf) so all assertions run in a single execution
	// and the operator sees every gap between MinerU reality and the design's
	// normalizer contract in one go.
	if output.Markdown == "" {
		t.Errorf("expected non-empty markdown body, got empty string")
	}

	mathPattern := regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$]+\$`)
	if !mathPattern.MatchString(output.Markdown) {
		t.Errorf("expected at least one inline ($...$) or display ($$...$$) math span, found none")
	}

	if !strings.Contains(output.Markdown, "#") {
		t.Errorf("expected at least one '#' heading marker for structure preservation, found none")
	}

	for _, line := range strings.Split(output.Markdown, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "Figure ") || strings.HasPrefix(trimmed, "Fig. ") {
			t.Errorf("expected figure-caption lines to be stripped by normalization, found: %q", line)
			break
		}
	}
}
