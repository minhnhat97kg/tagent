package transcript

import "encoding/json"

// parsePi reads one pi session entry. The session header is not a record;
// other entry kinds keep their links but carry no items.
func parsePi(line []byte, _ string) (record, bool) {
	var l struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		ParentID  string `json:"parentId"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Role       string          `json:"role"`
			Model      string          `json:"model"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"toolCallId"`
			ToolName   string          `json:"toolName"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &l) != nil || l.Type == "session" || l.ID == "" {
		return record{}, false
	}
	var items []Item
	switch {
	case l.Type != "message":
	case l.Message.Role == "user" || l.Message.Role == "assistant":
		items = withModel(toItems(l.Message.Role, blocks(l.Message.Content)), l.Message.Model)
	case l.Message.Role == "toolResult":
		items = []Item{{Role: "tool", Tool: &Tool{Name: l.Message.ToolName, CallID: l.Message.ToolCallID, Output: clip(resultText(l.Message.Content), maxOutput)}}}
	}
	return newRecord(l.ID, l.ParentID, l.Timestamp, items), true
}
