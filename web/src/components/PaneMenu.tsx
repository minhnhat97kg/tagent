import { useState } from "react"
import { LayoutPanelLeft } from "lucide-react"
import { toast } from "sonner"
import { act, api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { PANE_TEMPLATE, type ExtInfo } from "@/lib/ext"
import { extId, extName, PANES, paneTitle, type PaneId } from "@/lib/paneLayout"

// PaneMenu is the header's "Add pane" button: the panes that are not in the layout (built-in and custom), a new
// custom pane, and deleting a custom pane.
export function PaneMenu({ shown, exts, onAdd, onChanged }: { shown: PaneId[]; exts: ExtInfo[]; onAdd: (id: PaneId) => void; onChanged: () => Promise<unknown> }) {
  const [form, setForm] = useState(false)
  const missing = [...PANES, ...exts.map(e => extId(e.name))].filter(id => !shown.includes(id))
  const remove = (name: string) => {
    if (confirm(`Delete the custom pane ${name}? Its file and saved data are removed.`)) void act(api(`/api/ext/${encodeURIComponent(name)}`, { method: "DELETE" }).then(onChanged))
  }
  return <>
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" className="max-md:hidden pointer-coarse:size-9" aria-label="Add pane" title="Add pane"><LayoutPanelLeft /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Add pane</DropdownMenuLabel>
        {missing.length === 0 && <DropdownMenuItem disabled>All panes are shown</DropdownMenuItem>}
        {missing.map(id => <DropdownMenuItem key={id} onSelect={() => onAdd(id)}>{paneTitle(id)}{exts.find(e => extId(e.name) === id)?.agent && <span className="text-muted-foreground"> · agent</span>}</DropdownMenuItem>)}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => setForm(true)}>New custom pane…</DropdownMenuItem>
        {exts.map(e => <DropdownMenuItem key={e.name} variant="destructive" onSelect={() => remove(e.name)}>Delete {e.name}…</DropdownMenuItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
    {form && <NewPane onClose={() => setForm(false)} onCreated={name => { setForm(false); void onChanged().then(() => onAdd(extId(name))) }} />}
  </>
}

function NewPane({ onClose, onCreated }: { onClose: () => void; onCreated: (name: string) => void }) {
  const [name, setName] = useState("")
  const [html, setHtml] = useState(PANE_TEMPLATE)
  const [busy, setBusy] = useState(false)
  const ok = extName(extId(name)) !== undefined && html.trim() !== ""
  const save = () => {
    setBusy(true)
    post("/api/ext", { name, html }).then(() => onCreated(name), (e: Error) => { setBusy(false); toast.error(e.message) })
  }
  return <Dialog open onOpenChange={open => { if (!open) onClose() }}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>New custom pane</DialogTitle>
        <DialogDescription>An HTML page in a sandboxed frame. It is yours, so it may call the whole API: only run code you trust.</DialogDescription>
      </DialogHeader>
      <Input value={name} onChange={e => setName(e.target.value)} placeholder="name (letters, digits, - and _)" aria-label="Pane name" autoFocus />
      <textarea value={html} onChange={e => setHtml(e.target.value)} aria-label="Pane HTML" spellCheck={false} rows={12}
        className="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50" />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>Cancel</Button>
        <Button disabled={!ok || busy} onClick={save}>Create</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
