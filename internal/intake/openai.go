package intake

import (
	"encoding/json"
	"fmt"
)

// openAIMessages turns the Ollama-shaped continuation (tool calls with object
// arguments, results named by tool) into the OpenAI shape (ids, string arguments).
func openAIMessages(in []any) []any {
	out := make([]any, 0, len(in))
	id := ""
	for _, m := range in {
		switch v := m.(type) {
		case map[string]any:
			if calls, ok := v["tool_calls"].([]any); ok && len(calls) == 1 {
				fn := calls[0].(map[string]any)["function"].(map[string]any)
				args, _ := json.Marshal(fn["arguments"])
				id = fmt.Sprintf("call_%d", len(out))
				out = append(out, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": id, "type": "function", "function": map[string]any{"name": fn["name"], "arguments": string(args)}}}})
				continue
			}
			out = append(out, m)
		case map[string]string:
			if v["role"] == "tool" {
				out = append(out, map[string]string{"role": "tool", "tool_call_id": id, "content": v["content"]})
				continue
			}
			out = append(out, m)
		default:
			out = append(out, m)
		}
	}
	return out
}
