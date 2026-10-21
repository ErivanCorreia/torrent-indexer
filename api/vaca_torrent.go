package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var vacaTorrent = IndexerMeta{
	Label: "vaca_torrent",
	// The website was rebranded to "VaqueiroCine" and moved to a new domain.
	// The old domain only serves a landing page that redirects to it.
	URL:         utils.GetIndexerURLFromEnv("INDEXER_VACA_TORRENT_URL", "https://vaqueirofilmes1.com/"),
	SearchURL:   utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_SEARCH_URL", "?q="),
	PagePattern: utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_PAGE_PATTERN", "pagina/%s/"),
}

var (
	vacaBrTagRe   = regexp.MustCompile(`(?i)<br\s*/?>`)
	vacaHtmlTagRe = regexp.MustCompile(`<[^>]*>`)
)

// buildVacaTorrentURL builds the target URL for the given base URL.
func buildVacaTorrentURL(baseURL, q, page string) string {
	if q != "" {
		return fmt.Sprintf("%s%s%s", baseURL, vacaTorrent.SearchURL, q)
	}

	if page == "" || page == "1" {
		return baseURL
	}

	return fmt.Sprintf(fmt.Sprintf("%s%s", baseURL, vacaTorrent.PagePattern), page)
}

// extractVacaTorrentLinks extracts the detail page links from the catalog listing.
func extractVacaTorrentLinks(doc *goquery.Document) []string {
	var links []string
	seen := make(map[string]struct{})

	addLink := func(link string) {
		if link == "" {
			return
		}
		if _, ok := seen[link]; ok {
			return
		}
		seen[link] = struct{}{}
		links = append(links, link)
	}

	doc.Find("a.movie-card[href], .movie-card a[href], a[href*=\"/filme/\"], a[href*=\"/movie/\"]").
		Each(func(_ int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			addLink(link)
		})

	// fallback: cards that are not wrapped in an anchor
	doc.Find(".movie-card").Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Closest("a").Attr("href")
		addLink(link)
	})

	return links
}

// extractVacaText returns the text of the selection with the line breaks
// converted into spaces, e.g. for titles split by <br>.
func extractVacaText(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}

	html, err := s.Html()
	if err != nil {
		return strings.TrimSpace(s.Text())
	}

	html = vacaBrTagRe.ReplaceAllString(html, " ")
	html = vacaHtmlTagRe.ReplaceAllString(html, "")

	return strings.TrimSpace(html)
}

// extractVacaTitle extracts the title of the movie page.
func extractVacaTitle(doc *goquery.Document) string {
	title := extractVacaText(doc.Find("h1.movie-title").First())
	if title == "" {
		title = extractVacaText(doc.Find("h1").First())
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find("meta[property=\"og:title\"]").AttrOr("content", ""))
	}

	// remove the release year if it is part of the title
	title = strings.TrimSpace(strings.Split(title, "(")[0])

	return title
}

// extractVacaOriginalTitle extracts the original title of the movie page.
func extractVacaOriginalTitle(doc *goquery.Document) string {
	original := strings.TrimSpace(doc.Find(".movie-original").First().Text())
	if original == "" {
		original = strings.TrimSpace(doc.Find("meta[property=\"og:title\"]").AttrOr("content", ""))
	}

	return original
}

// extractVacaDetails extracts the "label -> value" pairs of the movie details,
// e.g. "Idioma:" -> "Português / Inglês".
func extractVacaDetails(doc *goquery.Document) map[string]string {
	details := make(map[string]string)

	doc.Find(".detail-item").Each(func(_ int, s *goquery.Selection) {
		label := strings.TrimSpace(s.Find(".detail-label").First().Text())
		label = strings.TrimSuffix(label, ":")
		if label == "" {
			return
		}
		value := strings.TrimSpace(vacaHtmlTagRe.ReplaceAllString(s.Text(), " "))
		value = strings.TrimSpace(strings.TrimPrefix(value, s.Find(".detail-label").First().Text()))
		if value == "" {
			return
		}
		details[label] = value
	})

	return details
}

// extractVacaAudio extracts the audio languages from a list of languages
// separated by "/", "|" or ",".
func extractVacaAudio(value string) []schema.Audio {
	var audio []schema.Audio

	langs := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == '|' || r == ','
	})

	for _, lang := range langs {
		lang = strings.TrimSpace(lang)
		if lang == "" {
			continue
		}
		if a := schema.GetAudioFromString(lang); a != nil {
			audio = append(audio, *a)
		}
	}

	return audio
}

// vacaMagnetEntry holds the data of a single download row of the torrent table.
type vacaMagnetEntry struct {
	Link  string
	Row   string
	Qual  string
	Size  string
	Audio []schema.Audio
}

// extractVacaMagnetEntries extracts the magnet links and their metadata from the
// torrent table of the movie page.
func extractVacaMagnetEntries(doc *goquery.Document, pageAudio []schema.Audio) []vacaMagnetEntry {
	var entries []vacaMagnetEntry

	extractMagnet := func(s *goquery.Selection) string {
		href, exists := s.Attr("href")
		if exists && strings.HasPrefix(href, "magnet:") {
			return href
		}

		// the download button may carry the magnet link in a data attribute
		for _, attr := range []string{"data-magnet", "data-href", "data-url", "data-link"} {
			if value, ok := s.Attr(attr); ok && strings.HasPrefix(value, "magnet:") {
				return value
			}
		}

		return ""
	}

	rows := doc.Find(".torrent-table tbody tr")
	if rows.Length() == 0 {
		rows = doc.Find(".torrent-table tr")
	}

	rows.Each(func(_ int, s *goquery.Selection) {
		link := ""
		s.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			link = extractMagnet(a)
			return link == ""
		})
		if link == "" {
			link = extractMagnet(s.Find(".td-btn").First())
		}

		row := strings.TrimSpace(vacaHtmlTagRe.ReplaceAllString(s.Text(), " "))
		rowAudio := pageAudio
		if strings.Contains(strings.ToUpper(row), "DUBLA") {
			rowAudio = []schema.Audio{schema.AudioPortuguese}
		}

		entries = append(entries, vacaMagnetEntry{
			Link:  link,
			Row:   row,
			Qual:  strings.TrimSpace(s.Find(".td-qual").First().Text()),
			Size:  strings.TrimSpace(s.Find(".td-size").First().Text()),
			Audio: rowAudio,
		})
	})

	// fallback for layouts where the magnets are not inside the torrent table
	if len(entries) == 0 {
		doc.Find("a[href^=\"magnet:\"]").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if href == "" {
				return
			}
			entries = append(entries, vacaMagnetEntry{Link: href, Audio: pageAudio})
		})
	}

	return entries
}

func (i *Indexer) HandlerVacaTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := vacaTorrent

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)

	var links []string
	targetURL, _, err := i.fetchFirstMatchingURL(
		ctx,
		candidateURLs(metadata),
		func(baseURL string) string {
			return buildVacaTorrentURL(baseURL, q, page)
		},
		func(d *goquery.Document) bool {
			links = extractVacaTorrentLinks(d)
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
		return getTorrentsVacaTorrent(ctx, i, link, targetURL)
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

// vacaTorrentDetails holds the metadata extracted from a movie page.
type vacaTorrentDetails struct {
	Title         string
	OriginalTitle string
	Year          string
	Audio         []schema.Audio
	Entries       []vacaMagnetEntry
	IMDB          string
	Date          time.Time
}

// parseVacaTorrentDetails extracts the metadata of a movie page.
func parseVacaTorrentDetails(doc *goquery.Document) vacaTorrentDetails {
	title := extractVacaTitle(doc)
	originalTitle := extractVacaOriginalTitle(doc)

	movieDetails := extractVacaDetails(doc)
	pageAudio := extractVacaAudio(movieDetails["Idioma"])
	if len(pageAudio) == 0 {
		pageAudio = extractVacaAudio(movieDetails["Áudio"])
	}

	var year string

	// the year can be found on the release tag or on the original title
	if tag := strings.TrimSpace(doc.Find(".tag-year").First().Text()); tag != "" {
		year = tag
	} else if y := findYearFromText(movieDetails["Lançamento"], title); y != "" {
		year = y
	} else {
		year = findYearFromText("", originalTitle)
	}

	// find any link from imdb
	imdbLink := getIMDBLinkFromDocument(doc)

	return vacaTorrentDetails{
		Title:         title,
		OriginalTitle: originalTitle,
		Year:          year,
		Audio:         pageAudio,
		Entries:       extractVacaMagnetEntries(doc, pageAudio),
		IMDB:          imdbLink,
		Date:          getPublishedDateFromMeta(doc),
	}
}

func getTorrentsVacaTorrent(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	details := parseVacaTorrentDetails(doc)
	title := details.Title
	year := details.Year
	imdbLink := details.IMDB
	date := details.Date
	magnetEntries := details.Entries

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// only the rows with a magnet link can be indexed
	var indexedEntries []vacaMagnetEntry
	for _, entry := range magnetEntries {
		if entry.Link == "" {
			continue
		}
		indexedEntries = append(indexedEntries, entry)
	}

	// for each magnet link, create a new indexed torrent
	for _, entry := range indexedEntries {
		entry := entry
		go func(entry vacaMagnetEntry) {
			magnet, err := magnet.ParseMagnetUri(entry.Link)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", entry.Link).Msg("Failed to parse magnet URI")
				chanIndexedTorrent <- schema.IndexedTorrent{}
				return
			}
			releaseTitle := magnet.DisplayName
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			magnetAudio := getAudioFromTitle(releaseTitle, entry.Audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			processedTitle := processVacaTorrentTitle(title, magnetAudio, "")

			mySize := entry.Size
			if mySize == "" {
				if rowSizes := findSizesFromText(entry.Row); len(rowSizes) > 0 {
					mySize = rowSizes[0]
				}
			}
			if mySize == "" {
				go func() {
					_, _ = i.magnetMetadataAPI.FetchMetadata(ctx, entry.Link)
				}()
			}

			ixt := schema.IndexedTorrent{
				Title:         releaseTitle,
				OriginalTitle: processedTitle,
				Details:       link,
				Year:          year,
				IMDB:          imdbLink,
				Audio:         magnetAudio,
				MagnetLink:    entry.Link,
				Date:          date,
				InfoHash:      infoHash,
				Trackers:      trackers,
				LeechCount:    peer,
				SeedCount:     seed,
				Size:          mySize,
			}
			chanIndexedTorrent <- ixt
		}(entry)
	}

	for i := 0; i < len(indexedEntries); i++ {
		it := <-chanIndexedTorrent
		// Skip empty torrents (failed to parse)
		if it.InfoHash != "" {
			indexedTorrents = append(indexedTorrents, it)
		}
	}

	return indexedTorrents, nil
}

func processVacaTorrentTitle(title string, audio []schema.Audio, season string) string {
	// Remove common suffixes
	title = strings.Replace(title, " – Download", "", -1)
	title = strings.Replace(title, " - Download", "", -1)
	title = strings.TrimSpace(title)

	// Add season if present
	if season != "" {
		title = fmt.Sprintf("%s S%s - %sª Temporada", title, fmt.Sprintf("%02s", season), season)
	}

	// Add audio ISO 639-2 code to title between ()
	title = appendAudioISO639_2Code(title, audio)

	return title
}
