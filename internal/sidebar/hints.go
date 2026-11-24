package sidebar

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func wrappedInput(input string, width, height int) string {
	lines := strings.Split(ansi.Wrap(input+"▏", max(1, width-2), ""), "\n")
	limit := max(1, height/3)
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	lines[0] = "❯ " + strings.TrimPrefix(lines[0], "  ")
	return strings.Join(lines, "\n")
}

// keyHints keeps keys distinct from their quieter labels.
func (m Model) keyHints(pairs ...string) string {
	var out []string
	for _, pair := range pairs {
		key, label, _ := strings.Cut(pair, " ")
		out = append(out, m.boardStyle(m.theme.Accent).Bold(true).Render(key)+" "+m.boardStyle(m.theme.TextMuted).Render(label))
	}
	line := strings.Join(out, "  ")
	if w := ansi.StringWidth(line); w > m.width && m.width > 4 {
		// Too long for one row: run the text, one column every other tick.
		loop := line + "   " + line
		return ansi.Cut(loop, (m.animFrame/2)%(w+3), (m.animFrame/2)%(w+3)+m.width)
	}
	return boardFit(line, m.width)
}
