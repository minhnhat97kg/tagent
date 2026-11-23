// The sidebar's bubbletea loop: Model/Update, messages, and building the
// plain row list from tmux.QuerySessions(). View() lives in view.go, and
// goroutine/signal startup lives in run.go — split by responsibility, same
// package.
package sidebar

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tagent/internal/clock"
	"tagent/internal/git"
	"tagent/internal/hook"
	"tagent/internal/metrics"
	"tagent/internal/roles"
	"tagent/internal/scrape"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
	"tagent/internal/worktree"
)

const (
	// refreshInterval is the cycle for rebuilding the list from tmux.QuerySessions().
	refreshInterval = 3 * time.Second
	// animInterval is the cycle for advancing the spinner frame, and also the
	// debounce cadence for writing global options (cursor/repo filter) that
	// changed locally.
	animInterval = 200 * time.Millisecond
	// footerHeight is the number of key-hint lines at the bottom of the pane
	// (renderKeyHints, view.go) — also drawn outside the viewport, for the same
	// reason as filterBarHeight.
	footerHeight = 1
)

// Row is one line in the list: either a repo group header or an agent pane.
// Icon/color are NOT precomputed here — renderRows looks them up in
// theme.go/Icons at draw time (Status/Agent is raw data, sufficient for the
// lookup).
type Row struct {
	BoardTarget string
	IsHeader    bool
	// Repo agent definitions are display-only rows below running panes.
	IsRepoAgent   bool
	RepoAgentName string
	// Header is the displayed repo group name, meaningful only when
	// IsHeader. This IS the group by git repo root (group.go/GroupPanesByRepo),
	// no longer the tmux session name as in version 3a.
	Header string
	// HeaderFocus is true when this group has a pane that is both active and
	// in the active window (RepoGroup.HasFocus) — used to color the header
	// with Theme.Accent.
	HeaderFocus bool

	// Status/Attention/Agent/Base/PaneID are meaningful only when !IsHeader.
	Status    tmux.PaneStatus
	Attention bool
	Agent     tmux.AgentType
	Base      string
	PaneID    string
	// Ports are the TCP ports currently listening in this pane's process tree
	// (scanned every 10 seconds by a background worker — workers.go), sorted
	// ascending. Empty means no ports (or the first scan hasn't finished yet).
	Ports []int
	// Branch is the pane's git worktree branch name (tmux.PaneInfo.Worktree.Branch,
	// i.e. @pane_worktree_branch — written by the AGENT's own hook, NOT the
	// spawn marker @agent-sidebar-spawned-branch), used for display in the
	// delete-confirmation popup — NOT shown on the row itself (part 3 does
	// not draw a branch label on the pane row, out of scope for task 7).
	//
	// KNOWN LIMITATION (found during manual verification, see task 7 report):
	// this field is empty until the REAL agent has actually run in that pane
	// and its SessionStart hook has reported the "worktree" field — during
	// the first few seconds after spawn (before the agent manages to start
	// up), the delete-confirmation popup shows the paneID instead of the
	// branch name (the fallback right below). This does NOT affect the
	// correctness of the delete itself: worktree.Remove re-reads the real
	// spawn marker from tmux at execution time, independent of this field —
	// this is purely display text. Could be fixed by reading the spawn
	// marker (worktree.ParseSpawnMarkers) via a tea.Cmd right when the popup
	// opens instead of reading the already-available PaneInfo, in exchange
	// for one asynchronous round trip before the popup appears — left for
	// later if needed.
	Branch string
	// SidebarSpawned is true when the window containing this pane was
	// created by the sidebar's spawn flow (tmux.PaneInfo.SidebarSpawned) —
	// the ONLY condition under which the "x" key (delete worktree) has any
	// effect on this row.
	SidebarSpawned bool

	// NeedsConfirm/JustDone: two view-only overlays from part 6 (see the
	// StatusConfirm/StatusDone comment in theme.go) — they do NOT change the
	// real Status, only the displayed icon/color (iconForRow/colorForRow,
	// view.go).
	NeedsConfirm bool
	JustDone     bool
	// Dirty is true when this pane's most recent git fetch (bottom summary
	// panel, detail.go — runs ONLY for the pane under the cursor, not a
	// background worker for every pane; see the comment in workers.go about
	// dropping the continuous git worker entirely) found uncommitted changed
	// files. "Last known": a pane the cursor has never visited in this
	// session always has Dirty == false, not because it's actually clean —
	// a deliberate trade-off to avoid spawning an extra git subprocess for
	// EVERY pane on every tick, per the decision already recorded in
	// workers.go.
	Dirty bool
	// AheadBehind is a short "↑N ↓M" badge from the pane's last-known git
	// fetch (same last-known/cursor-only caching trade-off as Dirty above —
	// see rowExtras.AheadBehind). Empty when never fetched or no upstream.
	AheadBehind string
	// Elapsed is the elapsed time of the current run (elapsedLabel), empty
	// when the pane has no run currently being timed (StartedAt == 0).
	Elapsed string
	// Pinned is true when the user pinned this pane to the top of the list
	// (the "p" key) — process-memory only, lost on sidebar restart (see
	// Model.pinnedPanes).
	Pinned     bool
	SizedByWeb bool
	// Selected is true when this pane is part of the current multi-select
	// (the "v" key), driving bulk delete on "x" — see Model.selectedPanes.
	Selected bool

	// IsSubagentChild is true when this row is a subagent branch drawn
	// nested right under its parent pane row, directly in the root list —
	// NO LONGER inside the detail popup (real user feedback: "show the tree
	// at the root", "shouldn't need to press i to see it"). SubagentLabel is
	// the ALREADY-BUILT branch text (including the ├─/└─ connectors — see
	// treeBranch, view.go), meaningful only when IsSubagentChild. Rows of
	// this kind are NOT counted/landed on by the cursor (see paneRowCount/
	// paneRowAt/rowIndexOfCursor) — exactly like how IsHeader is skipped.
	IsSubagentChild bool
	SubagentLabel   string

	// IsInfoChild is true when this row is the "what it's doing" line drawn
	// nested RIGHT under the agent's name (real user feedback: "the current
	// thinking/process should be shown right under the agent's name").
	// InfoLabel is the user's prompt that has NOT yet become a response
	// (tmux.PaneInfo.Prompt when !PromptIsResponse — written by the hook from
	// OnUserPromptSubmit, part 2), meaningful only when Status==Running: a
	// pane that isn't running has no "current" thing to report — see
	// processLine.
	IsInfoChild bool
	InfoLabel   string
	// InfoAction marks the info line as "the user must act" (ActionLine):
	// drawn in the confirm color instead of muted.
	InfoAction bool
	// InfoKey is set on an answer line of a waiting agent's prompt: the key
	// that picks it.
	InfoKey string
	// GitBranch is the branch shown after the agent's name, on its row.
	GitBranch string
}

// rowExtras bundles data that does NOT come from tmux.QuerySessions() but
// needs to be mixed in when building rows: listening ports (portscan, 10s
// worker) and the session name assigned by the agent (session, 10s worker —
// tmux.PaneInfo.SessionName is left empty at the query layer, see that
// field's comment). Bundled instead of adding two separate parameters so
// buildRows's signature doesn't bloat every time another secondary data
// source is added.
type rowExtras struct {
	Ports        map[string][]int
	SessionNames map[string]string
	// Dirty: cache of "most recent git fetch" keyed by pane id — see the
	// Row.Dirty and Model.dirtyByPane comments.
	Dirty map[string]bool
	// AheadBehind: cache of "most recent git fetch" ahead/behind badge keyed
	// by pane id — same last-known/cursor-only trade-off as Dirty, see
	// Model.aheadBehindByPane.
	AheadBehind map[string]string
	// Pinned: the set of pane ids the user pinned to the top of the list —
	// see Row.Pinned and Model.pinnedPanes.
	Pinned map[string]bool
	// Selected: the multi-select set for bulk delete — see Row.Selected and
	// Model.selectedPanes.
	Selected map[string]bool
	// FilterQuery: a case-insensitive substring filter over agent/name/status
	// (the "/" key) — empty means no filter. Applied in buildRows BEFORE
	// grouping, so a repo group with no matching pane doesn't show up at all.
	FilterQuery string
	// CurrentWindow is the window the sidebar sits in — its header is
	// highlighted.
	CurrentWindow string
	// Dialogs are waiting agents' prompts, shown with their answers.
	Dialogs map[string]scrape.Dialog
	// DialogOpen: panes whose answers are expanded (Enter on the "!" row).
	DialogOpen map[string]bool
}

// buildRows builds the flat row list: group by git repo (group.go) → filter
// by repo (popup.go) → rows. ALWAYS shows every pane status (real user
// feedback: "remove the agent filter tab, always show the list without
// filtering" — the earlier version had an additional Running/Waiting/...
// status filter that has been fully removed).
// processLine returns a pane's currently running prompt — only when the
// pane is Running AND the prompt is still the user's REQUEST, not yet
// overwritten by OnStop into a response (tmux.PaneInfo.Prompt/PromptIsResponse
// — written by the hook from OnUserPromptSubmit/OnStop, part 2). This is the
// most recent "what it's doing" signal available without adding any new
// option/hook: OnUserPromptSubmit writes Prompt at the same time it sets
// Status=Running, and it only changes back (into a response, Status other
// than Running) at OnStop — for the entire run, Prompt IS what the agent is
// doing. Empty when the pane isn't running — there's no "current" thing to
// report.
// ActionLine says what the user has to do for a pane that's blocked on them,
// shown inline under its row. Empty when the agent doesn't need anyone.
func ActionLine(p tmux.PaneInfo) string {
	switch p.Status {
	case tmux.StatusError:
		if p.WaitReason != "" {
			return "failed: " + p.WaitReason
		}
		return "failed — check the pane"
	case tmux.StatusWaiting:
	default:
		// Watchdog flags (internal/scrape/watchdog.go) on a working agent.
		switch {
		case p.Conflict != "":
			return "editing " + p.Conflict
		case p.Stuck != "" && p.Status == tmux.StatusRunning:
			return "stuck? no activity for " + p.Stuck + "m"
		}
		return ""
	}
	switch {
	case hook.IsPermissionWaitReason(p.WaitReason) && p.WaitReason != "elicitation_dialog":
		return "needs approval"
	case p.WaitReason == "elicitation_dialog":
		return "has a question for you"
	case p.WaitReason == "idle_prompt":
		return "waiting for your input"
	}
	return "needs your input"
}

// elapsedLabel builds the elapsed time of the current run, kept short so it
// fits the last column of the row (herdr lesson 9.1: a secondary token, tight
// budget) — seconds under 1 minute, minutes+seconds under 1 hour, "1h+" from
// 1 hour onward (no need for finer precision here — an agent running past 1
// hour is already unusual enough to be worth flagging, no need to know
// exactly how long). Empty when StartedAt isn't set yet (pane not running —
// ClearRunState already unset @pane_started_at on Stop, see handlers.go).
func elapsedLabel(p tmux.PaneInfo) string {
	return elapsedLabelFromStartedAt(p.StartedAt)
}

// elapsedLabelFromStartedAt is the shared logic used by elapsedLabel (single
// pane) and groupElapsedLabel (whole repo group) — factored out so the
// seconds/minutes/hours formatting rule isn't duplicated.
func elapsedLabelFromStartedAt(startedAt int64) string {
	if startedAt <= 0 {
		return ""
	}
	secs := clock.NowUnix() - startedAt
	switch {
	case secs < 0:
		return ""
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm%ds", secs/60, secs%60)
	default:
		return "1h+"
	}
}

// aheadBehindLabel builds the short "↑N ↓M" badge shown on a pane row from
// its git.AheadBehind (nil when the branch has no upstream, or the fetch
// hasn't run yet — both render as empty, no badge). Ahead/behind are shown
// only when non-zero, so a branch that's merely behind doesn't show a
// pointless "↑0".
func aheadBehindLabel(ab *git.AheadBehind) string {
	if ab == nil || (ab.Ahead == 0 && ab.Behind == 0) {
		return ""
	}
	var parts []string
	if ab.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", ab.Ahead))
	}
	if ab.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", ab.Behind))
	}
	return strings.Join(parts, " ")
}

// paneUnitRows builds exactly ONE block of rows for a single pane: the pane
// row, then (if present) the "what it's doing" line and the subagent tree
// nested right below it — SEPARATED from buildRows so it can be called from
// both the priority branch and the group-by-repo branch without duplicating
// code.
func paneUnitRows(p tmux.PaneInfo, extras rowExtras) []Row {
	rows := []Row{{
		Status:         p.Status,
		Attention:      p.Attention,
		Agent:          p.Agent,
		Base:           computeBaseName(p, extras),
		BoardTarget:    p.BoardTarget,
		PaneID:         p.PaneID,
		Ports:          extras.Ports[p.PaneID],
		Branch:         p.Worktree.Branch,
		SidebarSpawned: p.SidebarSpawned,
		NeedsConfirm:   needsConfirm(p.Status, p.WaitReason),
		JustDone:       p.Unseen,
		Dirty:          extras.Dirty[p.PaneID],
		AheadBehind:    extras.AheadBehind[p.PaneID],
		Elapsed:        elapsedLabel(p),
		Pinned:         extras.Pinned[p.PaneID],
		Selected:       extras.Selected[p.PaneID],
		GitBranch:      PaneBranch(p),
	}}
	// The line nested under the agent's name: what the user must do
	// (ActionLine) when the agent is blocked on them — nothing otherwise,
	// matching the expanded view (user feedback: "keep the list compact").
	if d, ok := extras.Dialogs[p.PaneID]; ok {
		// Approval inbox: the question and its answers right in the list;
		// with the cursor on this agent, the option's key answers it.
		q := stripEmoji(d.Question)
		if !extras.DialogOpen[p.PaneID] {
			// Collapsed: just the "!" — Enter reveals the answers.
			rows = append(rows, Row{IsInfoChild: true, InfoLabel: q + " · enter", InfoAction: true})
		} else {
			rows = append(rows, Row{IsInfoChild: true, InfoLabel: q, InfoAction: true})
			for _, o := range d.Options {
				rows = append(rows, Row{IsInfoChild: true, InfoLabel: stripEmoji(o.Label), InfoKey: o.Key})
			}
		}
	} else if action := ActionLine(p); action != "" {
		rows = append(rows, Row{IsInfoChild: true, InfoLabel: action, InfoAction: true})
	}
	// The subagent branch nested RIGHT after that — "show the tree at the
	// root", not inside the detail popup (real user feedback, part 6). Empty
	// when the pane has no subagents (subagentRows, view.go).
	rows = append(rows, subagentRows(p.Subagents)...)
	return rows
}

// computeBaseName is the display name shown on a pane row: the agent-given
// session name (tmux.PaneInfo.SessionID → extras.SessionNames) if known,
// else the basename of the pane's path. Factored out of paneUnitRows so
// matchesFilterQuery can match against the SAME name the user sees, not the
// raw path.
func computeBaseName(p tmux.PaneInfo, extras rowExtras) string {
	if p.Name != "" {
		return p.Name
	}
	base := filepath.Base(p.Path)
	if p.SessionID != "" {
		if name, ok := extras.SessionNames[p.SessionID]; ok && name != "" {
			base = name
		}
	}
	return base
}

// matchesFilterQuery reports whether a pane matches the "/" text filter — a
// case-insensitive substring match against the displayed name, the agent
// name, or the status string. query is assumed already lowercased/trimmed
// by the caller (buildRows).
func matchesFilterQuery(p tmux.PaneInfo, extras rowExtras, query string) bool {
	if strings.Contains(strings.ToLower(computeBaseName(p, extras)), query) {
		return true
	}
	if strings.Contains(strings.ToLower(string(p.Agent)), query) {
		return true
	}
	return strings.Contains(strings.ToLower(string(p.Status)), query)
}

// buildRows builds the agent list in tmux order — session, window, pane,
// exactly like the status bar (user feedback: "sort by window position") —
// under one small header per window that has agents ("3 window"; prefixed
// with the session name when there are several sessions), so it's clear
// which window each pane is in. The sidebar's own window is highlighted. A
// pane that needs the user stays in place and carries an inline action line
// (ActionLine). The repo filter (r) and text filter (/) still apply.
func buildRows(sessions []tmux.SessionInfo, repoFilter string, extras rowExtras) []Row {
	// Repo of each pane, keyed by path (a pane's repo depends only on it).
	repoOf := map[string]string{}
	if repoFilter != RepoFilterAll {
		for _, g := range GroupPanesByRepo(sessions) {
			for _, p := range g.Panes {
				repoOf[p.Path] = g.Name
			}
		}
	}
	query := strings.ToLower(strings.TrimSpace(extras.FilterQuery))
	var rows []Row
	for _, s := range sessions {
		for _, w := range s.Windows {
			var unit []Row
			for _, p := range w.Panes {
				if repoFilter != RepoFilterAll && repoOf[p.Path] != repoFilter {
					continue
				}
				if query != "" && !matchesFilterQuery(p, extras, query) {
					continue
				}
				unit = append(unit, paneUnitRows(p, extras)...)
			}
			if len(unit) == 0 {
				continue
			}
			title := w.WindowIndex + " " + w.WindowName
			if len(sessions) > 1 {
				title = s.SessionName + ":" + title
			}
			current := extras.CurrentWindow != "" && w.WindowID == extras.CurrentWindow
			rows = append(rows, Row{IsHeader: true, Header: title, HeaderFocus: current})
			rows = append(rows, unit...)
		}
	}
	return rows
}

// focusedPane is the agent pane the user is looking at in window (its active
// pane), "" when that's not an agent pane (e.g. the sidebar itself).
func focusedPane(sessions []tmux.SessionInfo, window string) string {
	for _, s := range sessions {
		for _, w := range s.Windows {
			if w.WindowID != window {
				continue
			}
			for _, p := range w.Panes {
				if p.PaneActive {
					return p.PaneID
				}
			}
		}
	}
	return ""
}

// paneIndexOf is the cursor index of paneID among selectable rows.
func paneIndexOf(rows []Row, paneID string) (int, bool) {
	i := 0
	for _, r := range rows {
		if !isSelectablePaneRow(r) {
			continue
		}
		if r.PaneID == paneID {
			return i, true
		}
		i++
	}
	return 0, false
}

// ─── Cursor ─────────────────────────────────────────────────────────────────
//
// The cursor is an INDEX in the list of pane rows (headers excluded), like
// the reference implementation (state/global.rs `selected_pane_row`), but
// applyRows re-resolves it by pane id on every rebuild: a pane added,
// removed or regrouped above it no longer moves the highlight to another
// pane (the reference only clamps `>= len`).

// isSelectablePaneRow reports whether row r is an actual pane row that the
// cursor can select — headers, subagent branches (IsSubagentChild), and the
// "what it's doing" line (IsInfoChild) are all excluded, for the same reason
// and in the same place, so the three kinds of "not a pane" row never drift
// out of sync across the three functions below.
func isSelectablePaneRow(r Row) bool {
	return !r.IsHeader && !r.IsSubagentChild && !r.IsInfoChild && !r.IsRepoAgent
}

// paneRowCount counts the selectable pane rows — this is the "length" the
// cursor moves/clamps against, corresponding to `pane_row_targets` in the
// reference implementation.
func paneRowCount(rows []Row) int {
	n := 0
	for _, r := range rows {
		if isSelectablePaneRow(r) {
			n++
		}
	}
	return n
}

// paneRowAt returns the FULL Row of the `cursor`-th pane row (counting skips
// headers and subagent branches). ok = false when cursor is out of range or
// the list is empty.
func paneRowAt(rows []Row, cursor int) (Row, bool) {
	i := 0
	for _, r := range rows {
		if !isSelectablePaneRow(r) {
			continue
		}
		if i == cursor {
			return r, true
		}
		i++
	}
	return Row{}, false
}

// paneIDAt returns the pane id of the `cursor`-th pane row — see paneRowAt.
func paneIDAt(rows []Row, cursor int) (string, bool) {
	row, ok := paneRowAt(rows, cursor)
	return row.PaneID, ok
}

// rowIndexOfCursor returns the INDEX WITHIN rows (headers included) of the
// `cursor`-th pane row — this is the "line" that scroll.go needs to keep the
// cursor within the viewport. ok = false when no pane row matches (empty
// list or cursor out of range); in that case there's nothing to scroll to.
func rowIndexOfCursor(rows []Row, cursor int) (int, bool) {
	i := 0
	for idx, r := range rows {
		if !isSelectablePaneRow(r) {
			continue
		}
		if i == cursor {
			return idx, true
		}
		i++
	}
	return 0, false
}

// clampCursor clamps the cursor into a valid range after the list changes.
// Like the reference implementation's `rebuild_row_targets`: it ONLY pulls
// back to `count-1` when the cursor has gone past the end of the list; an
// empty list leaves the old value unchanged (nothing to clamp into, and the
// old value is still useful if the list grows back). There's an added
// lower-bound clamp (`cursor < 0`) that the reference implementation doesn't
// need because Rust's `usize` can't go negative — Go uses int, so it's
// clamped here just to be safe.
func clampCursor(cursor, count int) int {
	if count == 0 {
		return cursor
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= count {
		return count - 1
	}
	return cursor
}

// moveCursor adds delta to cursor, clamping at the edges — no wraparound.
// Returns the new cursor and whether it actually moved (like the reference
// implementation's `move_pane_selection`: false when the list is empty or
// already at the edge, never snapping the cursor back to 0).
func moveCursor(cursor, delta, count int) (int, bool) {
	if count == 0 {
		return cursor, false
	}
	next := cursor + delta
	if next < 0 || next >= count {
		return cursor, false
	}
	return next, true
}

// RefreshMsg carries the session list that just finished being queried from
// tmux — the result of a tmux.QuerySessions() call completed inside a
// tea.Cmd, whether triggered by the periodic tick or by SIGUSR1.
type RefreshMsg struct {
	SizeOwners map[string]bool
	Sessions   []tmux.SessionInfo
	Theme      Theme
	Icons      Icons
	Avatar     AvatarConfig
	// Dialogs: the prompt on screen of each waiting agent (approval inbox).
	Dialogs map[string]scrape.Dialog
}

// AnimTickMsg advances the animation frame, used for the spinner/colors in
// part 3b, and is also the debounce cadence for any locally changed global
// option write (cursor/filter/repo filter).
type AnimTickMsg struct{}

// refreshTickMsg is the periodic tick that triggers one rebuild of the list,
// and schedules itself again for the next tick.
type refreshTickMsg struct{}

// refreshRequestMsg is a request to rebuild the list IMMEDIATELY, without
// waiting for the periodic tick — SIGUSR1 sends this kind via p.Send(). Unlike
// refreshTickMsg: after handling it, it does NOT schedule a new tick, so it
// doesn't stack on top of the regular tick cycle already running.
type refreshRequestMsg struct{}

// cursorLoadedMsg carries the @sidebar_cursor value read from tmux at
// startup — so the cursor position survives reopening the sidebar. This
// value COMES FROM tmux (possibly written by another sidebar in a different
// window), so once applied it does NOT mark cursorDirty — otherwise it would
// write the just-read value right back.
type cursorLoadedMsg struct {
	Value int
}

// cursorSavedMsg is the result of one @sidebar_cursor write. Success = false
// (tmux.SetOptionResult failed) means cursorDirty MUST be left set so the
// next AnimTick retries the write — pays off debt 3a Minor 2
// (global.rs:86-101: flush_pending_cursor_save only clears the flag when
// save_cursor() succeeds).
type cursorSavedMsg struct {
	Success bool
}

// repoFilterLoadedMsg carries the @sidebar_repo_filter value read at
// startup — same reasoning as cursorLoadedMsg: once applied it does not mark
// dirty.
type repoFilterLoadedMsg struct{ Value string }

// repoFilterSavedMsg is the corresponding write result — same
// retry-on-failure mechanism as cursorSavedMsg.
type repoFilterSavedMsg struct{ Success bool }

// Model is the sidebar's TUI state.
type Model struct {
	agentFocus       int
	definitionCursor int
	workflowFocus    bool
	workflowCursor   int
	workflowDelete   string
	taskForm         string
	taskInput        string
	taskStep         int
	promptPane       string // target of the "send prompt" form (i)

	taskNotice string

	output io.Writer
	// dialogs: prompts of waiting agents by pane id (approval inbox).
	dialogs map[string]scrape.Dialog
	// standalone: running outside tmux (standalone.go), with no window of
	// its own; panels follow the selected agent instead.
	standalone bool
	repoDir    string
	// helpOpen shows the shortcut list (`?`); helpScroll is its first line.
	helpAll    bool
	helpOpen   bool
	helpScroll int
	// dialogOpen: panes whose approval answers are shown.
	dialogOpen map[string]bool

	// avatar is the avatar config (avatar.go), refreshed with the theme.
	avatar AvatarConfig

	// windowID is the window this sidebar sits in (current-window header,
	// focus following); lastFocus is the agent pane last seen focused there.
	windowID  string
	lastFocus string
	// digestKicked is the agents' state this sidebar last asked a line for
	// (digestKick, digest.go).
	digestKicked   string
	digestKickedAt time.Time

	// notes/notesScope: the shared-context board of the project of the pane
	// under the cursor, shown below the list when there's room (view.go).
	notes          []tasks.Task
	attachments    []tasks.TicketAttachment // selected ticket only; refreshed by syncAttachments
	attachmentsKey string
	notesScope     string
	dashboard      bool
	metricState    metrics.State
	metricScope    string
	metricBusy     bool
	metricTick     int
	taskFocus      bool
	boardMode      bool
	boardDrawer    bool
	boardWorkflows []workflow.Workflow // listed under the board
	boardFlow      bool                // workflow view: the configured statuses and who acts in each
	detailScroll   int
	taskTitle      bool // Tasks title is focused (Enter opens Board).
	boardDir       string
	flowWorkflow   string
	boardHistory   []tasks.Event
	boardConfig    tasks.BoardConfig
	taskCursor     int
	taskDelete     bool
	chartDays      int

	// tmuxPane is the sidebar pane's own $TMUX_PANE — for future operations
	// that need to distinguish "my own pane" (task 3, selecting a pane other
	// than itself).
	tmuxPane string

	// sessions is the most recent raw result of tmux.QuerySessions() — kept
	// around so changing the repo filter can rebuild rows IMMEDIATELY,
	// without waiting for the next 1s refresh tick (buildRows needs the
	// original Path, which is lost once flattened into rows).
	sessions []tmux.SessionInfo
	// repoNames: cache of allRepoNames, refreshed in applyRows.
	repoNames []string

	rows      []Row
	viewport  viewport.Model
	scroll    ScrollState
	width     int
	height    int
	animFrame int

	// cursor is the index of the currently selected pane row (headers
	// excluded) — see the "Cursor" comment above buildRows.
	cursor int
	// cursorDirty marks that the cursor has changed and has NOT YET been
	// SUCCESSFULLY written to @sidebar_cursor — a debounce mechanism: holding
	// down j/k continuously produces only ONE tmux write at the next
	// AnimTick. Only cleared when cursorSavedMsg reports success (see
	// cursorSavedMsg) — a failed write stays dirty and is retried next tick.
	// Ports the debounce+retry idea from state/global.rs.
	cursorDirty bool
	// cursorSaveInFlight is true while a saveCursorCmd is waiting for tmux to
	// respond — blocks the next AnimTick from firing another write on top of
	// it before knowing whether the previous one succeeded or failed.
	cursorSaveInFlight bool

	// repoFilter is the name of the repo currently being filtered, or
	// RepoFilterAll. Changed via the popup (Enter confirms the selection) —
	// debounced write to @sidebar_repo_filter exactly like cursor/filter.
	repoFilter             string
	repoFilterDirty        bool
	repoFilterSaveInFlight bool

	// popup is the "at most one popup open" state machine (popup.go).
	popup Popup

	theme Theme
	icons Icons

	// dirtyByPane is the cache of "most recent git fetch" keyed by pane id —
	// ONLY updated for the pane currently under the cursor (applyDetailMsg);
	// keyed by id so other panes' values stay at their "last known" state
	// when the cursor moves away, and are NOT cleared. Deliberately not a
	// background worker scanning every pane (see the comment on Row.Dirty
	// and workers.go) — this is why the Dirty badge can be stale/missing for
	// a pane the cursor has never visited in this session.
	dirtyByPane map[string]bool

	// portsByPane is the most recent result of the background port-scan
	// worker (workers.go, every 10 seconds), keyed by pane id — recomputed
	// every time a portScanMsg arrives (computePortsByPane, workers.go) from
	// the process snapshot + the current m.sessions.
	portsByPane map[string][]int
	// sessionNames is the most recent result of the background session-name
	// scan worker (workers.go, every 10 seconds), keyed by session id.
	sessionNames map[string]string

	// ─── Worktree-delete confirmation popup (drives worktree.Remove) ────
	removeConfirmOpen   bool
	removeConfirmPaneID string
	removeConfirmBranch string
	removeConfirmError  string
	// removeConfirmPaneIDs is set instead of removeConfirmPaneID/Branch when
	// the popup was opened from a multi-select bulk delete (the "v"/"x"
	// keys) — non-empty means "bulk mode", applying the chosen RemoveMode to
	// every id in this list instead of a single pane.
	removeConfirmPaneIDs []string

	// selectedPanes is the multi-select set (the "v" key) driving bulk
	// delete — process-memory only. Cleared whenever the bulk confirm popup
	// closes, by cancel or by confirm.
	selectedPanes map[string]bool

	// lastRemoved is the most recently removed worktree+branch (captured by
	// removeCmd right before deletion, detail.go), consumed by the "u" key
	// (undoRemoveCmd) — single-slot, process-memory only. A later removal
	// overwrites it; there is no undo history beyond the last one.
	lastRemoved *removedWorktree

	// aheadBehindByPane is the cache of "most recent git fetch" ahead/behind
	// badge keyed by pane id — same last-known/cursor-only trade-off as
	// dirtyByPane above.
	aheadBehindByPane map[string]string

	// pinnedPanes is the set of pane ids pinned to the top of the list (the
	// "p" key) — process-memory only, lost on sidebar restart. A pane that
	// disappears (agent exits) simply falls out of the next rebuilt row list;
	// its stale id staying in this set is harmless (never looked up again).
	pinnedPanes map[string]bool
	sizeOwners  map[string]bool

	// filterQuery/filterEditing: the "/" text filter (name/agent/status
	// substring, case-insensitive). filterEditing is true while the user is
	// actively typing (all non-control keys are consumed as filter text);
	// pressing Enter or Esc while editing exits edit mode. Esc while NOT
	// editing clears the filter entirely. Process-memory only.
	filterQuery   string
	filterEditing bool
}

// NewModel builds the initial Model, with an empty list until the first
// RefreshMsg arrives. repoFilter defaults to "all" until repoFilterLoadedMsg
// arrives; theme/icons default until the first RefreshMsg loads from tmux
// (see refreshCmd).
func NewModel(tmuxPane string) Model {
	return Model{
		output:     os.Stdout,
		tmuxPane:   tmuxPane,
		viewport:   viewport.New(0, 0),
		repoFilter: RepoFilterAll,
		theme:      DefaultTheme(),
		icons:      DefaultIcons(),
		chartDays:  7,
	}
}

// EnableDashboard uses the same responsive view in both sidebar and zoomed modes.
func (m *Model) EnableDashboard() { m.dashboard = true }

// Init fires the command to build the list for the first time, starts both
// tick loops, and reads @sidebar_cursor/@sidebar_repo_filter so state
// survives reopening the sidebar.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		refreshCmd(), tickRefreshCmd(), tickAnimCmd(),
		loadCursorCmd(), loadRepoFilterCmd(),
	)
}

// loadCursorCmd reads @sidebar_cursor (a subprocess) inside a tea.Cmd,
// without blocking Update. A value that fails to parse (never written, or
// garbage) is treated as 0.
func loadCursorCmd() tea.Cmd {
	return func() tea.Msg {
		raw, ok := tmux.GetOption(tmux.SidebarCursor)
		if !ok {
			return cursorLoadedMsg{Value: 0}
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return cursorLoadedMsg{Value: 0}
		}
		return cursorLoadedMsg{Value: n}
	}
}

// loadRepoFilterCmd reads @sidebar_repo_filter at startup. An unrecognized or
// empty value falls back to RepoFilterAll via ParseRepoFilter (popup.go).
func loadRepoFilterCmd() tea.Cmd {
	return func() tea.Msg {
		raw, _ := tmux.GetOption(tmux.SidebarRepoFilter)
		return repoFilterLoadedMsg{Value: ParseRepoFilter(raw)}
	}
}

// saveCursorCmd writes the cursor to @sidebar_cursor (a subprocess) inside a
// tea.Cmd, returning cursorSavedMsg so Update knows whether the write
// succeeded (see cursorDirty — debt 3a Minor 2).
func saveCursorCmd(cursor int) tea.Cmd {
	return func() tea.Msg {
		ok := tmux.SetOptionResult(tmux.SidebarCursor, strconv.Itoa(cursor))
		return cursorSavedMsg{Success: ok}
	}
}

// saveRepoFilterCmd writes the repo filter to @sidebar_repo_filter, with the
// same retry mechanism as saveCursorCmd.
func saveRepoFilterCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ok := tmux.SetOptionResult(tmux.SidebarRepoFilter, name)
		return repoFilterSavedMsg{Success: ok}
	}
}

// selectPaneCmd calls tmux.SelectPane (a subprocess) inside a tea.Cmd,
// without blocking Update.
func selectPaneCmd(paneID, fromPane string) tea.Cmd {
	return func() tea.Msg {
		tmux.SelectPane(paneID, fromPane)
		return nil
	}
}

// refreshCmd calls tmux.QuerySessions() and tmux.GetAllGlobalOptions() (two
// subprocess calls) inside the same tea.Cmd, so the color palette/icons
// (theme.go) reload "on every refresh" as required by the Theme/Icons
// comment — without blocking Update (runs in the separate goroutine
// bubbletea gives this Cmd).
func refreshCmd() tea.Cmd {
	return func() tea.Msg {
		sessions := tmux.QuerySessions()
		opts := tmux.GetAllGlobalOptions()

		msg := RefreshMsg{
			Sessions:   sessions,
			Theme:      ThemeFromOptions(opts),
			Icons:      IconsFromOptions(opts),
			Avatar:     avatarConfigFromOptions(opts),
			Dialogs:    readDialogs(sessions),
			SizeOwners: readSizeOwners(sessions),
		}
		return msg
	}
}

// readDialogs captures the screen of every waiting agent and parses its
// prompt, so the list can offer the answers inline.
func readDialogs(sessions []tmux.SessionInfo) map[string]scrape.Dialog {
	out := map[string]scrape.Dialog{}
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Status != tmux.StatusWaiting {
					continue
				}
				if screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", p.PaneID); ok {
					if d, ok := scrape.ParseDialog(screen); ok {
						out[p.PaneID] = d
					}
				}
			}
		}
	}
	return out
}

func tickRefreshCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// tickAnimCmd fires on the wall clock's animInterval boundaries (tea.Every),
// the same clock avatarTick() reads — tea.Tick would drift by the update's
// own time and every few ticks skip a GIF frame (a visible jump).
func tickAnimCmd() tea.Cmd {
	return tea.Every(animInterval, func(time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// applyRows rebuilds rows from m.sessions + the current repoFilter, clamps
// the cursor, and loads the viewport content — used in every branch that
// changes either of those two things (RefreshMsg, repo filter change, popup
// confirmation).
func (m *Model) applyRows() {
	m.recomputeRepoNames()
	extras := rowExtras{
		Ports:         m.portsByPane,
		SessionNames:  m.sessionNames,
		Dirty:         m.dirtyByPane,
		AheadBehind:   m.aheadBehindByPane,
		Pinned:        m.pinnedPanes,
		Selected:      m.selectedPanes,
		FilterQuery:   m.filterQuery,
		CurrentWindow: m.windowID,
		Dialogs:       m.dialogs,
		DialogOpen:    m.dialogOpen,
	}
	// The cursor follows its pane, not its index: a pane added, removed or
	// regrouped above it would otherwise move the highlight to another one.
	prev, hadPrev := paneIDAt(m.rows, m.cursor)
	m.rows = buildRows(m.sessions, m.repoFilter, extras)
	for i := range m.rows {
		m.rows[i].SizedByWeb = m.sizeOwners[m.rows[i].PaneID]
	}
	if dir := m.focusDir(prev); dir != "" {
		harness, _ := tmux.GetOption(tmux.AgentOption)
		m.rows = append(m.rows, repoAgentRows(dir, harness)...)
	}
	if idx, ok := paneIndexOf(m.rows, prev); hadPrev && ok {
		m.cursor = idx
	}
	m.cursor = clampCursor(m.cursor, paneRowCount(m.rows))
	m.syncViewport()
}

func repoAgentRows(dir, harness string) []Row {
	defs := tasks.LocalDefinitions(dir, harness)
	rows := []Row{{IsRepoAgent: true, Header: "Global agents"}}
	for _, d := range roles.ListGlobal(harness) {
		rows = append(rows, Row{IsRepoAgent: true, RepoAgentName: d.Name})
	}
	if len(rows) == 1 {
		rows = append(rows, Row{IsRepoAgent: true, RepoAgentName: "(none configured)"})
	}
	rows = append(rows, Row{IsRepoAgent: true, Header: "Local agents"})
	if len(defs) == 0 {
		return append(rows, Row{IsRepoAgent: true, RepoAgentName: "(none in this repo)"})
	}
	for _, d := range defs {
		rows = append(rows, Row{IsRepoAgent: true, RepoAgentName: d.Name})
	}
	return rows
}

// syncViewport rebuilds the viewport content from the current rows/cursor,
// scrolling to the cursor (scroll.go — pays off debt 3a Minor 3), then
// applies that offset to viewport.YOffset. While the repo popup is open, the
// viewport content is the popup list (view.go), not scrolled to the pane
// cursor.
func (m *Model) syncViewport() {
	if m.removeConfirmOpen {
		// The worktree-delete confirmation popup REPLACES the viewport
		// content, the same way the repo-selection popup below does — the
		// sidebar has no layered compositing. Bulk mode (removeConfirmPaneIDs
		// non-empty) reuses the exact same popup, just with a synthetic
		// "N worktrees" title instead of one branch name — no view.go change
		// needed.
		paneID, branch := m.removeConfirmPaneID, m.removeConfirmBranch
		if n := len(m.removeConfirmPaneIDs); n > 0 {
			paneID, branch = "", fmt.Sprintf("%d worktrees", n)
		}
		m.viewport.SetContent(renderRemoveConfirmPopup(paneID, branch, m.removeConfirmError, m.viewport.Width, m.theme))
		m.viewport.SetYOffset(0)
		return
	}

	if m.popup.IsRepoOpen() {
		names := RepoNames(m.allRepoNames())
		selected := m.popup.RepoSelected()
		m.viewport.SetContent(renderRepoPopup(names, selected, m.viewport.Width, m.theme))

		// Important 2 (final review 3b): previously SetContent did not reset
		// YOffset, so the popup was drawn from whatever offset was left over
		// from the pane list (e.g. after scrolling to the bottom) — losing
		// both the title line and the selected line, with no key to recover
		// since the popup has no manual scroll-up. Build a NEW ScrollState
		// matching the popup content (line 0 is the repoPopupTitle title,
		// the selected line is at selected+1) then EnsureVisible — this both
		// fixes the stale-offset overflow and auto-scrolls correctly if the
		// repo list is longer than the pane height.
		m.scroll = ScrollState{}
		m.scroll.EnsureVisible(selected+1, len(names)+1, m.viewport.Height)
		m.viewport.SetYOffset(m.scroll.Offset)
		return
	}

	m.viewport.SetContent(renderRowsAt(m.rows, m.viewport.Width, m.cursor, m.theme, m.icons, m.animFrame/2))
	if idx, ok := rowIndexOfCursor(m.rows, m.cursor); ok {
		m.scroll.EnsureVisible(idx, len(m.rows), m.viewport.Height)
		m.viewport.SetYOffset(m.scroll.Offset)
	}
}

// detailRefreshCmd builds the detailFetchCmd for the pane CURRENTLY UNDER
// THE CURSOR (paneIDAt(m.rows, m.cursor) — no more popup), reusing the
// activity mtime cache ONLY WHEN that cache belongs to this exact pane —
// switching panes makes the old cache meaningless (matches the same
// condition as version 4a's activityRefreshCmd). Returns nil when there's no
// pane under the cursor (empty list, or cursor sitting on a header) — there
// is nothing to ask for.
func (m *Model) detailRefreshCmd() tea.Cmd {
	id, ok := paneIDAt(m.rows, m.cursor)
	if !ok {
		return nil
	}
	info, _ := paneInfoByID(m.sessions, id)
	return detailFetchCmd(id, info.Path)
}

// applyDetailMsg applies the detailFetchCmd result to Model — ignored if the
// cursor has already moved to a different pane before this Cmd returns
// (stale result; the next detailRefreshCmd round/periodic tick will re-ask for the
// correct current pane).
// focusDir is the repo the definitions list follows: a pane of the sidebar's
// own window (so other sessions do not change it); outside tmux (standalone)
// the selected agent's directory, else where tagent was started.
func (m Model) focusDir(selected string) string {
	for _, s := range m.sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Path == "" || p.PaneID == m.tmuxPane {
					continue
				}
				if !m.standalone && m.windowID != "" && w.WindowID == m.windowID || m.standalone && p.PaneID == selected {
					return p.Path
				}
			}
		}
	}
	if m.standalone {
		dir, _ := os.Getwd()
		return dir
	}
	return ""
}

func (m *Model) applyDetailMsg(msg detailMsg) {
	cursorID, ok := paneIDAt(m.rows, m.cursor)
	if msg.PaneID == "" || !ok || msg.PaneID != cursorID {
		return
	}
	if m.dirtyByPane == nil {
		m.dirtyByPane = map[string]bool{}
	}
	m.dirtyByPane[msg.PaneID] = msg.GitData.ChangedFileCount() > 0
	if m.aheadBehindByPane == nil {
		m.aheadBehindByPane = map[string]string{}
	}
	m.aheadBehindByPane[msg.PaneID] = aheadBehindLabel(msg.GitData.AheadBehind)
	if m.notesScope != msg.Scope || m.repoDir != msg.Dir {
		m.boardDir = ""
	}
	m.notes, m.notesScope = msg.Notes, msg.Scope
	m.taskCursor = max(0, min(m.taskCursor, len(m.notes)-1))
	m.repoDir = msg.Dir
}

// allRepoNames returns the names of EVERY repo that currently has an agent
// pane, WITHOUT applying the status filter/repo filter — this is the list
// the repo-selection popup must see (matches repo_names() in the reference
// implementation, filter.rs:131-137: iterates state.repo_groups, i.e. the
// raw groups before filtering, not the already-filtered display rows). If
// repo names were derived from m.rows (already filtered), a repo whose
// panes are all hidden by the status filter would disappear from the popup
// entirely — incorrect relative to the reference implementation's behavior.
// Returns the cached version, recomputed in applyRows (the only place
// m.sessions changes). This function used to call GroupPanesByRepo — i.e.
// os.Lstat/os.ReadFile probing for .git for every pane — directly inside
// Update; while the repo popup is open, every j/k press ran it twice
// (handlePopupKey + syncViewport), so I/O scaled with keystroke rate.
func (m *Model) allRepoNames() []string {
	return m.repoNames
}

func (m *Model) recomputeRepoNames() {
	groups := GroupPanesByRepo(m.sessions)
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	m.repoNames = names
}

// Update handles a message. No branch calls tmux.QuerySessions() directly —
// it always goes through refreshCmd() so the subprocess call stays inside a
// tea.Cmd.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(taskActionMsg); ok {
		m.attachmentsKey = ""
	}
	next, cmd := m.update(msg)
	if nm, ok := next.(Model); ok {
		return nm.syncAttachments(), cmd
	}
	return next, cmd
}

// syncAttachments caches the selected ticket's attachment list so renders and
// keypresses do not stat files; it reloads when the selection or key changes.
func (m Model) syncAttachments() Model {
	key := ""
	if m.boardMode && m.taskCursor >= 0 && m.taskCursor < len(m.notes) {
		key = m.boardDir + "\x00" + m.notes[m.taskCursor].ID
	}
	if key == m.attachmentsKey {
		return m
	}
	m.attachmentsKey, m.attachments = key, nil
	if key != "" {
		m.attachments, _ = tasks.AttachmentList(m.notesScope, m.boardDir, m.notes[m.taskCursor].ID)
	}
	return m
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case boardConfigMsg:
		if msg.err != nil {
			m.taskNotice = "! Board config: " + msg.err.Error()
		} else {
			m.boardConfig = msg.config
			m.boardWorkflows = workflow.List(tasks.MainRoot(m.boardDir))
			m.taskNotice = "✓ Board config reloaded"
		}
		return m, nil
	case workflowsEditedMsg:
		m.boardWorkflows = workflow.List(tasks.MainRoot(m.boardDir))
		m.workflowCursor = max(0, min(m.workflowCursor, len(m.boardWorkflows)-1))
		if msg.err != nil {
			m.taskNotice = msg.err.Error()
		}
		return m, nil
	case taskActionMsg:
		m.taskNotice = msg.Text
		m.notes = tasks.List(m.notesScope)
		m.boardHistory, _ = tasks.ReadEvents(m.notesScope, 0)
		m.taskCursor = max(0, min(m.taskCursor, len(m.notes)-1))
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The key-hint line is drawn separately by view.go, OUTSIDE the
		// viewport — the remainder is the scrollable pane list. (The fixed
		// activity/history panel was removed: user feedback "don't show
		// history in the left pane".)
		m.resizeViewport()
		m.syncViewport()
		return m, nil

	case RefreshMsg:
		m.sessions = msg.Sessions
		m.sizeOwners = msg.SizeOwners
		m.theme = msg.Theme
		m.icons = msg.Icons
		m.dialogs = msg.Dialogs
		if msg.Avatar != m.avatar {
			m.avatar = msg.Avatar
			m.resizeViewport()
		}
		m.applyRows()
		// Follow focus: when the user switches to another agent pane in this
		// window, the highlight moves to it (the cursor stays free for j/k
		// until focus changes again).
		if f := focusedPane(m.sessions, m.windowID); f != "" && f != m.lastFocus {
			m.lastFocus = f
			if idx, ok := paneIndexOf(m.rows, f); ok && idx != m.cursor {
				m.cursor = idx
				m.cursorDirty = true
				m.syncViewport()
			}
		}

		avatarCmd := m.digestKick()

		// The bottom summary panel (part 6, replacing the "i" popup) tracks
		// the pane CURRENTLY UNDER THE CURSOR, always refetching git/activity
		// every periodic tick — no more gating on the popup (the detail popup was
		// removed entirely, real user feedback: "show it directly at the
		// root"). detailRefreshCmd returns nil itself when there's no pane
		// under the cursor.
		m.metricTick++
		if m.dashboard && !m.metricBusy && (m.metricTick == 1 || m.metricTick%4 == 0) {
			m.metricBusy = true
			return m, tea.Batch(m.detailRefreshCmd(), avatarCmd, m.metricRefreshCmd())
		}
		return m, tea.Batch(m.detailRefreshCmd(), avatarCmd)

	case metricMsg:
		m.metricBusy = false
		if msg.Scope == m.notesScope || m.notesScope == "" {
			m.metricState, m.metricScope = msg.State, msg.Scope
		}
		return m, nil

	case refreshTickMsg:
		// Steady tick: ask for fresh data AND schedule the next tick itself.
		// Kept separate from refreshRequestMsg so a burst of SIGUSR1 doesn't
		// double the number of ticks running.
		return m, tea.Batch(refreshCmd(), tickRefreshCmd())

	case refreshRequestMsg:
		return m, refreshCmd()

	case detailMsg:
		m.applyDetailMsg(msg)
		if m.dashboard && m.metricScope != m.notesScope && !m.metricBusy {
			m.metricBusy = true
			return m, m.metricRefreshCmd()
		}
		return m, nil

	case portScanMsg:
		m.portsByPane = computePortsByPane(m.sessions, msg.Listening, msg.Snapshot)
		m.applyRows()
		return m, nil

	case sessionNamesMsg:
		m.sessionNames = msg.Names
		m.applyRows()
		return m, nil

	case removeResultMsg:
		if msg.Success {
			m.removeConfirmOpen = false
			m.removeConfirmError = ""
			if msg.Undo != nil {
				m.lastRemoved = msg.Undo
			}
			// Fetch the new list RIGHT AWAY — the window/worktree just
			// disappeared, don't wait for the next periodic tick to see it.
			return m, refreshCmd()
		}
		// The error stays in the popup so the user sees it and can retry
		// (like the reference implementation's confirm_remove: the popup
		// does NOT auto-close on error).
		m.removeConfirmError = msg.Err
		m.syncViewport()
		return m, nil

	case cursorLoadedMsg:
		// Comes from tmux at startup — apply it directly, WITHOUT setting
		// cursorDirty (otherwise the next AnimTick would write the exact
		// value just read right back). @sidebar_cursor is shared by every
		// sidebar, so once this sidebar knows which pane is focused in its
		// own window (focus following, RefreshMsg), that wins: the shared
		// value is some other window's selection.
		if m.lastFocus != "" {
			return m, nil
		}
		m.cursor = clampCursor(msg.Value, paneRowCount(m.rows))
		m.syncViewport()
		return m, nil

	case repoFilterLoadedMsg:
		m.repoFilter = msg.Value
		m.applyRows()
		return m, nil

	case cursorSavedMsg:
		m.cursorSaveInFlight = false
		if msg.Success {
			m.cursorDirty = false
		}
		return m, nil

	case repoFilterSavedMsg:
		m.repoFilterSaveInFlight = false
		if msg.Success {
			m.repoFilterDirty = false
		}
		return m, nil

	case AnimTickMsg:
		m.animFrame++
		// The bubble's height follows what the avatar says (bubbleRows),
		// which can change between refreshes (the hour turns): keep the
		// list's height in step so the frame never outgrows the pane.
		if m.height > 0 && m.viewport.Height != max(0, m.height-footerHeight-m.listTop()) {
			m.resizeViewport()
		}
		if m.animFrame%2 == 0 {
			m.syncViewport() // marquee: one step every 2 ticks (~400ms)
		}
		cmds := []tea.Cmd{tickAnimCmd()}
		// Debounce: collapse every change since the previous AnimTick into
		// exactly ONE tmux write here per kind — don't fire another one
		// while the previous write is still waiting for tmux to respond
		// (cursorSaveInFlight/...).
		if m.cursorDirty && !m.cursorSaveInFlight {
			m.cursorSaveInFlight = true
			cmds = append(cmds, saveCursorCmd(m.cursor))
		}
		if m.repoFilterDirty && !m.repoFilterSaveInFlight {
			m.repoFilterSaveInFlight = true
			cmds = append(cmds, saveRepoFilterCmd(m.repoFilter))
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

// OpenSettings opens the settings screen (`tagent config`) in a tmux
// popup over the client looking at pane. The popup blocks until closed, so
// it runs as a command; the sidebar picks changed options up on its next
// refresh.
func OpenSettings(pane string) tea.Cmd {
	return func() tea.Msg {
		bin, err := os.Executable()
		if err != nil {
			bin = "tagent"
		}
		args := []string{"display-popup", "-E", "-w", "76", "-h", "32"}
		// The popup inherits the terminal background, so a dark theme's light
		// text vanished on a light terminal (and vice versa): paint it.
		bg, fg := "colour255", "colour235"
		if t, _ := tmux.GetOption(tmux.AgentTheme); strings.EqualFold(strings.TrimSpace(t), "dark") {
			bg, fg = "colour234", "colour255"
		}
		args = append(args, "-s", "bg="+bg+",fg="+fg)
		if pane != "" {
			args = append(args, "-t", pane)
		}
		tmux.RunTmux(append(args, shellQuote(bin)+" config")...)
		return refreshRequestMsg{}
	}
}

// avatarH is the height the avatar takes at the top (0 when off).
func (m Model) avatarH() int {
	switch {
	case !m.avatar.enabled():
		return 0
	case m.height >= m.tallHeight()+avatarListMin:
		return m.tallHeight()
	}
	return avatarHeight
}

// resizeViewport gives the list what's left under the avatar and above the
// key-hint line.
func (m *Model) resizeViewport() {
	m.viewport.Width = m.width
	if m.horizontal() {
		m.viewport.Width = m.width / 3
	}
	m.viewport.Height = max(0, m.height-footerHeight-m.listTop())
	m.syncViewport()
}

func (m Model) horizontal() bool { return m.width >= 90 && m.width > 2*m.height }

func (m Model) avatarWidth() int {
	if m.horizontal() {
		return m.width / 3
	}
	return m.width
}

func (m Model) listTop() int {
	if m.horizontal() {
		return 0
	}
	return m.avatarH()
}

// quickAction acts on the most urgent alert (avatar.go: Act) — space, or a
// click on the avatar.
func (m Model) quickAction() (tea.Model, tea.Cmd) {
	alerts := Alerts(m.sessions) // same alerts as the avatar shows
	if len(alerts) == 0 {
		return m, nil
	}
	a, cfg, from := alerts[0], m.avatar, m.tmuxPane
	return m, func() tea.Msg {
		Act(a, cfg, from)
		return refreshRequestMsg{}
	}
}

// boardTaskAt maps a click on a visible board row to its task.
func (m Model) boardTaskAt(x, y int) (tasks.Task, bool) {
	if len(m.notes) == 0 || m.notesScope == "" {
		return tasks.Task{}, false
	}
	free, firstY := 0, 0
	if m.horizontal() {
		left, middle := m.width/3, m.width/3
		if x < left+middle || x >= m.width {
			return tasks.Task{}, false
		}
		free, firstY = m.height-footerHeight, 2
	} else {
		if x < 0 || x >= m.viewport.Width {
			return tasks.Task{}, false
		}
		free = m.viewport.Height - len(m.rows)
		if free < contextMinLines {
			return tasks.Task{}, false
		}
		lines := m.contextLines(m.viewport.Width)
		firstY = m.listTop() + m.viewport.Height - len(lines) + 2
	}
	visible := m.visibleBoardTasks(free)
	idx := y - firstY
	if idx < 0 || idx >= len(visible) {
		return tasks.Task{}, false
	}
	return visible[idx], true
}

// handleMouse: a click on the expand button (start of the bottom line)
// behaves like the o key; a click on a pane row selects it; the wheel moves
// the cursor like j/k. Everything else is ignored.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.dashboard && (m.boardMode || m.taskForm != "" || m.helpOpen) {
		if m.boardMode && m.taskForm == "" && !m.helpOpen && !m.taskDelete {
			if msg.Button == tea.MouseButtonWheelDown {
				return m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
			}
			if msg.Button == tea.MouseButtonWheelUp {
				return m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
			}
		}
		return m, nil // Dashboard hit targets are not valid while an overlay replaces it.
	}
	if m.dashboard && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		rows, _ := m.companionPanel(max(6, m.width-1))
		g := m.dashGeometry(rows)
		taskY, taskX := g.top, g.left
		if m.width < 90 {
			taskY, taskX = g.top+g.first, 0
		}
		if msg.X >= taskX && msg.Y == taskY {
			m.agentFocus, m.taskFocus, m.taskTitle = 2, true, true
			return m, nil
		}
		// Agent definitions panel: the lower part of the agents column
		// (same split as agentPanels).
		ah := g.bodyHeight
		if m.width < 90 {
			ah = g.first
		}
		small := max(4, ah/3)
		defsY := g.top + max(4, ah-small)
		if (m.width < 90 || msg.X < g.left) && msg.Y > defsY && msg.Y < defsY+small-1 {
			if idx := msg.Y - defsY - 1; idx < len(m.focusedDefinitions()) {
				m.agentFocus, m.taskFocus, m.definitionCursor = 1, false, idx
				return m, nil
			}
		}
		if msg.X >= taskX && msg.Y >= taskY+1 && msg.Y < taskY+g.second-1 {
			order := m.boardOrder()
			pos := slices.Index(order, m.taskCursor)
			// One row per ticket; the same window as taskLines (height g.second-2).
			idx := max(0, pos-max(1, g.second-2-3)) + (msg.Y - taskY - 1)
			if idx >= 0 && idx < len(order) {
				m.taskCursor = order[idx]
				m.taskTitle = false
				m.taskFocus, m.agentFocus = true, 2
				m = m.openBoard()
				m, cmd, _ := m.boardKey("enter")
				return m, cmd
			}
		}
	}

	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.tmuxPane != "" && !m.popup.IsRepoOpen() && !m.removeConfirmOpen {
		if task, ok := m.boardTaskAt(msg.X, msg.Y); ok {
			pane, scope, id := m.tmuxPane, m.notesScope, task.ID
			return m, func() tea.Msg { OpenBoardTask(pane, scope, id); return nil }
		}
	}
	if m.dashboard && (msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp) {
		// The wheel scrolls the task list when the pointer is over it.
		rows, _ := m.companionPanel(max(6, m.width-1))
		g := m.dashGeometry(rows)
		taskY, taskX := g.top, g.left
		if m.width < 90 {
			taskY, taskX = g.top+g.first, 0
		}
		if msg.X >= taskX && msg.Y >= taskY && msg.Y < taskY+g.second {
			if msg.Button == tea.MouseButtonWheelDown && m.taskCursor+1 < len(m.notes) {
				m.taskCursor++
			} else if msg.Button == tea.MouseButtonWheelUp && m.taskCursor > 0 {
				m.taskCursor--
			}
			return m, nil
		}
	}
	switch {
	case msg.Button == tea.MouseButtonWheelDown:
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	case msg.Button == tea.MouseButtonWheelUp:
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	case msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress:
		return m, nil
	case msg.Y == m.height-1 && msg.X < lipgloss.Width(expandButton):
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	case m.horizontal() && msg.Y < m.height-footerHeight && msg.X < m.avatarWidth() && m.avatar.enabled():
		return m.quickAction()
	case !m.horizontal() && msg.Y < m.avatarH():
		return m.quickAction()
	case msg.Y >= m.listTop() && msg.Y-m.listTop() < m.viewport.Height && (!m.horizontal() || (msg.X >= m.avatarWidth() && msg.X < m.avatarWidth()+m.viewport.Width)) && !m.popup.IsRepoOpen() && !m.removeConfirmOpen:
		// Row under the click → its index among selectable pane rows.
		line := m.viewport.YOffset + msg.Y - m.listTop()
		if line >= len(m.rows) || !isSelectablePaneRow(m.rows[line]) {
			return m, nil
		}
		idx := 0
		for _, r := range m.rows[:line] {
			if isSelectablePaneRow(r) {
				idx++
			}
		}
		if idx != m.cursor {
			m.cursor = idx
			m.cursorDirty = true
			m.syncViewport()
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd()
			}
		}
	}
	return m, nil
}

// handleKey handles a key. While a popup is open, navigation keys belong to
// the popup (clamped at the edges, no wraparound — popup.go); the "r" key
// that opens the popup only works when the popup is CLOSED, like the
// reference implementation (app/input.rs: the is_repo_popup_open() branch
// fully blocks the regular key branch below it).
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.taskNotice = "" // a task action's result shows until the next key
	if m.helpOpen {
		switch msg.String() {
		case "a":
			m.helpAll, m.helpScroll = true, 0
		case "j", "down":
			m.helpScroll++
		case "k", "up":
			m.helpScroll = max(0, m.helpScroll-1)
		default:
			m.helpOpen, m.helpScroll, m.helpAll = false, 0, false
		}
		return m, nil
	}
	if msg.String() == "?" && !m.popup.IsRepoOpen() && !m.removeConfirmOpen && !m.filterEditing {
		m.helpOpen, m.helpAll, m.helpScroll = true, false, 0
		return m, nil
	}
	if m.taskForm != "" {
		return m.handleTaskForm(msg)
	}
	if m.dashboard && m.taskDelete {
		switch msg.String() {
		case "y":
			if m.taskCursor < len(m.notes) {
				scope, id := m.notesScope, m.notes[m.taskCursor].ID
				m.taskDelete = false
				return m, func() tea.Msg { err := tasks.Remove(scope, id); return actionResult("Task deleted", err) }
			}
		case "n", "esc":
			m.taskDelete = false
		}
		return m, nil
	}
	if m.dashboard && !m.popup.IsRepoOpen() && !m.removeConfirmOpen && !m.filterEditing {
		if m.boardMode {
			var cmd tea.Cmd
			var handled bool
			m, cmd, handled = m.boardKey(msg.String())
			if handled {
				return m, cmd
			}
		}
		switch msg.String() {
		case "tab":
			m.agentFocus = (m.agentFocus + 1) % 3
			m.taskFocus = m.agentFocus == 2
			m.definitionCursor = 0
			return m, nil
		case "[":
			if m.chartDays == 30 {
				m.chartDays = 7
			} else if m.chartDays == 7 {
				m.chartDays = 1
			}
			return m, nil
		case "]":
			if m.chartDays == 1 {
				m.chartDays = 7
			} else if m.chartDays == 7 {
				m.chartDays = 30
			}
			return m, nil
		}
		if m.agentFocus == 0 {
			if id, ok := paneIDAt(m.rows, m.cursor); ok {
				switch msg.String() {
				case "l": // the agent's recent output, without leaving the dashboard
					return m, popup(m.tmuxPane, "tmux capture-pane -p -e -S -300 -t "+shellQuote(id)+" | less -R +G", func() tea.Msg { return nil })
				case "i": // type a prompt into the agent
					m.taskForm, m.taskInput, m.promptPane = "i", "", id
					return m, nil
				}
			}
		}
		if m.agentFocus == 1 {
			defs := m.focusedDefinitions()
			switch msg.String() {
			case "j", "down":
				if m.definitionCursor+1 < len(defs) {
					m.definitionCursor++
				}
				return m, nil
			case "k", "up":
				if m.definitionCursor > 0 {
					m.definitionCursor--
				}
				return m, nil
			case "e", "enter":
				if m.definitionCursor < len(defs) {
					path := defs[m.definitionCursor].Path
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "vi"
					}
					return m, popup(m.tmuxPane, editor+" "+shellQuote(path), func() tea.Msg { return refreshRequestMsg{} })
				}
				return m, nil
			}
		}
		if m.taskFocus {
			key := msg.String()
			if key == "b" {
				return m.openBoard(), nil
			}
			if key == "j" || key == "down" {
				if m.taskTitle {
					m.taskTitle = false
					return m, nil
				}
			}
			if key == "k" || key == "up" {
				order := m.boardOrder()
				if len(order) == 0 || order[0] == m.taskCursor {
					m.taskTitle = true
					return m, nil
				}
			}
			if key != "q" && key != "ctrl+c" && key != "," {
				if m.boardDir == "" || m.boardDir != m.repoDir && m.repoDir != "" {
					m = m.boardContext()
				}
				m.boardDrawer, m.boardFlow, m.workflowFocus = false, false, false
				m, cmd, _ := m.boardKey(key)
				return m, cmd
			}
		}
	}
	if m.removeConfirmOpen {
		return m.handleRemoveConfirmKey(msg)
	}
	if m.popup.IsRepoOpen() {
		return m.handlePopupKey(msg)
	}
	if m.filterEditing {
		return m.handleFilterKey(msg)
	}

	// Approval inbox: on a waiting agent, its prompt's keys answer it.
	if id, ok := paneIDAt(m.rows, m.cursor); ok {
		if d, ok := m.dialogs[id]; ok && m.dialogOpen[id] {
			for _, o := range d.Options {
				if msg.String() == o.Key {
					key := o.Key
					delete(m.dialogs, id) // gone once answered; refresh confirms
					m.applyRows()
					return m, func() tea.Msg {
						scrape.Answer(id, key)
						return refreshRequestMsg{}
					}
				}
			}
		}
	}

	switch msg.String() {
	case "N":
		m.taskForm, m.taskStep = "new-agent", 0
		return m, nil
	case "q", "ctrl+c":
		// cursorDirty still being set means the most recent @sidebar_cursor
		// write is still waiting for the next AnimTick (debounce ~200ms) —
		// quitting right here would lose it. Flush that write first, then
		// Quit.
		if m.cursorDirty {
			return m, tea.Sequence(saveCursorCmd(m.cursor), tea.Quit)
		}
		return m, tea.Quit

	case "n":
		// Jump to the next agent that needs the user (waiting or error).
		n := paneRowCount(m.rows)
		for i := 1; i <= n; i++ {
			idx := (m.cursor + i) % n
			if r, ok := paneRowAt(m.rows, idx); ok && (r.Status == tmux.StatusWaiting || r.Status == tmux.StatusError) {
				m.cursor, m.cursorDirty = idx, true
				m.syncViewport()
				return m, m.detailRefreshCmd()
			}
		}
		return m, nil

	case "j", "down":
		if next, moved := moveCursor(m.cursor, 1, paneRowCount(m.rows)); moved {
			m.cursor = next
			m.cursorDirty = true
			m.syncViewport()
			// The bottom summary panel tracks the cursor (part 6, replacing
			// the "i" popup — real user feedback: "show it directly at the
			// root") — moving panes must refetch git/activity RIGHT AWAY,
			// not wait for the periodic tick.
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd()
			}
		}
		return m, nil

	case "k", "up":
		if next, moved := moveCursor(m.cursor, -1, paneRowCount(m.rows)); moved {
			m.cursor = next
			m.cursorDirty = true
			m.syncViewport()
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd()
			}
		}
		return m, nil

	case " ":
		return m.quickAction()

	case ",":
		return m, OpenSettings(m.tmuxPane)

	case "o":
		// Expand this pane into the full dashboard (expand.go). The respawn
		// replaces this process, so nothing after it matters.
		if m.tmuxPane == "" {
			return m, nil
		}
		pane := m.tmuxPane
		expand := func() tea.Msg { SetExpanded(pane, true); return nil }
		if m.cursorDirty {
			return m, tea.Sequence(saveCursorCmd(m.cursor), expand)
		}
		return m, expand

	case "r":
		m.popup.ToggleRepo(m.allRepoNames(), m.repoFilter)
		m.syncViewport()
		return m, nil

	case "enter":
		// No row at all (empty list or cursor out of range) does nothing.
		// The jump fires tmux's after-select-pane hook, which clears
		// @pane_unseen (agent-sidebar.conf).
		if paneID, ok := paneIDAt(m.rows, m.cursor); ok {
			// A pending approval: the first Enter shows its answers, the
			// next one jumps to the pane.
			if _, pending := m.dialogs[paneID]; pending && !m.dialogOpen[paneID] {
				m.dialogOpen = map[string]bool{paneID: true}
				m.applyRows()
				return m, nil
			}
			return m, selectPaneCmd(paneID, m.tmuxPane)
		}
		return m, nil

	case "v":
		// Toggle multi-select on the pane under the cursor (bulk delete via
		// "x" below). Any row can be selected; ineligible ones (not
		// SidebarSpawned) are simply skipped when "x" collects the list —
		// same silent-ignore behavior as the single-row case.
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			if m.selectedPanes == nil {
				m.selectedPanes = map[string]bool{}
			}
			if m.selectedPanes[row.PaneID] {
				delete(m.selectedPanes, row.PaneID)
			} else {
				m.selectedPanes[row.PaneID] = true
			}
			m.applyRows()
		}
		return m, nil

	case "x":
		// Bulk path: if anything is selected, "x" collects the SELECTED rows
		// eligible for delete (SidebarSpawned) and opens the confirm popup
		// in bulk mode — takes priority over the single-row path below, even
		// if the cursor also happens to sit on an eligible row.
		if len(m.selectedPanes) > 0 {
			var ids []string
			for _, r := range m.rows {
				// Reads m.selectedPanes directly (the source of truth)
				// rather than r.Selected (a copy synced by buildRows) — a
				// pane that was selected then disappeared from the list is
				// simply absent from m.rows, so this loop never has to
				// special-case it.
				if m.selectedPanes[r.PaneID] && r.SidebarSpawned {
					ids = append(ids, r.PaneID)
				}
			}
			if len(ids) > 0 {
				m.removeConfirmOpen = true
				m.removeConfirmPaneIDs = ids
				m.removeConfirmPaneID = ""
				m.removeConfirmBranch = ""
				m.removeConfirmError = ""
				m.syncViewport()
				return m, nil
			}
		}
		// Only a row created by the sidebar's own spawn flow (SidebarSpawned)
		// can open the delete confirmation popup — this is the project's
		// only destructive path, silently ignored on a regular row instead
		// of reporting an error (the reference implementation has a flash
		// message here; the Go sidebar has no flash mechanism yet, see the
		// report).
		if row, ok := paneRowAt(m.rows, m.cursor); ok && row.SidebarSpawned {
			m.removeConfirmOpen = true
			m.removeConfirmPaneID = row.PaneID
			m.removeConfirmBranch = row.Branch
			m.removeConfirmError = ""
			m.syncViewport()
		}
		return m, nil

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Jump straight to the Nth pane row (1-indexed, matching what the
		// user counts on screen) — same cursor-move-and-refetch as j/k
		// above, just landing directly on an index instead of stepping.
		n := int(msg.String()[0] - '1')
		if n < paneRowCount(m.rows) {
			m.cursor = n
			m.cursorDirty = true
			m.syncViewport()
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd()
			}
		}
		return m, nil

	case "f": // free in the Agents keymap: reclaim sizing from a browser
		if m.dashboard && m.agentFocus != 0 {
			return m, nil
		}
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			return m, handoffCmd(row.PaneID)
		}
		return m, nil
	case "p":
		// Pin/unpin the pane under the cursor — process-memory only (see
		// Model.pinnedPanes). Pinned panes float to a "Pinned" group right
		// after "Needs you" (buildRows).
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			if m.pinnedPanes == nil {
				m.pinnedPanes = map[string]bool{}
			}
			if m.pinnedPanes[row.PaneID] {
				delete(m.pinnedPanes, row.PaneID)
			} else {
				m.pinnedPanes[row.PaneID] = true
			}
			m.applyRows()
		}
		return m, nil

	case "S":
		// Manually clear a stuck subagent tree — the normal cleanup path
		// (OnSubagentStop) only fires when a subagent finishes NORMALLY; a
		// force-killed or crashed subagent leaves its entry orphaned
		// forever with no other way to clear it. ponytail: a real fix would
		// track per-entry liveness/TTL; this manual escape hatch is the
		// lazy version, upgrade if orphaned entries turn out to be common.
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			return m, clearSubagentsCmd(row.PaneID)
		}
		return m, nil

	case "/":
		m.filterEditing = true
		return m, nil

	case "u":
		// Undo the most recently removed worktree+branch (detail.go:
		// removeCmd/undoRemoveCmd) — single-slot, consumed on use.
		if m.lastRemoved != nil {
			w := *m.lastRemoved
			m.lastRemoved = nil
			return m, tea.Sequence(undoRemoveCmd(w), refreshCmd())
		}
		return m, nil
	}
	return m, nil
}

// clearSubagentsCmd unsets @pane_subagents for paneID (a subprocess) inside
// a tea.Cmd — see the "S" key above. The next refresh tick (already
// scheduled every second) picks up the change; no need to force an
// immediate refresh here.
func clearSubagentsCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		tmux.UnsetPaneOption(paneID, tmux.PaneSubagents)
		return nil
	}
}

// handleFilterKey handles keys while the "/" text filter is being edited —
// Esc cancels and clears the filter, Enter keeps it and stops editing
// (navigation keys work normally again), Backspace removes the last rune,
// any other printable rune is appended. Runs buildRows again on every
// change so the list narrows live as the user types.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterEditing = false
		m.filterQuery = ""
		m.applyRows()
		return m, nil

	case "enter":
		m.filterEditing = false
		return m, nil

	case "backspace":
		if m.filterQuery != "" {
			r := []rune(m.filterQuery)
			m.filterQuery = string(r[:len(r)-1])
			m.applyRows()
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.filterQuery += string(msg.Runes)
		m.applyRows()
	}
	return m, nil
}

// handleRemoveConfirmKey handles keys while the worktree-delete confirmation
// popup is open — ported from the reference implementation's
// is_remove_confirm_open() branch in app/input.rs: Esc/n closes without
// changing anything, c deletes ONLY the window (keeps worktree+branch),
// Enter/y deletes both the window and the worktree+branch.
func (m Model) handleRemoveConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.removeConfirmOpen = false
		m.removeConfirmError = ""
		m.removeConfirmPaneIDs = nil
		m.selectedPanes = nil
		m.syncViewport()
		return m, nil

	case "c":
		return m, m.removeConfirmCmd(worktree.RemoveWindowOnly)

	case "enter", "y":
		return m, m.removeConfirmCmd(worktree.RemoveWindowAndWorktree)
	}
	return m, nil
}

// removeConfirmCmd builds the delete command(s) for the confirm popup —
// a single removeCmd in the normal single-row case, or one removeCmd per id
// batched with tea.Batch in bulk mode (removeConfirmPaneIDs non-empty, set
// by the "v"/"x" multi-select flow in handleKey). Consumes
// removeConfirmPaneIDs/selectedPanes: the selection is spent the moment the
// bulk delete is confirmed, same as it is on cancel (handleRemoveConfirmKey
// above).
func (m *Model) removeConfirmCmd(mode worktree.RemoveMode) tea.Cmd {
	if len(m.removeConfirmPaneIDs) == 0 {
		return removeCmd(m.removeConfirmPaneID, mode)
	}
	ids := m.removeConfirmPaneIDs
	m.removeConfirmPaneIDs = nil
	m.selectedPanes = nil
	cmds := make([]tea.Cmd, len(ids))
	for i, id := range ids {
		cmds[i] = removeCmd(id, mode)
	}
	return tea.Batch(cmds...)
}

// handlePopupKey handles keys while the repo popup is open — ported from the
// is_repo_popup_open() branch in app/input.rs: j/k (clamped at the edges, NO
// wraparound, quite unlike the status filter's left/right — verified
// against the reference implementation), Enter confirms, Esc closes without
// changing anything.
func (m Model) handlePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	repos := m.allRepoNames()
	switch msg.String() {
	case "esc":
		m.popup.CloseRepo()
		m.syncViewport()
		return m, nil

	case "j", "down":
		m.popup.RepoNavDown(len(RepoNames(repos)))
		m.syncViewport()
		return m, nil

	case "k", "up":
		m.popup.RepoNavUp()
		m.syncViewport()
		return m, nil

	case "enter":
		if filter, ok := m.popup.ConfirmRepo(repos); ok {
			m.repoFilter = filter
			m.repoFilterDirty = true
			m.applyRows()
			return m, nil
		}
		m.syncViewport()
		return m, nil
	}
	return m, nil
}
