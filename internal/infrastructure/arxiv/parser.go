// Package arxiv implements paper.Fetcher for the arXiv source. This file
// provides the pure Atom feed parser; the HTTP composite lives in fetcher.go
// (to be added in task 4.2).
package arxiv

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
)

// parseFeed decodes arXiv's Atom feed into source-neutral paper.Entry values.
// It returns paper.ErrUpstreamMalformed on any decode failure or when the
// feed contains arXiv's wrapped error entry (<id>http://arxiv.org/api/errors#...</id>).
// A structurally valid feed with zero entries returns ([]paper.Entry{}, nil).
func parseFeed(body []byte) ([]paper.Entry, error) {
	var feed xmlFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("%w: xml decode: %v", paper.ErrUpstreamMalformed, err)
	}

	entries := make([]paper.Entry, 0, len(feed.Entries))
	for _, xe := range feed.Entries {
		// Detect arXiv's Atom-wrapped error entries, which arrive as HTTP 200
		// but carry an <id> pointing at /api/errors#...
		if strings.Contains(xe.ID, "/api/errors") {
			return nil, fmt.Errorf("%w: arxiv error entry: %s", paper.ErrUpstreamMalformed, strings.TrimSpace(xe.Summary))
		}

		if strings.TrimSpace(xe.ID) == "" {
			return nil, fmt.Errorf("%w: entry missing <id>", paper.ErrUpstreamMalformed)
		}

		sourceID, version := splitAbsID(xe.ID)

		submittedAt, err := parseAtomTime(xe.Published)
		if err != nil {
			return nil, fmt.Errorf("%w: parse <published>: %v", paper.ErrUpstreamMalformed, err)
		}
		// <updated> is always present in arxiv responses, but be lenient: an
		// absent/empty value is mapped to a zero time rather than a hard
		// failure, matching the design's "prefer leniency for arxiv-optional
		// fields" policy.
		var updatedAt time.Time
		if strings.TrimSpace(xe.Updated) != "" {
			updatedAt, err = parseAtomTime(xe.Updated)
			if err != nil {
				return nil, fmt.Errorf("%w: parse <updated>: %v", paper.ErrUpstreamMalformed, err)
			}
		}

		authors := make([]string, 0, len(xe.Authors))
		for _, a := range xe.Authors {
			authors = append(authors, strings.TrimSpace(a.Name))
		}

		categories := make([]string, 0, len(xe.Categories))
		for _, c := range xe.Categories {
			categories = append(categories, c.Term)
		}

		entries = append(entries, paper.Entry{
			SourceID:        sourceID,
			Version:         version,
			Title:           normalizeSpace(xe.Title),
			Authors:         authors,
			Abstract:        normalizeSpace(xe.Summary),
			PrimaryCategory: xe.PrimaryCategory.Term,
			Categories:      categories,
			SubmittedAt:     submittedAt,
			UpdatedAt:       updatedAt,
			PDFURL:          pickPDFLink(xe.Links),
			AbsURL:          strings.TrimSpace(xe.ID),
		})
	}

	return entries, nil
}

// xmlFeed mirrors the subset of the arXiv Atom feed we consume. Namespaces
// are expressed per element so that both default-Atom and arxiv-scoped
// elements are captured by encoding/xml.
type xmlFeed struct {
	XMLName xml.Name   `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []xmlEntry `xml:"http://www.w3.org/2005/Atom entry"`
}

type xmlEntry struct {
	ID              string             `xml:"http://www.w3.org/2005/Atom id"`
	Title           string             `xml:"http://www.w3.org/2005/Atom title"`
	Summary         string             `xml:"http://www.w3.org/2005/Atom summary"`
	Published       string             `xml:"http://www.w3.org/2005/Atom published"`
	Updated         string             `xml:"http://www.w3.org/2005/Atom updated"`
	Authors         []xmlAuthor        `xml:"http://www.w3.org/2005/Atom author"`
	Links           []xmlLink          `xml:"http://www.w3.org/2005/Atom link"`
	PrimaryCategory xmlPrimaryCategory `xml:"http://arxiv.org/schemas/atom primary_category"`
	Categories      []xmlCategory      `xml:"http://www.w3.org/2005/Atom category"`
}

type xmlAuthor struct {
	Name string `xml:"http://www.w3.org/2005/Atom name"`
}

type xmlLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

type xmlPrimaryCategory struct {
	XMLName xml.Name `xml:"http://arxiv.org/schemas/atom primary_category"`
	Term    string   `xml:"term,attr"`
}

type xmlCategory struct {
	Term string `xml:"term,attr"`
}

// splitAbsID strips the canonical `http://arxiv.org/abs/` prefix from an entry
// <id> and splits the trailing `vN` version suffix, if present. Non-prefixed
// ids are returned as-is so the caller can still surface something useful.
func splitAbsID(id string) (sourceID, version string) {
	trimmed := strings.TrimSpace(id)
	const prefix = "http://arxiv.org/abs/"
	tail := strings.TrimPrefix(trimmed, prefix)
	// Split a trailing `vN` (N decimal digits). Walk back from the end.
	for i := len(tail) - 1; i >= 0; i-- {
		if tail[i] < '0' || tail[i] > '9' {
			if tail[i] == 'v' && i > 0 && i < len(tail)-1 {
				return tail[:i], tail[i:]
			}
			break
		}
	}
	return tail, ""
}

// pickPDFLink returns the href of the <link title="pdf"> child, or the empty
// string if none is present. Per design the URL is read from the feed, never
// reconstructed.
func pickPDFLink(links []xmlLink) string {
	for _, l := range links {
		if l.Title == "pdf" {
			return l.Href
		}
	}
	return ""
}

func parseAtomTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(s))
}

// normalizeSpace collapses internal whitespace in a free-text field (Atom
// feeds frequently wrap <title> and <summary> across lines with leading
// indentation).
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
