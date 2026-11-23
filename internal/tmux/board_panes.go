package tmux

import "strings"

// BoardPanes reads ownership metadata without agent detection or cleanup. A
// dispatched pane can still own a ticket while its harness metadata is absent.
func BoardPanes() ([]PaneInfo, error) {
	out, err := RunTmuxCapture("list-panes", "-a", "-F", "#{pane_id}|#{q:@board_target}|#{q:@pane_cwd}|#{q:pane_current_path}")
	if err != nil {
		return nil, err
	}
	var panes []PaneInfo
	for _, line := range strings.Split(out, "\n") {
		p := splitTmuxFields(line, '|')
		if len(p) != 4 || p[1] == "" {
			continue
		}
		path := p[2]
		if path == "" {
			path = p[3]
		}
		panes = append(panes, PaneInfo{PaneID: p[0], BoardTarget: p[1], Path: path})
	}
	return panes, nil
}
