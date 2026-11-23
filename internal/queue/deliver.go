package queue

import "tagent/internal/tmux"

// Busy reports whether a message sent to an agent in this status waits in the queue
// instead of being typed into its turn. The CLI and the web send use this one rule.
func Busy(status tmux.PaneStatus) bool {
	return status == tmux.StatusRunning || status == tmux.StatusBackground || status == tmux.StatusWaiting
}
