package sidebar

import "tmux-agent/internal/intake"

// reloadIntake preserves local editor/input state while observing remote turns.
func (m *Model) reloadIntake() {
	if m.intakeState == nil {
		return
	}
	old := m.intakeState
	shared, err := intake.LoadConversation(old.Scope)
	if err != nil {
		m.taskNotice = err.Error()
		return
	}
	if shared.Revision == old.Shared.Revision {
		return
	}
	s := *old
	s.Shared = shared
	s.Idea, s.Turns, s.Raw = shared.Idea, shared.Turns, shared.Raw
	p := shared.Response
	s.Question, s.Answer, s.ApprovedCommand, s.Escalate, s.Draft = p.Question, p.Answer, p.Command, p.Escalate, p.Draft
	s.Busy = shared.Status == "running" || shared.Status == "approving"
	s.Output = ""
	if shared.Status == "idle" && p.Question == "" && p.Answer == "" && p.Escalate == "" && len(shared.Messages) > 0 {
		s.Answer = shared.Messages[len(shared.Messages)-1].Text
	}
	if shared.Status == "failed" || shared.Status == "uncertain" {
		s.Answer = "Request failed; type a new request to retry."
		if shared.Status == "uncertain" {
			s.Output = "Approval interrupted: inspect the project; outcome uncertain."
		}
	}
	m.intakeState = &s
}
