package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var starck_filmes = IndexerMeta{
	Label:       "starck_filmes",
	URL:         utils.GetIndexerURLFromEnv("INDEXER_STARCK_FILMES_URL", "https://starckfilmes-v26.com/"),
	SearchURL:   "?s=",
	PagePattern: "page/%s",
	// The website rotates its domain every few months (e.g. -v10 -> -v21 -> -v26)
	// and the retired domains only redirect to the current one. When the redirect
	// target does not resolve, the indexer would be completely down, so the known
	// domains are kept as fallbacks.
	FallbackURLs: []string{
		"https://www.starckfilmes-v26.com/",
		"https://www.starckfilmes-v21.com/",
		"https://www.starckfilmes-v10.com/",
	},
}

// buildStarckURL builds the target URL for the given base URL.
func buildStarckURL(baseURL, q, page string) string {
	if q != "" {
		return fmt.Sprintf("%s%s%s", baseURL, starck_filmes.SearchURL, q)
	}

	if page == "" {
		page = "1"
	}

	return fmt.Sprintf(fmt.Sprintf("%s%s", baseURL, starck_filmes.PagePattern), page)
}

// extractStarckLinks extracts the detail page links from the catalog listing.
func extractStarckLinks(doc *goquery.Document) []string {
	var links []string

	doc.Find(".item").Each(func(_ int, s *goquery.Selection) {
		link, exists := s.Find("div.sub-item > a").First().Attr("href")
		if !exists || link == "" {
			link, _ = s.Find("a[href]").First().Attr("href")
		}
		if link != "" {
			links = append(links, link)
		}
	})

	return links
}

func (i *Indexer) HandlerStarckFilmesIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := starck_filmes

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)

	// The site rotates its domain, so try the known domains until one of them
	// answers with a valid catalog listing.
	var links []string
	targetURL, _, err := i.fetchFirstMatchingURL(
		ctx,
		candidateURLs(metadata),
		func(baseURL string) string {
			return buildStarckURL(baseURL, q, page)
		},
		func(d *goquery.Document) bool {
			links = extractStarckLinks(d)
			return len(links) > 0
		},
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode error response")
		}
		i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
		return
	}

	logging.InfoWithRequest(r).Str("target_url", targetURL).Msg("Processing indexer request")

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		_ = i.requester.ExpireDocument(ctx, targetURL)
		logging.DebugWithRequest(r).Str("url", targetURL).Msg("No links found, expiring cache")
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentStarckFilmes(ctx, i, link, targetURL)
	})

	// Apply post-processors
	postProcessedTorrents := indexedTorrents
	for _, processor := range i.postProcessors {
		postProcessedTorrents = processor(i, r, postProcessedTorrents)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results:      postProcessedTorrents,
		Count:        len(postProcessedTorrents),
		IndexedCount: len(indexedTorrents),
	})
	if err != nil {
		logging.Error().Err(err).Msg("Failed to encode response")
	}
}

// starckFilmesDetails holds the metadata extracted from a movie page.
type starckFilmesDetails struct {
	Title       string
	Year        string
	Audio       []schema.Audio
	Size        []string
	MagnetLinks []string
}

// parseStarckFilmesDetails extracts the metadata of a movie page.
//
// The magnet links are not stored in the "href" attribute, they are encoded in
// the "data-u" attribute and need to be decoded.
func parseStarckFilmesDetails(doc *goquery.Document) starckFilmesDetails {
	post := doc.Find(".post")
	capa := post.Find(".capa")
	title := strings.TrimSpace(capa.Find(".post-description > h2").Text())
	postButtons := post.Find(".post-buttons")

	var magnetLinks []string

	postButtons.Find("a[href^=\"magnet\"]").Each(func(_ int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		if magnetLink != "" {
			magnetLinks = append(magnetLinks, magnetLink)
		}
	})

	postButtons.Find("a[data-u]").Each(func(_ int, s *goquery.Selection) {
		dataU, _ := s.Attr("data-u")
		magnetLink, err := utils.DecodeStarckDataU(dataU)
		if err != nil {
			logging.Warn().Err(err).Str("data_u", dataU).Msg("Failed to decode data-u attribute, skipping")
			return
		}
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []schema.Audio
	var year string
	var size []string

	capa.Find(".post-description p").Each(func(_ int, s *goquery.Selection) {
		// pattern:
		// Nome Original: 28 Weeks Later
		// Lançamento: 2007
		// Duração: 1h 40 min
		// Gênero: Terror, Suspense, Ficção
		// Formato: MKV
		// Tamanho: 2.45 GB
		// Qualidade de Video: 10
		// Qualidade do Audio: 10
		// Idioma: Português | Inglês
		// Legenda: Português, Inglês, Espanhol
		var text strings.Builder
		s.Find("span").Each(func(_ int, span *goquery.Selection) {
			text.WriteString(span.Text())
			text.WriteString(" ")
		})
		audio = append(audio, findAudioFromText(text.String())...)
		y := findYearFromText(text.String(), title)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text.String())...)
	})

	return starckFilmesDetails{
		Title:       title,
		Year:        year,
		Audio:       audio,
		Size:        utils.StableUniq(size),
		MagnetLinks: magnetLinks,
	}
}

func getTorrentStarckFilmes(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	date := getPublishedDateFromRawString(link)

	details := parseStarckFilmesDetails(doc)
	title := details.Title
	year := details.Year
	audio := details.Audio
	size := details.Size
	magnetLinks := details.MagnetLinks

	// TODO: find any link from imdb
	imdbLink := ""

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", magnetLink).Msg("Failed to parse magnet URI")
			}
			releaseTitle := strings.TrimSpace(magnet.DisplayName)
			// url decode the title
			releaseTitle, err = url.QueryUnescape(releaseTitle)
			if err != nil {
				logging.Error().Err(err).Str("title", releaseTitle).Msg("Failed to URL decode title")
				releaseTitle = strings.TrimSpace(magnet.DisplayName)
			}
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			for i, tracker := range trackers {
				unescapedTracker, err := url.QueryUnescape(tracker)
				if err != nil {
					logging.Error().Err(err).Str("tracker", tracker).Msg("Failed to URL decode tracker")
				}
				trackers[i] = strings.TrimSpace(unescapedTracker)
			}
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			title := processTitle(title, magnetAudio)

			// if the number of sizes is equal to the number of magnets, then assign the size to each indexed torrent in order
			var mySize string
			if len(size) == len(magnetLinks) {
				mySize = size[it]
			}
			if mySize == "" {
				go func() {
					_, _ = i.magnetMetadataAPI.FetchMetadata(ctx, magnetLink)
				}()
			}

			ixt := schema.IndexedTorrent{
				Title:         releaseTitle,
				OriginalTitle: title,
				Details:       link,
				Year:          year,
				IMDB:          imdbLink,
				Audio:         magnetAudio,
				MagnetLink:    magnetLink,
				Date:          date,
				InfoHash:      infoHash,
				Trackers:      trackers,
				LeechCount:    peer,
				SeedCount:     seed,
				Size:          mySize,
			}
			chanIndexedTorrent <- ixt
		}(it, magnetLink)
	}

	for i := 0; i < len(magnetLinks); i++ {
		it := <-chanIndexedTorrent
		indexedTorrents = append(indexedTorrents, it)
	}

	return indexedTorrents, nil
}
