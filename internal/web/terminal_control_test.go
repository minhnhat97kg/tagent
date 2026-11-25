package web

import (
	"strings"
	"testing"
)

func TestControlReaderBlocks(t *testing.T) {
	r := &controlReader{pane: "%5"}
	captured, failed := newControlCmd(), newControlCmd()
	r.expect(captured)
	r.expect(failed)
	stream := strings.Join([]string{
		"%begin 1 1 0", "%end 1 1 0", // attach
		"%begin 2 7 0", "%error 2 7 0", // hook failure: not ours, ignored
		"%session-changed $1 x",
		"%begin 3 42 1", "%output %5 captured", "%end fake", "%end 3 42 1", // marker-like text is content
		"%output %5 \\033[31mred\\033[0m", "%output %6 other pane",
		"%begin 4 43 1", "%error 4 43 1",
	}, "\n") + "\n"
	events := make(chan controlEvent, 64)
	r.run(strings.NewReader(stream), events, make(chan struct{}))
	close(events)
	var kinds []controlKind
	var got []controlEvent
	for e := range events {
		kinds = append(kinds, e.kind)
		got = append(got, e)
	}
	want := []controlKind{controlAttached, controlCmdDone, controlOutput, controlCmdDone, controlEnd}
	if len(kinds) != len(want) {
		t.Fatalf("events %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("events %v, want %v", kinds, want)
		}
	}
	if !captured.ok || strings.Join(captured.out, "|") != "%output %5 captured|%end fake" {
		t.Fatalf("capture %v %q", captured.ok, captured.out)
	}
	if failed.ok {
		t.Fatal("error block reported as success")
	}
	if string(got[2].output) != "\x1b[31mred\x1b[0m" {
		t.Fatalf("output %q", got[2].output)
	}
}

func TestControlReaderRejectsUnexpectedReply(t *testing.T) {
	r := &controlReader{pane: "%5"}
	events := make(chan controlEvent, 4)
	r.run(strings.NewReader("%begin 1 9 1\n%end 1 9 1\n"), events, make(chan struct{}))
	close(events)
	if e := <-events; e.kind != controlFail {
		t.Fatalf("reply without a command: %v", e.kind)
	}
}
