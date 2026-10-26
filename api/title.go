package handler

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/ErivanCorreia/torrent-indexer/schema"
	"github.com/ErivanCorreia/torrent-indexer/utils"
)

var (
	// adSeparatorRE normalizes the "-." / ".-" artifacts left by adware
	// domains, e.g. "MOVIE-.1080p" -> "MOVIE.1080p".
	adSeparatorRE = regexp.MustCompile(`\s*(?:\.-|-\s*\.)\s*`)
	// separatorRE normalizes pipes and underscores used as separators.
	separatorRE = regexp.MustCompile(`\s*[|_]+\s*`)
	multiDotRE  = regexp.MustCompile(`(?:\.\s*){2,}`)
	spaceRE     = regexp.MustCompile(`\s+`)
	emptyPairRE = regexp.MustCompile(`(?:\[\s*\]|\(\s*\)|\{\s*\})`)

	nonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

	episodeTokenRE = regexp.MustCompile(`(?i)s\d{1,3}(?:e\d{1,4})?`)
	resolutionRE   = regexp.MustCompile(`(?i)(?:2160p|1080p|720p|480p|4k|uhd|full[-. ]?hd)`)
	sourceRE       = regexp.MustCompile(`(?i)(?:web[-. ]?dl|webrip|blu[-. ]?ray|brrip|bdrip|remux|dvdrip|hdtv|hdrip)`)
	codecRE        = regexp.MustCompile(`(?i)(?:x264|x265|h\.?264|h\.?265|hevc|avc|av1|xvid|divx)`)
	hdrRE          = regexp.MustCompile(`(?i)(?:hdr10\+?|hdr|dolby[ .]?vision|dovi|dv|sdr)`)
	audioCodecRE   = regexp.MustCompile(`(?i)(?:ac3|eac3|ddp?\d*(?:[ .]\d+)?|dts|truehd|atmos|aac\d*(?:[ .]\d+)?|flac|mp3|opus|(?:2|5|7)\.1|2\.0)`)

	// technicalTokenRE extracts the tokens that Sonarr/Radarr use to identify
	// the quality of a release. Token boundaries are validated manually so
	// that tokens separated by a single character are not skipped.
	technicalTokenRE = regexp.MustCompile(`(?i)(?:s\d{1,3}(?:e\d{1,4})?|2160p|1080p|720p|480p|4k|uhd|full[-. ]?hd|web[-. ]?dl|webrip|blu[-. ]?ray|brrip|bdrip|remux|dvdrip|hdtv|hdrip|x264|x265|h\.?264|h\.?265|hevc|avc|av1|xvid|divx|hdr10\+?|hdr|dolby[ .]?vision|dovi|dv|sdr|dual|dublado|nacional|legendado|ac3|eac3|ddp?\d*(?:[ .]\d+)?|dts|truehd|atmos|aac\d*(?:[ .]\d+)?|flac|mp3|opus|(?:2|5|7)\.1|2\.0)`)

	technicalTokenFullRE = regexp.MustCompile(`(?i)^(?:s\d{1,3}(?:e\d{1,4})?|2160p|1080p|720p|480p|4k|uhd|full[-. ]?hd|web[-. ]?dl|webrip|blu[-. ]?ray|brrip|bdrip|remux|dvdrip|hdtv|hdrip|x264|x265|h\.?264|h\.?265|hevc|avc|av1|xvid|divx|hdr10\+?|hdr|dolby[ .]?vision|dovi|dv|sdr|dual|dublado|nacional|legendado|ac3|eac3|ddp?\d*(?:[ .]\d+)?|dts|truehd|atmos|aac\d*(?:[ .]\d+)?|flac|mp3|opus|(?:2|5|7)\.1|2\.0)$`)
)

var diacriticsReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y", "ÿ", "y",
)

var titleStopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "o": {}, "os": {}, "as": {},
	"de": {}, "da": {}, "do": {}, "das": {}, "dos": {},
	"e": {}, "of": {}, "and": {}, "in": {}, "on": {}, "for": {}, "to": {},
	"part": {}, "parte": {},
}

var technicalTokens = map[string]struct{}{
	// containers
	"mkv": {}, "mp4": {}, "avi": {}, "m4v": {}, "mov": {}, "wmv": {},
	// source / quality
	"web": {}, "dl": {}, "webdl": {}, "webrip": {}, "rip": {}, "blu": {}, "ray": {}, "bluray": {},
	"brrip": {}, "bdrip": {}, "remux": {}, "dvdrip": {}, "dvd": {}, "hdtv": {}, "hdrip": {},
	"cam": {}, "ts": {}, "tc": {}, "scr": {}, "screener": {},
	"1080p": {}, "720p": {}, "2160p": {}, "480p": {}, "4k": {}, "uhd": {}, "full": {}, "fullhd": {}, "hd": {}, "sd": {},
	// codecs
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "hevc": {}, "avc": {}, "av1": {}, "xvid": {}, "divx": {},
	"10bit": {}, "8bit": {},
	// audio
	"aac": {}, "ac3": {}, "eac3": {}, "dd": {}, "ddp": {}, "dts": {}, "truehd": {}, "atmos": {},
	"flac": {}, "mp3": {}, "opus": {}, "dual": {}, "dublado": {}, "legendado": {}, "nacional": {},
	"audio": {}, "sub": {}, "subs": {}, "legenda": {},
	"multi": {}, "latino": {}, "castellano": {}, "espanhol": {}, "espanol": {},
	"portugues": {}, "ingles": {}, "english": {}, "spanish": {},
	// hdr
	"hdr": {}, "hdr10": {}, "sdr": {}, "dv": {}, "dovi": {}, "dolby": {}, "vision": {},
	// release / page noise
	"torrent": {}, "download": {}, "baixar": {}, "assistir": {}, "online": {}, "completo": {},
	"temporada": {}, "season": {}, "episodio": {}, "episode": {}, "ep": {}, "serie": {}, "filme": {},
	"movie": {}, "tv": {}, "proper": {}, "repack": {}, "extended": {}, "remastered": {}, "uncut": {},
	"limited": {}, "internal": {}, "dubbed": {}, "subbed": {},
}

// sanitizeReleaseTitle normalizes a release name coming from a magnet display
// name (dn) or from the magnet metadata API.
func sanitizeReleaseTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	// Magnet display names may be (double) URL-encoded. PathUnescape is used
	// instead of QueryUnescape so a literal "+" is preserved.
	if decoded, err := url.PathUnescape(title); err == nil {
		title = decoded
	}

	title = strings.ReplaceAll(title, "\u00a0", " ")
	title = utils.RemoveWebsiteReferences(title)

	title = adSeparatorRE.ReplaceAllString(title, ".")
	title = separatorRE.ReplaceAllString(title, " ")
	title = multiDotRE.ReplaceAllString(title, ".")
	title = spaceRE.ReplaceAllString(title, " ")

	title = strings.Trim(title, " .-_|")
	title = emptyPairRE.ReplaceAllString(title, "")
	title = strings.TrimSpace(strings.Trim(title, " .-_|"))

	return title
}

// sanitizeDisplayTitle normalizes the page title that is exposed as
// original_title. It keeps the human-readable format (spaces) and only removes
// website/page noise.
func sanitizeDisplayTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	title = strings.ReplaceAll(title, "\u00a0", " ")
	title = utils.RemoveWebsiteReferences(title)
	title = strings.TrimRight(strings.TrimSpace(title), " -|–—")

	// Remove page noise before normalizing separators, otherwise a suffix
	// such as " | Download" becomes " Download" and is no longer recognized.
	for _, suffix := range []string{
		" - Download", " – Download", " | Download",
		" - Assistir Online", " | Assistir Online",
		" - Torrent", " | Torrent", " – Torrent",
		" - Baixar", " | Baixar",
	} {
		title = strings.TrimSuffix(strings.TrimSpace(title), suffix)
	}

	title = separatorRE.ReplaceAllString(title, " ")
	title = emptyPairRE.ReplaceAllString(title, "")
	title = spaceRE.ReplaceAllString(title, " ")

	return strings.Trim(strings.TrimSpace(title), "-| ")
}

// isUsableReleaseTitle reports whether a release name is safe to expose as the
// title field. It rejects empty names, names made only of technical tokens and
// names that start with quality tokens (the usual shape of an adware-prefixed
// magnet display name).
func isUsableReleaseTitle(releaseTitle, originalTitle string) bool {
	releaseTitle = strings.TrimSpace(releaseTitle)
	if releaseTitle == "" {
		return false
	}

	normalized := normalizeTitleForMatch(releaseTitle)
	if normalized == "" {
		return false
	}

	if hasTechnicalPrefix(normalized) {
		return false
	}

	// Single-character titles (e.g. "A", "X") are only accepted when the page
	// title confirms them.
	if len(normalized) < 2 {
		return normalizeTitleForMatch(originalTitle) == normalized
	}

	if !hasMeaningfulToken(normalized) {
		return false
	}

	trimmed := strings.TrimSpace(releaseTitle)
	if strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "-") {
		return false
	}

	return true
}

// buildReleaseTitle builds a release name in the format expected by
// Prowlarr/Sonarr/Radarr: "<title> (<year>) <quality tokens>".
func buildReleaseTitle(originalTitle, year, releaseTitle string, audio []schema.Audio) string {
	base := sanitizeDisplayTitle(originalTitle)
	if base == "" {
		base = sanitizeReleaseTitle(releaseTitle)
	}
	base = appendYearToTitle(base, year)
	if base == "" {
		// Never emit an audio-only title such as "DUBLADO" when there is no
		// usable title at all. Let FallbackPostTitle handle this case.
		return ""
	}

	tokens := extractTechnicalTokens(sanitizeReleaseTitle(releaseTitle))
	if len(tokens) == 0 {
		tokens = audioReleaseTokens(audio)
	}

	// Do not duplicate tokens that are already present in the base, e.g. when
	// there is no separate page title and the base is the sanitized release
	// name itself.
	baseTokens := make(map[string]struct{})
	for _, token := range extractTechnicalTokens(base) {
		baseTokens[dedupKey(token)] = struct{}{}
	}
	tokens = slices.DeleteFunc(tokens, func(token string) bool {
		_, ok := baseTokens[dedupKey(token)]
		return ok
	})

	if len(tokens) == 0 {
		return strings.TrimSpace(base)
	}

	return strings.TrimSpace(base + " " + strings.Join(tokens, " "))
}

func audioReleaseTokens(audio []schema.Audio) []string {
	brazilian, other := false, false
	for _, a := range audio {
		switch a.String() {
		case "brazilian":
			brazilian = true
		case "":
			// unknown audio, ignore
		default:
			other = true
		}
	}

	if brazilian && other {
		return []string{"DUAL"}
	}
	if brazilian {
		return []string{"DUBLADO"}
	}

	return nil
}

// extractTechnicalTokens extracts, canonicalizes, deduplicates and orders the
// technical tokens of a release name.
func extractTechnicalTokens(title string) []string {
	if title == "" {
		return nil
	}

	matches := technicalTokenRE.FindAllStringIndex(title, -1)
	if len(matches) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))

	for _, m := range matches {
		start, end := m[0], m[1]

		if start > 0 && isAlphaNumericByte(title[start-1]) {
			continue
		}
		if end < len(title) && isAlphaNumericByte(title[end]) {
			continue
		}

		token := canonicalTechnicalToken(title[start:end])
		if token == "" {
			continue
		}

		key := dedupKey(token)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		return nil
	}

	slices.SortStableFunc(tokens, func(a, b string) int {
		return technicalTokenRank(a) - technicalTokenRank(b)
	})

	return tokens
}

func technicalTokenRank(token string) int {
	switch {
	case episodeTokenRE.MatchString(token):
		return 0
	case resolutionRE.MatchString(token):
		return 1
	case sourceRE.MatchString(token):
		return 2
	case codecRE.MatchString(token):
		return 3
	case hdrRE.MatchString(token):
		return 4
	case audioCodecRE.MatchString(token):
		return 6
	default:
		// audio language markers (DUAL, DUBLADO, NACIONAL, ...)
		return 5
	}
}

func canonicalTechnicalToken(token string) string {
	token = strings.TrimSpace(token)
	lower := strings.ToLower(token)
	key := dedupKey(token)

	if episodeTokenRE.MatchString(token) {
		return strings.ToUpper(token)
	}

	switch lower {
	case "2160p", "1080p", "720p", "480p", "4k", "uhd":
		return lower
	}

	switch key {
	case "FULLHD":
		return "1080p"
	case "WEBDL":
		return "WEB-DL"
	case "WEBRIP":
		return "WEBRip"
	case "BLURAY":
		return "BluRay"
	case "BRRIP":
		return "BRRip"
	case "BDRIP":
		return "BDRip"
	case "REMUX":
		return "REMUX"
	case "DVDRIP":
		return "DVDRip"
	case "HDTV":
		return "HDTV"
	case "HDRIP":
		return "HDRip"
	case "X264", "H264":
		return "x264"
	case "X265", "H265":
		return "x265"
	case "HEVC":
		return "HEVC"
	case "AVC":
		return "AVC"
	case "AV1":
		return "AV1"
	case "XVID":
		return "XviD"
	case "DIVX":
		return "DivX"
	case "HDR10":
		return "HDR10"
	case "HDR10+":
		return "HDR10+"
	case "HDR":
		return "HDR"
	case "DOVI", "DOLBYVISION":
		return "DoVi"
	case "SDR":
		return "SDR"
	case "DUAL":
		return "DUAL"
	case "DUBLADO":
		return "DUBLADO"
	case "NACIONAL":
		return "NACIONAL"
	case "LEGENDADO":
		return "LEGENDADO"
	case "AC3":
		return "AC3"
	case "EAC3":
		return "EAC3"
	case "DTS":
		return "DTS"
	case "TRUEHD":
		return "TrueHD"
	case "ATMOS":
		return "Atmos"
	case "FLAC":
		return "FLAC"
	case "MP3":
		return "MP3"
	case "OPUS":
		return "Opus"
	}

	switch {
	case strings.HasPrefix(key, "DDP"), strings.HasPrefix(key, "DD"), strings.HasPrefix(key, "AAC"):
		return strings.ToUpper(token)
	}

	return strings.ToUpper(token)
}

func dedupKey(token string) string {
	replacer := strings.NewReplacer(".", "", " ", "", "-", "", "_", "")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(token)))
}

func hasTechnicalPrefix(normalized string) bool {
	for _, token := range strings.Fields(normalized) {
		if _, ok := titleStopWords[token]; ok {
			continue
		}
		return isTechnicalToken(token)
	}
	return false
}

func hasMeaningfulToken(normalized string) bool {
	for _, token := range strings.Fields(normalized) {
		if len(token) < 2 {
			continue
		}
		if _, ok := titleStopWords[token]; ok {
			continue
		}
		if isTechnicalToken(token) {
			continue
		}
		if isAllDigits(token) && len(token) < 4 {
			continue
		}
		return true
	}
	return false
}

func isTechnicalToken(token string) bool {
	if _, ok := technicalTokens[token]; ok {
		return true
	}
	if technicalTokenFullRE.MatchString(token) {
		return true
	}
	return isAllDigits(token) && len(token) < 4
}

func normalizeTitleForMatch(title string) string {
	title = strings.ToLower(title)
	title = diacriticsReplacer.Replace(title)
	title = nonAlnumRE.ReplaceAllString(title, " ")
	return strings.Join(strings.Fields(title), " ")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isAlphaNumericByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
