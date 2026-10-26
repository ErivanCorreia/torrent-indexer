package handler

import (
	"reflect"
	"testing"

	"github.com/ErivanCorreia/torrent-indexer/schema"
)

func TestSanitizeReleaseTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "polluted rede display name",
			input: "BAIXARAPIDO.COM-.WEB-DL.1080P.MKV.5.1.-DUBLADO-DUAL-AUDIO-.Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			want:  "WEB-DL.1080P.MKV.5.1.DUBLADO-DUAL-AUDIO.Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
		},
		{
			name:  "url encoded title",
			input: "Moana%202026%201080p%20WEB-DL",
			want:  "Moana 2026 1080p WEB-DL",
		},
		{
			name:  "valid scene name is preserved",
			input: "The.Office.US.S01E01.1080p.WEB-DL",
			want:  "The.Office.US.S01E01.1080p.WEB-DL",
		},
		{
			name:  "uppercase scene name is preserved",
			input: "THE.OFFICE.US.S01E01.1080p.WEB-DL",
			want:  "THE.OFFICE.US.S01E01.1080p.WEB-DL",
		},
		{
			name:  "tv in title is preserved",
			input: "The.Office.TV.S01E01.1080p.WEB-DL",
			want:  "The.Office.TV.S01E01.1080p.WEB-DL",
		},
		{
			name:  "word to in title is preserved",
			input: "Back.TO.The.Future.1985.1080p",
			want:  "Back.TO.The.Future.1985.1080p",
		},
		{
			name:  "word in in title is preserved",
			input: "The.IN.Between.2022.1080p",
			want:  "The.IN.Between.2022.1080p",
		},
		{
			name:  "generic domain before tokens",
			input: "Moana.2026.1080p.example.xyz.1080p",
			want:  "Moana.2026.1080p.1080p",
		},
		{
			name:  "generic domain inside brackets",
			input: "[ www.example.xyz ] Movie.2026.1080p",
			want:  "Movie.2026.1080p",
		},
		{
			name:  "generic domain at the start",
			input: "www.site.com - Movie 1080p",
			want:  "Movie 1080p",
		},
		{
			name:  "trailing separator",
			input: "Movie.2026.1080p.WEB-DL.",
			want:  "Movie.2026.1080p.WEB-DL",
		},
		{
			name:  "pipe separator",
			input: "Movie | 1080p",
			want:  "Movie 1080p",
		},
		{
			name:  "repeated dots",
			input: "Movie..2026...1080p",
			want:  "Movie.2026.1080p",
		},
		{
			name:  "underscore separator",
			input: "Movie_2026_1080p",
			want:  "Movie 2026 1080p",
		},
		{
			name:  "dot dash artifact",
			input: "Movie.-1080p",
			want:  "Movie.1080p",
		},
		{
			name:  "dash dot artifact",
			input: "Movie-.1080p",
			want:  "Movie.1080p",
		},
		{
			name:  "empty",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeReleaseTitle(tt.input); got != tt.want {
				t.Fatalf("sanitizeReleaseTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeDisplayTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "download suffix and website",
			input: "Moana – Download | www.site.com",
			want:  "Moana",
		},
		{
			name:  "pipe download suffix",
			input: "Moana | Download",
			want:  "Moana",
		},
		{
			name:  "assistir online suffix",
			input: "Moana - Assistir Online",
			want:  "Moana",
		},
		{
			name:  "year is preserved",
			input: "Moana (2026)",
			want:  "Moana (2026)",
		},
		{
			name:  "website at the end",
			input: "A Origem | www.site.com",
			want:  "A Origem",
		},
		{
			name:  "website inside brackets",
			input: "[ www.example.xyz ] Moana",
			want:  "Moana",
		},
		{
			name:  "extra spaces",
			input: "  Moana   ",
			want:  "Moana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeDisplayTitle(tt.input); got != tt.want {
				t.Fatalf("sanitizeDisplayTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsUsableReleaseTitle(t *testing.T) {
	tests := []struct {
		name          string
		releaseTitle  string
		originalTitle string
		want          bool
	}{
		{
			name:          "valid movie release",
			releaseTitle:  "Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			originalTitle: "Moana",
			want:          true,
		},
		{
			name:          "valid series release",
			releaseTitle:  "The.Office.US.S01E01.1080p.WEB-DL",
			originalTitle: "The Office",
			want:          true,
		},
		{
			name:          "adware prefix is rejected",
			releaseTitle:  "WEB-DL.1080P.MKV.5.1.DUBLADO-DUAL-AUDIO.Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			originalTitle: "Moana",
			want:          false,
		},
		{
			name:          "technical only is rejected",
			releaseTitle:  "WEB-DL.1080p.DUAL",
			originalTitle: "Moana",
			want:          false,
		},
		{
			name:          "empty is rejected",
			releaseTitle:  "",
			originalTitle: "Moana",
			want:          false,
		},
		{
			name:          "truncated with dot is rejected",
			releaseTitle:  "Moana.",
			originalTitle: "Moana",
			want:          false,
		},
		{
			name:          "truncated with dash is rejected",
			releaseTitle:  "Moana-",
			originalTitle: "Moana",
			want:          false,
		},
		{
			name:          "short title confirmed by page",
			releaseTitle:  "Up.2009.1080p",
			originalTitle: "Up",
			want:          true,
		},
		{
			name:          "numeric title",
			releaseTitle:  "1917.2019.1080p",
			originalTitle: "1917",
			want:          true,
		},
		{
			name:          "one letter title",
			releaseTitle:  "A.2024.1080p",
			originalTitle: "A",
			want:          true,
		},
		{
			name:          "synthetic title is usable",
			releaseTitle:  "Moana (2026) 1080p WEB-DL DUAL",
			originalTitle: "Moana",
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUsableReleaseTitle(tt.releaseTitle, tt.originalTitle); got != tt.want {
				t.Fatalf("isUsableReleaseTitle(%q, %q) = %v, want %v", tt.releaseTitle, tt.originalTitle, got, tt.want)
			}
		})
	}
}

func TestExtractTechnicalTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "movie release",
			input: "Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			want:  []string{"1080p", "WEB-DL", "x264", "DUAL", "5.1"},
		},
		{
			name:  "series release",
			input: "Show.S02E03.2160p.WEBRip.DDP5.1.HDR",
			want:  []string{"S02E03", "2160p", "WEBRip", "HDR", "DDP5.1"},
		},
		{
			name:  "full hd becomes 1080p",
			input: "Movie.1080p.FULL.HD.WEB-DL.DUBLADO",
			want:  []string{"1080p", "WEB-DL", "DUBLADO"},
		},
		{
			name:  "hdr tokens",
			input: "Movie.2160p.x265.DV.HDR10.DUAL.5.1",
			want:  []string{"2160p", "x265", "DV", "HDR10", "DUAL", "5.1"},
		},
		{
			name:  "no technical tokens",
			input: "Movie.middle.individual",
			want:  nil,
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTechnicalTokens(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("extractTechnicalTokens(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildReleaseTitle(t *testing.T) {
	tests := []struct {
		name          string
		originalTitle string
		year          string
		releaseTitle  string
		audio         []schema.Audio
		want          string
	}{
		{
			name:          "rebuilds from page title and magnet tokens",
			originalTitle: "Moana",
			year:          "2026",
			releaseTitle:  "WEB-DL.1080P.MKV.5.1.-DUBLADO-DUAL-AUDIO.-Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			want:          "Moana (2026) 1080p WEB-DL x264 DUBLADO DUAL 5.1",
		},
		{
			name:          "year already in page title",
			originalTitle: "Moana (2026)",
			year:          "2026",
			releaseTitle:  "1080p.WEB-DL",
			want:          "Moana (2026) 1080p WEB-DL",
		},
		{
			name:          "no technical tokens",
			originalTitle: "Moana",
			year:          "2026",
			releaseTitle:  "",
			want:          "Moana (2026)",
		},
		{
			name:          "dual audio fallback",
			originalTitle: "Moana",
			year:          "2026",
			releaseTitle:  "",
			audio:         []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish},
			want:          "Moana (2026) DUAL",
		},
		{
			name:          "dubbed audio fallback",
			originalTitle: "Moana",
			year:          "2026",
			releaseTitle:  "",
			audio:         []schema.Audio{schema.AudioPortuguese},
			want:          "Moana (2026) DUBLADO",
		},
		{
			name:          "does not duplicate tokens already in the base",
			originalTitle: "Moana.2026.1080p.WEB-DL",
			releaseTitle:  "Moana.2026.1080p.WEB-DL",
			want:          "Moana.2026.1080p.WEB-DL",
		},
		{
			name:         "does not emit an audio-only title",
			releaseTitle: "",
			audio:        []schema.Audio{schema.AudioPortuguese},
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildReleaseTitle(tt.originalTitle, tt.year, tt.releaseTitle, tt.audio)
			if got != tt.want {
				t.Fatalf("buildReleaseTitle(%q, %q, %q, %v) = %q, want %q", tt.originalTitle, tt.year, tt.releaseTitle, tt.audio, got, tt.want)
			}
		})
	}
}

func TestNormalizeTitles(t *testing.T) {
	torrents := []schema.IndexedTorrent{
		{
			Title:         "BAIXARAPIDO.COM-.WEB-DL.1080P.MKV.5.1.-DUBLADO-DUAL-AUDIO-.Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			OriginalTitle: "Moana – Download | www.site.com",
			Year:          "2026",
		},
		{
			Title:         "Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			OriginalTitle: "Moana",
			Year:          "2026",
		},
		{
			Title:         "",
			OriginalTitle: "Moana",
			Year:          "2026",
		},
		{
			Title:         "WEB-DL.1080p.DUAL",
			OriginalTitle: "Moana",
			Year:          "2026",
		},
	}

	got := NormalizeTitles(nil, nil, torrents)

	wantTitles := []string{
		"Moana (2026) 1080p WEB-DL x264 DUBLADO DUAL 5.1",
		"Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
		"",
		"Moana (2026) 1080p WEB-DL DUAL",
	}
	wantOriginalTitles := []string{"Moana", "Moana", "Moana", "Moana"}

	for i := range got {
		if got[i].Title != wantTitles[i] {
			t.Errorf("torrent %d title = %q, want %q", i, got[i].Title, wantTitles[i])
		}
		if got[i].OriginalTitle != wantOriginalTitles[i] {
			t.Errorf("torrent %d original title = %q, want %q", i, got[i].OriginalTitle, wantOriginalTitles[i])
		}
	}
}

// TestNormalizeTitlesRedeSamples covers the real (URL-decoded) magnet display
// names found in testdata/rede_detail.html.
func TestNormalizeTitlesRedeSamples(t *testing.T) {
	torrents := []schema.IndexedTorrent{
		{
			Title:         "BAIXARAPIDO.COM-.WEB-DL.1080P.MKV.5.1.-DUBLADO-DUAL-AUDIO-.Moana.2026.1080p.WEB-DL.x264.DUAL.5.1",
			OriginalTitle: "Moana",
			Year:          "2026",
			Audio:         []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish},
		},
		{
			Title:         "BAIXARAPIDO.COM-.WEB-DL.1080P.SEM-COMPACTAÇÃO.MKV.5.1.-DUBLADO-DUAL-AUDIO-.Moana.2026.1080p.FULL.HD.WEB-DL.DUAL.5.1",
			OriginalTitle: "Moana",
			Year:          "2026",
			Audio:         []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish},
		},
		{
			Title:         "BAIXARAPIDO.COM-.WEB-DL.2160P.SEM-COMPACTAÇÃO.MKV.5.1.-DUBLADO-DUAL-AUDIO-.Moana 2026 WEB-DL 2160p x265 DV HDR10 DUAL 5.1",
			OriginalTitle: "Moana",
			Year:          "2026",
			Audio:         []schema.Audio{schema.AudioPortuguese, schema.AudioEnglish},
		},
	}

	got := NormalizeTitles(nil, nil, torrents)

	want := []string{
		"Moana (2026) 1080p WEB-DL x264 DUBLADO DUAL 5.1",
		"Moana (2026) 1080p WEB-DL DUBLADO DUAL 5.1",
		"Moana (2026) 2160p WEB-DL x265 DV HDR10 DUBLADO DUAL 5.1",
	}

	for i := range got {
		if got[i].Title != want[i] {
			t.Errorf("torrent %d title = %q, want %q", i, got[i].Title, want[i])
		}
	}
}
