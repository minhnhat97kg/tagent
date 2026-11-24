package sidebar

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// ticketsKey is the ticket lanes, on the board screen and (through
// tasksCardKey) on the dashboard's Tasks card.
func (m Model) ticketsKey(key string) (Model, tea.Cmd, bool) {
	if len(m.notes) > 0 && (key == "a" || key == "A") {
		m.taskForm = "attachments"
		if key == "A" {
			m.taskForm = "attach"
		}
		m.taskStep = 0
		m.taskInput = ""
		m.attachmentsKey = ""
		return m.syncAttachments(), nil, true
	}
	if key == "tab" || key == "shift+tab" {
		if m.boardMode && !m.boardDrawer {
			m.focus = cardWorkflows
			return m, nil, true
		}
	}
	switch key {
	case "t":
		m.boardMode = true
		m.agentsOpen = !m.agentsOpen
		m.agentDetail, m.agentCursor = false, 0
		m.focus = cardTasks
		if m.agentsOpen {
			m.focus = cardWorkflows
		}
		if m.agentsOpen {
			m = m.loadAgents()
		}
		return m, nil, true
	case "w":
		m.boardMode = true
		m.boardFlow, m.boardConfigView, m.boardDrawer = true, false, false
		m.flowWorkflow = ""
		return m, nil, true
	case "B": // read-only board config summary; E edits, like the workflow view
		m.boardMode = true
		m.boardFlow, m.boardConfigView, m.boardDrawer = true, true, false
		m.flowWorkflow, m.boardConfigScroll = "", 0
		return m, nil, true
	case "enter", "i":
		if !m.boardMode { // opened on the dashboard's Tasks card: back goes there
			m.detailFromDash = true
		}
		m.boardMode = true
		m.boardDrawer = !m.boardDrawer
		if !m.boardDrawer {
			m = m.closeDetail()
		}
		m.detailScroll = 0
		m.detailTab = detailTabDetails
		m.detailDescOpen, m.detailSysOpen = false, false
		return m, nil, true
	case "esc":
		if m.boardDrawer {
			return m.closeDetail(), nil, true
		}
		m.boardMode = false
		return m, nil, true
	case "q", "b":
		m.boardMode, m.boardDrawer, m.detailFromDash = false, false, false
		return m, nil, true
	case "c", "C": // comment on the selected ticket (a blocked one prefills the @role)
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, true, ""), true
		}
		return m, nil, true
	case "E": // edit .tasks/board.yaml
		return m, editBoardConfig(m.boardDir, m.tmuxPane), true
	case "n": // new ticket: pick its type (default: the board's), then write it in $EDITOR
		m.taskForm, m.taskStep = "new-task", m.workflowRow(m.boardConfig.DefaultWorkflow)
		return m, nil, true
	case "s":
		if len(m.notes) > 0 {
			m.taskForm, m.taskStep = "start", 0
			if w := m.notes[m.taskCursor].Workflow; w != "" {
				m.taskStep = m.workflowRow(w) // a stored type is what enter starts
			}
		}
		return m, nil, true
	case "p": // priority cycle: normal -> high -> low -> normal
		if len(m.notes) > 0 {
			scope, id, next := m.notesScope, m.notes[m.taskCursor].ID, tasks.NextPriority(m.notes[m.taskCursor].Priority)
			return m, func() tea.Msg { return actionResult("Priority "+next, tasks.SetPriority(scope, id, next)) }, true
		}
		return m, nil, true
	case "D": // dependencies: ticket picker, space toggles, Enter saves
		if len(m.notes) > 0 {
			m.taskForm, m.taskStep, m.taskInput = "needs", 0, strings.Join(m.notes[m.taskCursor].Needs, ",")
		}
		return m, nil, true
	case "m": // move: status picker, enter saves
		if len(m.notes) > 0 {
			cfg, e := tasks.ConfigForTask(m.boardDir, m.boardConfig, m.notes[m.taskCursor])
			if e != nil {
				m.taskNotice = e.Error()
				return m, nil, true
			}
			m.taskForm, m.taskStep = "status", max(0, slices.Index(cfg.Columns, m.notes[m.taskCursor].Status))
		}
		return m, nil, true
	case "@": // assign: role / agent picker, enter saves
		if len(m.notes) > 0 {
			m.taskForm, m.taskStep = "assign", 0
			if names := m.notes[m.taskCursor].Assignees; len(names) > 0 {
				m.taskStep = slices.Index(m.assignNames(), names[0]) + 1 // 0 is "unassigned"
			}
		}
		return m, nil, true
	case "e":
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, false, ""), true
		}
		return m, nil, true
	case "x":
		m.taskDelete = len(m.notes) > 0
		return m, nil, true
	}
	step, move := 0, false
	switch key {
	case "j", "down":
		step = 1
	case "k", "up":
		step = -1
	case "h", "l":
		if !m.boardDrawer {
			m = m.boardNavigateColumn(key)
		}
		return m, nil, true
	case "H":
		step, move = -1, true
	case "L":
		step, move = 1, true
	default:
		return m, nil, false // only this pane's keys act here; the global ones are tried next
	}
	if len(m.notes) == 0 {
		return m, nil, true
	}
	if move {
		cfg, e := tasks.ConfigForTask(m.boardDir, m.boardConfig, m.notes[m.taskCursor])
		if e != nil {
			m.taskNotice = e.Error()
			return m, nil, true
		}
		columns := cfg.Columns
		target := slices.Index(columns, m.notes[m.taskCursor].Status) + step
		if target >= 0 && target < len(columns) {
			return m, m.moveTask(columns[target]), true
		}
		return m, nil, true
	}
	return m.laneStep(step), nil, true
}

// closeDetail closes the ticket drawer. A drawer opened from the dashboard's
// Tasks card goes back to that card (the ticket stays selected); one opened on
// the board stays on the board.
func (m Model) closeDetail() Model {
	m.boardDrawer = false
	if m.detailFromDash {
		m.boardMode, m.detailFromDash = false, false
	}
	return m
}

// tasksCardKey is the dashboard Tasks card: the ticket keys without leaving
// the dashboard; b opens the board.
func (m Model) tasksCardKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "tab", "shift+tab", "q", "ctrl+c", ",", "o", "B", "[", "]":
		return m, nil, false
	case "b":
		return m.openBoard(), nil, true
	}
	if m.boardDir == "" || m.boardDir != m.repoDir && m.repoDir != "" {
		m = m.boardContext()
	}
	m.boardDrawer, m.boardFlow = false, false
	m, cmd, _ := m.ticketsKey(key)
	return m, cmd, true
}

// workflowRow is the picker row for a workflow name: its index in boardWorkflows,
// else the row after them (the default or none row).
func (m Model) workflowRow(name string) int {
	if i := slices.IndexFunc(m.boardWorkflows, func(w workflow.Workflow) bool { return w.Name == name }); i >= 0 && name != "" {
		return i
	}
	return len(m.boardWorkflows)
}

// ticketHints is the ticket keys; nav says where Tab and esc go.
func ticketHints(nav hint) []hint {
	return []hint{
		{"h/l", "lane", "h l", "select lane: todo / in progress / blocked / done (or click its tab)"},
		{"j/k", "card", "j k ↑ ↓", "select ticket"},
		{"n", "new", "n", "new ticket: pick its type, then write it in $EDITOR"},
		{"s", "start", "s", "start: workflow picker"},
		{"enter", "detail", "enter i", "ticket detail"},
		{"B", "config", "B", "board config summary (default workflow, roles, triggers; E edits)"},
		{"p", "priority", "p / D", "cycle priority / edit dependencies"},
		{"D", "deps", "", ""},
		{"e", "edit", "e", "edit in $EDITOR"},
		{"C", "comment", "c C", "comment on ticket (blocked: prefills @role reply)"},
		{"H/L", "status", "H L", "previous / next status (each card shows its status)"},
		{"m", "move", "m", "move to a status (picker, enter saves)"},
		{"@", "assign", "@", "assign to a role or agent (picker, enter saves)"},
		{"a", "files", "a / A", "attachments: view or open (images, pdf) / attach file"},
		{"x", "delete", "x", "delete (confirm)"},
		{"w", "workflow", "w", "workflow view"},
		{"", "", "t", "swap Workflows / Agents tab (and back)"},
		{"", "", "E", "edit .tasks/board.yaml"},
		{"", "", "?", "help"},
		nav,
	}
}

func (m Model) tasksCardHints() []hint {
	return ticketHints(hint{"", "", "Tab / b", "focus next panel / open board"})
}

func (m Model) boardTicketsHints() []hint {
	return ticketHints(hint{"", "", "Tab / esc", "focus workflows / back"})
}

// moveTask moves the selected ticket to status and ends its metrics run when it
// leaves "doing"; H/L and the status picker both use it.
func (m Model) moveTask(status string) tea.Cmd {
	scope, id := m.notesScope, m.notes[m.taskCursor].ID
	return func() tea.Msg {
		_, err := tasks.Update(scope, id, status, nil, tasks.MutationOptions{Actor: "user"})
		if err == nil && status != "doing" {
			err = metrics.End(scope, id)
		}
		return actionResult("Task moved to "+status, err)
	}
}

// statusColumns is the statuses the selected ticket can be in.
func (m Model) statusColumns() []string {
	cfg, err := tasks.ConfigForTask(m.boardDir, m.boardConfig, m.notes[m.taskCursor])
	if err != nil {
		return nil
	}
	return cfg.Columns
}
