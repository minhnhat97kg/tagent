import { expect, test } from "vitest"
import { parseRoute } from "./useRoute"

// One row per route in the table of plan 110; old links keep their pages.
test.each([
  ["/", { page: "home" }],
  ["/board", { page: "home" }],
  ["/nope", { page: "home" }],
  ["/pane/nope", { page: "home" }],
  ["/pane/usage", { page: "pane", pane: "usage" }],
  ["/pane/tasks", { page: "pane", pane: "tasks" }],
  ["/pane/agents", { page: "agents" }],
  ["/agents", { page: "agents" }],
  ["/agent/%251", { page: "home", agent: "%1" }],
  ["/agents/%251", { page: "home", agent: "%1" }],
  ["/agents/%251/x", { page: "home" }],
  ["/pane/chat", { page: "pane", pane: "chat" }],
  ["/agent/", { page: "home" }],
  ["/agent/%E0%A4%A", { page: "home" }],
  ["/pane/workflows", { page: "flows", name: "" }],
  ["/workflows", { page: "flows", name: "" }],
  ["/workflows/feature%20x", { page: "flows", name: "feature x" }],
  ["/workflows/feature/61", { page: "run", ticket: "61", dir: null }],
  ["/runs/61?dir=%2Fp%2Fa", { page: "run", ticket: "61", dir: "/p/a" }],
  ["/runs/61", { page: "run", ticket: "61", dir: null }],
  ["/board/config", { page: "boardConfig" }],
  ["/routing", { page: "routing" }],
])("%s", (route, want) => {
  expect(parseRoute(route)).toEqual(want)
})
