package notify

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tagent/internal/tmux"
)

// Remote notifications: the same events as the desktop notification, pushed
// to your phone when you're away from the machine.
//
//	set -g @agent_notify_url https://ntfy.sh/<your-topic>
//	set -g @agent_notify_url 'https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<ID>'
//
// ntfy (or anything accepting a plain-text POST) gets the body with the
// title in a Title header; a Telegram bot URL gets a `text` form field.
const notifyURLOption = "@agent_notify_url"

var remoteClient = &http.Client{Timeout: 3 * time.Second} // hooks wait on this

// sendRemote posts the notification to @agent_notify_url, if set. Errors
// are swallowed: a phone push must never break a hook.
func Send(title, body string) {
	target, ok := tmux.GetOption(notifyURLOption)
	if !ok || strings.TrimSpace(target) == "" {
		return
	}
	postRemote(target, title, body)
}

func postRemote(target, title, body string) error {
	var req *http.Request
	var err error
	if _, _, _, ok := TelegramTarget(target); ok {
		form := url.Values{"text": {title + "\n" + body}}
		req, err = http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequest(http.MethodPost, target, strings.NewReader(body))
		if err == nil {
			req.Header.Set("Title", title)
			req.Header.Set("Tags", "robot")
		}
	}
	if err != nil {
		return err
	}
	client := *remoteClient
	if _, _, _, ok := TelegramTarget(target); ok {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification HTTP status %d", resp.StatusCode)
	}
	return nil
}
