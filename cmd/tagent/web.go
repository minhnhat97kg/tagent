package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"tagent/internal/board"
	"tagent/internal/config"
	"tagent/internal/tmux"
	"tagent/internal/web"
)

// cmdWeb serves the browser UI. Bound to localhost by default; --addr
// 0.0.0.0:7777 opens it to the network (use a tunnel or VPN, it is plain HTTP).
func cmdWeb(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:7777", "listen address (0.0.0.0:7777 for remote access)")
	token := fs.String("token", os.Getenv("TMUX_AGENT_WEB_TOKEN"), "access token (default: random)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *token == "" {
		*token = web.NewToken()
	}
	dir, _ := os.Getwd()
	fmt.Fprintln(stdout, "tagent web — open:")
	for _, u := range web.URLs(*addr, *token) {
		fmt.Fprintln(stdout, "  "+u)
	}
	if err := web.Serve(*addr, *token, dir, dispatchBoardAction); err != nil {
		fmt.Fprintln(stderr, "web:", err)
		return exitFailure
	}
	return exitOK
}

// runBackground is the scraper's other work: board triggers always, plus the
// web UI when @agent_web is on (its server runs the triggers itself).
func runBackground() {
	tmux.SetOption("@agent_web_status", "⊕ off") // replaced below when on
	if setting("@agent_web") == "on" {
		addr, token := setting("@agent_web_addr"), setting("@agent_web_token")
		if token == "" {
			token = web.NewToken()
			if s, ok := config.Find("@agent_web_token"); ok {
				config.Set(s, token)
			}
		}
		tmux.SetOption("@agent_web_url", web.URLs(addr, token)[0])
		_, port, _ := net.SplitHostPort(addr)
		tmux.SetOption("@agent_web_status", "⊕ :"+port)
		dir, _ := os.UserHomeDir()
		// The scraper being replaced keeps the port until its next tick.
		for range 15 {
			err := web.Serve(addr, token, dir, dispatchBoardAction)
			if !errors.Is(err, syscall.EADDRINUSE) {
				fmt.Fprintln(os.Stderr, "web:", err)
				tmux.SetOption("@agent_web_status", "⊗ :"+port)
				break
			}
			time.Sleep(time.Second)
		}
	}
	board.Discover(context.Background(), board.Registered, dispatchBoardAction)
}

func setting(key string) string {
	s, _ := config.Find(key)
	return config.Get(s)
}
