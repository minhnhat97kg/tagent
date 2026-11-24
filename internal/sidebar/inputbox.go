package sidebar

import tea "github.com/charmbracelet/bubbletea"

// editText is the one text-input rule every TUI form shares (comment box, task and agent
// forms, workflow name, new agent): backspace/delete, space and typed or pasted runes
// edit the end of text. It reports whether the key was an edit.
func editText(text string, key tea.KeyMsg) (string, bool) {
	switch key.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		if r := []rune(text); len(r) > 0 {
			text = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		text += " "
	case tea.KeyRunes:
		text += string(key.Runes)
	default:
		return text, false
	}
	return text, true
}
