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
	OtherN   int         `json:"otherN,omitempty"`  // number of the "Type something" row: select it, then type
	Summary  string      `json:"summary,omitempty"` // the answers so far on a review screen
}
type AskOption struct {
	N           int    `json:"n"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Checked     bool   `json:"checked,omitempty"` // multi-select box ticked
}

var (
	optionLine = regexp.MustCompile(`^[\s❯↑↓>]*(\d+)\.\s+(.*\S)\s*$`)
	ruleLine   = regexp.MustCompile(`^\s*─{3,}\s*$`)
	moreLine   = regexp.MustCompile(`^\s*[↑↓]\s*\d+ more lines?\s*$`)
	checkBox   = regexp.MustCompile(`^\[(.)\]\s*(.*)$`)
	headerLine = regexp.MustCompile(`^\s*(?:←\s*)?[☐☒✔]\s*([^\s→][^→]*?)(\s{2,}.*)?\s*$`)
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
		return parseReview(lines)
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
		if m := checkBox.FindStringSubmatch(o.Label); m != nil { // multi-select: "[ ] Label" / "[✔] Label"
			a.Multi = true
			o.Checked = m[1] != " "
			o.Label = m[2]
		}
		if strings.HasPrefix(o.Label, "Type something") {
			a.Other, a.OtherN = true, o.N
			continue
		}
		if strings.HasPrefix(o.Label, "Chat about this") {
			a.Other = true
			continue
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

// parseReview reads the "Review your answers" screen shown after a multi-select (no "Enter to select" footer).
func parseReview(lines []string) *Ask {
	at := -1
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-14; i-- {
		if strings.Contains(lines[i], "Review your answers") {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	a := &Ask{Header: "Review", Question: "Review your answers"}
	var summary []string
	for _, line := range lines[at+1:] {
		if m := optionLine.FindStringSubmatch(line); m != nil {
			n := 0
			for _, c := range m[1] {
				n = n*10 + int(c-'0')
			}
			a.Options = append(a.Options, AskOption{N: n, Label: m[2]})
			continue
		}
		if t := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "●→")); t != "" && !ruleLine.MatchString(line) && len(a.Options) == 0 {
			summary = append(summary, t)
		}
	}
	if len(a.Options) == 0 {
		return nil
	}
	a.Summary = strings.Join(summary, " · ")
	return a
}
