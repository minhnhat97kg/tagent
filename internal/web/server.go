// Package web is the browser UI (`tmux-agent web`): a JSON API over the same
// tmux state the sidebar reads, plus the embedded React app in dist/.
// Actions that already exist as commands (new, kill, answer) run the
// tmux-agent binary itself, so the web and the CLI behave the same.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"tmux-agent/internal/board"

	"tmux-agent/internal/roles"
	"tmux-agent/internal/scrape"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

//go:embed all:dist
var dist embed.FS

// Serve runs the server until it fails. token guards every request; the
// page link carries it once and the browser keeps it as a cookie.
func Serve(addr, token, dir string, fire ...board.Fire) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &server{token: token, dir: dir}
	if len(fire) > 0 {
		go s.discoverBoards(ctx, fire[0])
	}
	return http.ListenAndServe(addr, s.handler())
}
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/project", s.project)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/chat", s.chat)
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.HandleFunc("GET /api/pane/stream", s.stream)
	mux.HandleFunc("POST /api/pane/input", s.input)
	mux.HandleFunc("POST /api/pane/send", s.send)
	mux.HandleFunc("POST /api/pane/answer", s.answer)
	mux.HandleFunc("POST /api/pane/kill", s.kill)
	mux.HandleFunc("POST /api/agents", s.newAgent)
	mux.HandleFunc("POST /api/tasks", s.taskOp)
	sub, _ := fs.Sub(dist, "dist")
	mux.Handle("/", http.FileServerFS(sub))
	return s.auth(mux)
}

// NewToken is a random access token.
func NewToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// URLs are the addresses the server can be opened at, the token included.
func URLs(addr, token string) []string {
	host, port, _ := net.SplitHostPort(addr)
	hosts := []string{host}
	if host == "" || host == "0.0.0.0" || host == "::" {
		hosts = []string{"localhost"}
		if as, err := net.InterfaceAddrs(); err == nil {
			for _, a := range as {
				if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
					hosts = append(hosts, ip.IP.String())
				}
			}
		}
	}
	var out []string
	for _, h := range hosts {
		out = append(out, "http://"+net.JoinHostPort(h, port)+"/?token="+token)
	}
	return out
}

type server struct {
	token, dir string
	boardDirs  sync.Map
	deps       apiDeps
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && s.ok(t) {
			http.SetCookie(w, &http.Cookie{Name: "tmux_agent", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 90 * 24 * 3600})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		c, err := r.Cookie("tmux_agent")
		if err != nil || !s.ok(c.Value) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, "<p style='font-family:sans-serif'>tmux-agent: open the link printed by <code>tmux-agent web</code> (it carries the access token).</p>")
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) ok(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) { http.Error(w, err.Error(), http.StatusBadRequest) }

func readBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// self runs the tmux-agent binary with args and returns its output.
func self(args ...string) (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		return "", errors.New(strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ---- agents ----

type dialogJSON struct {
	Question string          `json:"question"`
	Options  []scrape.Option `json:"options"`
}

type agentJSON struct {
	Pane      string      `json:"pane"`
	Name      string      `json:"name"`
	Agent     string      `json:"agent"`
	Status    string      `json:"status"`
	Session   string      `json:"session"`
	Window    string      `json:"window"`
	Path      string      `json:"path"`
	Project   string      `json:"project"`
	Prompt    string      `json:"prompt"`
	Wait      string      `json:"wait,omitempty"`
	Started   int64       `json:"started,omitempty"`
	Branch    string      `json:"branch,omitempty"`
	Hidden    bool        `json:"hidden,omitempty"`
	Dialog    *dialogJSON `json:"dialog,omitempty"`
	Subagents []string    `json:"subagents,omitempty"`
}

func (s *server) agents() []agentJSON {
	out := []agentJSON{}
	for _, se := range tmux.QuerySessions() {
		hidden := tmux.DisplayMessage(se.SessionName+":", "#{@agent_hidden}") == "1"
		for _, wi := range se.Windows {
			for _, p := range wi.Panes {
				status := string(p.Status)
				if p.Status == tmux.StatusIdle && p.Unseen {
					status = "done"
				}
				a := agentJSON{Pane: p.PaneID, Name: p.Name, Agent: string(p.Agent), Status: status, Session: se.SessionName,
					Window: wi.WindowIndex + ":" + wi.WindowName, Path: p.Path, Project: filepath.Base(tasks.MainRoot(p.Path)),
					Prompt: p.Prompt, Wait: p.WaitReason, Branch: p.Worktree.Branch, Hidden: hidden, Subagents: p.Subagents}
				if p.Status.IsActive() {
					a.Started = p.StartedAt
				}
				if p.Status == tmux.StatusWaiting {
					if screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", p.PaneID); ok {
						if d, ok := scrape.ParseDialog(screen); ok {
							a.Dialog = &dialogJSON{d.Question, d.Options}
						}
					}
				}
				out = append(out, a)
			}
		}
	}
	return out
}

// projects are the repos with a running agent, plus the server's own dir.
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	agents := s.agents()
	seen := map[string]bool{}
	projects := []string{}
	for _, d := range append([]string{s.dir}, func() (ps []string) {
		for _, a := range agents {
			ps = append(ps, a.Path)
		}
		return
	}()...) {
		if d == "" {
			continue
		}
		if root := tasks.MainRoot(d); !seen[root] {
			seen[root] = true
			projects = append(projects, root)
		}
	}
	host, _ := os.Hostname()
	writeJSON(w, map[string]any{"agents": agents, "projects": projects, "host": host, "harnesses": []string{"claude", "codex", "opencode", "gemini", "pi", "copilot", "amp"}})
}

var paneRe = regexp.MustCompile(`^%\d+$`)

// agentPane rejects anything but an agent's pane id: the API must not type
// into shells or editors.
func agentPane(id string) error {
	if !paneRe.MatchString(id) || tmux.GetPaneOptionValue(id, tmux.PaneAgent) == "" {
		return fmt.Errorf("%q is not an agent pane", id)
	}
	return nil
}

type paneShot struct {
	Screen string `json:"screen"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	CX     int    `json:"cx"`
	CY     int    `json:"cy"`
	Cursor bool   `json:"cursor"`
}

// stream pushes the pane's screen (with colors) whenever it changes.
// ponytail: full screen per change, ~10/s; diff frames if bandwidth matters.
func (s *server) stream(w http.ResponseWriter, r *http.Request) {
	pane := r.URL.Query().Get("pane")
	if err := agentPane(pane); err != nil {
		fail(w, err)
		return
	}
	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	last := ""
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		info, ok := tmux.RunTmux("display-message", "-p", "-t", pane, "#{pane_width} #{pane_height} #{cursor_x} #{cursor_y} #{cursor_flag}")
		if !ok {
			fmt.Fprint(w, "event: gone\ndata: {}\n\n")
			fl.Flush()
			return
		}
		screen, _ := tmux.RunTmux("capture-pane", "-e", "-p", "-t", pane)
		if key := info + screen; key != last {
			last = key
			f := strings.Fields(info)
			n := func(i int) int {
				if i < len(f) {
					v, _ := strconv.Atoi(f[i])
					return v
				}
				return 0
			}
			b, _ := json.Marshal(paneShot{Screen: strings.TrimRight(screen, "\n"), Cols: n(0), Rows: n(1), CX: n(2), CY: n(3), Cursor: n(4) == 1})
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

type paneReq struct {
	Pane string `json:"pane"`
	Data string `json:"data"` // raw terminal input (input) / text (send) / option key (answer)
}

func (s *server) paneBody(w http.ResponseWriter, r *http.Request) (paneReq, bool) {
	var p paneReq
	if err := readBody(r, &p); err != nil {
		fail(w, err)
		return p, false
	}
	if err := agentPane(p.Pane); err != nil {
		fail(w, err)
		return p, false
	}
	return p, true
}

// input forwards raw keystrokes from the browser terminal, byte for byte.
func (s *server) input(w http.ResponseWriter, r *http.Request) {
	p, ok := s.paneBody(w, r)
	if !ok || p.Data == "" {
		return
	}
	args := []string{"send-keys", "-t", p.Pane, "-H"}
	for _, b := range []byte(p.Data) {
		args = append(args, fmt.Sprintf("%02x", b))
	}
	tmux.RunTmux(args...)
}

func (s *server) send(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.paneBody(w, r); ok && strings.TrimSpace(p.Data) != "" {
		tmux.TypeLine(p.Pane, p.Data)
	}
}

func (s *server) answer(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.paneBody(w, r); ok {
		if _, err := self("answer", p.Pane, p.Data); err != nil {
			fail(w, err)
		}
	}
}

func (s *server) kill(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.paneBody(w, r); ok {
		if _, err := self("kill", p.Pane); err != nil {
			fail(w, err)
		}
	}
}

func (s *server) newAgent(w http.ResponseWriter, r *http.Request) {
	var o struct {
		Agent, Model, Cwd, Role, Name, Prompt string
		Worktree                              bool
	}
	if err := readBody(r, &o); err != nil {
		fail(w, err)
		return
	}
	if o.Cwd == "" {
		o.Cwd = s.dir
	}
	if st, err := os.Stat(o.Cwd); err != nil || !st.IsDir() {
		fail(w, fmt.Errorf("no directory %q", o.Cwd))
		return
	}
	args := []string{"new", "--cwd", o.Cwd}
	for flag, v := range map[string]string{"--agent": o.Agent, "--model": o.Model, "--repo-agent": o.Role, "--name": o.Name} {
		if v != "" {
			args = append(args, flag, v)
		}
	}
	if o.Worktree {
		args = append(args, "--worktree")
	}
	if o.Prompt == "" {
		args = append(args, "--no-wait")
	}
	pane, err := self(args...)
	if err != nil {
		fail(w, err)
		return
	}
	pane = strings.Fields(pane + " ")[0]
	if o.Prompt != "" && paneRe.MatchString(pane) {
		go tmux.TypeLine(pane, o.Prompt)
	}
	writeJSON(w, map[string]string{"pane": pane})
}

// ---- project: tickets and agent definitions ----

func (s *server) projectDir(r *http.Request) string {
	if d := r.URL.Query().Get("dir"); d != "" {
		return d
	}
	return s.dir
}

func (s *server) project(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	s.boardDirs.Store(tasks.MainRoot(dir), true)
	t := tasks.List(tasks.Scope(dir))
	if t == nil {
		t = []tasks.Task{}
	}
	harness, _ := tmux.GetOption(tmux.AgentOption)
	defs := roles.NamesIn(dir, harness)
	sort.Strings(defs)
	if defs == nil {
		defs = []string{}
	}
	writeJSON(w, map[string]any{"dir": dir, "tasks": t, "roles": defs})
}

func (s *server) taskOp(w http.ResponseWriter, r *http.Request) {
	var o struct {
		Dir, Op, ID, Text, Status string
		Assignees                 []string
	}
	if err := readBody(r, &o); err != nil {
		fail(w, err)
		return
	}
	if o.Dir == "" {
		o.Dir = s.dir
	}
	s.boardDirs.Store(tasks.MainRoot(o.Dir), true)
	scope := tasks.Scope(o.Dir)
	var err error
	switch o.Op {
	case "add":
		_, err = tasks.Add(scope, o.Text, o.Assignees, tasks.MutationOptions{Actor: "user"})
	case "set":
		_, err = tasks.Update(scope, o.ID, o.Status, nil, tasks.MutationOptions{Actor: "user"})
	case "edit":
		err = tasks.Edit(scope, o.ID, o.Text)
	case "rm":
		err = tasks.Remove(scope, o.ID)
	default:
		err = fmt.Errorf("unknown op %q", o.Op)
	}
	if err != nil {
		fail(w, err)
	}
}
