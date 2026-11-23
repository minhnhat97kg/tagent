import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { NewAgentComposer } from "@/components/NewAgentComposer"

export function NewAgentDialog({ onOpenChange, dir }: { onOpenChange: (open: boolean) => void; dir?: string }) {
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="gap-3 sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="text-[15px]">New agent</DialogTitle>
          <DialogDescription className="text-[12px]">Pick who to start. Double-click a tile to open it right away.</DialogDescription>
        </DialogHeader>
        <NewAgentComposer dir={dir} onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}
