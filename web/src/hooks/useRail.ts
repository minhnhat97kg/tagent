import { useState } from "react"
import { getRailCollapsed, setRailCollapsed } from "@/lib/rail"

// useRailCollapsed is the left rail's state: expanded shows Usage, Agents and
// Tasks; collapsed shows an icon strip. The choice is remembered per device.
export function useRailCollapsed(): [boolean, (collapsed: boolean) => void] {
  const [collapsed, setCollapsed] = useState(getRailCollapsed)
  return [
    collapsed,
    (next: boolean) => {
      setCollapsed(next)
      setRailCollapsed(next)
    },
  ]
}
