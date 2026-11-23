import { useState } from "react"
import { toast } from "sonner"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { enc, nav } from "@/hooks/useRoute"
import { baseName } from "@/lib/status"
import { useAppState } from "@/state"
import type { Chat } from "@/types"

// The receptionist is not a tmux pane: it answers questions and drafts tickets in the Chat view.
export const RECEPTIONIST = "receptionist"

// NewAgentComposer starts an agent in a project. Used on Home and in NewAgentDialog.
export function NewAgentComposer({ onDone }: { onDone?: () => void }) {
  const { state } = useAppState()
  const [prompt, setPrompt] = useState("")
  const [harness, setHarness] = useState("")
  const [model, setModel] = useState("")
  const [cwd, setCwd] = useState("")
  const [worktree, setWorktree] = useState(false)
  const [busy, setBusy] = useState(false)

  const harnesses = [...(state?.harnesses ?? []), RECEPTIONIST]
  const projects = state?.projects ?? []
  const agent = harness || harnesses[0] || ""
  const dir = cwd || projects[0] || ""

  async function submit() {
    if (!prompt.trim()) {
      toast.error("Write what the agent should do first")
      return
    }
    setBusy(true)
    try {
      if (agent === RECEPTIONIST) {
        const chat = await api<Chat>(`/api/chat?dir=${enc(dir)}`)
        await post("/api/chat", { dir, op: "send", revision: chat.revision, text: prompt })
        setPrompt("")
        onDone?.()
        nav("/chat")
        return
      }
      const r = await post<{ pane: string }>("/api/agents", {
        Agent: agent,
        Model: model,
        Cwd: dir,
        Role: "",
        Name: "",
        Prompt: prompt,
        Worktree: worktree,
      })
      setPrompt("")
      onDone?.()
      nav("/agent/" + enc(r.pane))
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3 rounded-lg border bg-card p-3">
      <Textarea
        autoFocus={!!onDone}
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            void submit()
          }
        }}
        placeholder={agent === RECEPTIONIST ? "Ask the receptionist: which agents are running? Draft a ticket for…" : "What should an agent do?"}
        className="min-h-24 resize-none border-0 bg-transparent px-1 shadow-none focus-visible:ring-0"
      />
      <div className="flex flex-wrap items-center gap-2">
        {harnesses.length > 0 && (
          <Select value={agent} onValueChange={setHarness}>
            <SelectTrigger size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {harnesses.map((h) => (
                <SelectItem key={h} value={h}>
                  {h}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {agent !== RECEPTIONIST && <Input
          value={model}
          onChange={(e) => setModel(e.target.value)}
          placeholder="model"
          className="h-8 w-28"
        />}
        {projects.length > 0 && (
          <Select value={dir} onValueChange={setCwd}>
            <SelectTrigger size="sm" className="max-w-48">
              <SelectValue>{baseName(dir)}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {projects.map((p) => (
                <SelectItem key={p} value={p}>
                  {baseName(p)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {agent !== RECEPTIONIST && <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Switch checked={worktree} onCheckedChange={setWorktree} />
          Worktree
        </label>}
        <Button className="ml-auto" disabled={busy} onClick={() => void submit()}>
          Start
        </Button>
      </div>
    </div>
  )
}
