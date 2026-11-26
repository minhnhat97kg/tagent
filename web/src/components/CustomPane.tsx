import { act, send } from "@/api"
import { ExtPane } from "@/components/ExtPane"
import { Panel } from "@/components/Panel"
import { Button } from "@/components/ui/button"
import type { ExtInfo } from "@/lib/ext"

// CustomPane is a custom pane as a home pane: the sandboxed frame under a header that says who made it. A pane an
// agent made is read-only until the user allows actions; the server ties that to the HTML as it is now.
export function CustomPane({ info, reload }: { info: ExtInfo; reload: () => void }) {
  const { name, agent, actions } = info
  const grant = (allow: boolean) => {
    if (allow && !confirm(`Allow actions for ${name}? It was made by an agent. It will be able to call any API route (start agents, type into terminals) until its HTML changes.`)) return
    void act(send("PUT", `/api/ext/${encodeURIComponent(name)}/grant`, { allow }).then(reload))
  }
  return <Panel className="min-h-0 flex-1" title={name} bodyClass="flex flex-col overflow-hidden"
    right={agent && <>
      <span data-badge className={actions ? "text-amber-600 dark:text-amber-400" : undefined} title={actions ? "Made by an agent; you allowed actions for this version" : "Made by an agent: it can read, not change anything"}>
        agent · {actions ? "actions allowed" : "read-only"}
      </span>
      <Button variant="outline" size="xs" className="pointer-coarse:h-8" onClick={() => grant(!actions)}>{actions ? "Revoke" : "Allow actions"}</Button>
    </>}>
    <ExtPane name={name} />
  </Panel>
}
