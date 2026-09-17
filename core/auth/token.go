package auth

import (
	"fmt"
	"os"
	"strings"
)

// ParseAntigravityToken extracts a bearer token from user-supplied text.
// It accepts a bare token, or a pasted header line such as
// "Authorization: Bearer ya29....". Blank lines and #-comments are skipped.
func ParseAntigravityToken(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(strings.ToLower(line), "authorization:"); idx >= 0 {
			line = line[idx+len("authorization:"):]
		}
		line = strings.TrimSpace(line)
		if len(line) > 7 && strings.EqualFold(line[:7], "bearer ") {
			line = strings.TrimSpace(line[7:])
		}
		return line
	}
	return ""
}

// ReadAntigravityTokenFile loads an OAuth access token from a file.
func ReadAntigravityTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := ParseAntigravityToken(string(data))
	if token == "" {
		return "", fmt.Errorf("no token found in %s", path)
	}
	return token, nil
}

// firstLine returns the first line of text.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			return line
		}
	}
	return ""
}
