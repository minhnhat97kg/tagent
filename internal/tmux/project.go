package tmux

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"
)

// OwnerOption is the session option holding a project session's owner path.
const OwnerOption = "@tagent_owner"

// SessionName is "tagent:<folder>-<4 hex of sha1(owner)>". tmux rewrites '.'
// and ':' in names, so the folder is sanitized here and the name stays stable.
func SessionName(owner string) string {
	base := strings.NewReplacer(".", "_", ":", "_", " ", "_").Replace(filepath.Base(owner))
	sum := sha1.Sum([]byte(owner))
	return fmt.Sprintf("tagent:%s-%x", base, sum[:2])
}

// ProjectSession returns the id of owner's session, creating it (and the tmux
// server, if none runs) on first use. spare is the new session's empty first
// window: kill it once the first agent window exists. Sessions are targeted by
// id because ':' in the name breaks -t parsing.
func ProjectSession(owner string) (id, spare string, err error) {
	name := SessionName(owner)
	list, _ := RunTmux("list-sessions", "-F", "#{session_id} #{session_name}")
	for _, line := range strings.Split(list, "\n") {
		if i, n, _ := strings.Cut(strings.TrimSpace(line), " "); n == name {
			return i, "", nil
		}
	}
	out, err := RunTmuxCapture("new-session", "-d", "-s", name, "-c", owner, "-P", "-F", "#{session_id} #{window_id}")
	id, spare, _ = strings.Cut(strings.TrimSpace(out), " ")
	if err != nil || id == "" {
		return "", "", fmt.Errorf("could not start session %s: %v", name, err)
	}
	RunTmux("set-option", "-t", id, OwnerOption, owner)
	return id, spare, nil
}
