import { act, post } from "@/api"
import { Button } from "@/components/ui/button"
import type { Dialog } from "@/types"

// DialogCard shows an agent's approval question with one button per option.
export function DialogCard({ pane, dialog }: { pane: string; dialog: Dialog }) {
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-medium whitespace-pre-wrap">{dialog.question}</p>
      <div className="flex flex-wrap gap-2">
        {(dialog.options ?? []).map((o, i) => (
          <Button
            key={o.Key}
            size="sm"
            variant={i === 0 ? "default" : "outline"}
            onClick={() => void act(post("/api/pane/answer", { pane, data: o.Key }))}
          >
            {o.Label}
          </Button>
        ))}
      </div>
    </div>
  )
}
