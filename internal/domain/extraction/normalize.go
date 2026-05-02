package extraction

import (
	"regexp"
	"strings"
)

// Pre-compiled regexes shared by every Normalize call. Each rewrites or strips
// one source-of-noise that would otherwise pollute the LLM-bound markdown:
// unifying math delimiters keeps formula tokens parseable downstream; stripping
// references/tables/images/captions removes content that is either citation
// boilerplate or non-textual blocks that LLMs cannot consume; collapsing
// blank-line runs hides the holes those strips leave behind. Package-level
// vars are safe to reuse since regexes carry no per-call state.
var (
	// MinerU emits inline math as `\(...\)`; downstream renderers expect the
	// `$...$` form, so this rewrite normalises every extractor to one shape.
	// Non-greedy dot-all lets us cross newlines safely even though inline math
	// is conventionally single-line.
	reMathInlineAlt = regexp.MustCompile(`(?s)\\\(([\s\S]+?)\\\)`)

	// Same rationale as inline math, but for display blocks (`\[...\]` →
	// `$$...$$`). Multi-line display formulas are common, so dot-all matters.
	reMathDisplayAlt = regexp.MustCompile(`(?s)\\\[([\s\S]+?)\\\]`)

	// References / bibliography / works-cited tails are dropped because they
	// are low-signal citation lists that bloat word count and dilute the
	// summarisation context; matching only the heading line and truncating
	// keeps the body intact up to that boundary. Heading levels 1-6 all
	// qualify; the heading text must be exact (no "References Cited").
	reReferencesHeading = regexp.MustCompile(`(?i)^#{1,6}\s+(references|bibliography|works cited)\s*$`)

	// Tables are stripped because GFM rows decompose into pipe-delimited prose
	// that is unreadable as text. The separator row (dashes with optional
	// alignment colons) is the unambiguous marker that the preceding `|` line
	// is a real table header rather than prose that happens to use pipes.
	reTableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$`)

	// Stand-alone image lines are dropped because the LLM pipeline is
	// text-only and `![alt](url)` carries no extractable signal. Inline
	// images embedded in prose are intentionally preserved.
	reImageLine = regexp.MustCompile(`^!\[.*\]\(.*\)\s*$`)

	// Stripping tables / images / captions leaves blank-line holes; collapsing
	// runs of three or more newlines into two preserves paragraph separation
	// without leaking the gap structure to the consumer.
	reMultiBlank = regexp.MustCompile(`\n{3,}`)
)

// Normalize applies the project's normalization contract to raw markdown
// emitted by an Extractor: math delimiter unification, references-tail
// truncation, table / image / figure-caption stripping, whitespace word
// count, and title selection. Pure — no I/O, no time, no random — so the
// output is fully deterministic for a given input.
//
// fallbackTitle is used as the artifact title when the normalized body has
// no level-1 (#) heading. Callers typically pass the source PDF basename.
func Normalize(markdown, fallbackTitle string) NormalizedArtifact {
	body := rewriteMathDelimiters(markdown)
	body = truncateAtReferences(body)
	body = stripBlocks(body)
	body = collapseBlankRuns(body)
	body = strings.TrimRight(body, "\n")

	title := selectTitle(body, fallbackTitle)
	wordCount := len(strings.Fields(body))

	return NormalizedArtifact{
		Title:        title,
		BodyMarkdown: body,
		WordCount:    wordCount,
	}
}

// rewriteMathDelimiters unifies TeX-style `\(...\)` and `\[...\]` math fences
// into the `$...$` / `$$...$$` form the rest of the pipeline assumes. Default
// `$`-fenced math is already in canonical form and untouched.
func rewriteMathDelimiters(in string) string {
	out := reMathDisplayAlt.ReplaceAllString(in, "$$$$$1$$$$")
	out = reMathInlineAlt.ReplaceAllString(out, "$$$1$$")
	return out
}

// truncateAtReferences drops the first references / bibliography / works-cited
// heading and every line after it. The match runs on the trimmed line so
// indentation in front of the heading is tolerated, but the heading text
// itself must be exact (case-insensitive).
func truncateAtReferences(in string) string {
	lines := strings.Split(in, "\n")
	for i, line := range lines {
		if reReferencesHeading.MatchString(strings.TrimSpace(line)) {
			return strings.Join(lines[:i], "\n")
		}
	}
	return in
}

// stripBlocks removes GFM tables, stand-alone image lines, and figure
// captions in a single line-oriented pass. Surrounding paragraphs are left
// untouched; later whitespace collapse closes the gaps.
func stripBlocks(in string) string {
	lines := strings.Split(in, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// GFM table: a `|`-prefixed header line whose successor is a
		// dash-cell separator row. Drop the entire contiguous run of
		// pipe-prefixed lines.
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && reTableSeparator.MatchString(lines[i+1]) {
			j := i + 2
			for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
				j++
			}
			i = j - 1
			continue
		}

		// Stand-alone image line.
		if reImageLine.MatchString(trimmed) {
			continue
		}

		// Figure caption. The trailing space after the prefix is required
		// so unrelated words ("Figureheads", "Figment") are not dropped.
		if strings.HasPrefix(trimmed, "Figure ") || strings.HasPrefix(trimmed, "Fig. ") {
			continue
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// collapseBlankRuns rewrites any run of three or more newlines into exactly
// two so paragraph structure stays predictable after block stripping.
func collapseBlankRuns(in string) string {
	return reMultiBlank.ReplaceAllString(in, "\n\n")
}

// selectTitle returns the trimmed text of the first level-1 heading in the
// post-normalized body, or fallbackTitle when no such heading exists. Only
// `# ` (single hash + mandatory space) at the start of a line qualifies —
// `## Subtitle` is intentionally ignored.
func selectTitle(body, fallbackTitle string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallbackTitle
}
