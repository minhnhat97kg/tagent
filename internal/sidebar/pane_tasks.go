package sidebar

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
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
			m.workflowFocus = true
			return m, nil, true
		}
	}
	switch key {
	case "t":
		m.boardMode = true
		m.agentsOpen = !m.agentsOpen
		m.workflowFocus, m.agentDetail, m.agentCursor = m.agentsOpen, false, 0
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
		m.boardMode = true
		m.boardDrawer = !m.boardDrawer
		m.detailScroll = 0
		m.detailTab = detailTabDetails
		m.detailDescOpen, m.detailSysOpen = false, false
		return m, nil, true
	case "esc":
		if m.boardDrawer {
			m.boardDrawer = false
			return m, nil, true
		}
		m.boardMode = false
		return m, nil, true
	case "q", "b":
		m.boardMode, m.boardDrawer = false, false
		return m, nil, true
	case "c", "C": // comment on the selected ticket (a blocked one prefills the @role)
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, true), true
		}
		return m, nil, true
	case "E": // edit .tasks/board.yaml
		return m, editBoardConfig(m.boardDir, m.tmuxPane), true
	case "n":
		return m, editTaskText(m.notesScope, tasks.Task{}, m.tmuxPane, false), true
	case "s":
		if len(m.notes) > 0 {
			m.taskForm, m.taskStep = "start", 0
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
	case "e":
		if len(m.notes) > 0 {
			return m, editTaskText(m.notesScope, m.notes[m.taskCursor], m.tmuxPane, false), true
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
			scope, id, status := m.notesScope, m.notes[m.taskCursor].ID, columns[target]
			return m, func() tea.Msg {
				_, err := tasks.Update(scope, id, status, nil, tasks.MutationOptions{Actor: "user"})
				if err == nil && status != "doing" {
					err = metrics.End(scope, id)
				}
				return actionResult("Task moved to "+status, err)
			}, true
		}
		return m, nil, true
	}
	return m.laneStep(step), nil, true
}
