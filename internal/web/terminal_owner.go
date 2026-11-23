package web

import (
	"fmt"
	"strings"
	"sync"
	"tagent/internal/tmux"
)

type terminalOwners struct {
	mu  sync.Mutex
	run func(...string) (string, bool)
}

func (o *terminalOwners) command(args ...string) (string, bool) {
	if o.run != nil {
		return o.run(args...)
	}
	return tmux.RunTmux(args...)
}
func (o *terminalOwners) option(pane, key string) (string, bool) {
	v, ok := o.command("display-message", "-p", "-t", pane, "#{"+key+"}")
	return strings.TrimSpace(v), ok
}

// tmux, not this process, is authoritative across servers and the TUI.
func (o *terminalOwners) resize(pane, viewer string, size terminalSize, claim, auto bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	owner, ok := o.option(pane, "@agent_size_owner")
	if !ok {
		return fmt.Errorf("cannot read pane size owner")
	}
	if !claim && owner != "web:"+viewer && (owner != "" || !auto) {
		return nil
	}
	if !strings.HasPrefix(owner, "web:") {
		original, ok := o.command("show-options", "-w", "-v", "-t", pane, "window-size")
		if !ok {
			return fmt.Errorf("cannot read window sizing policy")
		}
		policy := "inherit"
		if original = strings.TrimSpace(original); original != "" {
			policy = "value:" + original
		}
		if _, ok = o.command("set-option", "-p", "-t", pane, "@agent_size_policy", policy); !ok {
			return fmt.Errorf("cannot save window policy")
		}
	}
	if _, ok = o.command("set-option", "-p", "-t", pane, "@agent_size_owner", "web:"+viewer); !ok {
		return fmt.Errorf("cannot claim size owner")
	}
	if _, ok = o.command("resize-window", "-t", pane, "-x", fmt.Sprint(size.Cols), "-y", fmt.Sprint(size.Rows)); !ok {
		if owner == "" {
			o.command("set-option", "-p", "-t", pane, "@agent_size_owner", "")
		} else {
			o.command("set-option", "-p", "-t", pane, "@agent_size_owner", owner)
		}
		return fmt.Errorf("cannot resize pane window")
	}
	// In a window with several panes the window size is shared out; zoom this pane so it gets all of it.
	if info, ok := o.command("display-message", "-p", "-t", pane, "#{window_panes} #{window_zoomed_flag}"); ok {
		if f := strings.Fields(info); len(f) == 2 && f[0] != "1" && f[1] == "0" {
			if _, ok := o.command("resize-pane", "-Z", "-t", pane); ok {
				o.command("set-option", "-p", "-t", pane, "@agent_size_zoomed", "1")
			}
		}
	}
	return nil
}
func (o *terminalOwners) owns(pane, viewer string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	owner, ok := o.option(pane, "@agent_size_owner")
	return ok && owner == "web:"+viewer
}
func (o *terminalOwners) release(pane, viewer string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	owner, ok := o.option(pane, "@agent_size_owner")
	if !ok || owner != "web:"+viewer {
		return nil
	}
	policy, _ := o.option(pane, "@agent_size_policy")
	if _, ok = o.command("set-option", "-p", "-t", pane, "@agent_size_owner", ""); !ok {
		return fmt.Errorf("cannot release owner")
	}
	restore := func() bool {
		if strings.HasPrefix(policy, "value:") {
			_, ok := o.command("set-option", "-w", "-t", pane, "window-size", strings.TrimPrefix(policy, "value:"))
			return ok
		}
		_, ok := o.command("set-option", "-w", "-u", "-t", pane, "window-size")
		return ok
	}
	if zoomed, _ := o.option(pane, "@agent_size_zoomed"); zoomed == "1" {
		if info, ok := o.command("display-message", "-p", "-t", pane, "#{window_zoomed_flag}"); ok && strings.TrimSpace(info) == "1" {
			o.command("resize-pane", "-Z", "-t", pane)
		}
		o.command("set-option", "-p", "-u", "-t", pane, "@agent_size_zoomed")
	}
	first := restore()
	_, resized := o.command("resize-window", "-A", "-t", pane)
	last := restore()
	o.command("set-option", "-p", "-u", "-t", pane, "@agent_size_policy")
	if !first || !resized || !last {
		return fmt.Errorf("could not restore tmux sizing")
	}
	return nil
}
