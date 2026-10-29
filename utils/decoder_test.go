package utils_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ErivanCorreia/torrent-indexer/utils"
)

func TestDecodeAdLink(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		encodedStr string
		want       string
		wantErr    bool
	}{
		{
			name:       "Valid encoded string",
			encodedStr: "jVzYmJjZxYjYwMDZiVjZ2UTMmJGM3EmZ4E2M2cDZ0UGN4UmN5EWOlpDapRnY64mc11Dd49jO0VmbnFWb",
			want:       "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c",
			wantErr:    false,
		},
		{
			name:       "Invalid encoded string",
			encodedStr: "invalid_encoded_string",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "Empty string",
			encodedStr: "",
			want:       "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.DecodeAdLink(tt.encodedStr)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DecodeAdLink() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DecodeAdLink() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("DecodeAdLink() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase64Decode(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid base64 string",
			input:   "bWFnbmV0Oj94dD11cm46YnRpaDpoMWIxOWYxNmM0MmMyNWMxNGZhNmNhNzY2NGNhNzZlN2Y2NDZhM2Q2NGY=",
			want:    "magnet:?xt=urn:btih:h1b19f16c42c25c14fa6ca7664ca76e7f646a3d64f",
			wantErr: false,
		},
		{
			name:    "Invalid base64 string",
			input:   "invalid_base64_string",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.Base64Decode(tt.input)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Base64Decode() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Base64Decode() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("Base64Decode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeStarckXID(t *testing.T) {
	const magnetLink = "magnet:?xt=urn:btih:ebb772b765d813251317d1a0db9c19e59226bc80&dn=Moana.2026.WEB-DL.1080p.x264.DUAL.5.1-STARCKFILMES&xl=2671924577&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce"

	tests := []struct {
		name    string
		encoded string
		want    string
		wantErr bool
	}{
		{
			name:    "current layout (base64 after the shuffle)",
			encoded: shuffleByStep(base64.StdEncoding.EncodeToString([]byte(magnetLink)), 3),
			want:    magnetLink,
		},
		{
			name:    "current layout without base64 padding",
			encoded: shuffleByStep(base64.RawStdEncoding.EncodeToString([]byte(magnetLink)), 3),
			want:    magnetLink,
		},
		{
			name:    "invalid xid",
			encoded: "invalid_xid_value",
			wantErr: true,
		},
		{
			name:    "empty xid",
			encoded: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.DecodeStarckXID(tt.encoded)
			if gotErr != nil {
				if !tt.wantErr {
					t.Fatalf("DecodeStarckXID() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DecodeStarckXID() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("DecodeStarckXID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// shuffleByStep applies the same character shuffle used by starck-filmes, so
// the tests can build encoded values that mirror the real page.
func shuffleByStep(original string, step int) string {
	runes := []rune(original)
	shuffled := make([]rune, len(runes))
	used := make([]bool, len(runes))
	index := 0

	for i := 0; i < len(runes); i++ {
		for used[index] {
			index = (index + 1) % len(runes)
		}
		used[index] = true
		shuffled[index] = runes[i]
		index = (index + step) % len(runes)
	}

	return string(shuffled)
}

func TestDecodeStarckXIDKeepsInfoHash(t *testing.T) {
	// Guard against a decoder that returns some other magnet URI: the xid
	// below belongs to a real starck-filmes download button.
	xid := "bjNWZBFSJnZTb0JmcGVjJ01TO1JjZG9Hd4AHdlJDMh10Y1E2cltmMl4kc6YiYl5nM2RkcpZma0FDcwpmalFGYjlmatI2L3VmNyNzLvJmbiZSNuUzazYXQ1gTZuYDb5gmNxVjM0kzJlITM1NkMBZTNhMjbxkmN252OvQSdxUWYy5TRjBmZkFSYuZjb0lmcj9jM11Tb1lmZlNHNlATJlknMyR0MyEjPlZXMiVkYkYzclgCMwUkJzZmQvRScuUGPyVURuNiLvUneyNWR09mZ0VWZ2FSYs5Wd24Gcugyau5WcB02Ylk2Ml10MlE0L2EjO4ITMwYCM5UjJyYTRuJmVGF0YuVWbC5mL19Ub1RmbMNmLpNjYlEXJwInOmRDdyBHPwIXL9VnakgHcyRCN0UjczQHQuMSRlUFMyV0RBEiTlUCMy4kR1YnLlRjMyEkYtZWU0N1crRmZBFXUjIkauN2bLV3RyBkLllnbMlnTlRUbyVWYTVWJrNnerhWcsViPk5TavIWcy0mMucDYlg2M390MtETJxgTMwNzMBMDN3YDJmQTdzJHJGITY9JWdG5WYuRWbw53JuVTbuN3YBV2JuUTYmJ2dGUHJmITd9JHdGIWZ9RXdwhWJwRTbwNGJB9TJkNTaBJWJGUTJuJTbGJ3JGJTdnJTJGQTduNHdBJHNhJjYhk2a2t3OlgScuUiYy53RiJmapFXbuJSbr5mZp9Wc153b3QmYlNWMlx0JkEnLxRmMyRzPlMXJ3VTJkNTcBJCNGUjYzkWQ25SO1USbyUmRyNiRpUmYyFXRuImbm1mdh9Hc1Inb9RmapNHblRiJ01ncnRHZyMWPlJXMoV0YkEXclJCMkUkdzYCQl5SMlUkdyZSR0UiczUmQyFTRjIna1A2JyVTcyJCcG5yYw5Wdt5Wb1JHbsNma1NWYpNiY05XcuImZmFXdjQHalI2M9V0dyEWL0RnNwhDJ5MTelNiMBUkJzZTQhJTbGYmJ55TNvJjdGkWdl5HMjRkZrZSMhZib05ncuVjYu1mY1F2ZvlHbhAmclxiMpZ0b0EmclVjMz1kZ1YXZlJHM2AkalZWM1N0NlEiLl5mM0NkcvYmblFSMrUkezZCQy5TLjYmc5wmN1ljLtkmLlNmMhlkJzZTdhNCbBUmNz5jQvkTd2EWOz5SMjUzZycSRlZmM0FkcuZjdh1Wbo5mdj5HavRWdwFWJy5T"

	got, err := utils.DecodeStarckXID(xid)
	if err != nil {
		t.Fatalf("DecodeStarckXID() failed: %v", err)
	}

	const infoHash = "ebb772b765d813251317d1a0db9c19e59226bc80"
	if !strings.HasPrefix(got, "magnet:?xt=urn:btih:"+infoHash) {
		t.Errorf("DecodeStarckXID() = %q, want the magnet with info hash %s", got, infoHash)
	}
}

func TestDecodeStarckDataU(t *testing.T) {
	tests := []struct {
		name    string
		dataU   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid data-u magnet link",
			dataU:   "mb6ab5g78n4de63tb0:7d?2bx12t69=1bu55r94na2:86beft8fi50h0c:1c",
			want:    "magnet:?xt=urn:btih:bb746b7216159a8e8501658d30db29b5426ff0cc",
			wantErr: false,
		},
		{
			name:    "Invalid data-u string",
			dataU:   "invalid_data_u_string",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Empty data-u",
			dataU:   "",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.DecodeStarckDataU(tt.dataU)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DecodeStarckDataU() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DecodeStarckDataU() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("DecodeStarckDataU() = %v, want %v", got, tt.want)
			}
		})
	}
}
