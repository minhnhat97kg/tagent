import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"

const KEYS: [string, string][] = [
  ["j / ↓", "Next agent"],
  ["k / ↑", "Previous agent"],
  ["n", "Next agent that needs you"],
  ["Enter", "Focus the agent's terminal"],
  ["Ctrl+Shift+F6", "Leave terminal; focus agent header"],
  ["Esc", "Send Escape to agent (while terminal focused)"],
  ["?", "Show this help"],
]

export function HelpDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>Active when no text field or terminal has focus.</DialogDescription>
        </DialogHeader>
        <table className="w-full text-sm">
          <tbody>
            {KEYS.map(([k, d]) => (
              <tr key={k} className="border-b last:border-0">
                <td className="py-1.5 pr-4 font-mono text-xs whitespace-nowrap">{k}</td>
                <td className="py-1.5 text-muted-foreground">{d}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <section className="space-y-2 text-sm"><h3 className="font-semibold">Receptionist chat</h3><p>Enter sends; Shift+Enter inserts a newline. Type @ for names, panes and roles; arrows select, Tab/Enter completes, Escape dismisses.</p><p className="font-mono text-xs">/agents · /new · /board · /run &lt;workflow&gt; · /clear</p><p>Changes always need approval. New chat drops active history, not backups, and cannot undo actions.</p></section>
      </DialogContent>
    </Dialog>
  )
}
