package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTelegramTarget(t *testing.T) {
	for _, s := range []string{"http://api.telegram.org/bot123/sendMessage?chat_id=9", "https://api.telegram.org.evil/bot123/sendMessage?chat_id=9", "https://user@api.telegram.org/bot123/sendMessage?chat_id=9", "https://api.telegram.org/bot123/sendMessage?chat_id=-9", "https://ntfy.sh/test"} {
		if _, _, _, ok := TelegramTarget(s); ok {
			t.Fatalf("accepted %s", s)
		}
	}
	if _, c, _, ok := TelegramTarget("https://api.telegram.org/bot123/sendMessage?chat_id=9"); !ok || c != "9" {
		t.Fatal("valid rejected")
	}
}
func TestTelegramBound(t *testing.T) {
	for _, s := range []string{strings.Repeat("a", 5000), strings.Repeat("界", 5000), strings.Repeat("😀", 5000)} {
		s = TelegramText(s+"secret", "secret")
		if len(utf16.Encode([]rune(s))) >= 4096 || strings.Contains(s, "secret") {
			t.Fatal("unsafe text")
		}
	}
}
func TestTelegramErrors(t *testing.T) {
	for _, body := range []string{`{"ok":false}`, `invalid`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		if TelegramCall(context.Background(), srv.Client(), srv.URL+"/", "getUpdates", nil, nil) {
			t.Fatal("accepted error")
		}
		srv.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if TelegramCall(ctx, http.DefaultClient, "http://127.0.0.1/", "getUpdates", nil, nil) {
		t.Fatal("cancel ignored")
	}
}
