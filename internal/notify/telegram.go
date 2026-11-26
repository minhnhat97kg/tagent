package notify

// Telegram is a chat channel: the scraper long-polls getUpdates, routes messages
// to agents and replies with their answers (or the dialog they stopped at). It
// never sends anything on its own. Agents are driven by re-running this binary
// (tagent send/new/answer), so this package stays a leaf: no import of
// cmd/tagent, scrape or tasks.
//
// Settings live in <config dir>/telegram.env (kept 0600); a tmux option is the
// per-key fallback:
//
//	token=<bot token>    @agent_telegram_token   empty = off
//	chats=<id>,<id>      @agent_telegram_chats   allowlist; others are ignored
//	agent=receptionist   @agent_telegram_agent   default chat target

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unicode"

	"tagent/internal/apppath"
	"tagent/internal/tmux"
	"tagent/internal/transcript"
)

const (
	tokenOption = "@agent_telegram_token"
	chatsOption = "@agent_telegram_chats"
	agentOption = "@agent_telegram_agent"

	maxMessages   = 500  // remembered bot messages (reply-to routing)
	maxChunks     = 3    // longest reply sent, in messages; the end is kept
	maxUnits      = 3800 // Telegram caps a message at 4096 UTF-16 units; the rest is wrappers
	nameRunes     = 40
	alertDedupe   = 15 * time.Minute // one card per open dialog; answering releases it
	replyTimeout  = 11 * time.Minute // tagent send waits up to 10m
	exitTimeout   = 124              // tagent wait's timeout exit code
	pollSeconds   = 25
	defaultTarget = "receptionist"
)

var (
	telegramAPI = "https://api.telegram.org" // seam for tests
	httpClient  = &http.Client{}             // every call is bounded by its context
	runTagent   = execTagent                 // seam: tests fake the agent CLI
	listAgents  = listAgentPanes             // seam: tests fake the pane list
	envPath     = defaultEnvPath             // seam: tests point at a temp file
	polling     atomic.Bool
	commandsFor atomic.Value // the token whose command menu is registered

	nameRe = regexp.MustCompile(`^%?[\w.\-/]+$`)
	paneRe = regexp.MustCompile(`^%\d+$`)
	keyRe  = regexp.MustCompile(`^([0-9]{1,2}|y|n)$`)

	ignoredMu sync.Mutex
	ignored   = map[int64]bool{}
	targetMu  sync.Mutex
	targetMux = map[string]*sync.Mutex{}
)

// ── config and state ──────────────────────────────────────────────────────

type tgConfig struct {
	token, defaultAgent string
	allowed             map[int64]bool
}

// defaultEnvPath is empty in test binaries: a test must never message a real
// chat.
func defaultEnvPath() string {
	if testing.Testing() {
		return ""
	}
	return filepath.Join(apppath.ConfigDir(), "telegram.env")
}

// fileConfig reads the key=value lines of telegram.env and keeps it private.
func fileConfig() map[string]string {
	m := map[string]string{}
	path := envPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	_ = os.Chmod(path, 0o600)
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return m
}

// setting is the file value of key, else the tmux option.
func setting(file map[string]string, key, option string) string {
	if v := file[key]; v != "" {
		return v
	}
	v, _ := tmux.GetOption(option)
	return strings.TrimSpace(v)
}

func loadConfig() tgConfig {
	file := fileConfig()
	c := tgConfig{token: setting(file, "token", tokenOption), allowed: map[int64]bool{}}
	for _, f := range strings.Split(setting(file, "chats", chatsOption), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64); err == nil {
			c.allowed[id] = true
		}
	}
	if c.defaultAgent = setting(file, "agent", agentOption); c.defaultAgent == "" {
		c.defaultAgent = defaultTarget
	}
	return c
}

type msgRef struct {
	Chat int64  `json:"chat"`
	Msg  int64  `json:"msg"`
	Pane string `json:"pane"`
	PID  string `json:"panePID"`
}

type alertMark struct {
	Hash string `json:"hash"`
	At   int64  `json:"at"` // unix ms
}

type tgState struct {
	Offset   int64                `json:"offset"`
	Targets  map[string]string    `json:"targets,omitempty"`
	Messages []msgRef             `json:"messages,omitempty"`
	Alerts   map[string]alertMark `json:"alerts,omitempty"`
}

func statePath() string { return filepath.Join(apppath.ConfigDir(), "telegram.json") }

func loadState() tgState {
	var st tgState
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// updateState is the only writer. The scraper and hook processes both write,
// so flock serialises them; the file is replaced by rename, never half-written.
func updateState(fn func(*tgState)) {
	dir := apppath.ConfigDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	lock, err := os.OpenFile(statePath()+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX) != nil {
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	st := loadState()
	fn(&st)
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "telegram-*.json")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), statePath()) != nil {
		os.Remove(tmp.Name())
	}
}

func rememberMessage(chat, msg int64, a agentPane) {
	updateState(func(st *tgState) {
		st.Messages = append(st.Messages, msgRef{Chat: chat, Msg: msg, Pane: a.ID, PID: a.PID})
		if over := len(st.Messages) - maxMessages; over > 0 {
			st.Messages = st.Messages[over:]
		}
	})
}

func findMessage(chat, msg int64) (msgRef, bool) {
	st := loadState()
	for i := len(st.Messages) - 1; i >= 0; i-- {
		if m := st.Messages[i]; m.Chat == chat && m.Msg == msg {
			return m, true
		}
	}
	return msgRef{}, false
}

// claimAlert reserves the alert for chat/pane/hash; false when the same dialog
// was already sent within alertDedupe (a message queued behind a dialog must
// not show the same card twice).
func claimAlert(chat int64, pane, hash string) bool {
	claimed := false
	key := strconv.FormatInt(chat, 10) + "|" + pane
	updateState(func(st *tgState) {
		now := time.Now().UnixMilli()
		if m, ok := st.Alerts[key]; ok && m.Hash == hash && now-m.At < alertDedupe.Milliseconds() {
			return
		}
		if st.Alerts == nil {
			st.Alerts = map[string]alertMark{}
		}
		st.Alerts[key] = alertMark{Hash: hash, At: now}
		claimed = true
	})
	return claimed
}

// releaseAlert forgets a claim: the send failed, or the dialog was answered.
func releaseAlert(chat int64, pane string) {
	updateState(func(st *tgState) { delete(st.Alerts, strconv.FormatInt(chat, 10)+"|"+pane) })
}

func testingBinary() bool { return testing.Testing() }

// ── Telegram API ──────────────────────────────────────────────────────────

// tgCall posts one Bot API method. Errors never contain the token: net/http
// errors embed the request URL, so the token is replaced before returning.
func tgCall(ctx context.Context, token, method string, params, out any) error {
	scrub := func(err error) error {
		if token == "" {
			return err
		}
		return errors.New(strings.ReplaceAll(err.Error(), token, "<token>"))
	}
	b, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramAPI+"/bot"+token+"/"+method, bytes.NewReader(b))
	if err != nil {
		return scrub(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return scrub(err)
	}
	defer resp.Body.Close()
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d", method, resp.StatusCode)
	}
	if !env.OK {
		return fmt.Errorf("telegram %s: %s", method, env.Description)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

type update struct {
	ID       int64     `json:"update_id"`
	Message  *message  `json:"message"`
	Callback *callback `json:"callback_query"`
}

type message struct {
	ID      int64    `json:"message_id"`
	Chat    chatInfo `json:"chat"`
	Text    string   `json:"text"`
	ReplyTo *message `json:"reply_to_message"`
}

type chatInfo struct {
	ID int64 `json:"id"`
}

type callback struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *message `json:"message"`
}

// sendMsg posts HTML text. A non-zero agent makes the message a reply route to it.
func sendMsg(ctx context.Context, token string, chat int64, text string, markup any, a agentPane) error {
	params := map[string]any{"chat_id": chat, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true}
	if markup != nil {
		params["reply_markup"] = markup
	}
	var sent struct {
		ID int64 `json:"message_id"`
	}
	if err := tgCall(ctx, token, "sendMessage", params, &sent); err != nil {
		return err
	}
	if a.ID != "" {
		rememberMessage(chat, sent.ID, a)
	}
	return nil
}

func say(cfg tgConfig, chat int64, text string) {
	sayAbout(cfg, chat, agentPane{}, text)
}

// sayAbout sends a short note; a pane makes it a reply route (every bot
// message about a pane is recorded, so replies to it reach that pane).
func sayAbout(cfg tgConfig, chat int64, a agentPane, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = sendMsg(ctx, cfg.token, chat, firstPiece(text), nil, a)
}

// ── dialog cards ──────────────────────────────────────────────────────────

// alert answers a chat whose agent stopped at a dialog: a card saying note,
// with one button per option; a tap answers through the same `tagent answer`
// path. It is only sent in reply to the chat's own message or tap.
func alert(ctx context.Context, token string, chat int64, note string, a agentPane) {
	question, opts := readDialog(ctx, a.ID)
	// The dialog may not be drawn yet: retry briefly.
	for i := 0; len(opts) == 0 && i < 3 && isWaiting(a.ID); i++ {
		time.Sleep(300 * time.Millisecond)
		question, opts = readDialog(ctx, a.ID)
	}
	if !claimAlert(chat, a.ID, dialogHash(a.ID, opts)) {
		return
	}
	var rows [][]button
	screen := ""
	if len(opts) > 0 {
		for _, o := range opts {
			rows = append(rows, []button{btn(trunc(o.Key+"  "+o.Label, 40), "a|"+a.ID+"|"+o.Key+"|"+dialogHash(a.ID, opts))})
		}
		out, _ := tmux.CaptureTail(a.ID, 40)
		screen = dialogContext(out, question, opts)
	}
	rows = append(rows, []button{btn("👀 Screen", "s|"+a.ID), btn("💬 Chat", "u|"+a.target())})
	if len(opts) == 0 {
		question = ""
	}
	text := alertCard(a, paneContext(ctx, a.ID), note, question, screen)
	if sendMsg(ctx, token, chat, text, keyboard(rows...), a) != nil {
		releaseAlert(chat, a.ID)
	}
}

type dialogOpt struct{ Key, Label string }

// readDialog reads the open dialog's question and options with `tagent answer <pane> ?`.
func readDialog(ctx context.Context, pane string) (question string, opts []dialogOpt) {
	out, _, code := runTagent(ctx, "answer", pane, "?")
	if code != 0 {
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, l := range lines[1:] {
		if f := strings.Fields(l); len(f) >= 2 {
			opts = append(opts, dialogOpt{Key: f[0], Label: strings.Join(f[1:], " ")})
		}
	}
	return strings.TrimSpace(lines[0]), opts
}

func dialogOpts(ctx context.Context, pane string) []dialogOpt {
	_, opts := readDialog(ctx, pane)
	return opts
}

// dialogHash fingerprints the dialog as drawn: the ~20 screen lines ending at
// the last option, letters and digits only. The question and options alone
// often repeat for different commands; the screen does not.
func dialogHash(pane string, opts []dialogOpt) string {
	screen, _ := tmux.CaptureTail(pane, 40)
	lines := strings.Split(strings.TrimRight(screen, "\n "), "\n")
	end := len(lines) - 1
	for i := len(lines) - 1; i >= 0; i-- {
		if isOption(lines[i], opts) {
			end = i
			break
		}
	}
	var b strings.Builder
	for _, l := range lines[max(0, end-19) : end+1] {
		for _, r := range l {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:6])
}

// firstPiece is the escaped head of s, for messages that are not split.
func firstPiece(s string) string {
	if p := split(s); len(p) > 0 {
		return p[0]
	}
	return ""
}

func isOption(line string, opts []dialogOpt) bool {
	for _, o := range opts {
		if strings.Contains(line, o.Label) {
			return true
		}
	}
	return false
}

// ── agents ────────────────────────────────────────────────────────────────

type agentPane struct {
	ID, Name, Board, Agent, Session, PID string
	Status                               tmux.PaneStatus
}

func (a agentPane) label() string {
	switch {
	case a.Name != "":
		return a.Name
	case a.Board != "":
		return a.Board
	}
	return a.ID
}

// target is what a button stores to choose this agent: its label, or the pane
// id when the label cannot go in callback data (board names carry "#").
func (a agentPane) target() string {
	if l := a.label(); nameRe.MatchString(l) && len("u|"+l) <= 64 { // Telegram callback_data cap
		return l
	}
	return a.ID
}

// listAgentPanes returns the panes that run an agent. Panes without one are
// never typed into.
func listAgentPanes() []agentPane {
	pids := map[string]string{}
	if out, ok := tmux.RunTmux("list-panes", "-a", "-F", "#{pane_id} #{pane_pid}"); ok {
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) == 2 {
				pids[f[0]] = f[1]
			}
		}
	}
	var out []agentPane
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Agent == "" {
					continue
				}
				out = append(out, agentPane{
					ID:      p.PaneID,
					Name:    tmux.GetPaneOptionValue(p.PaneID, tmux.PaneName),
					Board:   p.BoardTarget,
					Agent:   string(p.Agent),
					Session: tmux.GetPaneOptionValue(p.PaneID, tmux.PaneSessionID),
					PID:     pids[p.PaneID],
					Status:  p.Status,
				})
			}
		}
	}
	return out
}

// matchAgent finds a running agent by pane id, pane name or board name.
func matchAgent(target string) (agentPane, bool) {
	if target == "" {
		return agentPane{}, false
	}
	for _, a := range listAgents() {
		if a.ID == target || a.Name == target || a.Board == target {
			return a, true
		}
	}
	return agentPane{}, false
}

func isWaiting(pane string) bool {
	a, ok := matchAgent(pane)
	return ok && a.Status == tmux.StatusWaiting
}

// resolve finds the target's pane. An unknown name is started as a repo agent,
// from $HOME first, then each registered repo; dir is where it started ("" when
// it was already running).
func resolve(ctx context.Context, target string, dirs func() []string) (a agentPane, dir string, err error) {
	if a, ok := matchAgent(target); ok {
		return a, "", nil
	}
	if strings.HasPrefix(target, "%") {
		return agentPane{}, "", fmt.Errorf("no agent in pane %s", target)
	}
	home, _ := os.UserHomeDir()
	candidates := []string{home}
	if dirs != nil {
		repos := dirs()
		sort.Strings(repos)
		candidates = append(candidates, repos...)
	}
	for _, d := range candidates {
		out, errOut, code := runTagent(ctx, "new", "--repo-agent", target, "--cwd", d)
		if code == 0 {
			id := strings.TrimSpace(out)
			// The new pane registers itself a moment after it starts; a bare
			// record would make every reply read as "gone", so poll briefly.
			for i := 0; i < 10; i++ {
				if a, ok := matchAgent(id); ok {
					return a, d, nil
				}
				select {
				case <-ctx.Done():
					return agentPane{ID: id, Name: target}, d, nil
				case <-time.After(200 * time.Millisecond):
				}
			}
			return agentPane{ID: id, Name: target}, d, nil
		}
		if !strings.Contains(errOut, "unknown repo agent") {
			return agentPane{}, "", fmt.Errorf("could not start %s: %s", target, strings.TrimSpace(errOut))
		}
	}
	return agentPane{}, "", fmt.Errorf("no agent or repo agent named %q", target)
}

// ── inbound ───────────────────────────────────────────────────────────────

// PollTelegram starts one getUpdates long poll in the background; the scraper
// calls it every tick and it returns at once. At most one poll runs at a time.
func PollTelegram(dirs func() []string) {
	if !polling.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer polling.Store(false)
		cfg := loadConfig()
		if cfg.token == "" { // feature off: check again later, not every tick
			time.Sleep(10 * time.Second)
			return
		}
		// The file is shared by every tmux server, and Telegram allows one poller
		// per token. ponytail: replies reach the agents of whichever server wins.
		unlock, ok := lockPoll()
		if !ok {
			time.Sleep(10 * time.Second)
			return
		}
		defer unlock()
		if commandsFor.Load() != cfg.token {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if registerCommands(ctx, cfg.token) == nil {
				commandsFor.Store(cfg.token)
			}
			cancel()
		}
		if err := pollOnce(cfg, dirs); err != nil {
			time.Sleep(5 * time.Second)
		}
	}()
}

// lockPoll takes the cross-process poll lock without waiting.
func lockPoll() (func(), bool) {
	dir := apppath.ConfigDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return nil, false
	}
	f, err := os.OpenFile(filepath.Join(dir, "telegram.poll.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, false
	}
	return func() { f.Close() }, true // closing releases the lock
}

func pollOnce(cfg tgConfig, dirs func() []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), (pollSeconds+10)*time.Second)
	defer cancel()
	var ups []update
	err := tgCall(ctx, cfg.token, "getUpdates", map[string]any{
		"offset":          loadState().Offset,
		"timeout":         pollSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	}, &ups)
	if err != nil {
		return err
	}
	for _, u := range ups {
		// Offset first: a crash mid-handling must never replay a message.
		updateState(func(s *tgState) {
			if u.ID >= s.Offset {
				s.Offset = u.ID + 1
			}
		})
		handleUpdate(cfg, u, dirs)
	}
	return nil
}

func handleUpdate(cfg tgConfig, u update, dirs func() []string) {
	if m := u.Message; m != nil {
		if !cfg.allowed[m.Chat.ID] {
			ignore(m.Chat.ID)
			return
		}
		handleMessage(cfg, *m, dirs)
	}
	if cb := u.Callback; cb != nil && cb.Message != nil {
		if !cfg.allowed[cb.Message.Chat.ID] {
			ignore(cb.Message.Chat.ID)
			return
		}
		background(func() { handleCallback(cfg, *cb, dirs) })
	}
}

// background runs slow work (start, send, wait); tests replace it with a direct call.
var background = func(f func()) { go f() }

// ignore logs a non-allowlisted chat once. It is never answered.
func ignore(chat int64) {
	ignoredMu.Lock()
	defer ignoredMu.Unlock()
	if !ignored[chat] {
		ignored[chat] = true
		fmt.Fprintf(os.Stderr, "telegram: ignoring chat %d (not in @agent_telegram_chats)\n", chat)
	}
}

// handleMessage routes one allowlisted message. Order: reply to a bot message,
// @name one-off, /use and /agents, then plain text to the chat's target.
func handleMessage(cfg tgConfig, m message, dirs func() []string) {
	chat := m.Chat.ID
	text := strings.TrimSpace(m.Text)
	if text == "" {
		say(cfg, chat, "text only")
		return
	}
	if m.ReplyTo != nil {
		if ref, ok := findMessage(chat, m.ReplyTo.ID); ok {
			a, found := matchAgent(ref.Pane)
			if !found || a.PID != ref.PID {
				say(cfg, chat, "that agent is gone; message not sent")
				return
			}
			if !forwardable(text) {
				say(cfg, chat, forwardHint)
				return
			}
			background(func() { sendTo(cfg, chat, a, text) })
			return
		}
	}
	if strings.HasPrefix(text, "@") {
		name, rest, _ := strings.Cut(text[1:], " ")
		rest = strings.TrimSpace(rest)
		if !nameRe.MatchString(name) || rest == "" {
			say(cfg, chat, "usage: @name message")
			return
		}
		if !forwardable(rest) {
			say(cfg, chat, forwardHint)
			return
		}
		background(func() { deliver(cfg, dirs, chat, name, rest, false) })
		return
	}
	cmd, arg, _ := strings.Cut(text, " ")
	if at := strings.Index(cmd, "@"); at > 0 { // /use@botname
		cmd = cmd[:at]
	}
	switch cmd {
	case "/use":
		arg = strings.TrimSpace(arg)
		if !nameRe.MatchString(arg) {
			say(cfg, chat, "usage: /use <agent>")
			return
		}
		updateState(func(st *tgState) {
			if st.Targets == nil {
				st.Targets = map[string]string{}
			}
			st.Targets[strconv.FormatInt(chat, 10)] = arg
		})
		say(cfg, chat, "target: "+arg)
		return
	case "/agents":
		sendView(cfg, dirs, chat, "agents")
		return
	case "/tickets":
		sendView(cfg, dirs, chat, "tickets")
		return
	case "/start", "/help", "/status":
		sendView(cfg, dirs, chat, "home")
		return
	}
	if !forwardable(text) {
		say(cfg, chat, forwardHint)
		return
	}
	target, _ := loadState().Targets[strconv.FormatInt(chat, 10)]
	if target == "" { // nothing chosen yet: explain instead of guessing
		sendView(cfg, dirs, chat, "home")
		return
	}
	background(func() { deliver(cfg, dirs, chat, target, text, true) })
}

const forwardHint = "text starting with / or ! is not forwarded to agents"

const helpText = `Pick an agent first, then send plain text to it.
/use <agent>  choose the agent (or tap a button below)
@name text    one message to another agent
/agents       list agents
/help         this message
Reply to a bot message to answer that agent.
/tickets      open tickets`

// forwardable: "/" would run harness commands (/clear, /exit) and "!" is a
// shell escape that skips permission prompts.
func forwardable(text string) bool {
	return !strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "!")
}

// sendView sends a menu page as a new message.
func sendView(cfg tgConfig, dirs func() []string, chat int64, page string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	text, markup := view(ctx, cfg, dirs, page, strconv.FormatInt(chat, 10))
	_ = sendMsg(ctx, cfg.token, chat, text, markup, agentPane{})
}

// registerCommands fills the bot's command menu.
func registerCommands(ctx context.Context, token string) error {
	cmd := func(name, what string) map[string]string {
		return map[string]string{"command": name, "description": what}
	}
	return tgCall(ctx, token, "setMyCommands", map[string]any{"commands": []map[string]string{
		cmd("start", "Overview"), cmd("agents", "Agents and who to chat with"), cmd("tickets", "Open tickets"), cmd("help", "How to talk to agents"),
	}}, nil)
}

// deliver resolves target (starting it if needed) and sends text. With
// fallback, a target that is gone falls back to the default agent, and the
// chat is told so.
func deliver(cfg tgConfig, dirs func() []string, chat int64, target, text string, fallback bool) {
	ctx, cancel := context.WithTimeout(context.Background(), replyTimeout)
	defer cancel()
	// Held through start and send+wait: a second message to the same target
	// neither starts a second agent nor queues ahead of this one.
	unlock := lockTarget("t:" + target)
	defer unlock()
	a, dir, err := resolve(ctx, target, dirs)
	if err != nil && fallback && target != cfg.defaultAgent {
		if d, _, derr := resolve(ctx, cfg.defaultAgent, dirs); derr == nil {
			sayAbout(cfg, chat, d, target+" is gone; sent to "+d.label())
			a, err = d, nil
		}
	}
	if err != nil {
		say(cfg, chat, err.Error())
		return
	}
	if dir != "" {
		sayAbout(cfg, chat, a, "started "+a.label()+" in "+dir)
	}
	sendTo(cfg, chat, a, text)
}

// sendTo queues text to the agent (`tagent send`), waits for it to go idle and
// replies with its answer. A dialog that opens instead becomes an alert.
func sendTo(cfg tgConfig, chat int64, a agentPane, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), replyTimeout)
	defer cancel()
	unlock := lockTarget("p:" + a.ID)
	defer unlock()
	if a.Status == tmux.StatusWaiting {
		alert(ctx, cfg.token, chat, "is waiting for your answer; your message is queued", a)
	}
	out, errOut, code := runTagent(ctx, "send", a.ID, "--timeout", "10m", text)
	switch {
	case ctx.Err() != nil || code == exitTimeout:
		sayAbout(cfg, chat, a, a.label()+" is still running; the message stays queued")
		return
	case code != 0:
		sayAbout(cfg, chat, a, a.label()+": send failed: "+strings.TrimSpace(errOut))
		return
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[len(lines)-1] == string(tmux.StatusWaiting) {
		alert(ctx, cfg.token, chat, "needs your answer", a)
		return
	}
	reply(ctx, cfg.token, chat, a)
}

// reply sends the agent's last answer: the last assistant text of a claude or
// pi transcript, else the last 30 screen lines in a code block.
func reply(ctx context.Context, token string, chat int64, a agentPane) {
	body, plain := lastAnswer(ctx, a)
	for i, p := range chunks(body) {
		text := p
		if !plain {
			text = "<pre>" + text + "</pre>"
		}
		if i == 0 {
			text = "💬 <b>" + html.EscapeString(trunc(a.label(), nameRunes)) + "</b>\n" + rule + "\n" + text
		}
		_ = sendMsg(ctx, token, chat, text, nil, a)
	}
}

func lastAnswer(ctx context.Context, a agentPane) (string, bool) {
	if (tmux.IsClaude(a.Agent) || a.Agent == "pi") && a.Session != "" {
		if it, err := transcript.Latest(ctx, a.Agent, a.Session); err == nil && it.Text != "" {
			return it.Text, true
		}
	}
	screen, _ := tmux.CaptureTail(a.ID, 30)
	return strings.TrimRight(screen, "\n "), false
}

// split HTML-escapes s and cuts it into pieces of at most maxUnits UTF-16
// units, so escaping never pushes a message past Telegram's limit.
func split(s string) []string {
	var out []string
	var cur strings.Builder
	n := 0
	for _, r := range s {
		e := html.EscapeString(string(r))
		u := utf16Units(e)
		if n+u > maxUnits {
			out = append(out, cur.String())
			cur.Reset()
			n = 0
		}
		cur.WriteString(e)
		n += u
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func utf16Units(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// chunks returns the escaped pieces of s; past maxChunks it keeps the end.
func chunks(s string) []string {
	out := split(s)
	if len(out) == 0 {
		return []string{"(no output)"}
	}
	if len(out) > maxChunks {
		out = out[len(out)-maxChunks:]
		out[0] = "…" + out[0]
	}
	return out
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// lockTarget serialises work on one target: no two starts, no interleaved keystrokes.
func lockTarget(key string) func() {
	targetMu.Lock()
	m := targetMux[key]
	if m == nil {
		m = &sync.Mutex{}
		targetMux[key] = m
	}
	targetMu.Unlock()
	m.Lock()
	return m.Unlock
}

// handleCallback: "a|<pane>|<key>|<hash>" answers a dialog if it is still the
// one that was alerted; "u|<name>" sets the chat's target ("p|<name>" also
// redraws the agent list it was tapped in); "n|<page>" shows a
// menu page in place; "s|<pane>" sends the pane's screen.
func handleCallback(cfg tgConfig, cb callback, dirs func() []string) {
	chat := cb.Message.Chat.ID
	ctx, cancel := context.WithTimeout(context.Background(), replyTimeout)
	defer cancel()
	answer := func(text string) {
		_ = tgCall(ctx, cfg.token, "answerCallbackQuery", map[string]any{"callback_query_id": cb.ID, "text": text}, nil)
	}
	parts := strings.Split(cb.Data, "|")
	// buttons replaces the tapped message's keyboard with one line saying what happened.
	outcome := func(text string) {
		_ = tgCall(ctx, cfg.token, "editMessageReplyMarkup", map[string]any{"chat_id": chat, "message_id": cb.Message.ID,
			"reply_markup": keyboard([]button{btn(trunc(text, 60), "x")})}, nil)
	}
	switch {
	case len(parts) == 2 && parts[0] == "n" && (parts[1] == "home" || parts[1] == "agents" || parts[1] == "tickets"):
		text, markup := view(ctx, cfg, dirs, parts[1], strconv.FormatInt(chat, 10))
		_ = tgCall(ctx, cfg.token, "editMessageText", map[string]any{"chat_id": chat, "message_id": cb.Message.ID,
			"text": text, "parse_mode": "HTML", "reply_markup": markup}, nil)
		answer("")
	case len(parts) == 2 && parts[0] == "s" && paneRe.MatchString(parts[1]):
		a, ok := matchAgent(parts[1])
		if !ok {
			answer("that agent is gone")
			return
		}
		screen, _ := tmux.CaptureTail(a.ID, 30)
		text := card("👀", a.label()+" · "+a.ID, nil, "<pre>"+esc(strings.TrimRight(screen, "\n "), bodyUnits)+"</pre>")
		_ = sendMsg(ctx, cfg.token, chat, text, keyboard([]button{btn("💬 Chat", "u|"+a.target())}), a)
		answer("")
	case len(parts) == 4 && parts[0] == "a" && paneRe.MatchString(parts[1]) && keyRe.MatchString(parts[2]):
		pane, key, hash := parts[1], parts[2], parts[3]
		a, ok := matchAgent(pane)
		opts := dialogOpts(ctx, pane)
		if !ok || a.Status != tmux.StatusWaiting || dialogHash(pane, opts) != hash {
			answer("dialog changed")
			outcome("⌛ Dialog closed")
			releaseAlert(chat, pane)
			if ok && a.Status == tmux.StatusWaiting {
				alert(ctx, cfg.token, chat, "is still waiting", a)
			}
			return
		}
		if _, _, code := runTagent(ctx, "answer", pane, key); code != 0 {
			answer("failed")
			return
		}
		answer("sent " + key)
		label := key
		for _, o := range opts {
			if o.Key == key {
				label += "  " + o.Label
			}
		}
		outcome("✅ Answered: " + label)
		releaseAlert(chat, pane)
	case len(parts) == 2 && (parts[0] == "u" || parts[0] == "p") && nameRe.MatchString(parts[1]):
		updateState(func(st *tgState) {
			if st.Targets == nil {
				st.Targets = map[string]string{}
			}
			st.Targets[strconv.FormatInt(chat, 10)] = parts[1]
		})
		answer("target: " + parts[1])
		if parts[0] == "p" { // tapped in the list: redraw it with the new mark
			text, markup := view(ctx, cfg, dirs, "agents", strconv.FormatInt(chat, 10))
			_ = tgCall(ctx, cfg.token, "editMessageText", map[string]any{"chat_id": chat, "message_id": cb.Message.ID,
				"text": text, "parse_mode": "HTML", "reply_markup": markup}, nil)
		}
	default:
		answer("")
	}
}

// execTagent runs this binary's own subcommand. TMUX_PANE is cleared so the
// command is not signed as coming from the scraper's pane.
func execTagent(ctx context.Context, args ...string) (string, string, int) {
	exe, err := os.Executable()
	if err != nil {
		return "", err.Error(), -1
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "TMUX_PANE=")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	code := 0
	if err := cmd.Run(); err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	return out.String(), errb.String(), code
}
