package intake

func CancelProposal(scope string, revision uint64) (Conversation, error) {
	return conversationUpdate(scope, func(s *Conversation) error {
		if s.Revision != revision || s.Status == "running" || s.Status == "approving" || s.Status == "uncertain" {
			return ErrStale
		}
		s.Response = Result{}
		s.Idea = ""
		s.Turns = nil
		s.Status = "idle"
		s.append("assistant", "Cancelled.")
		return nil
	})
}
