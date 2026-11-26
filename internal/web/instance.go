package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"tagent/internal/board"
	"tagent/internal/sharedctx"
)

// Instance is what the running server publishes in web.lock.
type Instance struct {
	PID   int    `json:"pid"`
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

// Lock is web.lock. Whoever holds its flock runs the one tagent web server of
// the user; the flock dies with the process, so a crashed holder never blocks.
type Lock struct{ f *os.File }

func lockPath() string { return filepath.Join(filepath.Dir(sharedctx.Dir("global")), "web.lock") }

// Claim takes the lock. If another process holds it, it returns a nil Lock and
// that holder's Instance (Addr is empty until it listens).
func Claim() (*Lock, Instance, error) {
	path := lockPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, Instance{}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, Instance{}, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			var in Instance
			b, _ := os.ReadFile(path)
			json.Unmarshal(b, &in)
			return nil, in, nil
		}
		return nil, Instance{}, err
	}
	f.Truncate(0)
	return &Lock{f}, Instance{}, nil
}

// Release gives the lock back without serving.
func (l *Lock) Release() { l.f.Close() }

// Serve publishes this server in web.lock, then serves until it fails. The lock
// is released on return.
func (l *Lock) Serve(addr, token, dir string, fire ...board.Fire) error {
	defer l.f.Close()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(Instance{PID: os.Getpid(), Addr: ln.Addr().String(), Token: token})
	if _, err := l.f.WriteAt(b, 0); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &server{token: token, dir: dir}
	if len(fire) > 0 {
		go s.discoverBoards(ctx, fire[0])
	}
	return http.Serve(ln, s.handler())
}

// Forward adds dir to the running server and returns the workspace it is listed as.
func Forward(in Instance, dir string) (string, error) {
	u, err := url.Parse(URLs(in.Addr, in.Token)[0])
	if err != nil {
		return "", err
	}
	u.RawQuery, u.Path = "", "/api/workspaces"
	body, _ := json.Marshal(map[string]string{"Dir": dir})
	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.AddCookie(&http.Cookie{Name: "tmux_agent", Value: in.Token})
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("running tagent refused %s: %s", dir, bytes.TrimSpace(out))
	}
	var r struct{ Dir string }
	json.Unmarshal(out, &r)
	return r.Dir, nil
}
