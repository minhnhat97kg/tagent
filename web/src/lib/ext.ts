// A custom pane as GET /api/ext lists it. agent: an agent made it, so it may only read; actions: the user allowed more.
export type ExtInfo = { name: string; agent: boolean; actions: boolean }

// The HTML the "New custom pane" form starts from.
export const PANE_TEMPLATE = `<!doctype html>
<meta charset="utf-8">
<style>body{font:14px system-ui;margin:12px}</style>
<h3>Hello</h3>
<pre id="out">loading…</pre>
<script src="/ext/sdk.js"></script>
<script>
  tagent.api("/api/state").then(s => out.textContent = "host: " + s.host)
</script>
`
