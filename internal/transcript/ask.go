package transcript

import (
	"regexp"
	"strings"
)

// Ask is the question Claude's AskUserQuestion shows on screen. Claude writes the
// tool call to its transcript only after it is answered, so while it waits the
// screen is the one place it can be read from.
type Ask struct {
	Header   string      `json:"header,omitempty"`
	Question string      `json:"question"`
	Options  []AskOption `json:"options"`
	Multi    bool        `json:"multi,omitempty"`
	Other    bool        `json:"other,omitempty"`   // free text or "chat about this" exist: answer those in Terminal
	Clipped  bool        `json:"clipped,omitempty"` // the pane is too short to show every option
}
type AskOption struct {
	N           int    `json:"n"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

var (
	optionLine = regexp.MustCompile(`^[\s❯↑↓>]*(\d+)\.\s+(.*\S)\s*$`)
	ruleLine   = regexp.MustCompile(`^\s*─{3,}\s*$`)
	moreLine   = regexp.MustCompile(`^\s*[↑↓]\s*\d+ more lines?\s*$`)
	headerLine = regexp.MustCompile(`^\s*[☐☒✔←]\s*(.*\S)`)
)

// ParseAsk returns the pending AskUserQuestion on a Claude screen, or nil. Permission
// prompts have a different footer and are deliberately not matched: those stay in Terminal.
func ParseAsk(screen string) *Ask {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	foot := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "Enter to select") {
			foot = i
			break
		}
	}
	if foot < 0 {
		return nil
	}
	// The box sits at the bottom of the screen; an old footer in the scrollback is not a pending question.
	below := 0
	for _, l := range lines[foot+1:] {
		if strings.TrimSpace(l) != "" {
			below++
		}
	}
	if below > 4 {
		return nil
	}
	a := &Ask{Multi: strings.Contains(lines[foot], "Space")}
	var rev []AskOption
	started := false
	desc := "" // scanning upward, a description is met before the option it belongs to
	for i := foot - 1; i >= 0; i-- {
		line := lines[i]
		if moreLine.MatchString(line) {
			a.Clipped = true
			continue
		}
		if strings.TrimSpace(line) == "" || ruleLine.MatchString(line) {
			continue
		}
		if m := optionLine.FindStringSubmatch(line); m != nil {
			n := 0
			for _, c := range m[1] {
				n = n*10 + int(c-'0')
			}
			rev = append(rev, AskOption{N: n, Label: m[2], Description: desc})
			desc, started = "", true
			continue
		}
		if !started {
			continue
		}
		if strings.HasPrefix(line, "   ") {
			desc = strings.TrimSpace(strings.TrimSpace(line) + " " + desc)
			continue
		}
		if m := headerLine.FindStringSubmatch(line); m != nil {
			a.Header = m[1]
			break
		}
		a.Question = strings.TrimSpace(line)
		// The tab header ("☐ Pick one") sits just above the question.
		for j := i - 1; j >= 0 && j >= i-2; j-- {
			if m := headerLine.FindStringSubmatch(lines[j]); m != nil {
				a.Header = m[1]
			}
		}
		break
	}
	for i := len(rev) - 1; i >= 0; i-- {
		o := rev[i]
		if strings.HasPrefix(o.Label, "Type something") || strings.HasPrefix(o.Label, "Chat about this") {
			a.Other = true
			continue
		}
		if strings.HasPrefix(o.Label, "[") { // multi-select check boxes: "[ ] Label"
			a.Multi = true
		}
		a.Options = append(a.Options, o)
	}
	if len(a.Options) == 0 {
		return nil
	}
	if a.Question == "" {
		a.Clipped = true // the top of the box scrolled off
	}
	return a
}
