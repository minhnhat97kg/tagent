package sidebar

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/agents"
	"tagent/internal/roles"
)

// The N form in the Agents pane: the same fields the web's New agent composer
// sends to `tagent new` (repo agent, harness, model, worktree). The TUI has no
// project picker — it starts the agent in the repo of the pane under the
// cursor.

// newAgentFields are the form's rows, in the order they are shown.
const (
	newAgentRole = iota
	newAgentHarness
	newAgentModel
	newAgentWorktree
)

// newAgentForm is the form's state. taskInput holds the text of the row being
// edited (repo-agent or model); the other rows commit straight into f.
type newAgentForm struct {
	field    int
	roles    []string // repo agents to pick from; empty means the role row is free text
	role     string
	harness  int
	model    string
	worktree bool
}

// harnessName is the harness the form sends as --agent.
// models is the harness's pickable models; empty means the model row is free text.
func (f newAgentForm) models() []string { return agents.Models()[f.harnessName()] }

// typed reports whether the active row takes text: the repo agent, and the
// model only for a harness with no model list.
func (f newAgentForm) typed() bool {
	return (f.field == newAgentRole && len(f.roles) == 0) || (f.field == newAgentModel && len(f.models()) == 0)
}

func (f newAgentForm) harnessName() string {
	if f.harness < 0 || f.harness >= len(newAgentChoices) {
		return ""
	}
	return newAgentChoices[f.harness]
}

// commit saves the active text row's buffer into the form.
func (f *newAgentForm) commit(input string) {
	switch f.field {
	case newAgentRole:
		if f.typed() {
			f.role = strings.TrimSpace(input)
		}
	case newAgentModel:
		if f.typed() {
			f.model = strings.TrimSpace(input)
		}
	}
}

// buffer is the committed text of the active row, loaded into taskInput when
// the cursor moves onto it.
func (f newAgentForm) buffer() string {
	switch f.field {
	case newAgentRole:
		if f.typed() {
			return f.role
		}
	case newAgentModel:
		if f.typed() {
			return f.model
		}
	}
	return ""
}

// change moves the active picker row: the harness cycles, worktree toggles.
func (f *newAgentForm) change(dir int) {
	switch f.field {
	case newAgentHarness:
		n := len(newAgentChoices)
		if dir == 0 {
			dir = 1
		}
		f.harness = ((f.harness+dir)%n + n) % n
		f.model = "" // a model belongs to its harness
	case newAgentRole:
		opts := append([]string{""}, f.roles...)
		i := slices.Index(opts, f.role)
		if dir == 0 {
			dir = 1
		}
		f.role = opts[((i+dir)%len(opts)+len(opts))%len(opts)]
	case newAgentModel:
		// "" (harness default) then each listed model, cycling.
		opts := append([]string{""}, f.models()...)
		i := slices.Index(opts, f.model)
		if dir == 0 {
			dir = 1
		}
		f.model = opts[((i+dir)%len(opts)+len(opts))%len(opts)]
	case newAgentWorktree:
		f.worktree = dir > 0
	}
}

// handleNewAgentForm is the N form's key handler. Enter moves to the next row
// and starts the agent from the last one; ctrl+s starts it from any row; Esc
// cancels without starting anything.
func (m Model) handleNewAgentForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.newAgent
	text := f.typed()
	if s := key.String(); s != "enter" && s != "ctrl+s" {
		m.taskNotice = "" // typing or moving clears a validation error
	}
	switch key.Type {
	case tea.KeyEsc:
		m.taskForm, m.taskInput, m.newAgent = "", "", newAgentForm{}
		return m, nil
	case tea.KeyUp, tea.KeyDown, tea.KeyTab, tea.KeyShiftTab:
		f.commit(m.taskInput)
		step := 1
		if key.Type == tea.KeyUp || key.Type == tea.KeyShiftTab {
			step = -1
		}
		f.field = max(newAgentRole, min(newAgentWorktree, f.field+step))
		m.newAgent, m.taskInput = f, f.buffer()
		return m, nil
	case tea.KeyEnter:
		f.commit(m.taskInput)
		if f.field < newAgentWorktree {
			f.field++
			m.newAgent, m.taskInput = f, f.buffer()
			return m, nil
		}
		return m.submitNewAgent(f)
	case tea.KeyCtrlS:
		f.commit(m.taskInput)
		return m.submitNewAgent(f)
	case tea.KeyBackspace, tea.KeyDelete:
		if text {
			if r := []rune(m.taskInput); len(r) > 0 {
				m.taskInput = string(r[:len(r)-1])
			}
		}
		return m, nil
	case tea.KeySpace:
		if text {
			m.taskInput += " "
			return m, nil
		}
		if f.field == newAgentWorktree {
			f.worktree = !f.worktree
		} else {
			f.change(1)
		}
		m.newAgent = f
		return m, nil
	case tea.KeyLeft, tea.KeyRight:
		if !text {
			if key.Type == tea.KeyLeft {
				f.change(-1)
			} else {
				f.change(1)
			}
			m.newAgent = f
		}
		return m, nil
	case tea.KeyRunes:
		if text {
			m.taskInput += string(key.Runes)
			return m, nil
		}
		switch key.String() {
		case "j":
			f.field = min(newAgentWorktree, f.field+1)
		case "k":
			f.field = max(newAgentRole, f.field-1)
		case "h":
			f.change(-1)
		case "l":
			f.change(1)
		}
		m.newAgent = f
		return m, nil
	}
	return m, nil
}

// newAgentFormBody renders the form's rows with the active row marked. The
// active text row shows the tail of taskInput: that is what is being typed.
func (m Model) newAgentFormBody(width int) string {
	f := m.newAgent
	active := func(i int) string {
		if f.field == i {
			return "› "
		}
		return "  "
	}
	text := func(i int, label, committed, empty string) string {
		value := committed
		if value == "" {
			value = empty
		}
		if f.field == i {
			value = tailFit(m.taskInput+"▏", width-13)
			if m.taskInput == "" { // placeholder, as in the other text inputs
				value = m.boardStyle(m.theme.Accent).Render("▏") + m.boardStyle(m.theme.TextDim).Render(ansi.Truncate(empty, max(1, width-14), "…"))
			}
		}
		return boardFit(active(i)+fmt.Sprintf("%-11s", label)+value, width)
	}
	flag := func(i int, label, value string) string {
		return boardFit(active(i)+fmt.Sprintf("%-11s", label)+value, width)
	}
	modelRow := func() string {
		if f.typed() || f.field != newAgentModel && len(f.models()) == 0 {
			return text(newAgentModel, "model", f.model, "—")
		}
		return flag(newAgentModel, "model", cmp.Or(f.model, "default"))
	}
	roleRow := text(newAgentRole, "repo-agent", f.role, "none (plain agent)")
	if len(f.roles) > 0 {
		roleRow = flag(newAgentRole, "repo-agent", cmp.Or(f.role, "none (plain agent)"))
	}
	rows := m.newAgentRows(width, roleRow, flag, modelRow)
	if a, ok := agents.Find(m.newAgentCwd(), m.agentHarness(), f.role); ok && f.role != "" {
		// What the picked agent will be told: its first few lines.
		dim := m.boardStyle(m.theme.TextMuted)
		var lines []string
		for _, l := range strings.Split(a.Instructions, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, dim.Render(ansi.Truncate("  "+l, width, "…")))
			}
			if len(lines) == 4 {
				break
			}
		}
		rows += "\n\n" + strings.Join(lines, "\n")
	}
	return rows
}

func (m Model) newAgentRows(width int, roleRow string, flag func(int, string, string) string, modelRow func() string) string {
	f := m.newAgent
	return strings.Join([]string{
		roleRow,
		flag(newAgentHarness, "harness", f.harnessName()),
		modelRow(),
		flag(newAgentWorktree, "worktree", map[bool]string{true: "on", false: "off"}[f.worktree]),
	}, "\n")
}

// tailFit keeps the end of s — the part the user is typing.
func tailFit(s string, width int) string {
	r := []rune(s)
	if width <= 0 || len(r) <= width {
		return s
	}
	if width == 1 {
		return string(r[len(r)-1:])
	}
	return "…" + string(r[len(r)-(width-1):])
}

// submitNewAgent validates the form and starts the agent. The repo agent is
// optional, as it is in the web composer: an empty name starts a plain agent on
// the chosen harness. A validation error keeps the form open with the message
// inline (taskNotice); the command's own error comes back as a taskActionMsg,
// which also lands in taskNotice.
func (m Model) submitNewAgent(f newAgentForm) (tea.Model, tea.Cmd) {
	cwd := m.newAgentCwd()
	if f.role != "" {
		if _, ok := agents.Find(cwd, m.agentHarness(), f.role); !ok {
			var names []string
			for _, a := range agents.List(cwd, m.agentHarness(), nil) {
				names = append(names, a.Name)
			}
			return m.newAgentError(f, fmt.Sprintf("unknown repo agent %q (have: %s)", f.role, strings.Join(names, ", ")))
		}
	}
	m.taskForm, m.taskInput, m.newAgent = "", "", newAgentForm{}
	return m, spawnNewAgent(newAgentArgs(cwd, f))
}

// newAgentError keeps the form open on the repo-agent row with the reason.
func (m Model) newAgentError(f newAgentForm, reason string) (tea.Model, tea.Cmd) {
	f.field = newAgentRole
	m.newAgent, m.taskInput, m.taskNotice = f, f.role, "! "+reason
	return m, nil
}

// newAgentArgs is the `tagent new` command line for the form: the same flags
// the web's New agent action sends, plus --no-wait so the TUI stays responsive.
func newAgentArgs(cwd string, f newAgentForm) []string {
	args := []string{"new", "--no-wait", "--cwd", cwd}
	if f.role != "" {
		args = append(args, "--repo-agent", f.role)
	}
	if h := f.harnessName(); h != "" {
		args = append(args, "--agent", h)
	}
	if f.model != "" {
		args = append(args, "--model", f.model)
	}
	if f.worktree {
		args = append(args, "--worktree")
	}
	return args
}

// spawnNewAgent starts the pane in the background; the new row appears with
// the refresh the message triggers.
func spawnNewAgent(args []string) tea.Cmd {
	return func() tea.Msg {
		bin, err := os.Executable()
		if err != nil {
			return taskActionMsg{"! " + err.Error()}
		}
		if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
			return taskActionMsg{"! new: " + strings.TrimSpace(string(out))}
		}
		return refreshRequestMsg{}
	}
}

// newAgentCwd is the repo the agent starts in: the pane's repo, else this
// process's directory (standalone has no pane).
func (m Model) newAgentCwd() string {
	if m.standalone && m.launchDir != "" {
		return m.launchDir
	}
	// The pane's repo, unless it has no agents and the Agents pane's repo does.
	for _, d := range []string{m.repoDir, m.boardDir} {
		if d != "" && len(agents.List(d, m.agentHarness(), nil)) > 0 {
			return d
		}
	}
	for _, d := range []string{m.repoDir, m.boardDir} {
		if d != "" {
			return d
		}
	}
	d, _ := os.Getwd()
	return d
}

// defaultNewAgentHarness is the configured harness's row, else the first one.
func (m Model) defaultNewAgentHarness() int {
	newAgentChoices = roles.HarnessNames() // pick up edits to harnesses.yml
	for i, h := range newAgentChoices {
		if h == m.agentHarness() {
			return i
		}
	}
	return 0
}

// newAgentRoles are the repo agents the form can pick from.
func (m Model) newAgentRoles() []string {
	var names []string
	for _, a := range agents.List(m.newAgentCwd(), m.agentHarness(), nil) {
		names = append(names, a.Name)
	}
	return names
}
