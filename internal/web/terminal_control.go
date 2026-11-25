package web

import (
	"bufio"
	"io"
	"strings"
	"sync"
)

// tmux control mode answers each command written to its stdin with a block whose flags are 1, in
// the order written. Attach and hook blocks have flags 0. A block ends at the %end or %error with
// the same number as its %begin, so captured text that looks like a marker stays content.
type controlCmd struct {
	done chan struct{} // closed when the block ends; the parser never waits for event readers
	ok   bool
	out  []string
}

func newControlCmd() *controlCmd { return &controlCmd{done: make(chan struct{})} }

type controlKind int

const (
	controlOutput   controlKind = iota // decoded %output bytes for the pane
	controlCmdDone                     // one of our commands ended
	controlAttached                    // the attach block ended: setup may start
	controlEnd                         // tmux stdout closed: the pane is gone
	controlFail                        // protocol or attach failure: close without telling the client
)

// controlEvent reaches the socket writer in stdout order, so output, snapshot and gone never reorder.
type controlEvent struct {
	kind   controlKind
	output []byte
	cmd    *controlCmd
}

type controlReader struct {
	pane  string
	mu    sync.Mutex
	queue []*controlCmd
}

// expect queues a command before it is written to tmux stdin.
func (r *controlReader) expect(c *controlCmd) {
	r.mu.Lock()
	r.queue = append(r.queue, c)
	r.mu.Unlock()
}

func (r *controlReader) pop() *controlCmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return nil
	}
	c := r.queue[0]
	r.queue = r.queue[1:]
	return c
}

// run parses tmux stdout until it ends. Events go out in stream order; once done closes they are
// dropped, but stdout is still read so commands in flight resolve during teardown. It waits only
// to send an event, never on stdin or on a command's writer.
func (r *controlReader) run(stdout io.Reader, events chan<- controlEvent, done <-chan struct{}) {
	send := func(e controlEvent) {
		select {
		case events <- e:
		case <-done:
		}
	}
	outputPrefix := "%output " + r.pane + " "
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var (
		inBlock    bool
		num        string
		cur        *controlCmd // owner of the open flags 1 block
		attach     bool        // the open block is the attach block
		attachSeen bool
	)
	for scanner.Scan() {
		line := scanner.Text()
		if inBlock {
			if f := strings.Fields(line); len(f) >= 3 && (f[0] == "%end" || f[0] == "%error") && f[2] == num {
				inBlock = false
				ok := f[0] == "%end"
				switch {
				case attach:
					if !ok {
						send(controlEvent{kind: controlFail})
						return
					}
					attach = false
					send(controlEvent{kind: controlAttached})
				case cur != nil:
					cur.ok = ok
					close(cur.done)
					send(controlEvent{kind: controlCmdDone, cmd: cur})
					cur = nil
				}
				continue
			}
			if cur != nil {
				cur.out = append(cur.out, line)
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "%begin "):
			f := strings.Fields(line)
			if len(f) < 4 {
				send(controlEvent{kind: controlFail})
				return
			}
			inBlock, num, cur, attach = true, f[2], nil, false
			if f[3] == "1" {
				if cur = r.pop(); cur == nil {
					send(controlEvent{kind: controlFail}) // a reply to a command we did not send
					return
				}
			} else if !attachSeen {
				attach, attachSeen = true, true
			}
		case strings.HasPrefix(line, outputPrefix):
			data, err := decodeOutput(line[len(outputPrefix):])
			if err != nil {
				send(controlEvent{kind: controlFail})
				return
			}
			send(controlEvent{kind: controlOutput, output: data})
		}
	}
	send(controlEvent{kind: controlEnd})
}
