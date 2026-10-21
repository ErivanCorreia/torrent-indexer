package handler

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/schema"
)

// loadFixture loads an HTML fixture from the testdata folder.
func loadFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()

	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to open fixture %s: %v", name, err)
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}

	return doc
}

//
// starck-filmes
//

func TestStarckFilmesDefaultURL(t *testing.T) {
	if !strings.Contains(starck_filmes.URL, "starckfilmes") {
		t.Errorf("starck_filmes.URL = %q, expected a starckfilmes domain", starck_filmes.URL)
	}

	if len(starck_filmes.FallbackURLs) == 0 {
		t.Error("starck_filmes.FallbackURLs is empty, expected the known rotated domains")
	}
}

func TestStarckFilmesBuildURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		q    string
		page string
		want string
	}{
		{
			name: "should build the first page",
			base: "https://starckfilmes-v26.com/",
			want: "https://starckfilmes-v26.com/page/1",
		},
		{
			name: "should keep the requested page",
			base: "https://starckfilmes-v26.com/",
			page: "3",
			want: "https://starckfilmes-v26.com/page/3",
		},
		{
			name: "should build the search URL",
			base: "https://starckfilmes-v26.com/",
			q:    "moana",
			want: "https://starckfilmes-v26.com/?s=moana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildStarckURL(tt.base, tt.q, tt.page); got != tt.want {
				t.Errorf("buildStarckURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStarckFilmesExtractLinks(t *testing.T) {
	doc := loadFixture(t, "starck_listing.html")

	links := extractStarckLinks(doc)
	if len(links) == 0 {
		t.Fatal("extractStarckLinks() returned no links")
	}

	for _, link := range links {
		if link == "" {
			t.Error("extractStarckLinks() returned an empty link")
		}
		if !strings.Contains(link, "/catalog/") {
			t.Errorf("extractStarckLinks() = %q, expected a /catalog/ link", link)
		}
	}
}

func TestStarckFilmesParseDetails(t *testing.T) {
	doc := loadFixture(t, "starck_detail.html")

	details := parseStarckFilmesDetails(doc)

	if details.Title != "Moana" {
		t.Errorf("Title = %q, want %q", details.Title, "Moana")
	}

	if details.Year != "2026" {
		t.Errorf("Year = %q, want %q", details.Year, "2026")
	}

	if len(details.MagnetLinks) != 2 {
		t.Fatalf("len(MagnetLinks) = %d, want 2", len(details.MagnetLinks))
	}

	for _, magnetLink := range details.MagnetLinks {
		if !strings.HasPrefix(magnetLink, "magnet:?xt=urn:btih:") {
			t.Errorf("MagnetLink = %q, expected a magnet URI", magnetLink)
		}
	}

	if len(details.Size) != 2 {
		t.Errorf("len(Size) = %d, want 2 (%v)", len(details.Size), details.Size)
	}

	if len(details.Audio) == 0 {
		t.Error("len(Audio) = 0, expected the audio languages")
	}
}

func TestStarckFilmesPublishedDateFromLink(t *testing.T) {
	// the starck links carry the date as "dd-mm-yyyy" next to the release year,
	// e.g. "/catalog/moana-2026-21-10-2026/"
	got := getPublishedDateFromRawString("https://starckfilmes-v26.com/catalog/moana-2026-21-10-2026/")
	want := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("getPublishedDateFromRawString() = %v, want %v", got, want)
	}
}

//
// rede-torrent
//

func TestRedeTorrentDefaultURL(t *testing.T) {
	if !strings.Contains(rede_torrent.URL, "redestorrents.com") {
		t.Errorf("rede_torrent.URL = %q, expected the redestorrents.com domain", rede_torrent.URL)
	}
}

func TestRedeTorrentBuildURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		q    string
		page string
		want string
	}{
		{
			name: "should build the home URL",
			base: "https://redestorrents.com/",
			want: "https://redestorrents.com/",
		},
		{
			name: "should build the pagination URL",
			base: "https://redestorrents.com/",
			page: "2",
			want: "https://redestorrents.com/pagina/2/",
		},
		{
			name: "should build the search URL",
			base: "https://redestorrents.com/",
			q:    "moana",
			want: "https://redestorrents.com/index.php?busca=moana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildRedeTorrentURL(tt.base, tt.q, tt.page); got != tt.want {
				t.Errorf("buildRedeTorrentURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedeTorrentExtractLinks(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    int
	}{
		{name: "listing", fixture: "rede_listing.html", want: 20},
		{name: "search", fixture: "rede_search.html", want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := loadFixture(t, tt.fixture)

			links := extractRedeTorrentLinks(doc)
			if len(links) != tt.want {
				t.Errorf("len(extractRedeTorrentLinks()) = %d, want %d", len(links), tt.want)
			}

			for _, link := range links {
				if link == "" {
					t.Error("extractRedeTorrentLinks() returned an empty link")
				}
			}
		})
	}
}

func TestRedeTorrentParseDetails(t *testing.T) {
	doc := loadFixture(t, "rede_detail.html")

	details := parseRedeTorrentDetails(doc)

	if details.Title != "Moana" {
		t.Errorf("Title = %q, want %q", details.Title, "Moana")
	}

	if details.Year != "2026" {
		t.Errorf("Year = %q, want %q", details.Year, "2026")
	}

	if len(details.MagnetLinks) != 3 {
		t.Fatalf("len(MagnetLinks) = %d, want 3", len(details.MagnetLinks))
	}

	for _, magnet := range details.MagnetLinks {
		if !strings.HasPrefix(magnet.Link, "magnet:?xt=urn:btih:") {
			t.Errorf("MagnetLink = %q, expected a magnet URI", magnet.Link)
		}
		if magnet.Row == "" {
			t.Error("Row is empty, expected the download label")
		}
	}

	if len(details.Size) != 3 {
		t.Errorf("len(Size) = %d, want 3 (%v)", len(details.Size), details.Size)
	}

	if len(details.Audio) != 2 {
		t.Errorf("len(Audio) = %d, want 2 (%v)", len(details.Audio), details.Audio)
	}

	if details.IMDB != "https://www.imdb.com/title/tt27419466/" {
		t.Errorf("IMDB = %q, want %q", details.IMDB, "https://www.imdb.com/title/tt27419466/")
	}

	if details.Date.IsZero() {
		t.Error("Date is zero, expected the publication date")
	}
}

func TestRedeTorrentRowAudio(t *testing.T) {
	pageAudio := []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish}

	tests := []struct {
		name string
		row  string
		want []schema.Audio
	}{
		{
			name: "dual audio keeps both languages",
			row:  "DOWNLOAD TORRENT DUBLADO DUAL AUDIO 5.1 MKV WEB-DL 1080P",
			want: []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish},
		},
		{
			name: "dublado only keeps portuguese",
			row:  "DOWNLOAD TORRENT DUBLADO MKV WEB-DL 1080P",
			want: []schema.Audio{schema.AudioPortuguese},
		},
		{
			name: "nacional keeps portuguese",
			row:  "DOWNLOAD TORRENT NACIONAL MKV WEB-DL 1080P",
			want: []schema.Audio{schema.AudioPortuguese},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redeTorrentRowAudio(tt.row, pageAudio)
			if len(got) != len(tt.want) {
				t.Fatalf("redeTorrentRowAudio() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("redeTorrentRowAudio() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

//
// vaca-torrent
//

func TestVacaTorrentDefaultURL(t *testing.T) {
	if !strings.Contains(vacaTorrent.URL, "vaqueirofilmes") {
		t.Errorf("vacaTorrent.URL = %q, expected the vaqueirofilmes domain", vacaTorrent.URL)
	}
}

func TestVacaTorrentBuildURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		q    string
		page string
		want string
	}{
		{
			name: "should build the home URL",
			base: "https://vaqueirofilmes1.com/",
			want: "https://vaqueirofilmes1.com/",
		},
		{
			name: "should build the pagination URL",
			base: "https://vaqueirofilmes1.com/",
			page: "2",
			want: "https://vaqueirofilmes1.com/pagina/2/",
		},
		{
			name: "should build the search URL",
			base: "https://vaqueirofilmes1.com/",
			q:    "moana",
			want: "https://vaqueirofilmes1.com/?q=moana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildVacaTorrentURL(tt.base, tt.q, tt.page); got != tt.want {
				t.Errorf("buildVacaTorrentURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVacaTorrentParseDetails(t *testing.T) {
	doc := loadFixture(t, "vaca_template.html")

	details := parseVacaTorrentDetails(doc)

	if details.Title != "Homem-Aranha: Um Novo Dia" {
		t.Errorf("Title = %q, want %q", details.Title, "Homem-Aranha: Um Novo Dia")
	}

	if details.OriginalTitle != "Spider-Man: A New Day (2026)" {
		t.Errorf("OriginalTitle = %q, want %q", details.OriginalTitle, "Spider-Man: A New Day (2026)")
	}

	if details.Year != "2026" {
		t.Errorf("Year = %q, want %q", details.Year, "2026")
	}

	if len(details.Entries) != 6 {
		t.Errorf("len(Entries) = %d, want 6", len(details.Entries))
	}

	for _, entry := range details.Entries {
		if entry.Size == "" {
			t.Error("Entry.Size is empty, expected the file size")
		}
		if entry.Qual == "" {
			t.Error("Entry.Qual is empty, expected the quality")
		}
	}
}

//
// generic fallback mechanism
//

func TestCandidateURLs(t *testing.T) {
	meta := IndexerMeta{
		URL:          "https://primary.example.com/",
		FallbackURLs: []string{"https://primary.example.com/", "https://fallback.example.com/", ""},
	}

	got := candidateURLs(meta)
	want := []string{"https://primary.example.com/", "https://fallback.example.com/"}

	if len(got) != len(want) {
		t.Fatalf("candidateURLs() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidateURLs() = %v, want %v", got, want)
		}
	}
}
