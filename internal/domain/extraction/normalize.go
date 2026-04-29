package extraction

import (
	"regexp"
	"strings"
)

// Pre-compiled regexes shared by every Normalize call. These never carry
// per-call state, so package-level reuse is safe and avoids the parse cost on
// the hot path.
var (
	// reMathInlineAlt rewrites `\(...\)` (TeX inline form) into `$...$`. The
	// non-greedy `[\s\S]+?` body matches across newlines for safety even though
	// inline math is conventionally single-line.
	reMathInlineAlt = regexp.MustCompile(`(?s)\\\(([\s\S]+?)\\\)`)

	// reMathDisplayAlt rewrites `\[...\]` (TeX display form) into `$$...$$`.
	// Non-greedy and dot-all so multi-line display blocks survive verbatim.
	reMathDisplayAlt = regexp.MustCompile(`(?s)\\\[([\s\S]+?)\\\]`)

	// reReferencesHeading matches the trimmed text of a markdown heading line
	// that starts a references / bibliography / works-cited tail. Heading
	// levels 1-6 (`#` through `######`) all qualify; the heading text must be
	// exact (no decorations like "References Cited").
	reReferencesHeading = regexp.MustCompile(`(?i)^#{1,6}\s+(references|bibliography|works cited)\s*$`)

	// reTableSeparator matches a GFM table separator row whose cells are
	// composed of dashes (with optional alignment colons and inner spacing).
	// The presence of such a row immediately after a `|`-prefixed header is
	// what disambiguates a real table from prose that happens to use pipes.
	reTableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$`)

	// reImageLine matches a markdown image-only line (`![alt](url)`) with
	// optional trailing whitespace. Inline images embedded in prose are not
	// dropped — only stand-alone image lines.
	reImageLine = regexp.MustCompile(`^!\[.*\]\(.*\)\s*$`)

	// reMultiBlank collapses any run of three or more consecutive blank lines
	// into exactly two newlines, preserving paragraph separation while killing
	// the holes left by stripped tables / images / captions.
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
