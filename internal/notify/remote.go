package notify

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"tagent/internal/tmux"
)

// Remote notifications push the same events as desktop notifications to a
// plain-text POST endpoint, such as an ntfy topic:
//
// set -g @agent_notify_url https://ntfy.sh/<your-topic>
const notifyURLOption = "@agent_notify_url"

var remoteClient = &http.Client{Timeout: 3 * time.Second}

// Send posts a best-effort notification to the ntfy/webhook URL. Telegram is a
// chat channel and gets no notifications. Push failures must never break a hook.
func Send(title, body string) {
	if target, ok := tmux.GetOption(notifyURLOption); ok && strings.TrimSpace(target) != "" {
		postRemote(target, title, body)
	}
}

func postRemote(target, title, body string) error {
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Tags", "robot")
	resp, err := remoteClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification HTTP status %d", resp.StatusCode)
	}
	return nil
}
