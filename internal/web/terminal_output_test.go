package web

import (
	"bytes"
	"testing"
	"time"
)

func TestOutputBufferBoundsDelayAndKeepsBytes(t *testing.T) {
	var b outputBuffer
	t0 := time.Unix(100, 0)
	if b.add([]byte("ab"), t0, 1) {
		t.Fatal("a chunk with more queued behind it flushed at once")
	}
	if outputDelay > 16*time.Millisecond {
		t.Fatalf("delay %v exceeds the 16ms budget", outputDelay)
	}
	if b.add([]byte("cd"), t0.Add(5*time.Millisecond), 0) {
		t.Fatal("bytes flushed early inside a burst")
	}
	if got := string(b.take()); got != "abcd" {
		t.Fatalf("burst %q", got)
	}
	if !b.add([]byte("x"), t0.Add(time.Second), 0) {
		t.Fatal("an isolated chunk after a quiet period waited")
	}
	b.take()

	// A flood is cut into frames at the limit; no byte is lost or reordered.
	var sent, got []byte
	now := t0.Add(2 * time.Second)
	for i := 0; i < 100; i++ {
		chunk := bytes.Repeat([]byte{byte(i)}, 1000)
		sent = append(sent, chunk...)
		if b.add(chunk, now, 1) {
			got = append(got, b.take()...)
		}
		now = now.Add(time.Millisecond)
	}
	got = append(got, b.take()...)
	if !bytes.Equal(got, sent) {
		t.Fatalf("flood changed bytes: %d sent, %d kept", len(sent), len(got))
	}
}
