package tasks

import (
	"io"
	"strings"
	"testing"
)

func TestBriefAndLongBody(t *testing.T) {
	_, scope := testBoard(t)
	task, _ := Add(scope, "title", nil)
	body := strings.Repeat("detail ", 100) + " @reviewer"
	if e := Comment(scope, task.ID, "worker", body, nil); e != nil {
		t.Fatal(e)
	}
	got, _ := Get(scope, task.ID)
	c := got.Comments[0]
	if len(c.Attachments) != 1 || !strings.Contains(c.Body, "full text attached") {
		t.Fatal(c)
	}
	f, e := OpenAttachment(scope, task.ID, c.Attachments[0].File)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	if string(b) != body {
		t.Fatal("lost body")
	}
	for _, s := range []string{"old", "new1", "new2", "new3"} {
		Comment(scope, task.ID, "worker", s, nil)
	}
	Comment(scope, task.ID, "system", "audit", nil)
	got, _ = Get(scope, task.ID)
	s := Brief(got)
	if strings.Contains(s, ": old") || strings.Contains(s, ": audit") || !strings.Contains(s, "new1") {
		t.Fatal(s)
	}
	ev, _ := ReadEvents(scope, 0)
	found := false
	for _, e := range ev {
		if e.Kind == "mention" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost mention")
	}
}
