// Package web is the browser UI (`tagent web`): a JSON API over the same
// tmux state the sidebar reads, plus the embedded React app in dist/.
// Actions that already exist as commands (new, kill, answer) run the
// tagent binary itself, so the web and the CLI behave the same.
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
	"strings"
	"sync"

	"tagent/internal/agents"
	"tagent/internal/board"
	"tagent/internal/scrape"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
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
	mux.HandleFunc("GET /api/pane/ws", s.paneWS)
	mux.HandleFunc("POST /api/pane/input", s.input)
	mux.HandleFunc("POST /api/pane/send", s.send)
	mux.HandleFunc("POST /api/uploads", s.upload)
	mux.HandleFunc("GET /api/uploads/{id}/{name}", s.serveUpload)
	mux.HandleFunc("GET /api/agents/{pane}/transcript", s.transcript)
	mux.HandleFunc("GET /api/workflows", s.workflows)
	mux.HandleFunc("POST /api/workflows", s.workflows)
	mux.HandleFunc("DELETE /api/workflows", s.workflows)
	mux.HandleFunc("GET /api/roles", s.roles)
	mux.HandleFunc("POST /api/roles", s.roles)
	mux.HandleFunc("DELETE /api/roles", s.roles)
	mux.HandleFunc("POST /api/pane/answer", s.answer)
	mux.HandleFunc("POST /api/pane/kill", s.kill)
	mux.HandleFunc("POST /api/pane/action", s.agentAction)
	mux.HandleFunc("POST /api/agents", s.newAgent)
	mux.HandleFunc("POST /api/tasks", s.taskOp)
	mux.HandleFunc("GET /api/tasks/{id}/attachments", s.attachments)
	mux.HandleFunc("GET /api/tasks/{id}/run", s.run)
	mux.HandleFunc("GET /api/tickets/{id}/chat", s.ticketChat)
	mux.HandleFunc("POST /api/tickets/{id}/chat", s.ticketChat)
	mux.HandleFunc("POST /api/tasks/{id}/attachments", s.attachments)
	mux.HandleFunc("GET /api/config", s.settings)
	mux.HandleFunc("POST /api/config", s.settings)
	sub, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(sub)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "no-cache") // index.html must pick up new hashed assets
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // names carry a content hash
		}
		gzipped(w, r, files.ServeHTTP)
	})
	return s.auth(etagged(mux))
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
	pinned     sync.Map
	sizeOwners terminalOwners
	chatClaims sync.Map // panes whose size Chat borrowed for a clipped question
	chatLocks  sync.Map // per-ticket mutex around resolving/spawning a chat pane
	deps       apiDeps
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && s.ok(t) {
			http.SetCookie(w, &http.Cookie{Name: "tmux_agent", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 90 * 24 * 3600})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		c, err := r.Cookie("tmux_agent")
		if err != nil || !s.ok(c.Value) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, "<p style='font-family:sans-serif'>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p>")
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

// self runs the tagent binary with args and returns its output.
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
	Pinned    bool        `json:"pinned"`
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
	Stuck     string      `json:"stuck,omitempty"`
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
					Prompt: p.Prompt, Wait: p.WaitReason, Stuck: p.Stuck, Branch: p.Worktree.Branch, Hidden: hidden, Subagents: p.Subagents}
				_, a.Pinned = s.pinned.Load(p.PaneID)
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
	Automatic bool   `json:"automatic"`
	Alternate bool   `json:"alternate"`
	Mouse     bool   `json:"mouse"`
	Resizable bool   `json:"resizable"`
	Screen    string `json:"screen"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
	CX        int    `json:"cx"`
	CY        int    `json:"cy"`
	Cursor    bool   `json:"cursor"`
}

var paneInputLocks sync.Map

func lockPaneInput(pane string) func() {
	v, _ := paneInputLocks.LoadOrStore(pane, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

type paneReq struct {
	Dialog *dialogJSON `json:"dialog,omitempty"`
	Pane   string      `json:"pane"`
	Data   string      `json:"data"` // raw terminal input (input) / text (send) / option key (answer)
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
	unlock := lockPaneInput(p.Pane)
	defer unlock()
	if _, ok := tmux.RunTmux(args...); !ok {
		fail(w, fmt.Errorf("terminal input failed; delivery uncertain, not retried"))
	}
}

func (s *server) answer(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.paneBody(w, r); ok {
		unlock := lockPaneInput(p.Pane)
		defer unlock()
		if p.Dialog != nil {
			screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", p.Pane)
			current, found := scrape.ParseDialog(screen)
			expected, _ := json.Marshal(p.Dialog)
			actual, _ := json.Marshal(dialogJSON{current.Question, current.Options})
			if !ok || !found || string(expected) != string(actual) {
				fail(w, fmt.Errorf("approval changed; refresh before answering"))
				return
			}
		}
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
	defs := []string{}
	for _, a := range agents.List(dir, harness, nil) {
		if a.Source != "global" {
			defs = append(defs, a.Name)
		}
	}
	sort.Strings(defs)
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		fail(w, err)
		return
	}
	statuses := []map[string]string{}
	seen := map[string]bool{}
	for _, status := range cfg.Statuses {
		seen[status.ID] = true
		statuses = append(statuses, map[string]string{"id": status.ID, "role": status.Role, "run": status.Run})
	}
	// Workflow tickets move through their recipe's statuses (planning, building, ...), not the board's;
	// give each one a column before "blocked" so those tickets show up and move.
	at := len(statuses)
	for i, st := range statuses {
		if st["id"] == "blocked" {
			at = i
			break
		}
	}
	extra := []map[string]string{}
	for _, ticket := range t {
		wcfg, err := tasks.ConfigForTask(dir, cfg, ticket)
		if ticket.Workflow == "" || err != nil {
			continue
		}
		for _, status := range wcfg.Statuses {
			if !seen[status.ID] {
				seen[status.ID] = true
				extra = append(extra, map[string]string{"id": status.ID, "role": status.Role, "run": status.Run})
			}
		}
	}
	statuses = append(statuses[:at], append(extra, statuses[at:]...)...)
	workers := map[string]map[string]any{}
	history, _ := tasks.ReadEvents(tasks.Scope(dir), 0)
	reasons := map[string]string{}
	panes := tmux.QuerySessions()
	for _, ticket := range t {
		flow, err := tasks.ConfigForTask(dir, cfg, ticket)
		if err != nil {
			fail(w, err)
			return
		}
		role := ""
		for _, status := range flow.Statuses {
			if status.ID == ticket.Status {
				role = status.Role
			}
		}
		if ticket.Status == "blocked" {
			_, _, reasons[ticket.ID] = tasks.BlockContext(ticket, flow, history)
		}
		best := 0
		for _, session := range panes {
			for _, window := range session.Windows {
				for _, pane := range window.Panes {
					if tasks.MainRoot(pane.Path) != tasks.MainRoot(dir) {
						continue
					}
					name, id, _ := strings.Cut(pane.BoardTarget, "#")
					rank := 0
					if id == ticket.ID {
						rank = 2
						if name == role {
							rank = 3
						}
					} else if id == "" {
						for _, assigned := range ticket.Assignees {
							if assigned == pane.Name || assigned == name {
								rank = 1
							}
						}
					}
					if rank > best {
						best = rank
						if name == "" {
							name = pane.Name
						}
						workers[ticket.ID] = map[string]any{"role": name, "pane": pane.PaneID, "live": pane.Status.IsActive()}
					}
				}
			}
		}
	}
	flows := []map[string]any{}
	for _, wf := range workflow.List(tasks.MainRoot(dir)) {
		jobs := []map[string]string{}
		for _, j := range wf.Jobs {
			jobs = append(jobs, map[string]string{"id": j.ID, "status": j.Status, "agent": j.Agent, "run": j.Run})
		}
		e := ""
		if wf.Err != nil {
			e = wf.Err.Error()
		}
		flows = append(flows, map[string]any{"name": wf.Name, "description": wf.Description, "global": wf.Global, "file": wf.File, "jobs": jobs, "error": e})
	}
	writeJSON(w, map[string]any{"dir": dir, "tasks": t, "roles": defs, "statuses": statuses, "workers": workers, "blockedReasons": reasons, "workflows": flows})
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
	case "comment":
		if strings.TrimSpace(o.Text) == "" {
			err = fmt.Errorf("comment must not be empty")
		} else {
			err = tasks.Comment(scope, o.ID, "user", o.Text, nil, tasks.MutationOptions{Actor: "user"})
			if _, ok := err.(*tasks.CommentNotice); ok {
				writeJSON(w, map[string]string{"notice": err.Error()})
				return
			}
		}
	case "edit":
		err = tasks.Edit(scope, o.ID, o.Text)
	case "rm":
		err = tasks.Remove(scope, o.ID)
	case "start": // Status names a workflow, or "board" for the next column (same as the TUI's s picker)
		err = board.Start(o.Dir, o.ID, o.Status)
	default:
		err = fmt.Errorf("unknown op %q", o.Op)
	}
	if err != nil {
		fail(w, err)
	}
}
