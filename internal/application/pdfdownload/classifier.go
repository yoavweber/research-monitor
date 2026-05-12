// classifier.go: pure function classify(err) mapping pdf.Err* sentinels
// (and nil) to (status, category, description) for the per-entry
// result. Plus sanitize, which strips paths/credentials and truncates
// to 256 chars so the description never leaks filesystem details.
package pdfdownload

import (
	"errors"
	"regexp"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// sanitizedMaxLen bounds the description carried on a DownloadEntryResult.
// SSE frames and JSON status payloads include this string verbatim; a hard
// cap keeps a single failure from inflating a frame and protects the SSE
// fan-out from accidentally amplifying very long upstream messages.
const sanitizedMaxLen = 256

// categoryUnknown is the default classification bucket when the error
// does not match any of the pdf.ErrInvalidKey/ErrFetch/ErrStore sentinels.
// Documented as a literal string in design.md so dashboards and tests can
// pin it without importing this package.
const categoryUnknown = "unknown"

// absolutePathRegexp matches a unix-style absolute path with at least two
// segments. Two segments avoid stripping plain leading slashes; PDF errors
// in this system always carry a path like "/var/lib/pdfs/..." when they
// leak, never a bare "/foo".
var absolutePathRegexp = regexp.MustCompile(`/[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+`)

// bearerRegexp matches a "Bearer <token>" credential header pattern.
// Trading-system convention: redact the value, keep the marker so an
// operator can still recognise the failure shape.
var bearerRegexp = regexp.MustCompile(`(?i)\bbearer\s+\S+`)

// inlineSecretRegexp matches key=value pairs whose key is a known
// credential name. The value run is the longest non-whitespace token after
// "=" so we redact the whole secret even if it contains punctuation.
var inlineSecretRegexp = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api_key|apikey)\s*=\s*\S+`)

// classify maps a Store.Ensure outcome to the wire-level
// (status, category, description) triple recorded on each DownloadEntryResult.
//
// The category strings are the pdf.Category* constants for the three known
// sentinels and the literal "unknown" otherwise. Description is the result of
// sanitize(err); empty on nil err.
func classify(err error) (paper.DownloadEntryStatus, string, string) {
	switch {
	case err == nil:
		return paper.DownloadStatusSuccess, "", ""
	case errors.Is(err, pdf.ErrInvalidKey):
		return paper.DownloadStatusFailed, pdf.CategoryInvalidKey, sanitize(err)
	case errors.Is(err, pdf.ErrFetch):
		return paper.DownloadStatusFailed, pdf.CategoryFetch, sanitize(err)
	case errors.Is(err, pdf.ErrStore):
		return paper.DownloadStatusFailed, pdf.CategoryStore, sanitize(err)
	default:
		return paper.DownloadStatusFailed, categoryUnknown, sanitize(err)
	}
}

// sanitize produces a human-readable description suitable for SSE frames
// and JSON status payloads. It strips absolute filesystem paths and obvious
// credential tokens, then trims the result to sanitizedMaxLen so a single
// pathological upstream message cannot bloat downstream frames.
//
// The function operates on the error's text; callers do not need to wrap
// or unwrap before invoking it. A nil error returns the empty string.
func sanitize(err error) string {
	if err == nil {
		return ""
	}
	out := err.Error()
	out = inlineSecretRegexp.ReplaceAllString(out, "[REDACTED]")
	out = bearerRegexp.ReplaceAllString(out, "Bearer [REDACTED]")
	out = absolutePathRegexp.ReplaceAllString(out, "[PATH]")
	if len(out) > sanitizedMaxLen {
		out = out[:sanitizedMaxLen]
	}
	return out
}
