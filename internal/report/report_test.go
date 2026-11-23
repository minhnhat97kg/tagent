package report

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const valid = `{"contract":"TA-1","role":"builder","status":"DONE","files":["a.go","b.go"],"checks":{"vet":"ok","tests":12},"findings":["one"],"question":null}`

func TestParse(t *testing.T) {
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]byte(`{"role":"builder","status":"DONE"}`)); err == nil {
		t.Error("missing contract accepted")
	}
	if _, err := Parse([]byte(`{"contract":"x","role":"builder","status":"OK"}`)); err == nil {
		t.Error("bad status accepted")
	}
}

func TestSummary(t *testing.T) {
	r, _ := Parse([]byte(valid))
	want := "TA-1 builder DONE files=2 checks=tests=12,vet=ok findings=1\n- one"
	if got := Summary(r); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	q := "why?"
	r.Question = &q
	if !strings.Contains(Summary(r), "findings=1 question=why?") {
		t.Error("question missing")
	}
}

func TestInstruction(t *testing.T) {
	if s := Instruction("/x/r.json"); len(s) > 650 || !strings.Contains(s, "/x/r.json.tmp") {
		t.Errorf("bad instruction (%d): %s", len(s), s)
	}
}

func TestWaitAtomicWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "r.json")
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.WriteFile(p+".tmp", []byte(valid), 0o644)
		os.Rename(p+".tmp", p)
	}()
	r, err := Wait(p, 5*time.Second)
	if err != nil || r.Contract != "TA-1" {
		t.Fatalf("%v %+v", r, err)
	}
}

func TestWaitInPlaceWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(p, []byte(`{"contr`), 0o644)
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.WriteFile(p, []byte(valid), 0o644)
	}()
	if _, err := Wait(p, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitTimeout(t *testing.T) {
	start := time.Now()
	_, err := Wait(filepath.Join(t.TempDir(), "r.json"), 200*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 2*time.Second {
		t.Fatalf("%v after %v", err, time.Since(start))
	}
}
