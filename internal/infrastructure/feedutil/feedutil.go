package feedutil

import (
	"strings"
	"time"
)

// NormalizeSpace collapses internal whitespace in a free-text field (helpful for Atom
// feeds that frequently wrap text across lines with leading indentation).
func NormalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ParseAtomTime parses an RFC3339 formatted Atom timestamp.
func ParseAtomTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(s))
}
