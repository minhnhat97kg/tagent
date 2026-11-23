package notify

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTelegramCompletionKeepsPrefix(t *testing.T) {
	for _, body := range []string{strings.Repeat("a", 5000), strings.Repeat("😀", 5000)} {
		got := TelegramCompletion("begin secret "+body+"END", "secret")
		if !strings.HasPrefix(got, "begin [redacted]") || !strings.HasSuffix(got, "/read for more") || strings.Contains(got, "END") || len(utf16.Encode([]rune(got))) >= 4096 {
			t.Fatal("bad completion", got)
		}
	}
	if got := TelegramText(strings.Repeat("a", 5000)+"END", ""); !strings.HasSuffix(got, "END") {
		t.Fatal("screen tail lost")
	}
}
