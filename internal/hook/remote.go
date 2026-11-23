package hook

import "tmux-agent/internal/notify"

// Notify sends a one-off notification (desktop + phone) for events the
// hook handlers don't cover — the watchdog's stuck/conflict warnings. It
// respects @sidebar_notifications; the caller dedupes.
func Notify(title, body string) {
	if !LoadSettings().Enabled {
		return
	}
	notify.Send(title, body)
	SendDesktopNotification(title, body)
}
