import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { NewAgentComposer } from "@/components/NewAgentComposer"

export function NewAgentDialog({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>New agent</DialogTitle>
        </DialogHeader>
        <NewAgentComposer onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}
