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

	"github.com/ErivanCorreia/torrent-indexer/logging"
	"github.com/ErivanCorreia/torrent-indexer/magnet"
	"github.com/ErivanCorreia/torrent-indexer/schema"
	goscrape "github.com/ErivanCorreia/torrent-indexer/scrape"
	"github.com/ErivanCorreia/torrent-indexer/utils"
)

var vacaTorrent = IndexerMeta{
	Label: "vaca_torrent",
	// The website was rebranded to "Vaqueiro Filmes" and moved to a new domain.
	// The old domain only redirects to the new one, it is kept as a fallback.
	URL: utils.GetIndexerURLFromEnv("INDEXER_VACA_TORRENT_URL", "https://vaqueirofilmes.com/"),
	FallbackURLs: []string{
		utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_FALLBACK_URL", "https://vaqueirofilmes1.com/"),
	},
	// The search results are rendered by javascript on the page, the theme
	// exposes a JSON API that is used by the search box of the website.
	SearchURL: utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_SEARCH_URL",
		"wp-admin/admin-ajax.php?action=search_posts&lang=pt-BR&s="),
	PagePattern: utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_PAGE_PATTERN", "pt/page/%s/"),
}

// vacaCatalogPath is the path of the catalog page, it holds the listing of the
// movies and the series.
var vacaCatalogPath = utils.GetEnvOrDefault("INDEXER_VACA_TORRENT_CATALOG_PATH", "pt/")

var (
	vacaBrTagRe   = regexp.MustCompile(`(?i)<br\s*/?>`)
	vacaHtmlTagRe = regexp.MustCompile(`<[^>]*>`)
	vacaQualityRe = regexp.MustCompile(`(?i)\b(\d{3,4}p)\b`)
	vacaYearRe    = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	vacaAdGateRe  = regexp.MustCompile(`(?i)(preparando seu link|desbloqueie o acesso|aguarde \d+ segundos|abrir o an[uú]ncio)`)
)

// vacaSearchResult is an item of the JSON search API of the website.
type vacaSearchResult struct {
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Type      string `json:"type"`
	Year      string `json:"year"`
	Link      string `json:"link"`
	Idioma    string `json:"idioma"`
	IMDB      string `json:"imdb"`
}

// buildVacaTorrentURL builds the target URL for the given base URL.
func buildVacaTorrentURL(baseURL, q, page string) string {
	if q != "" {
		return fmt.Sprintf("%s%s%s", baseURL, vacaTorrent.SearchURL, url.QueryEscape(q))
	}

	if page == "" || page == "1" {
		return fmt.Sprintf("%s%s", baseURL, vacaCatalogPath)
	}

	return fmt.Sprintf("%s%s", baseURL, fmt.Sprintf(vacaTorrent.PagePattern, page))
}

// extractVacaTorrentLinks extracts the detail page links from the catalog.
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

	// the cards of the catalog point to the page of the movie or of the series
	doc.Find(".i-tem_ht a[href], a.i-tem_ht[href]").Each(func(_ int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if !exists {
			return
		}
		if strings.Contains(link, "/movie/") || strings.Contains(link, "/tv-shows/") {
			addLink(link)
		}
	})

	// fallback for layouts that do not use the card class
	if len(links) == 0 {
		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			if strings.Contains(link, "/movie/") || strings.Contains(link, "/tv-shows/") {
				addLink(link)
			}
		})
	}

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

// extractVacaTitle extracts the title of the movie page, without the release
// year that is rendered inside the heading.
func extractVacaTitle(doc *goquery.Document) string {
	heading := doc.Find("h1").First()
	if heading.Length() == 0 {
		return ""
	}

	clone := heading.Clone()
	clone.Find(".release_date, .tag-year").Remove()

	return strings.TrimSpace(clone.Text())
}

// extractVacaYear extracts the release year of the movie page.
func extractVacaYear(doc *goquery.Document) string {
	tag := strings.TrimSpace(doc.Find(".release_date").First().Text())
	if match := vacaYearRe.FindString(tag); match != "" {
		return match
	}

	return strings.TrimSpace(doc.Find(".tag-year").First().Text())
}

// extractVacaLinksPageURL returns the URL of the page that holds the download
// links of the movie.
//
// The button that opens the list of links encodes the URL in the data-u
// attribute, using base64.
func extractVacaLinksPageURL(doc *goquery.Document, baseURL string) string {
	if dataU, exists := doc.Find(".mf-top-btn[data-u]").First().Attr("data-u"); exists {
		// the padding may be missing
		if decoded, err := utils.Base64Decode(dataU + strings.Repeat("=", (4-len(dataU)%4)%4)); err == nil {
			if strings.Contains(decoded, "://") {
				return decoded
			}
		}
	}

	if href, exists := doc.Find("a.mf-card[href]").First().Attr("href"); exists {
		return absoluteVacaURL(baseURL, href)
	}

	return ""
}

// absoluteVacaURL resolves a possibly relative URL against the base URL.
func absoluteVacaURL(baseURL, link string) string {
	if link == "" {
		return ""
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return link
	}

	if parsed.IsAbs() {
		return parsed.String()
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return link
	}

	return base.ResolveReference(parsed).String()
}

// extractVacaAudio extracts the audio languages from a list of languages
// separated by "/", "|" or ",".
func extractVacaAudio(value string) []schema.Audio {
	var audio []schema.Audio

	langs := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == '|' || r == ',' || r == '•'
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

// vacaMagnetEntry holds the data of a single download of the movie.
type vacaMagnetEntry struct {
	Link  string
	Row   string
	Qual  string
	Size  string
	Audio []schema.Audio
	Subs  []string
}

// parseVacaDownloadLabel parses the label of a download button, e.g.
// "Português | Inglês • 1080p • 2.90 GB".
func parseVacaDownloadLabel(label string) (qual string, size string, audio []schema.Audio) {
	parts := strings.Split(label, "•")

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch {
		case i == 0:
			audio = extractVacaAudio(part)
			continue
		case size == "":
			if sizes := findSizesFromText(part); len(sizes) > 0 {
				size = sizes[0]
				continue
			}
			fallthrough
		default:
			// the quality may come with extra info, e.g. "2160p x265 HDR"
			if qual == "" && vacaQualityRe.MatchString(part) {
				qual = vacaQualityRe.FindString(part)
			}
		}
	}

	if qual == "" {
		qual = vacaQualityRe.FindString(label)
	}

	return qual, size, audio
}

// extractVacaSubs extracts the subtitles available for a download.
func extractVacaSubs(root *goquery.Selection) []string {
	var subs []string

	root.Find("[data-subs]").Each(func(_ int, s *goquery.Selection) {
		raw, exists := s.Attr("data-subs")
		if !exists {
			return
		}

		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}

		// the attribute holds a json array, e.g. ["PT-BR", "EN"]
		var langs []string
		if err := json.Unmarshal([]byte(raw), &langs); err != nil {
			langs = strings.Split(raw, ",")
		}

		for _, lang := range langs {
			lang = strings.Trim(strings.TrimSpace(lang), `"'`)
			if lang == "" {
				continue
			}
			subs = append(subs, lang)
		}
	})

	return subs
}

// extractVacaMagnetEntries extracts the downloads and their metadata from the
// page that holds the links of the movie.
func extractVacaMagnetEntries(doc *goquery.Document, pageAudio []schema.Audio) []vacaMagnetEntry {
	var entries []vacaMagnetEntry

	docSubs := extractVacaSubs(doc.Selection)

	// the download buttons hold the label with the quality, the audio and the
	// size of the file
	doc.Find("a.ml-btn-dl").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		label := strings.Join(strings.Fields(extractVacaText(s)), " ")
		qual, size, audio := parseVacaDownloadLabel(label)

		if len(audio) == 0 {
			audio = pageAudio
		}

		// each download has its own subtitles button
		subs := extractVacaSubs(s.Closest(".dl-btn-wrap"))
		if len(subs) == 0 {
			subs = docSubs
		}

		entries = append(entries, vacaMagnetEntry{
			Link:  href,
			Row:   label,
			Qual:  qual,
			Size:  size,
			Audio: audio,
			Subs:  subs,
		})
	})

	// fallback for layouts where the magnet links are available directly
	if len(entries) == 0 {
		doc.Find("a[href^=\"magnet:\"]").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if href == "" {
				return
			}

			entries = append(entries, vacaMagnetEntry{
				Link:  href,
				Audio: pageAudio,
				Subs:  docSubs,
			})
		})
	}

	return entries
}

// resolveVacaMagnet follows the download link and looks for the magnet link.
//
// The website serves the magnet link through a redirect that is protected by
// an advertisement gate. When the gate is in place, the response is the
// advertisement page and no magnet is returned.
func (i *Indexer) resolveVacaMagnet(ctx context.Context, link string) (string, bool) {
	body, err := i.requester.GetContent(ctx, link)
	if err != nil {
		logging.Debug().Err(err).Str("url", link).Msg("Failed to fetch the download link")
		return "", false
	}

	content := string(body)

	if vacaAdGateRe.MatchString(content) {
		return "", true
	}

	magnetRe := regexp.MustCompile(`magnet:\?[^"'\s<>\\]+`)
	match := magnetRe.FindString(content)
	if match == "" {
		return "", false
	}

	return match, false
}

// searchVacaTorrent searches the website through its JSON search API.
func (i *Indexer) searchVacaTorrent(ctx context.Context, q string) (string, []string, error) {
	var lastErr error

	for _, baseURL := range candidateURLs(vacaTorrent) {
		targetURL := fmt.Sprintf("%s%s%s", baseURL, vacaTorrent.SearchURL, url.QueryEscape(q))

		body, err := i.requester.GetContent(ctx, targetURL)
		if err != nil {
			lastErr = err
			logging.Warn().Err(err).
				Str("url", targetURL).
				Msg("Failed to search, trying the next candidate URL")
			continue
		}

		var results []vacaSearchResult
		if err := json.Unmarshal(body, &results); err != nil {
			lastErr = fmt.Errorf("failed to parse the search response of %s: %w", targetURL, err)
			logging.Warn().Err(err).
				Str("url", targetURL).
				Msg("Failed to parse the search response, trying the next candidate URL")
			continue
		}

		var links []string
		for _, result := range results {
			if result.Link == "" {
				continue
			}
			links = append(links, result.Link)
		}

		if len(links) == 0 {
			lastErr = fmt.Errorf("no results found at %s", targetURL)
			continue
		}

		return targetURL, links, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate URLs configured")
	}

	return "", nil, fmt.Errorf("failed to search the indexer: %w", lastErr)
}

// listVacaTorrent lists the movies of the catalog page.
func (i *Indexer) listVacaTorrent(ctx context.Context, page string) (string, []string, error) {
	var lastErr error

	for _, baseURL := range candidateURLs(vacaTorrent) {
		targetURL := buildVacaTorrentURL(baseURL, "", page)

		doc, err := i.fetchDocument(ctx, targetURL)
		if err != nil {
			lastErr = err
			logging.Warn().Err(err).
				Str("url", targetURL).
				Msg("Failed to fetch document, trying the next candidate URL")
			continue
		}

		links := extractVacaTorrentLinks(doc)
		if len(links) == 0 {
			lastErr = fmt.Errorf("no results found at %s", targetURL)
			logging.Warn().
				Str("url", targetURL).
				Msg("No results found, trying the next candidate URL")
			continue
		}

		return targetURL, links, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate URLs configured")
	}

	return "", nil, fmt.Errorf("failed to fetch any candidate URL: %w", lastErr)
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

	var links []string
	var targetURL string
	var err error

	if q != "" {
		targetURL, links, err = i.searchVacaTorrent(ctx, q)
	} else {
		targetURL, links, err = i.listVacaTorrent(ctx, page)
	}

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
		return getTorrentsVacaTorrent(ctx, i, link)
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
		logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode response")
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
	LinksPage     string
}

// parseVacaTorrentDetails extracts the metadata of a movie page.
func parseVacaTorrentDetails(doc *goquery.Document, baseURL string) vacaTorrentDetails {
	title := extractVacaTitle(doc)

	// the year of the release is rendered inside the heading
	year := extractVacaYear(doc)

	// find any link from imdb
	imdbLink := getIMDBLinkFromDocument(doc)

	return vacaTorrentDetails{
		Title:         title,
		OriginalTitle: title,
		Year:          year,
		Audio:         extractVacaAudio(doc.Find(".ml-hero-meta, .detail-item").First().Text()),
		Entries:       nil,
		IMDB:          imdbLink,
		Date:          getPublishedDateFromMeta(doc),
		LinksPage:     extractVacaLinksPageURL(doc, baseURL),
	}
}

func getTorrentsVacaTorrent(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent

	doc, err := getDocument(ctx, i, link, link)
	if err != nil {
		return nil, err
	}

	details := parseVacaTorrentDetails(doc, link)
	title := details.Title
	year := details.Year
	imdbLink := details.IMDB
	date := details.Date

	// the downloads are listed on a separate page
	if details.LinksPage == "" {
		return indexedTorrents, nil
	}

	linksDoc, err := getDocument(ctx, i, details.LinksPage, link)
	if err != nil {
		return nil, err
	}

	entries := extractVacaMagnetEntries(linksDoc, details.Audio)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// only the downloads with a magnet link can be indexed
	var indexedEntries []vacaMagnetEntry
	gated := 0

	for _, entry := range entries {
		if entry.Link == "" {
			continue
		}

		// the magnet link may be served directly by the page, otherwise it is
		// behind the redirect that is protected by the advertisement gate
		if !strings.HasPrefix(entry.Link, "magnet:") {
			magnetLink, isGated := i.resolveVacaMagnet(ctx, entry.Link)
			if isGated {
				gated++
			}
			entry.Link = magnetLink
		}

		if entry.Link == "" {
			continue
		}

		indexedEntries = append(indexedEntries, entry)
	}

	if gated > 0 {
		logging.Warn().
			Int("gated", gated).
			Str("url", link).
			Msg("The magnet links of this title are protected by an advertisement gate, they were skipped")
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

			processedTitle := processVacaTorrentTitle(title, "")

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

func processVacaTorrentTitle(title string, season string) string {
	// Remove common suffixes
	title = strings.Replace(title, " – Download", "", -1)
	title = strings.Replace(title, " - Download", "", -1)
	title = strings.TrimSpace(title)

	// Add season if present
	if season != "" {
		title = fmt.Sprintf("%s S%s - %sª Temporada", title, fmt.Sprintf("%02s", season), season)
	}

	return title
}
