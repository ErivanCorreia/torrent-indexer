package requester

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/ErivanCorreia/torrent-indexer/cache"
	"github.com/ErivanCorreia/torrent-indexer/logging"
	"github.com/ErivanCorreia/torrent-indexer/utils"
	"github.com/fereidani/httpdecompressor"
)

const (
	cacheKey = "shortLivedCache"
)

var challangeRegex = regexp.MustCompile(`(?i)(just a moment|cf-chl-bypass|under attack)`)

type Requster struct {
	fs                        *FlareSolverr
	c                         *cache.Redis
	httpClient                *http.Client
	shortLivedCacheExpiration time.Duration
}

func NewRequester(fs *FlareSolverr, c *cache.Redis, timeout time.Duration) *Requster {
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableCompression:  false,
			MaxIdleConns:        100,              // Increase connection pool
			MaxIdleConnsPerHost: 10,               // More connections per host
			IdleConnTimeout:     90 * time.Second, // Keep connections alive longer
			DisableKeepAlives:   false,            // Enable keep-alive
			ForceAttemptHTTP2:   true,             // Use HTTP/2 when possible
			// Respect the HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment variables.
			// It is useful when the indexer runs behind a corporate proxy or when
			// the local DNS resolver blocks some of the indexed domains.
			Proxy: http.ProxyFromEnvironment,
		},
	}

	return &Requster{fs: fs, httpClient: httpClient, c: c, shortLivedCacheExpiration: 30 * time.Minute}
}

func (i *Requster) SetShortLivedCacheExpiration(expiration time.Duration) {
	i.shortLivedCacheExpiration = expiration
}

func (i *Requster) GetDocument(ctx context.Context, url string, referer ...string) (io.ReadCloser, error) {
	var body io.ReadCloser

	// Extract referer if provided
	ref := ""
	if len(referer) > 0 {
		ref = referer[0]
	}

	// try request from short-lived cache
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	bodyByte, err := i.c.Get(ctx, key)
	if err == nil {
		logging.Debug().Str("url", url).Msg("Returning from short-lived cache")
		body = io.NopCloser(bytes.NewReader(bodyByte))
		return body, nil
	}

	// try request with plain client
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url %s: %w", url, err)
	}

	// Add browser-like headers to spoof a real browser
	spoofBrowserHeaders(req, ref)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		// try request with flare solverr
		var fsErr error
		body, fsErr = i.fs.Get(ctx, url, 3)
		if fsErr != nil {
			// keep the original error, it has more information about the failure
			return nil, fmt.Errorf("failed to do request for url %s: %w (flaresolverr: %v)", url, err, fsErr)
		}
	} else {
		defer resp.Body.Close()

		// Decompress response using httpdecompressor
		body, err = httpdecompressor.Reader(resp)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress response: %w", err)
		}
		defer body.Close()
	}

	// Pre-allocate buffer based on Content-Length if available
	var buf bytes.Buffer
	if resp != nil && resp.ContentLength > 0 {
		buf.Grow(int(resp.ContentLength))
	} else {
		buf.Grow(32 * 1024) // Default 32KB pre-allocation
	}

	// Use io.Copy instead of io.ReadAll for better performance
	_, err = io.Copy(&buf, body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	bodyByte = buf.Bytes()
	if hasChallange(bodyByte) {
		// try request with flare solverr
		body, err = i.fs.Get(ctx, url, 3)
		if err != nil {
			return nil, fmt.Errorf("response is a challenge for url %s: %w", url, err)
		}
		bodyByte, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		logging.Debug().Str("url", url).Msg("Request served from flaresolverr")
	} else {
		logging.Debug().Str("url", url).Msg("Request served from plain client")
	}

	// save response to cache if it's not a challange, body is not empty and is valid HTML
	switch {
	case hasChallange(bodyByte):
		return nil, fmt.Errorf("response is a challenge for url %s", url)
	case len(bodyByte) == 0:
		return nil, fmt.Errorf("empty response for url %s", url)
	case !utils.IsValidHTML(string(bodyByte)):
		return nil, fmt.Errorf("response is not a valid HTML document for url %s", url)
	}

	err = i.c.SetWithExpiration(ctx, key, bodyByte, i.shortLivedCacheExpiration)
	if err != nil {
		logging.Error().Err(err).Str("url", url).Msg("Failed to save response to cache")
	}
	logging.Debug().Str("url", url).Msg("Saved to cache")

	return io.NopCloser(bytes.NewReader(bodyByte)), nil
}

// GetContent fetches the raw content of a URL.
//
// Unlike GetDocument, the response is not required to be an HTML document,
// which is needed by the indexers that expose a JSON API.
func (i *Requster) GetContent(ctx context.Context, url string, referer ...string) ([]byte, error) {
	ref := ""
	if len(referer) > 0 {
		ref = referer[0]
	}

	// try request from short-lived cache
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	bodyByte, err := i.c.Get(ctx, key)
	if err == nil {
		logging.Debug().Str("url", url).Msg("Returning from short-lived cache")
		return bodyByte, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url %s: %w", url, err)
	}

	spoofBrowserHeaders(req, ref)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request for url %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := httpdecompressor.Reader(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress response: %w", err)
	}
	defer body.Close()

	bodyByte, err = io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(bodyByte) == 0 {
		return nil, fmt.Errorf("empty response for url %s", url)
	}

	logging.Debug().Str("url", url).Msg("Request served from plain client")

	if err := i.c.SetWithExpiration(ctx, key, bodyByte, i.shortLivedCacheExpiration); err != nil {
		logging.Error().Err(err).Str("url", url).Msg("Failed to save response to cache")
	} else {
		logging.Debug().Str("url", url).Msg("Saved to cache")
	}

	return bodyByte, nil
}

func (i *Requster) ExpireDocument(ctx context.Context, url string) error {
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	return i.c.Del(ctx, key)
}

// hasChallange checks if the body contains a challange by regex matching
func hasChallange(body []byte) bool {
	return challangeRegex.Match(body)
}
