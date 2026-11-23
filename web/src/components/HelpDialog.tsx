import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"

const KEYS: [string, string][] = [
  ["j / ↓", "Next agent"],
  ["k / ↑", "Previous agent"],
  ["n", "Next agent that needs you"],
  ["Enter", "Focus the agent's terminal"],
  ["Ctrl+Shift+F6", "Leave terminal; focus agent header"],
  ["Esc", "Send Escape to agent (while terminal focused)"],
  ["Terminal | Chat", "Switch an agent between its terminal and its conversation"],
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
              </DialogContent>
    </Dialog>
  )
}
