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

// Deliver records only successful output, independently for every endpoint.
func DeliveryCursor(scope, endpoint string, revision *uint64) (uint64, error) {
	if revision == nil {
		s, e := LoadConversation(scope)
		return s.Delivered[endpoint], e
	}
	var cursor uint64
	_, err := conversationTransition(scope, func(s *Conversation) error {
		if s.Delivered == nil {
			s.Delivered = map[string]uint64{}
		}
		cursor = s.Delivered[endpoint]
		if revision != nil && *revision > cursor {
			s.Delivered[endpoint] = *revision
			cursor = *revision
		}
		return nil
	}, false)
	return cursor, err
}
