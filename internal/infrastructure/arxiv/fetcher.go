package arxiv

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/infrastructure/httpclient"
)

// arxivFetcher composes URL construction, a generic byte-level shared.Fetcher,
// and the Atom parser into a single paper.Fetcher implementation for the arXiv
// source. It hides bytes, URLs, XML, and stdlib transport errors from the
// application layer; every failure exits as a paper.* sentinel.
type arxivFetcher struct {
	baseURL string
	http    shared.Fetcher
}

// NewArxivFetcher returns a paper.Fetcher backed by the arXiv query API.
// baseURL should be the absolute URL of the arxiv query endpoint (in
// production: "https://export.arxiv.org/api/query"). http is the injected
// byte-level fetcher used for the outbound call.
func NewArxivFetcher(baseURL string, http shared.Fetcher) paper.Fetcher {
	return &arxivFetcher{baseURL: baseURL, http: http}
}

// Fetch builds an arxiv-specific query URL from the source-neutral paper.Query,
// delegates the GET to the injected shared.Fetcher, and passes successful
// bytes through parseFeed. All failure modes are translated to paper.*
// sentinels; a non-empty entry slice is never returned alongside a non-nil
// error (requirement 4.4).
func (a *arxivFetcher) Fetch(ctx context.Context, q paper.Query) ([]paper.Entry, error) {
	u, err := buildArxivURL(a.baseURL, q)
	if err != nil {
		// A malformed base URL is an operator configuration problem; keep the
		// adapter's external contract (always return a paper.* sentinel) and
		// treat it as "no complete response".
		return nil, paper.ErrUpstreamUnavailable
	}

	body, err := a.http.Fetch(ctx, u)
	if err != nil {
		return nil, translateTransportError(err)
	}

	return parseFeed(body)
}

// buildArxivURL assembles the arxiv-specific query string from a paper.Query
// and appends it to baseURL. It uses `+OR+` as the literal category separator
// and URL-encodes the grouping parentheses (%28/%29) exactly as the arXiv
// HTTP API expects.
func buildArxivURL(baseURL string, q paper.Query) (string, error) {
	if _, err := url.Parse(baseURL); err != nil {
		return "", err
	}

	searchQuery := buildSearchQuery(q.Categories)

	values := url.Values{}
	values.Set("sortBy", "submittedDate")
	values.Set("sortOrder", "descending")
	values.Set("max_results", strconv.Itoa(q.MaxResults))

	// search_query is assembled manually because arXiv's API expects `+` as
	// the OR-separator token (not `%20` or the standard url.Values encoding).
	return baseURL + "?search_query=" + searchQuery + "&" + values.Encode(), nil
}

// buildSearchQuery formats the category list as arXiv expects.
// A single category has no grouping parentheses; multiple categories are
// wrapped in URL-encoded parens (%28/%29) with `+OR+` between them.
func buildSearchQuery(categories []string) string {
	switch len(categories) {
	case 0:
		return ""
	case 1:
		return "cat:" + categories[0]
	default:
		parts := make([]string, 0, len(categories))
		for _, c := range categories {
			parts = append(parts, "cat:"+c)
		}
		return "%28" + strings.Join(parts, "+OR+") + "%29"
	}
}

// translateTransportError maps shared.Fetcher failures to paper.* sentinels.
// The ordering is load-bearing: check shared.ErrBadStatus first so a 5xx
// response isn't miscategorized as a transport unavailability.
func translateTransportError(err error) error {
	if errors.Is(err, shared.ErrBadStatus) {
		return paper.ErrUpstreamBadStatus
	}
	if httpclient.IsTransportError(err) {
		return paper.ErrUpstreamUnavailable
	}
	// Conservative catch-all: an unclassified transport error means "no
	// complete response", which is the 504 semantic.
	return paper.ErrUpstreamUnavailable
}

