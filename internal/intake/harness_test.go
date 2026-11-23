package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHarnessNativeTools(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		ok          bool
	}{
		{"answer", `{"message":{"tool_calls":[{"function":{"name":"answer","arguments":{"text":"hello"}}}]}}`, true},
		{"question", `{"message":{"tool_calls":[{"function":{"name":"ask_question","arguments":{"text":"scope?"}}}]}}`, true},
		{"ticket", `{"message":{"tool_calls":[{"function":{"name":"propose_ticket","arguments":{"title":"Fix","goal":"Test"}}}]}}`, true},
		{"command", `{"message":{"tool_calls":[{"function":{"name":"propose_command","arguments":{"text":"tagent reload"}}}]}}`, true},
		{"shell", `{"message":{"tool_calls":[{"function":{"name":"propose_command","arguments":{"text":"tagent reload; touch /tmp/no"}}}]}}`, false},
		{"unknown", `{"message":{"tool_calls":[{"function":{"name":"shell","arguments":{"text":"ls"}}}]}}`, false},
		{"unknown argument", `{"message":{"tool_calls":[{"function":{"name":"answer","arguments":{"text":"hi","execute":true}}}]}}`, false},
		{"invalid draft", `{"message":{"tool_calls":[{"function":{"name":"propose_ticket","arguments":{"title":"Fix"}}}]}}`, false},
		{"prose", `{"message":{"content":"ANSWER: hi"}}`, false},
		{"multiple", `{"message":{"tool_calls":[{},{}]}}`, false},
		{"unsupported", `{"error":"model does not support tools"}`, false},
		{"oversize", strings.Repeat("x", 65537), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/chat" || r.Method != "POST" {
					t.Error("unexpected request", r.URL)
				}
				var request struct {
					Model    string
					Stream   bool
					Messages []struct{ Role, Content string }
					Tools    []struct{ Function struct{ Name string } }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Model != "test" || request.Stream || len(request.Messages) != 2 || len(request.Tools) != 5 {
					t.Fatalf("unexpected native request: %+v", request)
				}
				system := request.Messages[0]
				if system.Role != "system" || !strings.Contains(system.Content, "exactly one tool call, never prose") || !strings.Contains(system.Content, AllowedCommandReference) {
					t.Errorf("missing tool instructions or command allowlist: %s", system.Content)
				}
				for _, legacy := range []string{"Reply ONLY", "ANSWER:", "CMD:", "Q:", "->"} {
					if strings.Contains(system.Content, legacy) {
						t.Errorf("legacy response protocol %q in native prompt", legacy)
					}
				}
				if request.Messages[1].Role != "user" || request.Messages[1].Content != "snapshot" {
					t.Errorf("snapshot/history not preserved: %+v", request.Messages[1])
				}
				for i, name := range []string{"answer", "ask_question", "propose_ticket", "propose_command", "escalate"} {
					if request.Tools[i].Function.Name != name {
						t.Errorf("unexpected tool: %+v", request.Tools[i])
					}
				}
				fmt.Fprint(w, tc.reply)
			}))
			defer ts.Close()
			_, err := (Harness{Endpoint: ts.URL, Model: "test"}).Call(context.Background(), "snapshot")
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
		})
	}
}
func TestHarnessTransportFailureReleasesSubmission(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	c, err := ReserveSubmission("test", "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) }))
	defer ts.Close()
	RunReceptionist(Submission{Conversation: c, Scope: "test"}, Harness{Endpoint: ts.URL})
	c, err = LoadConversation("test")
	if err != nil || c.Status != "failed" {
		t.Fatal(c, err)
	}
	if _, err = ReserveSubmission("test", "retry", ""); err != nil {
		t.Fatal(err)
	}
}
