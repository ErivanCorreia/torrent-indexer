package utils

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var commonTLDs = []string{
	".com",
	".net",
	".org",
	".info",
	".biz",
	".co",
	".io",
	".xyz",
	".me",
	".tv",
	".cc",
	".us",
	".online",
	".site",
	".la",
	".se",
	".to",
}

var commonSubdomains = []string{
	"", // no prefix
	"www.",
}

var commonWebsiteSLDs = []string{
	"bludv",
	"torrentdosfilmes",
	"comando",
	"comandotorrents",
	"comandohds",
	"redetorrent",
	"torrenting",
	"baixarfilmesdubladosviatorrent",
	"hidratorrents",
	"wolverdonfilmes",
	"starckfilmes",
	"rapidotorrents",
	"sitedetorrents",
	"vamostorrent",
	"AZTORRENTS",
	// adware domains commonly embedded in magnet display names
	"baixarapido",
	"seuvideo",
	"systemads",
	"brplayer",
}

// genericDomainRE matches domain-like tokens that are not part of the known
// list above, e.g. "baixarapido.com" or "www.example.xyz".
var genericDomainRE = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?[a-z0-9][a-z0-9-]{2,}\.(?:com|net|org|info|biz|tv|xyz|me|co|la|se|to|link|online|site|club|top|vip|pro|app|dev|ws|sx|pw|icu|space|website|moe|live|cloud|store|shop|one|fun|name|blog|in|us|cc|br)(?:\.br)?\b`)

var websitePatterns = []string{
	`\[\s*ACESSE\s+%s\s*\]`,
	`\[?\s*%s(\s*\])?`,
}

var regexesOnce sync.Once
var regexes []*regexp.Regexp

func getRegexes() []*regexp.Regexp {
	regexesOnce.Do(func() {
		var websites strings.Builder
		websites.WriteString("(?i)(")
		for _, prefix := range commonSubdomains {
			for _, name := range commonWebsiteSLDs {
				for _, tld := range commonTLDs {
					websites.WriteString(fmt.Sprintf("%s%s%s|", prefix, name, tld))
				}
			}
		}
		// remove the last pipe character
		websites.WriteString(")")

		websitesStr := websites.String()
		websitesStr = strings.Replace(websitesStr, "|)", ")", 1)

		for _, pattern := range websitePatterns {
			regexes = append(regexes, regexp.MustCompile(fmt.Sprintf(pattern, websitesStr)))
		}
	})
	return regexes
}

// RemoveKnownWebsites removes known website patterns from the title.
// It uses a set of common prefixes, names, and TLDs to identify and remove
// website references from the title.
// It also removes any common patterns like "[ ACESSE bludv.com ]" or
// "[ bludv.se ]" or "bludv.xyz".
func RemoveKnownWebsites(title string) string {
	regexes := getRegexes()
	for _, re := range regexes {
		title = re.ReplaceAllString(title, "")
	}
	title = strings.TrimSpace(title)
	return title
}

// RemoveWebsiteReferences removes both the known website names and generic
// domain-like tokens from a release title.
//
// Generic domains are only removed when they are delimited like a website
// reference: by the start/end of the string, spaces, brackets, pipes,
// hyphens, or a dot after a non-ambiguous TLD. This avoids destroying
// legitimate release tokens such as "The.Office.US.S01E01".
func RemoveWebsiteReferences(title string) string {
	title = RemoveKnownWebsites(title)

	matches := genericDomainRE.FindAllStringIndex(title, -1)
	if len(matches) == 0 {
		return strings.TrimSpace(title)
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		if !isWebsiteBoundary(title, m[0], m[1]) {
			continue
		}
		b.WriteString(title[last:m[0]])
		b.WriteByte(' ')
		last = m[1]
	}
	b.WriteString(title[last:])

	return strings.TrimSpace(b.String())
}

// ambiguousDomainTLDs are TLDs that can also legitimately appear as a word in
// a release title, e.g. "The.Office.US" or "The.Office.TV". A domain preceded
// by a dot is only removed when its TLD is not ambiguous.
var ambiguousDomainTLDs = map[string]struct{}{
	"us": {}, "in": {}, "me": {}, "tv": {}, "co": {},
	"la": {}, "se": {}, "to": {}, "cc": {}, "br": {},
}

// hasAmbiguousTLD reports whether the domain is a two-label domain whose TLD
// is also a common word in release titles (e.g. "Office.US", "Office.TV").
func hasAmbiguousTLD(domain string) bool {
	parts := strings.Split(strings.ToLower(domain), ".")
	if len(parts) != 2 {
		return false
	}
	_, ambiguous := ambiguousDomainTLDs[parts[1]]
	return ambiguous
}

// isWebsiteBoundary reports whether the domain candidate at [start:end] is
// delimited like a website reference and can be safely removed.
func isWebsiteBoundary(title string, start, end int) bool {
	token := strings.ToLower(title[start:end])
	explicitURL := strings.Contains(token, "www.") || strings.HasPrefix(token, "http")
	ambiguous := hasAmbiguousTLD(token)

	// The character after the domain must look like a boundary: the end of
	// the string, a separator, or (for non-ambiguous TLDs) a dot.
	afterOK := explicitURL || end == len(title) || isWebsiteSeparator(title[end]) ||
		(title[end] == '.' && !ambiguous)
	if !afterOK {
		return false
	}

	if explicitURL || start == 0 || isWebsiteSeparator(title[start-1]) {
		return true
	}

	// A domain preceded by a dot is usually an adware suffix, but it can also
	// be part of a release title. Two-label domains with an ambiguous TLD
	// (e.g. "Office.US") are preserved.
	return title[start-1] == '.' && !ambiguous
}

func isWebsiteSeparator(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '[', ']', '(', ')', '{', '}', '|', '-':
		return true
	}
	return false
}
