// Built-in slash commands per harness, offered when the chat message starts with "/".
// ponytail: static lists; read the harness's own command/skill dirs if custom commands are wanted.
export const harnessCommands: Record<string, [string, string][]> = {
  claude: [
    ["/clear", "Start a new conversation"],
    ["/compact", "Summarise the conversation to free context"],
    ["/context", "Show context usage"],
    ["/cost", "Show session cost"],
    ["/model", "Change the model"],
    ["/resume", "Resume a previous conversation"],
    ["/review", "Review a pull request"],
    ["/init", "Write a CLAUDE.md for this repo"],
    ["/memory", "Edit memory files"],
    ["/status", "Show version, model and account"],
    ["/help", "List commands"],
  ],
  pi: [
    ["/new", "Start a new session"],
    ["/compact", "Summarise the conversation to free context"],
    ["/model", "Change the model"],
    ["/resume", "Resume a previous session"],
    ["/session", "Show session info"],
    ["/tree", "Browse the session tree"],
    ["/fork", "Fork from an earlier message"],
    ["/copy", "Copy the last reply"],
    ["/export", "Export the session to HTML"],
    ["/settings", "Open settings"],
    ["/hotkeys", "List keyboard shortcuts"],
  ],
}

export function commandMatches(harness: string | undefined, draft: string): [string, string][] {
  if (!draft.startsWith("/") || /\s/.test(draft)) return []
  return (harnessCommands[harness ?? ""] ?? []).filter(([c]) => c.startsWith(draft) && c !== draft)
}

// Models offered in the Chat model menu; "/model <alias>" switches without a picker.
// pi has no alias form, so its menu only opens the picker.
export const harnessModels: Record<string, string[]> = { claude: ["opus", "sonnet", "haiku"] }
