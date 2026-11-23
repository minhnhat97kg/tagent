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
		{"command", `{"message":{"tool_calls":[{"function":{"name":"propose_command","arguments":{"text":"tagent reload"}}}]}}`, false},
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
				if request.Model != "test" || request.Stream || len(request.Messages) != 2 || len(request.Tools) != 4+len(toolSpecs) {
					t.Fatalf("unexpected native request: %+v", request)
				}
				system := request.Messages[0]
				if system.Role != "system" || !strings.Contains(system.Content, "exactly one native tool call") || !strings.Contains(system.Content, "human approval") {
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
				for i, name := range []string{"answer", "ask_question", "propose_ticket", "escalate"} {
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
func TestNativeToolContinuationStopsForApproval(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	executions, requests := 0, 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
			return
		}
		requests++
		if requests == 1 {
			fmt.Fprint(w, `{"message":{"tool_calls":[{"function":{"name":"list_agents","arguments":{}}}]}}`)
			return
		}
		var messages []map[string]json.RawMessage
		_ = json.Unmarshal(body["messages"], &messages)
		if len(messages) != 4 || string(messages[3]["role"]) != `"tool"` || !strings.Contains(string(messages[3]["content"]), "agents result") {
			t.Error("missing native tool continuation", string(body["messages"]))
		}
		fmt.Fprint(w, `{"message":{"tool_calls":[{"function":{"name":"create_ticket","arguments":{"title":"Test","goal":"Coverage"}}}]}}`)
	}))
	defer ts.Close()
	c, e := ReserveSubmission("tools", "make a ticket", "")
	if e != nil {
		t.Fatal(e)
	}
	RunReceptionist(Submission{Conversation: c, Scope: "tools", Dir: t.TempDir()}, Harness{Endpoint: ts.URL, Executor: func(context.Context, string, string, []string) (string, error) {
		executions++
		return "agents result", nil
	}})
	c, e = LoadConversation("tools")
	if e != nil || c.Status != "proposal" || len(c.Response.Calls) != 1 || executions != 1 || requests != 2 {
		t.Fatal(c, e, executions, requests)
	}
	if c.Response.Calls[0].Name != "create_ticket" {
		t.Fatal(c.Response)
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
