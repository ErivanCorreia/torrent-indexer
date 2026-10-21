package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var rede_torrent = IndexerMeta{
	Label:       "rede_torrent",
	URL:         utils.GetIndexerURLFromEnv("INDEXER_REDE_TORRENT_URL", "https://redestorrents.com/"),
	SearchURL:   "index.php?busca=",
	PagePattern: "pagina/%s/",
	// "redetorrent.com" only redirects to the current domain, keep it as a fallback.
	FallbackURLs: []string{
		"https://redetorrent.com/",
	},
}

// buildRedeTorrentURL builds the target URL for the given base URL.
func buildRedeTorrentURL(baseURL, q, page string) string {
	if q != "" {
		return fmt.Sprintf("%s%s%s", baseURL, rede_torrent.SearchURL, q)
	}

	if page == "" {
		return baseURL
	}

	return fmt.Sprintf(fmt.Sprintf("%s%s", baseURL, rede_torrent.PagePattern), page)
}

// extractRedeTorrentLinks extracts the detail page links from the catalog listing.
func extractRedeTorrentLinks(doc *goquery.Document) []string {
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

	doc.Find("a.cover-link[href], article.custom-card").Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		if link == "" {
			link, _ = s.Closest("a").Attr("href")
		}
		addLink(link)
	})

	return links
}

// extractRedeTorrentSpecs extracts the "label -> value" pairs of the technical
// specifications block, e.g. "Tamanho do Arquivo" -> "2.67 GB / 6.21 GB".
func extractRedeTorrentSpecs(doc *goquery.Document) map[string]string {
	specs := make(map[string]string)

	doc.Find(".spec-card-glass").Each(func(_ int, s *goquery.Selection) {
		label := strings.TrimSpace(s.Find("small").First().Text())
		value := strings.TrimSpace(s.Find("strong").First().Text())
		if label == "" {
			return
		}
		specs[label] = value
	})

	return specs
}

// parseRedeTorrentAudio extracts the audio languages from a list of languages
// separated by "/", "|" or ",".
func parseRedeTorrentAudio(value string) []schema.Audio {
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

var (
	redeTorrentYearRe   = regexp.MustCompile(`\((\d{4})\)`)
	redeTorrentSuffixRe = regexp.MustCompile(`(?i)\s+(torrent|download|assistir|online)$`)
)

// parseRedeTorrentTitle extracts the title and the year from the page heading.
//
// The heading looks like: "Moana Torrent (2026) Dual Áudio 5.1 Download".
func parseRedeTorrentTitle(doc *goquery.Document) (title string, year string) {
	heading := strings.TrimSpace(doc.Find("h1").First().Text())
	if heading == "" {
		heading = strings.TrimSpace(doc.Find("meta[property=\"og:title\"]").AttrOr("content", ""))
	}

	if match := redeTorrentYearRe.FindStringSubmatch(heading); len(match) > 0 {
		year = match[1]
	}

	// everything from the parenthesis onwards is metadata
	if idx := strings.Index(heading, "("); idx > 0 {
		heading = heading[:idx]
	}
	heading = strings.TrimSpace(heading)

	// "Halloween - Coleção Completa" -> "Halloween"
	if idx := strings.Index(heading, " - "); idx > 0 {
		heading = heading[:idx]
	}

	title = strings.TrimSpace(redeTorrentSuffixRe.ReplaceAllString(heading, ""))

	return title, year
}

// redeTorrentRowAudio infers the audio languages of a single download row from
// its label, e.g. "DOWNLOAD TORRENT DUBLADO DUAL AUDIO 5.1 MKV WEB-DL 1080P".
func redeTorrentRowAudio(rowText string, pageAudio []schema.Audio) []schema.Audio {
	upper := strings.ToUpper(rowText)

	switch {
	case strings.Contains(upper, "DUAL"):
		audio := slices.Clone(pageAudio)
		if !slices.Contains(audio, schema.AudioPortuguese) {
			audio = append(audio, schema.AudioPortuguese)
		}
		return audio
	case strings.Contains(upper, "DUBLADO"), strings.Contains(upper, "NACIONAL"):
		return []schema.Audio{schema.AudioPortuguese}
	}

	return pageAudio
}

func (i *Indexer) HandlerRedeTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := rede_torrent

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
			return buildRedeTorrentURL(baseURL, q, page)
		},
		func(d *goquery.Document) bool {
			links = extractRedeTorrentLinks(d)
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
		return getTorrentsRedeTorrent(ctx, i, link, targetURL)
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

// redeTorrentMagnet holds the data of a single download row of the movie page.
type redeTorrentMagnet struct {
	Link string
	Row  string
}

// redeTorrentDetails holds the metadata extracted from a movie page.
type redeTorrentDetails struct {
	Title       string
	Year        string
	Audio       []schema.Audio
	Size        []string
	MagnetLinks []redeTorrentMagnet
	IMDB        string
	Date        time.Time
}

// parseRedeTorrentDetails extracts the metadata of a movie page.
func parseRedeTorrentDetails(doc *goquery.Document) redeTorrentDetails {
	title, year := parseRedeTorrentTitle(doc)

	date := getPublishedDateFromMeta(doc)
	if date.IsZero() {
		date = getPublishedDateFromRawString(doc.Find("time[itemprop=\"dateCreated\"]").AttrOr("datetime", ""))
	}

	specs := extractRedeTorrentSpecs(doc)
	pageAudio := parseRedeTorrentAudio(specs["Idioma / Áudio"])

	var magnetLinks []redeTorrentMagnet

	// each magnet link lives inside a download row that describes its quality
	doc.Find(".download-row").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Find("a[href^=\"magnet:\"]").First().Attr("href")
		if !exists || href == "" {
			return
		}
		row := strings.TrimSpace(s.Find(".download-name").First().Text())
		magnetLinks = append(magnetLinks, redeTorrentMagnet{Link: href, Row: row})
	})

	// fallback for layouts where the magnets are not wrapped in download rows
	if len(magnetLinks) == 0 {
		doc.Find("a[href^=\"magnet:\"]").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if href == "" {
				return
			}
			magnetLinks = append(magnetLinks, redeTorrentMagnet{Link: href})
		})
	}

	// find any link from imdb
	imdbLink := getIMDBLinkFromDocument(doc)

	return redeTorrentDetails{
		Title:       title,
		Year:        year,
		Audio:       pageAudio,
		Size:        utils.StableUniq(findSizesFromText(specs["Tamanho do Arquivo"])),
		MagnetLinks: magnetLinks,
		IMDB:        imdbLink,
		Date:        date,
	}
}

func getTorrentsRedeTorrent(ctx context.Context,
	i *Indexer,
	link, referer string,
) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	details := parseRedeTorrentDetails(doc)
	title := details.Title
	year := details.Year
	pageAudio := details.Audio
	size := details.Size
	magnetEntries := details.MagnetLinks
	imdbLink := details.IMDB
	date := details.Date

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, entry := range magnetEntries {
		it := it
		go func(it int, entry redeTorrentMagnet) {
			magnet, err := magnet.ParseMagnetUri(entry.Link)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", entry.Link).Msg("Failed to parse magnet URI")
				chanIndexedTorrent <- schema.IndexedTorrent{}
				return
			}
			releaseTitle := magnet.DisplayName
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			audio := redeTorrentRowAudio(entry.Row, pageAudio)
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			title := processTitle(title, magnetAudio)

			// if the number of sizes is equal to the number of magnets, then assign the size to each indexed torrent in order
			var mySize string
			if rowSizes := findSizesFromText(entry.Row); len(rowSizes) > 0 {
				mySize = rowSizes[0]
			} else if len(size) == len(magnetEntries) {
				mySize = size[it]
			}
			if mySize == "" {
				go func() {
					_, _ = i.magnetMetadataAPI.FetchMetadata(ctx, entry.Link)
				}()
			}

			ixt := schema.IndexedTorrent{
				Title:         releaseTitle,
				OriginalTitle: title,
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
		}(it, entry)
	}

	for i := 0; i < len(magnetEntries); i++ {
		it := <-chanIndexedTorrent
		if it.InfoHash != "" {
			indexedTorrents = append(indexedTorrents, it)
		}
	}

	return indexedTorrents, nil
}
