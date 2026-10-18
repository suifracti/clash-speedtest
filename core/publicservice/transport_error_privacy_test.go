package publicservice

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTransportErrorRedactsCommonCredentialForms(t *testing.T) {
	for _, message := range []string{
		"authorization: bearer private-value-one header rejected",
		"Basic private-value-one and BEARER private-value-two",
		"proxy error at https://alice:private-value-one@example.invalid/sub/private-value-two?token=private-value-three",
		`provider returned {"access_token":"private-value-one","refresh_token":"private-value-two"}`,
		"password=private-value-one client_secret:private-value-two api_key=private-value-three",
	} {
		got := sanitizeTransportError(errors.New(message))
		if strings.Contains(got, "private-value") {
			t.Errorf("credential survived in sanitized error: %q", got)
		}
	}
}
func TestTransportErrorBoundPreservesUTF8(t *testing.T) {
	got := sanitizeTransportError(errors.New(strings.Repeat("错", 39) + "x错误"))
	if !utf8.ValidString(got) {
		t.Fatal("bounded error contains an incomplete UTF-8 code point")
	}
}
