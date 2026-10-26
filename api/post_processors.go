package handler

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ErivanCorreia/torrent-indexer/logging"
	"github.com/ErivanCorreia/torrent-indexer/schema"
	"github.com/ErivanCorreia/torrent-indexer/utils"
	"github.com/hbollon/go-edlib"
)

// CleanupTitleWebsites removes website names from the release title and from
// the page title. Release names are sanitized more thoroughly later by
// NormalizeTitles.
func CleanupTitleWebsites(_ *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for i := range torrents {
		torrents[i].Title = utils.RemoveWebsiteReferences(torrents[i].Title)
		torrents[i].OriginalTitle = utils.RemoveWebsiteReferences(torrents[i].OriginalTitle)
	}
	return torrents
}

// AppendYearToTitle appends the year to the title when it is not already present.
// Example: "Coraline" with year "2009" becomes "Coraline (2009)".
func AppendYearToTitle(_ *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for i := range torrents {
		torrents[i].Title = appendYearToTitle(torrents[i].Title, torrents[i].Year)
	}
	return torrents
}

var yearRE = regexp.MustCompile(`^\d{4}$`)

func appendYearToTitle(title, year string) string {
	title = strings.TrimSpace(title)
	year = strings.TrimSpace(year)

	if title == "" || !yearRE.MatchString(year) {
		return title
	}

	// the year is already part of the title, e.g. "Coraline (2009)"
	if strings.Contains(title, year) {
		return title
	}

	return fmt.Sprintf("%s (%s)", title, year)
}

// SendToSearchIndexer sends the indexed torrents to the search indexer
func SendToSearchIndexer(i *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	go func() {
		_ = i.search.IndexTorrents(torrents)
	}()
	return torrents
}

// FullfilMissingMetadata fills in missing metadata for indexed torrents.
//
// The name returned by the metadata API is the real torrent name (it comes
// from the torrent's info dictionary), so it is preferred over the magnet
// display name whenever it is usable.
func FullfilMissingMetadata(i *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	if !i.magnetMetadataAPI.IsEnabled() {
		return torrents
	}

	return utils.ParallelFlatMap(torrents, func(it schema.IndexedTorrent) ([]schema.IndexedTorrent, error) {
		needsSize := it.Size == ""
		needsTitle := !isUsableReleaseTitle(sanitizeReleaseTitle(it.Title), it.OriginalTitle)
		needsOriginalTitle := it.OriginalTitle == ""
		if !needsSize && !needsTitle && !needsOriginalTitle {
			return []schema.IndexedTorrent{it}, nil
		}

		m, err := i.magnetMetadataAPI.FetchMetadata(r.Context(), it.MagnetLink)
		if err != nil {
			return []schema.IndexedTorrent{it}, nil
		}

		// convert size in bytes to a human-readable format
		it.Size = utils.FormatBytes(m.Size)

		// Use the metadata name when it is a valid release name. If the
		// current title is empty, keep it even when the heuristic rejects it:
		// NormalizeTitles will rebuild it from the page title.
		if name := sanitizeReleaseTitle(m.Name); name != "" {
			if isUsableReleaseTitle(name, it.OriginalTitle) || strings.TrimSpace(it.Title) == "" {
				it.Title = name
			}
		}
		logging.Debug().Str("info_hash", m.InfoHash).Str("size", it.Size).Msg("Retrieved metadata from MagnetMetadataAPI")

		// If files are present, add them to the indexed torrent
		if len(m.Files) > 0 {
			it.Files = make([]schema.File, len(m.Files))
			for fileIdx, file := range m.Files {
				it.Files[fileIdx] = schema.File{
					Path: file.Path,
					Size: utils.FormatBytes(file.Size),
				}
			}
		}

		// If "date" is zero, use the date from metadata if available
		if it.Date.IsZero() {
			it.Date = m.CreatedAt
		}

		return []schema.IndexedTorrent{it}, nil
	})
}

// NormalizeTitles sanitizes the release name and rebuilds it when the magnet
// display name is polluted (adware-prefixed, technical-only, truncated, etc.).
//
// The title field is kept as a release name that Prowlarr/Sonarr/Radarr can
// parse: "<title> (<year>) <episode> <quality> <source> <codec> <hdr> <audio>".
// The original_title field keeps the clean, human-readable page title.
func NormalizeTitles(_ *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for i := range torrents {
		torrents[i].OriginalTitle = sanitizeDisplayTitle(torrents[i].OriginalTitle)

		rawTitle := strings.TrimSpace(torrents[i].Title)
		if rawTitle == "" {
			// Let FallbackPostTitle apply the configured policy for empty
			// titles.
			continue
		}

		releaseTitle := sanitizeReleaseTitle(rawTitle)
		if !isUsableReleaseTitle(releaseTitle, torrents[i].OriginalTitle) {
			releaseTitle = buildReleaseTitle(
				torrents[i].OriginalTitle,
				torrents[i].Year,
				releaseTitle,
				torrents[i].Audio,
			)
		}

		torrents[i].Title = releaseTitle
	}
	return torrents
}

func FallbackPostTitle(i *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	emptyTitles := 0

	for idx := range torrents {
		torrents[idx].Title = strings.TrimSpace(torrents[idx].Title)
		if torrents[idx].Title == "" {
			originalTitle := sanitizeDisplayTitle(torrents[idx].OriginalTitle)
			if originalTitle == "" {
				emptyTitles++
				continue
			}
			if i.config.FallbackTitleEnabled {
				torrents[idx].Title = fmt.Sprintf("[UNSAFE] %s", originalTitle)
			} else {
				emptyTitles++
			}
		}
	}

	if emptyTitles > 0 && !i.magnetMetadataAPI.IsEnabled() {
		logging.WarnWithRequest(r).
			Int("empty_titles", emptyTitles).
			Msg("Some torrents have empty titles. Consider setting up MAGNET_METADATA_API (recommended) or set FALLBACK_TITLE_ENABLED=true.")
	}

	return torrents
}

func AddSimilarityCheck(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	q := normalizeTitleForMatch(r.URL.Query().Get("q"))

	for i, it := range torrents {
		title := normalizeTitleForMatch(fmt.Sprintf("%s %s", it.Title, it.OriginalTitle))
		splitLength := 2
		torrents[i].Similarity = edlib.JaccardSimilarity(title, q, splitLength)
	}

	// remove the ones with zero similarity
	if len(torrents) > 20 && r.URL.Query().Get("filter_results") != "" && r.URL.Query().Get("q") != "" {
		torrents = utils.Filter(torrents, func(it schema.IndexedTorrent) bool {
			return it.Similarity > 0
		})
	}

	// sort by similarity
	slices.SortFunc(torrents, func(i, j schema.IndexedTorrent) int {
		return int((j.Similarity - i.Similarity) * 1000)
	})

	return torrents
}

// ApplyLimit limits the number of results based on the "limit" query parameter
func ApplyLimit(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return torrents
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return torrents
	}

	if len(torrents) > limit {
		return torrents[:limit]
	}

	return torrents
}

// ApplySorting sorts the results based on "sortBy" and "sortDirection" query parameters
func ApplySorting(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	sortBy := r.URL.Query().Get("sortBy")
	q := r.URL.Query().Get("q")
	if sortBy == "" {
		if q == "" {
			sortBy = "date"
		} else {
			// for search queries, default to sorting by similarity
			sortBy = "similarity"
		}
	}

	sortDirection := r.URL.Query().Get("sortDirection")
	ascending := sortDirection == "asc"

	slices.SortFunc(torrents, func(i, j schema.IndexedTorrent) int {
		var cmp int
		switch sortBy {
		case "title":
			cmp = strings.Compare(strings.ToLower(i.Title), strings.ToLower(j.Title))
		case "original_title":
			cmp = strings.Compare(strings.ToLower(i.OriginalTitle), strings.ToLower(j.OriginalTitle))
		case "year":
			cmp = strings.Compare(i.Year, j.Year)
		case "date":
			if i.Date.Before(j.Date) {
				cmp = -1
			} else if i.Date.After(j.Date) {
				cmp = 1
			}
		case "seed_count", "seeders":
			cmp = i.SeedCount - j.SeedCount
		case "leech_count", "leechers":
			cmp = i.LeechCount - j.LeechCount
		case "size":
			// Parse size strings to bytes for accurate comparison
			iBytes := utils.ParseSize(i.Size)
			jBytes := utils.ParseSize(j.Size)
			if iBytes < jBytes {
				cmp = -1
			} else if iBytes > jBytes {
				cmp = 1
			}
		case "similarity":
			if i.Similarity < j.Similarity {
				cmp = -1
			} else if i.Similarity > j.Similarity {
				cmp = 1
			}
		default:
			return 0
		}

		if ascending {
			return cmp
		}
		return -cmp
	})

	return torrents
}

// FilterBy filters the results based on various query parameters
func FilterBy(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	// Parse filter parameters
	audioParam := r.URL.Query().Get("audio")
	yearParam := r.URL.Query().Get("year")
	imdbParam := r.URL.Query().Get("imdb")

	var requestedAudioTags []string
	if audioParam != "" {
		parts := strings.Split(audioParam, ",")
		for _, p := range parts {
			if t := strings.TrimSpace(strings.ToLower(p)); t != "" {
				requestedAudioTags = append(requestedAudioTags, t)
			}
		}
	}

	// If no filters are active, return original list
	if len(requestedAudioTags) == 0 && yearParam == "" && imdbParam == "" {
		return torrents
	}

	return utils.Filter(torrents, func(it schema.IndexedTorrent) bool {
		// Filter by Audio
		if len(requestedAudioTags) > 0 {
			hasAudio := false
			for _, audio := range it.Audio {
				if slices.Contains(requestedAudioTags, strings.ToLower(audio.String())) {
					hasAudio = true
					break
				}
			}
			if !hasAudio {
				return false
			}
		}

		// Filter by Year
		if yearParam != "" {
			if it.Year != yearParam {
				return false
			}
		}

		// Filter by IMDB ID
		if imdbParam != "" {
			if !strings.Contains(it.IMDB, imdbParam) {
				return false
			}
		}

		return true
	})
}
