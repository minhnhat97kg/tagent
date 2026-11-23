import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { baseName } from "@/lib/status"

export function ProjectSelect({ value, projects, onChange }: { value: string; projects: string[]; onChange: (dir: string) => void }) {
  if (!value) return null
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger size="sm" className="max-w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {projects.map((p) => (
          <SelectItem key={p} value={p}>
            {baseName(p)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
