package transcript

import "encoding/json"

// parseClaude reads one Claude Code line. Non-message entries keep their links
// (attachments and the like sit between messages) but carry no items.
func parseClaude(line []byte, sessionID string) (record, bool) {
	var l struct {
		Type        string `json:"type"`
		SessionID   string `json:"sessionId"`
		UUID        string `json:"uuid"`
		ParentUUID  string `json:"parentUuid"`
		Timestamp   string `json:"timestamp"`
		IsMeta      bool   `json:"isMeta"`
		IsSidechain bool   `json:"isSidechain"`
		Message     struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	// Records of another session in this file are not part of this conversation.
	if json.Unmarshal(line, &l) != nil || l.UUID == "" || (l.SessionID != "" && l.SessionID != sessionID) {
		return record{}, false
	}
	var items []Item
	if (l.Type == "user" || l.Type == "assistant") && !l.IsMeta && !l.IsSidechain {
		items = toItems(l.Type, blocks(l.Message.Content))
	}
	return newRecord(l.UUID, l.ParentUUID, l.Timestamp, items), true
}
