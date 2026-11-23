package tmux

import "fmt"

// CaptureTail is the shared CLI screen-read path.
func CaptureTail(pane string, lines int) (string, error) {
	return RunTmuxCapture("capture-pane", "-p", "-J", "-t", pane, "-S", fmt.Sprintf("-%d", lines))
}
