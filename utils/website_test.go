package utils

import "testing"

func TestRemoveWebsiteReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "known domain",
			input: "BAIXARAPIDO.COM-.Movie.2026.1080p",
			want:  "-.Movie.2026.1080p",
		},
		{
			name:  "generic domain at the start",
			input: "www.example.xyz - Movie.2026.1080p",
			want:  "- Movie.2026.1080p",
		},
		{
			name:  "generic domain inside brackets",
			input: "[ www.example.xyz ] Movie.2026.1080p",
			want:  "[   ] Movie.2026.1080p",
		},
		{
			name:  "generic domain at the end",
			input: "Movie.2026.1080p.example.xyz",
			want:  "Movie.2026.1080p.",
		},
		{
			name:  "known domain at the end",
			input: "Movie.2026.1080p.BAIXARAPIDO.COM",
			want:  "Movie.2026.1080p.",
		},
		{
			name:  "does not destroy The Office US",
			input: "The.Office.US.S01E01.1080p.WEB-DL",
			want:  "The.Office.US.S01E01.1080p.WEB-DL",
		},
		{
			name:  "does not destroy uppercase The Office US",
			input: "THE.OFFICE.US.S01E01.1080p.WEB-DL",
			want:  "THE.OFFICE.US.S01E01.1080p.WEB-DL",
		},
		{
			name:  "does not destroy The Office TV",
			input: "The.Office.TV.S01E01.1080p.WEB-DL",
			want:  "The.Office.TV.S01E01.1080p.WEB-DL",
		},
		{
			name:  "does not destroy Back to the Future",
			input: "Back.TO.The.Future.1985.1080p",
			want:  "Back.TO.The.Future.1985.1080p",
		},
		{
			name:  "generic domain between dots",
			input: "Moana.2026.1080p.example.xyz.1080p",
			want:  "Moana.2026.1080p. .1080p",
		},
		{
			name:  "does not destroy audio channels",
			input: "Movie.5.1.1080p",
			want:  "Movie.5.1.1080p",
		},
		{
			name:  "only a generic domain",
			input: "www.example.xyz",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveWebsiteReferences(tt.input); got != tt.want {
				t.Fatalf("RemoveWebsiteReferences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
