package tmux

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// nicknames are short and easy to say. None may be a harness or role name.
var nicknames = strings.Fields(`
	ada bao bamboo basil bean biscuit bobo cleo cocoa coco dino felix fig gigi ginger hana
	iris juno kiki kiwi koi lala latte lemon lilo luna lychee mango maple miso mochi momo
	nacho nemo noodle nori olive otto panda peach pepper pico pip plum poppy pudding ruby sage
	sesame sunny taco tofu toto waffle yuki yuzu ziggy zuzu apple berry cherry melon pearl robin`)

// nickname picks the first unused word, starting at seed: bao, then bao-2 once
// every word is taken. The same seed and used set give the same name, so
// processes naming one pane at the same time agree.
func nickname(seed int, used map[string]bool) string {
	for round := 1; ; round++ {
		for i := range nicknames {
			name := nicknames[(seed+i)%len(nicknames)]
			if round > 1 {
				name += "-" + strconv.Itoa(round)
			}
			if !used[name] {
				return name
			}
		}
	}
}

// nameAgentPanes gives each agent pane that has no name a nickname, stored in
// @pane_name so it stays for the pane's life (and across `tagent reload`).
// The seed is the pane's PID, not its id: ids restart at %0 with a new tmux
// server, and `restore` matches saved agents by name.
func nameAgentPanes(sessions []SessionInfo) {
	var used map[string]bool
	for i := range sessions {
		for j := range sessions[i].Windows {
			for k := range sessions[i].Windows[j].Panes {
				p := &sessions[i].Windows[j].Panes[k]
				if p.Name != "" || p.Agent == "" {
					continue
				}
				if used == nil { // every pane's name, also those not listed as agents
					out, ok := RunTmux("list-panes", "-a", "-F", "#{"+PaneName+"}")
					if !ok {
						return
					}
					used = map[string]bool{}
					for _, name := range strings.Split(out, "\n") {
						used[strings.TrimSpace(name)] = true
					}
				}
				p.Name = nickname(p.PanePID, used)
				used[p.Name] = true
				// -o: never overwrite a name set since the query.
				RunTmux("set", "-t", p.PaneID, "-p", "-o", PaneName, p.Name)
			}
		}
	}
}

// RenamePane gives pane a name no other pane has; "" clears it, and the next
// query gives an agent pane a new nickname.
func RenamePane(pane, name string) error {
	if strings.HasPrefix(name, "%") || strings.ContainsAny(name, " \t|") {
		return errors.New("a name has no spaces, \"|\" or leading %")
	}
	if name == "" {
		UnsetPaneOption(pane, PaneName)
		return nil
	}
	if owner := ResolvePane(name); owner != name && owner != pane {
		return fmt.Errorf("%q is already used by another pane (%s)", name, owner)
	}
	SetPaneOption(pane, PaneName, name)
	return nil
}

// PaneLabel is how a message calls a pane: its name (given now to an agent pane
// that has none yet), else the id.
func PaneLabel(pane string) string {
	if name := GetPaneOptionValue(pane, PaneName); name != "" {
		return name
	}
	QuerySessions()
	if name := GetPaneOptionValue(pane, PaneName); name != "" {
		return name
	}
	return pane
}
