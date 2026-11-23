package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
)

// TelegramTarget accepts only the private-chat Telegram endpoint, never lookalikes.
func TelegramTarget(raw string) (base, chat, token string, ok bool) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "api.telegram.org" || u.User != nil || u.Fragment != "" {
		return
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(p) != 2 || !strings.HasPrefix(p[0], "bot") || p[1] != "sendMessage" {
		return
	}
	if u.Path != "/"+p[0]+"/sendMessage" || u.RawPath != "" {
		return "", "", "", false
	}
	token = strings.TrimPrefix(p[0], "bot")
	for _, r := range token {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ':' || r == '_' || r == '-') {
			return "", "", "", false
		}
	}
	chat = u.Query().Get("chat_id")
	id, e := strconv.ParseInt(chat, 10, 64)
	if token == "" || e != nil || id <= 0 {
		return "", "", "", false
	}
	return "https://api.telegram.org/" + p[0] + "/", chat, token, true
}

// Both outgoing formatters share this budget so completion prefixes survive say().
const telegramTextUnits = 4090

// TelegramText bounds every outgoing message in UTF-16 units, preserving the tail.
func TelegramText(s, token string) string {
	if token != "" {
		s = strings.ReplaceAll(s, token, "[redacted]")
	}
	r := []rune(s)
	n := 0
	i := len(r)
	for i > 0 {
		k := utf16.RuneLen(r[i-1])
		if n+k > telegramTextUnits {
			break
		}
		n += k
		i--
	}
	if i > 0 {
		return "…" + string(r[i:])
	}
	return s
}

type TelegramUpdate struct {
	ID      int64 `json:"update_id"`
	Message struct {
		Text string `json:"text"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
	} `json:"message"`
}

// TelegramCall never returns credential-bearing transport errors.
func TelegramCall(ctx context.Context, client *http.Client, base, method string, form url.Values, dst any) bool {
	req, e := http.NewRequestWithContext(ctx, "POST", base+method, strings.NewReader(form.Encode()))
	if e != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := c.Do(req)
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var result struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result) != nil || !result.OK {
		return false
	}
	return dst == nil || json.Unmarshal(result.Result, dst) == nil
}
