import { post } from "@/api"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { nav } from "@/hooks/useRoute"
import { toast } from "sonner"

export function KillDialog({ pane, name, onOpenChange }: { pane: string; name: string; onOpenChange: (open: boolean) => void }) {
  async function kill() {
    try {
      await post("/api/pane/kill", { pane })
      onOpenChange(false)
      nav("/")
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Kill {name}?</DialogTitle>
          <DialogDescription>The agent's pane is closed. Its work in progress is lost.</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void kill()}>
            Kill
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
