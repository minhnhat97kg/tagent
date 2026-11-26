package sidebar

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// Standalone mode: tagent run in a plain terminal (no $TMUX) shows the
// dashboard on the left and the live screen of the selected agent's pane on
// the right. Enter focuses the pane (keys go to the agent), ctrl+] returns.

// Any of these returns from the agent to the list (ctrl+] alone did not reach
// some terminals/keyboards); a click on the list works too.
func isLeaveKey(s string) bool { return s == "ctrl+]" || s == "ctrl+\\" || s == "ctrl+^" }

type standaloneModel struct {
	left       Model
	w, h       int
	focusRight bool
	paneID     string // pane shown on the right
	shell      string // fallback shell pane
	shellAsked bool
	screen     string // its last capture
	prepared   string // pane whose window was last resized/zoomed
	preparedW  int
	preparedH  int
	capturing  bool
	cx, cy     int      // the agent's cursor, in screen cells
	cursorOn   bool     // and whether it is visible
	layout     int      // layoutSplit…
	leftView   string   // rendered dashboard, reused across screen polls
	borrowed   string   // window of the user's own session we resized
	idlePanes  []string // idle shells seen by the last cleanup sweep
}

type paneShotMsg struct {
	id, text string
	cx, cy   int
	cursorOn bool
}
type shotTickMsg struct{}

type shellMsg struct{ id string }

// openShell reuses an empty pane of the cwd's project session or starts a plain shell.
func openShell(cwd string) tea.Cmd {
	return func() tea.Msg {
		session, spare, err := tmux.ProjectSession(tasks.MainRoot(cwd))
		if err != nil {
			return shellMsg{}
		}
		target := session + ":"
		if spare != "" { // fresh session: its first window is the shell
			id, _ := tmux.RunTmuxCapture("display-message", "-p", "-t", spare, "#{pane_id}")
			return shellMsg{id: strings.TrimSpace(id)}
		}
		panes, _ := tmux.RunTmux("list-panes", "-s", "-t", session, "-F", "#{pane_id} #{@pane_agent}")
		for _, line := range strings.Split(panes, "\n") {
			if fields := strings.Fields(line); len(fields) == 1 {
				return shellMsg{id: fields[0]}
			}
		}
		id, _ := tmux.RunTmuxCapture("new-window", "-d", "-t", target, "-c", cwd, "-P", "-F", "#{pane_id}")
		return shellMsg{id: strings.TrimSpace(id)}
	}
}

// newStandaloneModel is the dashboard for a standalone run, scoped to the repo
// it was started in (not the cursor's pane, which may be in $HOME).
func newStandaloneModel() Model {
	left := NewModel("")
	left.EnableDashboard()
	left.standalone = true
	cwd, _ := os.Getwd()
	left.launchDir = tasks.MainRoot(cwd)
	return left
}

func runStandalone() int {
	left := newStandaloneModel()
	m := standaloneModel{left: left}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	done := make(chan struct{})
	startBackgroundWorkers(p, done)
	final, err := p.Run()
	close(done)
	if sm, ok := final.(standaloneModel); ok {
		restoreWindow(sm.borrowed)
	}
	if err != nil {
		return 1
	}
	return 0
}

// Layouts, cycled with z (the terminal can't resize the split by dragging).
const (
	layoutSplit     = iota // dashboard ~1/3, agent the rest
	layoutHalf             // half and half
	layoutAgent            // the agent alone
	layoutDashboard        // the dashboard alone
	layoutCount
)

var layoutNames = [layoutCount]string{"split", "half", "agent", "dashboard"}

func (m standaloneModel) leftWidth() int {
	switch m.layout {
	case layoutHalf:
		return m.w / 2
	case layoutAgent:
		return 0
	case layoutDashboard:
		return m.w
	}
	return min(48, max(36, m.w/3))
}

// rightX is the first column of the agent's screen.
func (m standaloneModel) rightX() int {
	if lw := m.leftWidth(); lw > 0 {
		return lw + 1
	}
	return 0
}

// listKeys: the dashboard's agent list has the keys (not a form, the board,
// the help or another panel), so Enter/N/z are ours to take.
func (m standaloneModel) listKeys() bool {
	l := m.left
	return l.focus == cardAgents && !l.boardMode && !l.helpOpen && l.taskForm == "" && !l.layoutEdit
}

// rightSize is the area inside the agent's frame: the frame takes 2 columns and
// 3 rows (top border with the title, bottom border, and the hint line under it).
func (m standaloneModel) rightSize() (int, int) {
	return max(0, m.w-m.rightX()-2), max(3, m.h-3)
}

func (m standaloneModel) Init() tea.Cmd {
	return tea.Batch(m.left.Init(), shotTick(false), cleanupTick())
}

// shotTick polls the agent's screen: fast while typing into it, slower when
// only watching.
func shotTick(typing bool) tea.Cmd {
	d := 150 * time.Millisecond
	if typing {
		d = 40 * time.Millisecond
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return shotTickMsg{} })
}

type shotNowMsg struct{}

// capture grabs the pane's screen and cursor in one tmux call.
func capture(id string) tea.Cmd {
	return func() tea.Msg {
		out, _ := tmux.RunTmux("capture-pane", "-e", "-p", "-t", id, ";",
			"display-message", "-p", "-t", id, "#{cursor_x} #{cursor_y} #{cursor_flag}")
		out = strings.TrimRight(out, "\n")
		msg := paneShotMsg{id: id}
		i := strings.LastIndexByte(out, '\n')
		if f := strings.Fields(out[i+1:]); len(f) == 3 {
			msg.cx, _ = strconv.Atoi(f[0])
			msg.cy, _ = strconv.Atoi(f[1])
			msg.cursorOn = f[2] == "1"
			out = out[:max(i, 0)]
		}
		msg.text = out
		return msg
	}
}

func (m standaloneModel) selected() string {
	id, _ := paneIDAt(m.left.rows, m.left.cursor)
	return id
}

// Update re-renders the dashboard only when a message could have changed it:
// screen polls arrive many times a second and leave it alone.
func (m standaloneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	sm := next.(standaloneModel)
	switch msg.(type) {
	case shotTickMsg, shotNowMsg, paneShotMsg:
		if sm.leftView != "" {
			return sm, cmd
		}
	}
	sm.leftView = lipgloss.NewStyle().Width(max(1, sm.leftWidth())).Height(sm.h).MaxHeight(sm.h).Render(sm.left.View())
	return sm, cmd
}

func (m standaloneModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		updated, cmd := m.left.Update(tea.WindowSizeMsg{Width: max(1, m.leftWidth()), Height: msg.Height})
		m.left = updated.(Model)
		return m.follow(cmd)

	case shotTickMsg:
		if m.capturing || m.paneID == "" {
			return m, shotTick(m.focusRight)
		}
		m.capturing = true
		return m, tea.Batch(shotTick(m.focusRight), capture(m.paneID))

	case cleanupTickMsg:
		return m, tea.Batch(cleanupTick(), sweepIdlePanes(m.idlePanes, []string{m.paneID, m.shell}))

	case cleanupMsg:
		m.idlePanes = msg.idle
		return m, nil

	case shotNowMsg: // right after a key: show the echo without waiting a tick
		if m.capturing || m.paneID == "" {
			return m, nil
		}
		m.capturing = true
		return m, capture(m.paneID)

	case fittedMsg:
		m.borrowed = msg.borrowed
		return m, nil

	case shellMsg:
		m.shell = msg.id
		return m.follow(nil)

	case paneShotMsg:
		m.capturing = false
		if msg.id == m.shell && msg.text == "" && tmux.DisplayMessage(msg.id, "#{pane_id}") == "" {
			m.shell, m.shellAsked = "", false
			return m.follow(nil)
		}
		if msg.id == m.paneID {
			m.screen, m.cx, m.cy, m.cursorOn = msg.text, msg.cx, msg.cy, msg.cursorOn
		}
		return m, nil

	case tea.KeyMsg:
		if m.focusRight {
			if isLeaveKey(msg.String()) {
				m.focusRight = false
				return m, nil
			}
			return m, tea.Batch(forwardKey(m.paneID, msg),
				tea.Tick(25*time.Millisecond, func(time.Time) tea.Msg { return shotNowMsg{} }))
		}
		if !m.listKeys() {
			break
		}
		switch msg.String() {
		case "enter":
			// A pending approval keeps its own Enter (reveal the answers).
			if _, pending := m.left.dialogs[m.selected()]; !pending || m.left.dialogOpen[m.selected()] {
				if m.paneID != "" {
					if m.layout == layoutDashboard {
						m.layout = layoutSplit
						return m.relayout()
					}
					m.focusRight = true
					return m, nil
				}
			}
		case "N":
			m.left.taskForm, m.left.taskInput, m.left.taskNotice = "new-agent", "", ""
			m.left.newAgent = newAgentForm{harness: m.left.defaultNewAgentHarness(), roles: m.left.newAgentRoles()}
			return m, nil
		case "z":
			m.layout = (m.layout + 1) % layoutCount
			return m.relayout()
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && m.left.drag != nil { // a press anywhere ends a drag whose release was lost
			m.left = m.left.cancelDrag()
		}
		dragging := m.left.drag != nil && msg.Action != tea.MouseActionPress // a drag may cross into the right pane or the separator
		if !dragging && m.layout != layoutDashboard && msg.X >= m.rightX() && msg.Y >= 0 {
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.paneID != "" {
				m.focusRight = true
			}
			if m.focusRight {
				rw, rh := m.rightSize()
				if x, y := msg.X-m.rightX()-1, msg.Y-1; x >= 0 && x < rw && y >= 0 && y < rh { // inside the frame
					forwardMouse(m.paneID, msg, x, y)
				}
			}
			return m, nil
		}
		if !dragging && (m.layout == layoutAgent || msg.X == m.leftWidth()) { // the separator (or no list shown)
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			m.focusRight = false // clicking the list leaves the agent
		}
	}
	updated, cmd := m.left.Update(msg)
	m.left = updated.(Model)
	if _, ok := msg.(RefreshMsg); ok && m.selected() == "" && m.shell == "" && !m.shellAsked {
		m.shellAsked = true
		cwd, _ := os.Getwd()
		cmd = tea.Batch(cmd, openShell(cwd))
	}
	return m.follow(cmd)
}

// relayout resizes the dashboard and the agent's pane to the current layout.
func (m standaloneModel) relayout() (tea.Model, tea.Cmd) {
	m.leftView = ""
	updated, cmd := m.left.Update(tea.WindowSizeMsg{Width: max(1, m.leftWidth()), Height: m.h})
	m.left = updated.(Model)
	m.prepared = "" // refit the pane
	return m.follow(cmd)
}

// follow keeps the right side on the selected pane, preparing its window.
func (m standaloneModel) follow(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	id := m.selected()
	if id == "" {
		id = m.shell
	}
	w, h := m.rightSize()
	if id == m.paneID && id == m.prepared && w == m.preparedW && h == m.preparedH {
		return m, cmd
	}
	if id != m.paneID {
		m.screen = ""
	}
	m.paneID, m.prepared, m.preparedW, m.preparedH = id, id, w, h
	prev := m.borrowed
	m.borrowed = ""
	return m, tea.Batch(cmd, func() tea.Msg {
		restoreWindow(prev)
		return fitPane(id, w, h)
	})
}

type fittedMsg struct{ borrowed string }

// fitPane makes the agent's pane exactly w×h by zooming it in its window.
// A window of the user's own sessions is borrowed even while a client shows
// it (that client sees it zoomed meanwhile) and handed back (restoreWindow)
// when we move on or quit.
func fitPane(id string, w, h int) tea.Msg {
	if id == "" {
		return nil
	}
	f := strings.Fields(tmux.DisplayMessage(id, "#{@agent_hidden} #{window_id} #{window_zoomed_flag}"))
	if len(f) != 3 {
		return nil
	}
	win, wasZoomed := f[1], f[2] == "1"
	active := tmux.DisplayMessage(win, "#{pane_id}") // window's active pane, given back later
	tmux.RunTmux("resize-window", "-t", win, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h))
	tmux.RunTmux("select-pane", "-t", id)
	if tmux.DisplayMessage(id, "#{window_zoomed_flag}") != "1" {
		tmux.RunTmux("resize-pane", "-Z", "-t", id)
	}
	if f[0] == "1" {
		return nil
	}
	if wasZoomed {
		active = "" // the user had it zoomed: give it back zoomed
	}
	return fittedMsg{borrowed: win + " " + active}
}

// restoreWindow undoes fitPane on a window of the user's own session
// ("<window> <pane active before>", no pane when it was zoomed already):
// unzoom, reselect that pane and let tmux size the window to its clients.
func restoreWindow(borrowed string) {
	win, active, _ := strings.Cut(borrowed, " ")
	if win == "" {
		return
	}
	if active != "" {
		if tmux.DisplayMessage(win, "#{window_zoomed_flag}") == "1" {
			tmux.RunTmux("resize-pane", "-Z", "-t", win)
		}
		tmux.RunTmux("select-pane", "-t", active)
	}
	tmux.RunTmux("set-option", "-wu", "-t", win, "window-size")
}

// keyQueue serializes keys typed into the agent: one goroutine runs the
// send-keys calls in order (tea.Cmds run concurrently and would reorder them).
var keyQueue = func() chan func() {
	q := make(chan func(), 512)
	go func() {
		for f := range q {
			f()
		}
	}()
	return q
}()

// forwardKey types a key press into the agent's pane.
func forwardKey(id string, k tea.KeyMsg) tea.Cmd {
	key, literal := tmuxKey(k)
	if id == "" || key == "" {
		return nil
	}
	keyQueue <- func() {
		if literal {
			tmux.RunTmux("send-keys", "-t", id, "-l", key)
		} else {
			tmux.RunTmux("send-keys", "-t", id, key)
		}
	}
	return nil
}

// forwardMouse passes a mouse event to the agent as an SGR report, but only
// when the agent asked for mouse input (x, y are 0-based inside its screen).
func forwardMouse(id string, e tea.MouseMsg, x, y int) {
	if id == "" || x < 0 {
		return
	}
	var code int
	switch e.Button {
	case tea.MouseButtonLeft:
		code = 0
	case tea.MouseButtonMiddle:
		code = 1
	case tea.MouseButtonRight:
		code = 2
	case tea.MouseButtonWheelUp:
		code = 64
	case tea.MouseButtonWheelDown:
		code = 65
	default:
		return
	}
	end := "M"
	if e.Action == tea.MouseActionRelease {
		end = "m"
	} else if e.Action == tea.MouseActionMotion {
		code += 32
	}
	seq := "\x1b[<" + strconv.Itoa(code) + ";" + strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1) + end
	keyQueue <- func() {
		if tmux.DisplayMessage(id, "#{mouse_any_flag}") == "1" {
			tmux.RunTmux("send-keys", "-t", id, "-l", seq)
		}
	}
}

var specialKeys = map[string]string{
	"enter": "Enter", "tab": "Tab", "backspace": "BSpace", "esc": "Escape",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right", "delete": "DC", "insert": "IC",
	"home": "Home", "end": "End", "pgup": "PPage", "pgdown": "NPage", " ": "Space",
}

// tmuxKey maps a Bubble Tea key to a `send-keys` argument; literal means
// send-keys -l (plain text). Modifiers (ctrl+, alt+, shift+) combine with a
// named key, an F-key or a single character.
func tmuxKey(k tea.KeyMsg) (key string, literal bool) {
	if k.Type == tea.KeyRunes && !k.Alt {
		return string(k.Runes), true
	}
	s := k.String()
	if s == "shift+tab" {
		return "BTab", false
	}
	mods := ""
	for {
		switch {
		case strings.HasPrefix(s, "ctrl+") && len(s) > 5:
			mods, s = mods+"C-", s[5:]
			continue
		case strings.HasPrefix(s, "alt+") && len(s) > 4:
			mods, s = mods+"M-", s[4:]
			continue
		case strings.HasPrefix(s, "shift+") && len(s) > 6:
			mods, s = mods+"S-", s[6:]
			continue
		}
		break
	}
	if name, ok := specialKeys[s]; ok {
		return mods + name, false
	}
	if len(s) >= 2 && s[0] == 'f' {
		if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= 20 {
			return mods + "F" + s[1:], false
		}
	}
	if len([]rune(s)) == 1 && mods != "" {
		return mods + s, false
	}
	return "", false
}

func (m standaloneModel) View() string {
	if m.w == 0 {
		return ""
	}
	lw := m.leftWidth()
	left := m.leftView
	if left == "" && lw > 0 {
		left = lipgloss.NewStyle().Width(lw).Height(m.h).MaxHeight(m.h).Render(m.left.View())
	}
	if m.layout == layoutDashboard {
		return left
	}
	rw, rh := m.rightSize()

	dim := lipgloss.NewStyle().Foreground(m.left.theme.TextDim)
	title := "no agent selected — N starts one in this directory"
	hint := [][2]string{{"N", "new agent"}, {"z", "layout: " + layoutNames[m.layout]}}
	if m.paneID != "" {
		title = m.paneID
		if m.paneID == m.shell && m.selected() == "" {
			title = "shell"
			hint = [][2]string{{"N", "start an agent"}}
		}
		if r, ok := paneRowAt(m.left.rows, m.left.cursor); ok {
			title = r.Base + " · " + string(r.Agent) + " · " + string(r.Status)
		}
		if m.focusRight {
			hint = [][2]string{{"typing into the agent", ""}, {"ctrl+]", "leave"}, {"ctrl+\\", "leave"}, {"click list", "leave"}}
		} else {
			hint = append([][2]string{{"enter", "type"}}, hint...)
		}
	}
	th := m.left.theme
	frame := th.Border
	if m.focusRight {
		frame = th.BorderFocus
	}
	fs := lipgloss.NewStyle().Foreground(frame)
	tl, tr, bl, br, hz, vt := "╭", "╮", "╰", "╯", "─", "│"
	if m.focusRight { // heavy border while typing into the agent
		tl, tr, bl, br, hz, vt = "┏", "┓", "┗", "┛", "━", "┃"
	}
	ts := lipgloss.NewStyle().Foreground(th.TextPrimary).Bold(true)
	label := " " + ansi.Truncate(title, max(1, rw-4), "…") + " "
	lines := []string{fs.Render(tl+hz) + ts.Render(label) + fs.Render(strings.Repeat(hz, max(0, rw-ansi.StringWidth(label)-1))+tr)}
	body := strings.Split(strings.TrimRight(m.screen, "\n"), "\n")
	if len(body) > rh { // keep the bottom: that's where the agent's prompt is
		body = body[len(body)-rh:]
	}
	skipped := len(strings.Split(strings.TrimRight(m.screen, "\n"), "\n")) - len(body)
	edge := fs.Render(vt)
	for i, l := range body {
		l = ansi.Truncate(l, rw, "")
		if m.cursorOn && i+skipped == m.cy && m.cx < rw {
			l = withCursor(l, m.cx, rw, m.focusRight)
		}
		lines = append(lines, edge+l+"\x1b[0m"+strings.Repeat(" ", max(0, rw-ansi.StringWidth(l)))+edge)
	}
	for len(lines) < rh+1 {
		lines = append(lines, edge+strings.Repeat(" ", rw)+edge)
	}
	lines = append(lines, fs.Render(bl+strings.Repeat(hz, rw)+br))
	keyStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var hints []string
	for _, h := range hint {
		if h[1] == "" {
			hints = append(hints, dim.Render(h[0]))
			continue
		}
		hints = append(hints, keyStyle.Render(h[0])+" "+dim.Render(h[1]))
	}
	lines = append(lines, " "+ansi.Truncate(strings.Join(hints, "  "), max(0, rw), "…"))
	// Joined by hand: lipgloss re-measuring every ANSI line of the agent's
	// screen on each poll was the main cost of a frame.
	sep := dim.Render("│")
	leftLines := strings.Split(left, "\n")
	var b strings.Builder
	for i := 0; i < m.h; i++ {
		l, r := "", ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(lines) {
			r = lines[i]
		}
		if lw > 0 {
			b.WriteString(l)
			b.WriteString(strings.Repeat(" ", max(0, lw-ansi.StringWidth(l))))
			b.WriteString(sep)
		}
		b.WriteString(r)
		b.WriteString("\x1b[0m")
		if i < m.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// withCursor draws the agent's cursor at column x of an ANSI line: reversed
// while typing into it, underlined otherwise.
func withCursor(line string, x, width int, active bool) string {
	line += strings.Repeat(" ", max(0, x+1-ansi.StringWidth(line)))
	cell := ansi.Strip(ansi.Cut(line, x, x+1))
	if cell == "" {
		cell = " "
	}
	style := "\x1b[4m"
	if active {
		style = "\x1b[7m"
	}
	return ansi.Cut(line, 0, x) + style + cell + "\x1b[0m" + ansi.Cut(line, x+1, max(x+1, width))
}
