package sidebar

import (
	"cmp"
	"os"
	"strings"
	"tagent/internal/intake"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// runChat runs the native receptionist; tests replace it with a fake endpoint.
var runChat = func(model string, s intake.Submission) { intake.RunReceptionist(s, intake.NativeHarness(model)) }

const chatGuidance = "Try /agents · /board · /new · /run <workflow>. @ mentions route prompts with approval. Enter sends; Alt+Enter/Ctrl+J newline. Ctrl+N or /clear starts a new chat. PgUp/PgDown scroll. Changes always need approval."

type intakeState struct {
	Shared                                           intake.Conversation
	Input                                            string
	Token                                            int64
	Idea, Question, Raw, Scope, Dir, Command, Answer string
	Turns                                            []intake.Turn
	Workflows                                        []string
	Draft                                            intake.Draft
	Busy                                             bool
	Output, Escalate                                 string
	EditorRevision                                   uint64
	Scroll                                           int
	Completion                                       int
	DismissCompletion                                bool
}

var intakeParseEdited = intake.Parse
var executeIntakeTool intake.ToolExecutor

type intakeEditReply struct {
	intakeReply
	Revision uint64
}
type intakeReply struct {
	Token int64
	Raw   string
	Err   error
}

func (m Model) intakeValidCurrent() bool {
	s := m.intakeState
	return s != nil && (!m.inlineIntake || m.repoDir == "" || m.repoDir == s.Dir) && s.Scope == m.notesScope && s.Dir == m.boardDir && (m.inlineIntake || m.taskForm == "intake")
}
func (m Model) intakeValid(token int64) bool {
	return m.intakeValidCurrent() && m.intakeState.Token == token
}

func (m Model) openIntake() (Model, tea.Cmd) {
	command, ok := tmux.GetOption("@agent_chat_model")
	if !ok || command == "" {
		command = "bonsai-2:9b"
	}
	m.taskInput = ""
	if command == "off" {
		if !m.inlineIntake {
			m.taskForm = "a"
		}
		m.taskNotice = "Intake disabled"
		return m, nil
	}
	s1 := &intakeState{Token: time.Now().UnixNano(), Scope: m.notesScope, Dir: m.boardDir, Command: command}
	for _, w := range workflow.List(tasks.MainRoot(m.boardDir)) {
		s1.Workflows = append(s1.Workflows, w.Name)
	}
	m.intakeState = s1
	m.reloadIntake()
	if !m.inlineIntake {
		m.taskForm = "intake"
	}
	return m, nil
}
func (m Model) intakeReply(msg intakeReply) (tea.Model, tea.Cmd) {
	if m.intakeState != nil && m.intakeState.Shared.Generation != 0 {
		m.reloadIntake()
	}
	return m, nil
}
func (m Model) handleIntake(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.intakeState
	if s != nil && !s.Busy && s.Shared.Status != "proposal" && !s.DismissCompletion {
		choices := intake.Completions(m.taskInput, intake.Recipients(s.Dir, m.sessions))
		if len(choices) > 0 {
			copy := *s
			s = &copy
			m.intakeState = s
			switch key.String() {
			case "esc":
				s.DismissCompletion = true
				return m, nil
			case "up":
				s.Completion = (s.Completion + len(choices) - 1) % len(choices)
				return m, nil
			case "down":
				s.Completion = (s.Completion + 1) % len(choices)
				return m, nil
			case "tab", "enter":
				m.taskInput = intake.CompleteMention(m.taskInput, choices[s.Completion%len(choices)].Token)
				s.Completion = 0
				return m, nil
			}
		}
	}
	if key.String() == "esc" {
		m.taskForm = ""
		m.intakeState = nil
		if m.inlineIntake {
			m.agentFocus, m.taskFocus = 0, false
		}
		m.inlineIntake = false
		return m, nil
	}
	if s != nil && (key.String() == "ctrl+n" || (key.String() == "ctrl+y" && s.Shared.Status == "uncertain") || (key.Type == tea.KeyEnter && strings.TrimSpace(m.taskInput) == "/clear")) {
		_, err := intake.ClearConversation(s.Scope, s.Shared.Revision, key.String() == "ctrl+y")
		if err != nil {
			m.taskNotice = err.Error()
			if s.Shared.Status == "uncertain" {
				m.taskNotice += "; after inspection press Ctrl+Y to acknowledge and clear (cannot undo mutations)"
			}
		} else {
			m.taskInput = ""
		}
		m.reloadIntake()
		return m, nil
	}
	if s != nil && (key.String() == "pgup" || key.String() == "pgdown") {
		copy := *s
		s = &copy
		m.intakeState = s
		if key.String() == "pgup" {
			s.Scroll += 5
		} else {
			s.Scroll = max(0, s.Scroll-5)
		}
		return m, nil
	}
	if s == nil || s.Busy {
		return m, nil
	}
	copy := *s
	s = &copy
	m.intakeState = s
	if key.Type == tea.KeyRunes || key.Type == tea.KeyBackspace {
		s.DismissCompletion = false
		s.Completion = 0
	}
	if key.String() == "alt+enter" || key.String() == "ctrl+j" || key.String() == "shift+enter" {
		m.taskInput += "\n"
		return m, nil
	}
	if len(s.Shared.Response.Calls) > 0 {
		if key.String() == "c" {
			_, err := intake.CancelProposal(s.Scope, s.Shared.Revision)
			if err != nil {
				m.taskNotice = err.Error()
			}
			m.reloadIntake()
			return m, nil
		}
		if key.Type != tea.KeyEnter {
			return m, nil
		}
		claimed, err := intake.ClaimApproval(s.Scope, s.Shared.Revision)
		if err != nil {
			m.taskNotice = err.Error()
			m.reloadIntake()
			return m, nil
		}
		s.Shared = claimed
		s.Busy = true
		token, scope, dir := s.Token, s.Scope, s.Dir
		return m, func() tea.Msg {
			_, err := intake.ExecuteClaimedTools(scope, dir, claimed, executeIntakeTool)
			return intakeReply{Token: token, Err: err}
		}
	}
	if s.Shared.Status == "uncertain" {
		return m, nil
	}
	if (s.Draft.Title != "" || s.Raw != "" && s.Question == "") && key.String() == "e" {
		text := s.Raw
		if s.Draft.Title != "" {
			text = intake.Block(s.Draft)
		}
		cmd, path, e := intake.Editor(text)
		if e != nil {
			m.taskNotice = e.Error()
			return m, nil
		}
		s.Busy = true
		token, revision := s.Token, s.Shared.Revision
		_ = cmd // the draft file is opened in a popup, not in this terminal-less process
		return m, editFile(m.tmuxPane, path, func() tea.Msg {
			defer os.Remove(path)
			b, e := os.ReadFile(path)
			return intakeEditReply{intakeReply{token, string(b), e}, revision}
		})
	}
	if s.Draft.Title != "" {
		if key.String() == "w" {
			choices := append([]string{""}, s.Workflows...)
			for i, w := range choices {
				if w == s.Draft.Workflow {
					s.Draft.Workflow = choices[(i+1)%len(choices)]
					if s.Shared.Revision != 0 {
						result := s.Shared.Response
						result.Draft = s.Draft
						updated, e := intake.EditProposal(s.Scope, s.Shared.Revision, result)
						if e != nil {
							m.taskNotice = e.Error()
							m.reloadIntake()
						} else {
							s.Shared = updated
						}
					}
					break
				}
			}
			return m, nil
		}
		if key.String() != "enter" && key.String() != "r" {
			return m, nil
		}
		d := s.Draft
		scope, dir := s.Scope, s.Dir
		start := key.String() == "r"
		if s.Shared.Revision != 0 {
			claimed, e := intake.ClaimApproval(s.Scope, s.Shared.Revision)
			if e != nil {
				m.taskNotice = e.Error()
				m.reloadIntake()
				return m, nil
			}
			s.Shared = claimed
		}
		m.taskForm = ""
		m.intakeState = nil
		return m, func() tea.Msg {
			finish := func(text string, err error) tea.Msg {
				if s.Shared.Generation != 0 {
					output := text
					if err != nil {
						output += "\n" + err.Error()
					}
					if _, e := intake.FinishApproval(scope, s.Shared.Generation, output); e != nil {
						err = e
					}
				}
				return actionResult(text, err)
			}
			text, e := intake.CreateDraft(dir, scope, d, start)
			return finish(text, e)
		}
	}
	switch key.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		r := []rune(m.taskInput)
		if len(r) > 0 {
			m.taskInput = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		m.taskInput += string(key.Runes)
	case tea.KeySpace:
		m.taskInput += " "
	case tea.KeyEnter:
		input := strings.TrimSpace(m.taskInput)
		if input == "" {
			return m, nil
		}
		if s.Answer != "" || s.Output != "" || s.Escalate != "" {
			s.Answer, s.Idea, s.Raw, s.Question, s.Output, s.Escalate = "", "", "", "", "", ""
			s.Turns, s.Draft, s.Token = nil, intake.Draft{}, time.Now().UnixNano()
		}
		if s.Idea == "" {
			s.Idea = input
		} else if s.Question != "" {
			s.Turns = append(append([]intake.Turn{}, s.Turns...), intake.Turn{Question: s.Question, Answer: input})
		} else {
			return m, nil
		}
		m.taskInput = ""
		s.Busy = true
		s.Raw = ""
		s.Question = ""
		s.Workflows = nil
		for _, w := range workflow.List(tasks.MainRoot(s.Dir)) {
			s.Workflows = append(s.Workflows, w.Name)
		}
		submission, e := intake.PrepareSubmission(s.Dir, s.Scope, input, m.sessions)
		if e != nil {
			s.Busy = false
			m.taskNotice = e.Error()
			m.reloadIntake()
			return m, nil
		}
		if submission.Direct {
			s.Busy = false
			m.reloadIntake()
			return m, nil
		}
		s.Shared = submission.Conversation
		token, command := s.Token, s.Command
		return m, func() tea.Msg {
			runChat(command, submission)
			return intakeReply{Token: token}
		}

	}
	return m, nil
}

// intakeView is a chat: bonsai on the left, you on the right, newest at the bottom
// next to the input line and the keys for the current step.
func (m Model) intakeView() string {
	return m.boardPanel("Intake", m.intakeLines(m.width-4, m.height-2), m.width, m.height, true)
}

func (m Model) intakeLines(inner, height int) []string {
	inner, height = max(1, inner), max(0, height)
	s := m.intakeState
	if m.inlineIntake && !m.intakeValidCurrent() {
		s = nil
	}
	if s == nil {
		lines := []string{chatGuidance, m.taskNotice}
		lines = lines[:min(height, len(lines))]
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], inner, "")
		}
		return lines
	}
	var lines []string
	say := func(text string, mine bool) {
		text = ansi.Strip(text)
		w := max(1, inner*3/4)
		label, style := "Receptionist", m.boardStyle(m.theme.TextPrimary)
		if mine {
			label, style = "you", m.boardStyle(m.theme.Accent)
		}
		body := []string{m.boardStyle(m.theme.TextDim).Render(label)}
		for _, para := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			for _, l := range strings.Split(ansi.Wrap(para, w, ""), "\n") {
				body = append(body, style.Render(l))
			}
		}
		for _, l := range body {
			if mine {
				l = strings.Repeat(" ", max(0, inner-ansi.StringWidth(l))) + l
			}
			lines = append(lines, l)
		}
		lines = append(lines, "")
	}
	renderer, _ := glamour.NewTermRenderer(glamour.WithStandardStyle("notty"), glamour.WithWordWrap(max(1, inner*3/4-2)))
	if s.Shared.Revision != 0 {
		if len(s.Shared.Messages) == 0 {
			say(chatGuidance, false)
		}
		for _, entry := range s.Shared.Messages {
			if entry.Tool != nil {
				say("┌ Tool · "+entry.Tool.Status, false)
				for _, c := range entry.Tool.Calls {
					say(c.Name+"\n"+c.Summary(), false)
				}
				if entry.Tool.Output != "" {
					say(entry.Tool.Output, false)
				}
				say("└", false)
			} else {
				text := ansi.Strip(entry.Text)
				if entry.Role == "assistant" && renderer != nil {
					if rendered, err := renderer.Render(text); err == nil {
						text = rendered
					}
				}
				say(text, entry.Role == "user")
			}
		}
	} else if s.Idea == "" {
		say(chatGuidance, false)
	} else {
		say(s.Idea, true)
	}
	if s.Shared.Revision == 0 {
		for _, t := range s.Turns {
			say(t.Question, false)
			say(t.Answer, true)
		}
	}
	keys, input := "", false
	switch {
	case s.Busy:
		say("…", false)
	case len(s.Shared.Response.Calls) > 0:
		keys = "Enter approve · c cancel · Ctrl+N new chat"
	case s.Answer != "":
		if s.Shared.Revision == 0 {
			say(s.Answer, false)
		}
		keys, input = "Enter new request · Esc close", true
	case s.Escalate != "":
		say("This needs the lead (Claude): "+s.Escalate+"\nYour request is copied to the clipboard.", false)
		keys, input = "Enter new request · Esc close", true
	case s.Output != "":
		if s.Shared.Revision == 0 {
			say(s.Output, false)
		}
		keys, input = "Enter new request · Esc close", true
	case s.Draft.Title != "":
		say(s.Draft.Text()+"\nWorkflow: "+cmp.Or(s.Draft.Workflow, "none"), false)
		keys = "Enter create · r start · w workflow · e edit · Esc"
	case s.Raw != "" && s.Question == "":
		say(s.Raw, false)
		keys = "e edit · Esc cancel"
	default:
		if s.Question != "" && s.Shared.Revision == 0 {
			say(s.Question, false)
		}
		keys, input = "Enter send · Esc cancel", true
	}
	var foot []string
	if input {
		foot = append(foot, strings.Split(wrappedInput(func() string {
			if m.inlineIntake {
				return s.Input
			}
			return m.taskInput
		}(), inner, height), "\n")...)
	}
	if !s.DismissCompletion && input {
		value := m.taskInput
		if m.inlineIntake {
			value = s.Input
		}
		for i, c := range intake.Completions(value, intake.Recipients(s.Dir, m.sessions)) {
			prefix := "  "
			if i == s.Completion {
				prefix = "› "
			}
			foot = append(foot, prefix+c.Label)
		}
	}
	if m.taskNotice != "" {
		foot = append(foot, m.taskNotice)
	}
	var pairs []string
	for _, pair := range strings.Split(keys, " · ") {
		pairs = append(pairs, pair)
	}
	foot = append(foot, m.keyHints(pairs...))
	if len(foot) > height {
		foot = foot[len(foot)-height:]
	}
	room := max(0, height-len(foot))
	end := len(lines) - min(s.Scroll, max(0, len(lines)-room))
	lines = lines[max(0, end-room):end]
	for len(lines) < room {
		lines = append([]string{""}, lines...)
	}
	lines = append(lines, foot...)
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], inner, "")
	}
	return lines
}
