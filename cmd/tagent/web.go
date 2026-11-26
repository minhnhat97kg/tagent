package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"tagent/internal/board"
	"tagent/internal/config"
	"tagent/internal/tmux"
	"tagent/internal/web"
)

// cmdWeb serves the browser UI. Bound to localhost by default; --addr
// 0.0.0.0:7777 opens it to the network (use a tunnel or VPN, it is plain HTTP).
// One server runs per user: a second `tagent web` adds its folder to that one.
func cmdWeb(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:7777", "listen address (0.0.0.0:7777 for remote access)")
	token := fs.String("token", os.Getenv("TMUX_AGENT_WEB_TOKEN"), "access token (default: random)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *token == "" {
		*token = savedWebToken()
	}
	dir, _ := os.Getwd()
	lock, running, err := claimWeb()
	if err != nil {
		fmt.Fprintln(stderr, "web:", err)
		return exitFailure
	}
	if lock == nil {
		if running.Addr == "" {
			fmt.Fprintln(stderr, "web: a tagent web server is starting but not listening yet; try again")
			return exitFailure
		}
		root, err := web.Forward(running, dir)
		if err != nil {
			fmt.Fprintln(stderr, "web:", err)
			return exitFailure
		}
		fmt.Fprintf(stdout, "added %s to running tagent: %s\n", root, web.URLs(running.Addr, running.Token)[0])
		return exitOK
	}
	if _, err := board.AddWorkspace(dir); err != nil {
		fmt.Fprintln(stderr, "web: workspace:", err)
	}
	fmt.Fprintln(stdout, "tagent web — open:")
	for _, u := range web.URLs(*addr, *token) {
		fmt.Fprintln(stdout, "  "+u)
	}
	if err := lock.Serve(*addr, *token, dir, dispatchBoardAction); err != nil {
		fmt.Fprintln(stderr, "web:", err)
		return exitFailure
	}
	return exitOK
}

// claimWeb takes web.lock, or returns the running server. It waits up to 2s for
// a holder that has the lock but has not published its address yet.
func claimWeb() (*web.Lock, web.Instance, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		lock, in, err := web.Claim()
		if err != nil || lock != nil || in.Addr != "" || time.Now().After(deadline) {
			return lock, in, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// runBackground is the scraper's other work: board triggers always, plus the
// web UI when @agent_web is on.
func runBackground() {
	tmux.SetOption("@agent_web_status", "⊕ off") // replaced below when on
	if setting("@agent_web") == "on" {
		go serveBackground(setting("@agent_web_addr"), savedWebToken())
	}
	board.Discover(context.Background(), board.Registered, dispatchBoardAction)
}

// serveBackground runs the web UI unless another tagent holds web.lock; then it
// shows that server and retries every second.
func serveBackground(addr, token string) {
	dir, _ := os.UserHomeDir()
	for {
		lock, running, err := web.Claim()
		if err != nil {
			fmt.Fprintln(os.Stderr, "web:", err)
			return
		}
		if lock == nil {
			if running.Addr != "" {
				showWeb(running.Addr, running.Token)
			}
			time.Sleep(time.Second)
			continue
		}
		showWeb(addr, token)
		if err := lock.Serve(addr, token, dir, dispatchBoardAction); err != nil {
			fmt.Fprintln(os.Stderr, "web:", err)
			_, port, _ := net.SplitHostPort(addr)
			tmux.SetOption("@agent_web_status", "⊗ :"+port)
		}
		return
	}
}

func showWeb(addr, token string) {
	_, port, _ := net.SplitHostPort(addr)
	tmux.SetOption("@agent_web_url", web.URLs(addr, token)[0])
	tmux.SetOption("@agent_web_status", "⊕ :"+port)
}

// savedWebToken is the access token kept in @agent_web_token, created on first
// use, so restarting or reloading the server never changes the link.
func savedWebToken() string {
	if t := setting("@agent_web_token"); t != "" {
		return t
	}
	t := web.NewToken()
	if s, ok := config.Find("@agent_web_token"); ok {
		config.Set(s, t)
	}
	return t
}

func setting(key string) string {
	s, _ := config.Find(key)
	return config.Get(s)
}
