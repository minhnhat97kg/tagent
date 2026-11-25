package web

import "time"

const (
	// outputDelay bounds how long a pane byte waits for more output before it is sent (the 16ms budget).
	outputDelay = 12 * time.Millisecond
	// outputLimit bounds the buffer: a flood is sent in frames of at most this size.
	outputLimit = 32 << 10
	// compressFrom skips per-message deflate for frames too small to gain anything.
	compressFrom = 128
)

// outputBuffer holds pane output for at most outputDelay, so bursts become a few frames. A chunk
// that arrives alone after a quiet period is flushed at once.
type outputBuffer struct {
	data []byte
	last time.Time // arrival of the previous chunk
}

// add buffers a chunk that arrived at now. queued is how many more events are waiting behind it.
// It reports whether the buffer must be written before the caller waits for anything else.
func (b *outputBuffer) add(p []byte, now time.Time, queued int) bool {
	isolated := len(b.data) == 0 && queued == 0 && now.Sub(b.last) >= outputDelay
	b.last = now
	b.data = append(b.data, p...)
	return isolated || len(b.data) >= outputLimit
}

// take returns everything buffered. The slice is only valid until the next add.
func (b *outputBuffer) take() []byte {
	out := b.data
	b.data = b.data[:0]
	return out
}
