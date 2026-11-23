package sidebar

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"tagent/internal/intake"
	"testing"
)

// fakeChat makes the next receptionist turn answer with one native tool call.
func fakeChat(t *testing.T, tool, args string) {
	fakeChatHandler(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"message":{"tool_calls":[{"function":{"name":%q,"arguments":%s}}]}}`, tool, args)
	})
}

// fakeChatHandler replaces the receptionist endpoint with h for this test.
func fakeChatHandler(t *testing.T, h http.HandlerFunc) {
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	runChat = func(model string, s intake.Submission) {
		intake.RunReceptionist(s, intake.Harness{Endpoint: ts.URL, Model: model})
	}
	t.Cleanup(func() {
		runChat = func(model string, s intake.Submission) { intake.RunReceptionist(s, intake.NativeHarness(model)) }
	})
}
