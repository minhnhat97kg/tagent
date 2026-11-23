package sidebar

import (
	"cmp"
	"os"
	"strings"
	"tagent/internal/clipboard"
	"tagent/internal/intake"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const chatGuidance = "Ask: which agents are running? · Draft: add tests · !reload proposes a command. Enter sends; drafts/commands need approval. Answers are read-only; type a new request or Esc. Snapshot: latest local data, not history."

type intakeState struct {
	Shared                                           intake.Conversation
	Input                                            string
	Token                                            int64
	Idea, Question, Raw, Scope, Dir, Command, Answer string
	Turns                                            []intake.Turn
	Workflows                                        []string
	Draft                                            intake.Draft
	Busy                                             bool
	ApprovedCommand, Output, Escalate                string
	EditingCommand                                   bool
	EditorRevision                                   uint64
}

var runSidebarReceptionist = intake.RunReceptionist

var intakeParseEdited = intake.Parse
var executeIntakeCommand = intake.Execute

type intakeCommandOutput struct {
	Token  int64
	Output string
	Err    error
}
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
	s := m.intakeState
	if !m.intakeValid(msg.Token) {
		return m, nil
	}
	if s.Shared.Generation != 0 {
		if msg.Err != nil {
			intake.CompleteSubmission(s.Scope, s.Shared.Generation, "", msg.Err, s.Workflows)
			copy := *s
			copy.Input = s.Idea
			copy.Idea = ""
			copy.Turns = nil
			copy.Busy = false
			copy.Token = time.Now().UnixNano()
			m.intakeState = &copy
			m.taskNotice = msg.Err.Error()
			return m, nil
		}
		m.reloadIntake()
		return m, nil
	}
	copy := *s
	s = &copy
	m.intakeState = s
	s.Busy = false
	if msg.Err != nil {
		if !m.inlineIntake {
			m.taskForm = "a"
		}
		if m.inlineIntake {
			// Restore the request as a fresh conversation, not an answer to
			// a question that was cleared when the failed request started.
			m.intakeState = &intakeState{
				Token: time.Now().UnixNano(), Input: s.Idea,
				Scope: s.Scope, Dir: s.Dir, Command: s.Command,
				Workflows: s.Workflows,
			}
		} else {
			m.taskInput = s.Idea
		}
		m.taskNotice = msg.Err.Error()
		return m, nil
	}
	s.Raw = msg.Raw
	response, e := intake.ParseResponse(msg.Raw)
	if e == nil && response.Answer != "" {
		s.Answer = response.Answer
		s.Raw = ""
		return m, nil
	}
	if e == nil && response.Command != "" {
		s.ApprovedCommand = response.Command
		s.Question = ""
		return m, nil
	}
	if e == nil && response.Escalate != "" {
		s.Escalate = response.Escalate
		s.Question = ""
		return m, tea.Printf("%s", clipboard.OSC52Sequence(s.Idea))
	}
	q, d := response.Question, response.Draft
	if e != nil || (q != "" && len(s.Turns) >= 3) {
		m.taskNotice = "Invalid reply: e edit or Esc cancel"
		return m, nil
	}
	d.Normalize(s.Workflows)
	s.Question = q
	s.Draft = d
	return m, nil
}
func (m Model) handleIntake(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.intakeState
	if key.String() == "esc" {
		m.taskForm = ""
		m.intakeState = nil
		if m.inlineIntake {
			m.agentFocus, m.taskFocus = 0, false
		}
		m.inlineIntake = false
		return m, nil
	}
	if s == nil || s.Busy {
		return m, nil
	}
	copy := *s
	s = &copy
	m.intakeState = s
	if s.Escalate != "" || s.Output != "" {
		return m, nil
	}
	if s.ApprovedCommand != "" {
		if !s.EditingCommand {
			switch key.String() {
			case "e":
				s.EditingCommand = true
				s.EditorRevision = s.Shared.Revision
				m.taskInput = s.ApprovedCommand
				return m, nil
			case "enter":
				if _, e := intake.CommandArgs(s.ApprovedCommand); e != nil {
					m.taskNotice = e.Error()
					return m, nil
				}
				if s.Shared.Revision != 0 {
					claimed, e := intake.ClaimApproval(s.Scope, s.Shared.Revision)
					if e != nil {
						m.taskNotice = e.Error()
						m.reloadIntake()
						return m, nil
					}
					s.Shared = claimed
				}
				s.Busy = true
				token, command, dir := s.Token, s.ApprovedCommand, s.Dir
				return m, func() tea.Msg {
					out, e := executeIntakeCommand(command, dir)
					if s.Shared.Generation != 0 {
						text := out
						if e != nil {
							text += "\n" + e.Error()
						}
						if text == "" {
							text = "Command completed"
						}
						_, finishErr := intake.FinishApproval(s.Scope, s.Shared.Generation, text)
						if finishErr != nil {
							e = finishErr
						}
					}
					return intakeCommandOutput{token, out, e}
				}
			}
			return m, nil
		}
		if key.Type == tea.KeyEnter {
			if _, e := intake.CommandArgs(m.taskInput); e != nil {
				m.taskNotice = e.Error()
				return m, nil
			}
			if s.Shared.Revision != 0 {
				result := s.Shared.Response
				result.Command = m.taskInput
				updated, e := intake.EditProposal(s.Scope, s.EditorRevision, result)
				if e != nil {
					m.taskNotice = e.Error()
					m.reloadIntake()
					return m, nil
				}
				s.Shared = updated
			}
			s.ApprovedCommand = m.taskInput
			s.EditingCommand = false
			m.taskInput = ""
			return m, nil
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
		}
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
		if s.Answer != "" {
			s.Answer, s.Idea, s.Raw, s.Question, s.ApprovedCommand, s.Output, s.Escalate = "", "", "", "", "", "", ""
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
		s.Shared = submission.Conversation
		token, model := s.Token, s.Command
		endpoint, _ := tmux.GetOption("@agent_chat_endpoint")
		if endpoint == "" {
			endpoint = "http://127.0.0.1:11434"
		}
		return m, func() tea.Msg {
			runSidebarReceptionist(submission, intake.Harness{Model: model, Endpoint: endpoint})
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
		w := max(1, inner*3/4)
		label, style := "bonsai", m.boardStyle(m.theme.TextPrimary)
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
	if s.Shared.Revision != 0 {
		for _, entry := range s.Shared.Messages {
			say(entry.Text, entry.Role == "user")
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
	case s.Answer != "":
		say(s.Answer, false)
		keys, input = "Enter new request · Esc close", true
	case s.Escalate != "":
		say("This needs the lead (Claude): "+s.Escalate+"\nYour request is copied to the clipboard.", false)
		keys = "Esc close"
	case s.Output != "":
		say(s.Output, false)
		keys = "Esc close"
	case s.ApprovedCommand != "":
		say("Run this?\n"+s.ApprovedCommand, false)
		keys, input = "Enter run · e edit · Esc cancel", s.EditingCommand
	case s.Draft.Title != "":
		say(s.Draft.Text()+"\nWorkflow: "+cmp.Or(s.Draft.Workflow, "none"), false)
		keys = "Enter create · r start · w workflow · e edit · Esc"
	case s.Raw != "" && s.Question == "":
		say(s.Raw, false)
		keys = "e edit · Esc cancel"
	default:
		if s.Question != "" {
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
	if len(lines) > room { // keep the newest messages
		lines = lines[len(lines)-room:]
	}
	for len(lines) < room {
		lines = append([]string{""}, lines...)
	}
	lines = append(lines, foot...)
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], inner, "")
	}
	return lines
}
