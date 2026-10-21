package handler

import (
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
)

// candidateURLs returns the ordered list of base URLs that should be tried for
// an indexer. The primary URL comes first, followed by the known fallbacks.
// Duplicates are removed while preserving the original order.
func candidateURLs(meta IndexerMeta) []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0, len(meta.FallbackURLs)+1)

	for _, u := range append([]string{meta.URL}, meta.FallbackURLs...) {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	return urls
}

// fetchDocument fetches a document from the given URL using the shared requester.
func (i *Indexer) fetchDocument(ctx context.Context, url string) (*goquery.Document, error) {
	resp, err := i.requester.GetDocument(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	return goquery.NewDocumentFromReader(resp)
}

// fetchFirstMatchingURL fetches the first candidate URL that returns a document
// accepted by the matcher.
//
// Some websites rotate their domain every few months and keep only a redirect
// from the old domain to the new one. When the redirect target does not resolve
// (or the old domain is already gone), the whole indexer would stop working.
// Trying a list of known domains keeps the indexer alive during the rotation.
//
// The matcher receives the parsed document and should report whether it contains
// the expected results. It is called for every candidate, so it must be
// side-effect free.
func (i *Indexer) fetchFirstMatchingURL(
	ctx context.Context,
	baseURLs []string,
	buildURL func(baseURL string) string,
	match func(doc *goquery.Document) bool,
) (string, *goquery.Document, error) {
	var lastErr error

	for _, baseURL := range baseURLs {
		targetURL := buildURL(baseURL)

		doc, err := i.fetchDocument(ctx, targetURL)
		if err != nil {
			lastErr = err
			logging.Warn().Err(err).
				Str("url", targetURL).
				Msg("Failed to fetch document, trying the next candidate URL")
			continue
		}

		if match != nil && !match(doc) {
			lastErr = fmt.Errorf("no results found at %s", targetURL)
			logging.Warn().
				Str("url", targetURL).
				Msg("No results found, trying the next candidate URL")
			continue
		}

		return targetURL, doc, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate URLs configured")
	}

	return "", nil, fmt.Errorf("failed to fetch any candidate URL: %w", lastErr)
}
