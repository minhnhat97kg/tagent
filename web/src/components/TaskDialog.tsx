import { act, post } from "@/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import type { Task } from "@/types"

export const STATUSES = [
  ["todo", "Todo"],
  ["doing", "Doing"],
  ["blocked", "Blocked"],
  ["done", "Done"],
] as const

// TaskDialog shows one task with its status buttons and delete. The caller passes the live task, so it updates on reload.
export function TaskDialog({ task: t, dir, onChange, onClose }: { task: Task; dir: string; onChange: () => void; onClose: () => void }) {
  const op = (body: Record<string, unknown>, close = false) =>
    void act(
      post("/api/tasks", { Dir: dir, ...body }).then(() => {
        onChange()
        if (close) onClose()
      }),
    )

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Task</DialogTitle>
          <DialogDescription>
            Created {new Date(t.created).toLocaleString()} · Updated {new Date(t.updated).toLocaleString()}
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm whitespace-pre-wrap">{t.text}</p>
        {!!t.assignees?.length && (
          <div className="flex flex-wrap gap-1">
            {t.assignees.map((a) => (
              <Badge key={a} variant="secondary">
                {a}
              </Badge>
            ))}
          </div>
        )}
        {t.note && <p className="text-sm whitespace-pre-wrap text-muted-foreground">{t.note}</p>}
        <div className="flex flex-wrap gap-2">
          {STATUSES.map(([s, label]) => (
            <Button key={s} size="sm" variant={t.status === s ? "default" : "outline"} onClick={() => op({ Op: "set", ID: t.id, Status: s })}>
              {label}
            </Button>
          ))}
        </div>
        <DialogFooter>
          <Button variant="destructive" size="sm" onClick={() => op({ Op: "rm", ID: t.id }, true)}>
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
