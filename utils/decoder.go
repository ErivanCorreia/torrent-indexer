package utils

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"
)

func DecodeAdLink(encodedStr string) (string, error) {
	if encodedStr == "" {
		return "", fmt.Errorf("empty string")
	}
	reversed := reverseString(encodedStr)

	decodedBytes, err := base64.StdEncoding.DecodeString(reversed)
	if err != nil {
		return "", err
	}

	htmlUnescaped := html.UnescapeString(string(decodedBytes))

	return htmlUnescaped, nil
}

func Base64Decode(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty string")
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decodedBytes), nil
}

// Helper function to reverse a string
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// unshuffleStringByStep reconstructs an original string from a shuffled version.
// The input was created by picking characters using a stepping cursor (advancing
// by step positions through the original, skipping already-used positions).
// This function inverts the process: it walks the same cursor sequence and places
// each shuffled[i] back into original[index].
func unshuffleStringByStep(shuffled string, step int) (string, error) {
	runes := []rune(shuffled)
	length := len(runes)
	if length == 0 {
		return "", fmt.Errorf("empty string")
	}
	if step <= 0 {
		return "", fmt.Errorf("step must be greater than 0")
	}

	original := make([]rune, length)
	used := make([]bool, length)
	index := 0

	for i := 0; i < length; i++ {
		for used[index] {
			index = (index + 1) % length
		}
		used[index] = true
		original[i] = runes[index]
		index = (index + step) % length
	}

	return string(original), nil
}

// DecodeStarckDataU decodes the obfuscated magnet link from the data-u
// attribute. This is indexer-specific and kept for the legacy page layout.
func DecodeStarckDataU(dataU string) (string, error) {
	if dataU == "" {
		return "", fmt.Errorf("empty data-u value")
	}

	return decodeStarckObfuscatedString(dataU)
}

// DecodeStarckXID decodes the obfuscated magnet link from the "xid" query
// parameter used by the current download buttons (e.g.
// https://filmedl.com/?xid=...).
//
// In this layout the site shuffles the characters of the base64-encoded magnet
// URI, while the legacy data-u attribute contained the shuffled magnet URI
// itself. Both forms are supported.
func DecodeStarckXID(xid string) (string, error) {
	if xid == "" {
		return "", fmt.Errorf("empty xid value")
	}

	return decodeStarckObfuscatedString(xid)
}

// decodeStarckObfuscatedString reverses the character shuffle and, when
// needed, the base64 layer applied by starck-filmes to its magnet links.
func decodeStarckObfuscatedString(encoded string) (string, error) {
	unshuffled, err := unshuffleStringByStep(encoded, 3)
	if err != nil {
		return "", fmt.Errorf("unshuffle failed: %w", err)
	}

	// Legacy layout: the unshuffled value is the magnet URI itself.
	if IsMagnetLink(unshuffled) {
		return unshuffled, nil
	}

	// Current layout: the unshuffled value is a base64-encoded magnet URI.
	decoded, err := decodeBase64String(unshuffled)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	if !IsMagnetLink(decoded) {
		return "", fmt.Errorf("decoded string is not a valid magnet link: %s", decoded)
	}

	return decoded, nil
}

// decodeBase64String decodes both padded and unpadded base64 values.
func decodeBase64String(value string) (string, error) {
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return string(decoded), nil
	}

	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return string(decoded), nil
	}

	// Some values lose their padding when the characters are shuffled, so add
	// it back before the last attempt.
	if remainder := len(value) % 4; remainder != 0 {
		value += strings.Repeat("=", 4-remainder)
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func IsMagnetLink(link string) bool {
	return len(link) > 8 && link[:8] == "magnet:?"
}
