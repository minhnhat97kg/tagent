import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { baseName } from "@/lib/status"

const ADD = "__add_workspace__" // never a folder; picking it opens the add dialog

export function ProjectSelect({ value, projects, onChange, onAdd }: { value: string; projects: string[]; onChange: (dir: string) => void; onAdd: () => void }) {
  return (
    <Select value={value} onValueChange={v => (v === ADD ? onAdd() : onChange(v))}>
      <SelectTrigger size="sm" className="max-w-56">
        <SelectValue placeholder="No workspace" />
      </SelectTrigger>
      <SelectContent>
        {projects.map((p) => (
          <SelectItem key={p} value={p} title={p}>
            {baseName(p)}
          </SelectItem>
        ))}
        <SelectItem value={ADD}>Add workspace…</SelectItem>
      </SelectContent>
    </Select>
  )
}
