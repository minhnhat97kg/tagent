# TUI kanban view (watch dashboard, Board tab)

The Board tab is `focusNotes` in `internal/watch`, rendered by `notesBox`/`noteDetailBox` in `view.go`. Today it is a flat list. Change it to a real kanban board whenever there is room.

## When the layout is wide enough
The rule is: width ≥ cols×18 + detail width, and height ≥ 14. Otherwise keep the current list layout as the fallback.

- **Left, about 65% of the width: the kanban.**
  - There is one column per configured column, from `tasks.LoadBoardConfig` for the repo dir. This config is already loaded somewhere in watch.go for status cycling, so reuse it.
  - A column header looks like `TODO 3`. It uses the column color: todo=muted, doing=accent, review=confirm/yellow, blocked=error, done=green/dim. Use existing theme colors, not new ones.
  - Cards are compact boxes or blocks, 2–3 lines each:
    - the title, truncated, in bold when selected;
    - an `@assignee` chip;
    - indicators for due (`⏰ 12-01`, red when overdue), `💬 n` for the comment count and `📎` when there are attachments;
    - a selected-card accent bar like the existing `▌`.
  - Each column scrolls independently so the selected card stays visible.
  - Tasks whose status is not in the configured columns appear in an extra trailing `?` column.
- **Right: a detail panel for the selected card.** It shows:
  - the title, status, assignees, due date and id;
  - the description, word-wrapped;
  - the comments: `author · time`, then the body with `@mentions` highlighted in accent, then attachment names.
  - The newest comments are visible first when space is short.

## Navigation in kanban mode
- `h`/`l` or `←`/`→` change the column; `j`/`k` or `↑`/`↓` move between cards in the column.
- `H`/`L` (shift) move the selected task to the previous or next column through `tasks.Update` with the existing MutationOptions (actor user), which fires the `status` event.
- The existing keys keep working on the selected card: space, b, s, d, a, and so on.
- Mouse click on a card selects it, if the existing mouse code makes that easy; skip it if it doesn't.
- Map the selection back to `m.cursor[focusNotes]` (index into `m.tasks`) so every existing action keeps working unchanged.

## Also
Update the footer and help hints for the new keys.

## Constraints
- No new dependencies. Use lipgloss/ansi as already used.
- Keep the code small: one new file, `internal/watch/kanban.go`, is fine.
- Add a test in `watch_test.go` that renders a model with 3 tasks in 2 columns at 160x40 and asserts that the column headers and card titles appear; also cover the narrow-fallback case.
  - Note that the existing `TestSettingsTab` hangs for a pre-existing reason; you may fix it if trivial (it references the removed `@agent_avatar_image`), otherwise skip it with `-skip`.
- Run `go build ./... && TMPDIR=/tmp go test ./internal/watch ./internal/tasks` and report real results. Do not commit.

## CORRECTION (wrong target)
`tmux-agent watch` runs `sidebar.Run()` (internal/sidebar dashboard: Overview / Running agents / Tasks / Git panels, `dashboardView` in internal/sidebar/dashboard.go). The internal/watch model is NOT used by any command (only watch.Stream). So the kanban must live in internal/sidebar:
- From the dashboard, with the Tasks panel focused, key `b` (and also Enter on the Tasks panel title / when no tasks) opens a full-screen **Board mode** that replaces the dashboard body (like `taskForm` takes over `dashboardView`). `esc`/`q`/`b` returns to the dashboard.
- Board mode = the kanban + detail layout from this spec (columns from config, cards, detail with comments, h/l j/k, H/L move, `c` edit config + trigger summary line, narrow fallback = single column list grouped by status). The existing task actions (a add, e edit, g assign, s start, u status, x delete, with the existing forms in task_actions.go) must work on the selected card; map selection back to m.taskCursor / m.notes.
- Make Board mode the obvious place: add `b board` to the Tasks-focus footer help and the help overlay (help.go).
- Port the rendering from internal/watch/kanban.go into internal/sidebar/board.go, then REVERT the kanban/config-editor changes in internal/watch (keep only the TestSettingsTab fix). Keep tasks.BoardExample shared.
- Tests in internal/sidebar: render Board mode at 160x40 with 3 tasks/2 columns (headers + titles present), narrow fallback, H moves status, c with a fake EDITOR. Run go build ./... && TMPDIR=/tmp go test ./internal/sidebar ./internal/watch ./internal/tasks ./cmd/... and report. Do not commit.

## Polish round (user feedback: in a ~95-col pane Board mode showed only the bare grouped list)
- Kanban must be the default whenever width ≥ ~60. Layout tiers:
  1. wide (≥ cols*22 + 44): columns + detail panel on the right (current).
  2. medium: columns only, full width, no detail panel; Enter (or `i`) toggles a detail overlay/drawer for the selected card (esc closes).
  3. Only below ~60 cols: the grouped list fallback, but styled like cards (column header chips with counts, cards with a left accent bar, blank line between groups), not bare text.
- Empty columns collapse to a narrow strip (~8 cols, vertical header + count) so non-empty columns get the space; columns share the remaining width evenly (no fixed 18/20 width).
- Cards: title wraps to 2 lines before truncating; assignees on their own line, shortened (drop common prefix like "netform-" if all share it? no — just truncate per name and show "+N" when more than 2); due shown only when set; 💬n / 📎 indicators.
- Detail panel: don't repeat the title inside the description when the text's first line == title; omit empty fields (no bare "due").
- The keys footer line must always be visible (it was cut off: only the Triggers line showed). Footer = 2 lines: keys, then triggers; truncate to width instead of dropping.
- Update tests for the three tiers (e.g. 95x40 must render column headers side by side) and collapsed empty columns. go build ./... && TMPDIR=/tmp go test ./internal/sidebar ./internal/tasks ./cmd/...; report. Do not commit.

## Task detail redesign (Jira-like)
Rework `boardDetail` in internal/sidebar/board.go. It is used by the wide right panel and by the Enter/i drawer. Model the layout on a Jira issue view, in a terminal.

- **Header**
  - Breadcrumb line, dim: `<project> / <ID>`. The ID is short (the last 6 chars, upper-case) and prefixed with a status-colored type glyph (☐ todo, ◐ doing, ◎ review, ⊘ blocked, ✔ done).
  - Title: bold and larger-feeling (TextPrimary, Bold), wrapped up to 3 lines.
  - A row of chips: a status chip (`m.chip`-like, with background in the status color and dark text, e.g. ` DOING ▾ `), then the due chip (red if overdue, amber if due within 2 days) if set.
- **Two columns when the detail width is ≥ 70**
  - Left, about 62%:
    - `Description`: a section label with a thin underline rule, then the wrapped text without the duplicated title line.
    - `Note` if set.
    - `Attachments`: each file as `📎 name`.
    - `Activity · n`: the comments as a timeline. Each one shows:
      - an author avatar glyph, made of a colored initial in a circle-ish chip using the agent color for known harness names, else accent;
      - **author**, then a dim relative time (`5m ago`, `2h ago`, `3d ago`);
      - the body indented, with @mentions in accent bold and attachments listed under the body;
      - a dim `│` timeline connector between comments.
    - System comments ("▶ triggered …", author system) render as a compact dim one-liner with ⚡.
  - Right, about 38%: a boxed "Details" panel of label/value rows (label dim and fixed-width, value primary):
    - Status
    - Assignees (one per line, each `● name` in its agent color; `Unassigned` dim)
    - Due
    - Created (relative + date)
    - Updated (relative)
    - Comments n
    - Attachments n
- **Narrower than 70:** a single column, in the order header → details rows → description → activity.
- **Scrolling:** if content exceeds the rows, keep the header fixed and let j/k-independent scrolling be skipped. Show newest activity last and clip from the top of the activity section so the latest comments stay visible, as now.
- **Constraints:**
  - Use existing theme colors only, with no new dependencies.
  - Every line must fit within w. Add a helper for relative time.
- **Tests:**
  - The detail at 100x30 shows the breadcrumb, the status chip text, "Details", "Activity", the comment author and the relative time.
  - The narrow (50 cols) variant shows a single column.
  - All lines must be ≤ w (use ansi.StringWidth).
- Run `go build ./... && TMPDIR=/tmp go test ./internal/sidebar` and report. Do not commit.
