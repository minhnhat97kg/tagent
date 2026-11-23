package intake

func CancelProposal(scope string, revision uint64) (Conversation, error) {
	return conversationUpdate(scope, func(s *Conversation) error {
		if s.Revision != revision || s.Status == "running" || s.Status == "approving" || s.Status == "uncertain" {
			return ErrStale
		}
		for i := range s.Messages {
			if s.Messages[i].Tool != nil && s.Messages[i].Tool.Status == "proposed" {
				s.Messages[i].Tool.Status = "cancelled"
			}
		}
		s.Response = Result{}
		s.Idea = ""
		s.Turns = nil
		s.Status = "idle"
		s.append("assistant", "Cancelled.")
		return nil
	})
}
