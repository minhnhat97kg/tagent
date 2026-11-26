import { useEffect, useState } from "react"
import { toast } from "sonner"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

interface Listing { path: string; parent: string; dirs: string[] }

const join = (dir: string, name: string) => (dir.endsWith("/") ? dir : dir + "/") + name

// AddWorkspaceDialog adds a folder to the workspaces: typed or pasted, or picked from the server's folders.
export function AddWorkspaceDialog({ open, onOpenChange, onAdded }: { open: boolean; onOpenChange: (open: boolean) => void; onAdded: (root: string) => void }) {
  const [path, setPath] = useState("")
  const [list, setList] = useState<Listing>()
  const [error, setError] = useState("")
  const browse = (p: string) => api<Listing>(`/api/dirs?path=${encodeURIComponent(p)}`).then(
    l => { setList(l); setPath(l.path); setError("") },
    (e: Error) => setError(e.message),
  )
  useEffect(() => { if (open) browse("") }, [open])
  const add = () => post<{ dir: string }>("/api/workspaces", { Dir: path }).then(
    r => { onAdded(r.dir); onOpenChange(false) },
    (e: Error) => toast.error(e.message),
  )
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="sm:max-w-lg">
      <DialogTitle>Add workspace</DialogTitle>
      <DialogDescription className="board-muted">Type or paste a folder, or pick one below. A git worktree adds its main repo.</DialogDescription>
      <label htmlFor="workspace-path" className="text-[12.5px] font-medium">Folder</label>
      <Input id="workspace-path" value={path} placeholder="/absolute/path or ~/project" onChange={e => setPath(e.target.value)} onKeyDown={e => { if (e.key === "Enter") browse(path) }} />
      {error && <p role="alert" className="text-[12.5px] text-destructive">{error}</p>}
      <div aria-label="Folders" className="flex max-h-64 flex-col overflow-auto rounded-lg border">
        {list?.parent && <button type="button" className="px-3 py-1.5 text-left text-[12.5px] hover:bg-accent" onClick={() => browse(list.parent)}>..</button>}
        {list?.dirs.map(name => <button key={name} type="button" className="px-3 py-1.5 text-left text-[12.5px] hover:bg-accent" onClick={() => browse(join(list.path, name))}>{name}</button>)}
        {list && list.dirs.length === 0 && <p className="px-3 py-1.5 text-[12.5px] text-muted-foreground">No subfolders</p>}
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button disabled={!path} onClick={add}>Select this folder</Button>
      </div>
    </DialogContent>
  </Dialog>
}
