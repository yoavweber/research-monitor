package extraction_test

import (
	"strings"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

func TestNormalizeMathDelimiterUnification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"inline alt form rewrites to single dollar", `\(x+y\)`, `$x+y$`},
		{"display alt form rewrites to double dollar", `\[a=b\]`, `$$a=b$$`},
		{"multi-line display alt form preserves inner content", "\\[\nfoo\n\\]", "$$\nfoo\n$$"},
		{"single dollar default form is preserved", `$x+y$`, `$x+y$`},
		{"double dollar default form is preserved", `$$a=b$$`, `$$a=b$$`},
		{"mixed forms rewrite alt and pass through default", `pre \(a\) mid $b$ end \[c\] tail $$d$$`, `pre $a$ mid $b$ end $$c$$ tail $$d$$`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := extraction.Normalize(tc.in, "fallback")

			if got.BodyMarkdown != tc.want {
				t.Errorf("BodyMarkdown = %q, want %q", got.BodyMarkdown, tc.want)
			}
		})
	}
}

func TestNormalizeReferencesTailTruncation(t *testing.T) {
	t.Parallel()

	t.Run("truncates from level-2 references heading", func(t *testing.T) {
		t.Parallel()

		in := "# Title\n\nbody paragraph\n\n## References\nentry one\nentry two\nentry three\nentry four\nentry five"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "References") {
			t.Errorf("BodyMarkdown still contains References heading: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "entry one") {
			t.Errorf("BodyMarkdown still contains references entry: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "body paragraph") {
			t.Errorf("BodyMarkdown lost pre-references content: %q", got.BodyMarkdown)
		}
	})

	t.Run("truncates from bibliography heading case-insensitively", func(t *testing.T) {
		t.Parallel()

		in := "# Title\n\nkeep me\n\n## bibliography\ndrop me"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "bibliography") {
			t.Errorf("BodyMarkdown still contains bibliography: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "drop me") {
			t.Errorf("BodyMarkdown still contains post-bibliography line: %q", got.BodyMarkdown)
		}
	})

	t.Run("truncates from works cited heading", func(t *testing.T) {
		t.Parallel()

		in := "# Title\n\nkeep me\n\n## Works Cited\ndrop me"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "Works Cited") {
			t.Errorf("BodyMarkdown still contains Works Cited: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "drop me") {
			t.Errorf("BodyMarkdown still contains post-works-cited line: %q", got.BodyMarkdown)
		}
	})

	t.Run("truncates from level-1 all-caps references heading", func(t *testing.T) {
		t.Parallel()

		in := "# Title\n\nkeep me\n\n# REFERENCES\ndrop me"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "REFERENCES") {
			t.Errorf("BodyMarkdown still contains REFERENCES: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "drop me") {
			t.Errorf("BodyMarkdown still contains post-references line: %q", got.BodyMarkdown)
		}
	})

	t.Run("no references heading leaves body intact", func(t *testing.T) {
		t.Parallel()

		in := "# Title\n\nparagraph one\n\nparagraph two"

		got := extraction.Normalize(in, "fallback")

		if !strings.Contains(got.BodyMarkdown, "paragraph one") {
			t.Errorf("BodyMarkdown lost paragraph one: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "paragraph two") {
			t.Errorf("BodyMarkdown lost paragraph two: %q", got.BodyMarkdown)
		}
	})
}

func TestNormalizeBlockSkipping(t *testing.T) {
	t.Parallel()

	t.Run("removes gfm table while preserving surrounding paragraphs", func(t *testing.T) {
		t.Parallel()

		in := "before paragraph\n\n| col1 | col2 |\n| --- | --- |\n| a | b |\n| c | d |\n\nafter paragraph"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "col1") {
			t.Errorf("BodyMarkdown still contains table header: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "---") {
			t.Errorf("BodyMarkdown still contains separator: %q", got.BodyMarkdown)
		}
		if strings.Contains(got.BodyMarkdown, "| a | b |") {
			t.Errorf("BodyMarkdown still contains data row: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "before paragraph") {
			t.Errorf("BodyMarkdown lost before paragraph: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "after paragraph") {
			t.Errorf("BodyMarkdown lost after paragraph: %q", got.BodyMarkdown)
		}
	})

	t.Run("removes image lines", func(t *testing.T) {
		t.Parallel()

		in := "intro line\n\n![alt](url.jpg)\n\ntrailing line"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "![alt](url.jpg)") {
			t.Errorf("BodyMarkdown still contains image markup: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "intro line") {
			t.Errorf("BodyMarkdown lost intro line: %q", got.BodyMarkdown)
		}
		if !strings.Contains(got.BodyMarkdown, "trailing line") {
			t.Errorf("BodyMarkdown lost trailing line: %q", got.BodyMarkdown)
		}
	})

	t.Run("removes figure caption with figure prefix", func(t *testing.T) {
		t.Parallel()

		in := "intro\n\nFigure 1: A caption\n\nbody"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "Figure 1:") {
			t.Errorf("BodyMarkdown still contains Figure caption: %q", got.BodyMarkdown)
		}
	})

	t.Run("removes figure caption with fig prefix", func(t *testing.T) {
		t.Parallel()

		in := "intro\n\nFig. 2: Another caption\n\nbody"

		got := extraction.Normalize(in, "fallback")

		if strings.Contains(got.BodyMarkdown, "Fig. 2:") {
			t.Errorf("BodyMarkdown still contains Fig caption: %q", got.BodyMarkdown)
		}
	})

	t.Run("preserves figureheads sentence as not a figure caption", func(t *testing.T) {
		t.Parallel()

		in := "intro\n\nFigureheads of state met yesterday\n\nbody"

		got := extraction.Normalize(in, "fallback")

		if !strings.Contains(got.BodyMarkdown, "Figureheads of state met yesterday") {
			t.Errorf("BodyMarkdown wrongly removed figureheads sentence: %q", got.BodyMarkdown)
		}
	})
}

func TestNormalizeTitleSelection(t *testing.T) {
	t.Parallel()

	t.Run("uses first level-1 heading as title", func(t *testing.T) {
		t.Parallel()

		in := "# Document Title\n\n## Section\n\nbody"

		got := extraction.Normalize(in, "fallback")

		if got.Title != "Document Title" {
			t.Errorf("Title = %q, want %q", got.Title, "Document Title")
		}
	})

	t.Run("falls back to caller filename when no level-1 heading exists", func(t *testing.T) {
		t.Parallel()

		in := "## Subtitle only\n\nbody paragraph"

		got := extraction.Normalize(in, "doc1.pdf")

		if got.Title != "doc1.pdf" {
			t.Errorf("Title = %q, want %q", got.Title, "doc1.pdf")
		}
	})

	t.Run("ignores level-2 heading and picks later level-1 heading", func(t *testing.T) {
		t.Parallel()

		in := "## Subtitle\n\n# Title\n\nbody"

		got := extraction.Normalize(in, "fallback")

		if got.Title != "Title" {
			t.Errorf("Title = %q, want %q", got.Title, "Title")
		}
	})

	t.Run("title selection runs after references truncation", func(t *testing.T) {
		t.Parallel()

		in := "## Foo\n# Real Title\n## References\nentry"

		got := extraction.Normalize(in, "fallback")

		if got.Title != "Real Title" {
			t.Errorf("Title = %q, want %q", got.Title, "Real Title")
		}
	})
}

func TestNormalizeWordCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty body counts zero", "", 0},
		{"two simple words count two", "hello world", 2},
		{"mixed whitespace tokens count three", "hello   world\n\nfoo", 3},
		{"math markup counts as single token", "alpha $x+y$ beta", 3},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := extraction.Normalize(tc.in, "fallback")

			if got.WordCount != tc.want {
				t.Errorf("WordCount = %d, want %d", got.WordCount, tc.want)
			}
		})
	}
}

func TestNormalizeWhitespaceCollapse(t *testing.T) {
	t.Parallel()

	in := "paragraph\n\n\n\n\nmore"

	got := extraction.Normalize(in, "fallback")

	want := "paragraph\n\nmore"
	if got.BodyMarkdown != want {
		t.Errorf("BodyMarkdown = %q, want %q", got.BodyMarkdown, want)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	t.Parallel()

	in := "# Title\n\nintro paragraph\n\n\n\n\\(x+y\\)\n\n![alt](url.jpg)\n\nFigure 1: cap\n\n## References\nentry"

	first := extraction.Normalize(in, "fallback")
	second := extraction.Normalize(first.BodyMarkdown, "fallback")

	if first.BodyMarkdown != second.BodyMarkdown {
		t.Errorf("BodyMarkdown not idempotent:\nfirst:  %q\nsecond: %q", first.BodyMarkdown, second.BodyMarkdown)
	}
	if first.WordCount != second.WordCount {
		t.Errorf("WordCount not idempotent: first=%d second=%d", first.WordCount, second.WordCount)
	}
}
