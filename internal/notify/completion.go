package notify

import (
	"strings"
	"unicode/utf16"
)

// TelegramCompletion preserves the beginning; screen reads retain their tail.
func TelegramCompletion(text, token string) string {
	if token != "" {
		text = strings.ReplaceAll(text, token, "[redacted]")
	}
	const suffix = "\n… /read for more"
	units := 0
	for i, r := range text {
		units += utf16.RuneLen(r)
		if units > telegramTextUnits-len(utf16.Encode([]rune(suffix))) {
			return text[:i] + suffix
		}
	}
	return text
}
