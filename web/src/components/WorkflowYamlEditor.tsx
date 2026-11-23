import { useEffect, useRef } from 'react'
import { EditorState, type Range } from '@codemirror/state'
import { Decoration, type DecorationSet, EditorView, ViewPlugin, type ViewUpdate, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { HighlightStyle, indentUnit, syntaxHighlighting } from '@codemirror/language'
import { yaml } from '@codemirror/lang-yaml'
import { lintGutter, setDiagnostics } from '@codemirror/lint'
import { tags as t } from '@lezer/highlight'
import { lineShape } from './workflowYaml'

// Lazy-loaded from WorkflowsPage (see WorkflowsPage.tsx) so CodeMirror stays out of the main bundle.
type Goto = { line: number; to?: number }
type Props = {
  value: string
  onChange: (value: string) => void
  readOnly: boolean
  errorLine: number // 1-based; 0 means the error has no line
  errorMessage: string
  goto?: Goto
  onCursor?: (line: number) => void
  label?: string
}

const highlight = HighlightStyle.define([
  { tag: t.propertyName, color: 'var(--foreground)', fontWeight: '600' },
  { tag: t.string, color: 'var(--yaml-string)' },
  { tag: [t.bool, t.number, t.null], color: 'var(--yaml-literal)' },
  { tag: t.comment, color: 'var(--muted-foreground)', fontStyle: 'italic' },
  { tag: [t.punctuation, t.separator, t.meta], color: 'var(--muted-foreground)' },
])

const theme = EditorView.theme({
  '&': { height: '100%', fontSize: '12px', backgroundColor: 'transparent', color: 'var(--foreground)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', lineHeight: '20px' },
  '.cm-content': { padding: '12px', caretColor: 'var(--foreground)' },
  '.cm-line': { padding: '0' },
  '.cm-gutters': { backgroundColor: 'transparent', border: 'none', color: 'var(--muted-foreground)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--muted)' },
  '.cm-activeLineGutter': { color: 'var(--foreground)' },
  '.cm-wf-expr': { color: 'var(--yaml-expr)', fontWeight: '600' },
})

const shapes = ViewPlugin.fromClass(class {
  decorations: DecorationSet
  constructor(view: EditorView) { this.decorations = build(view) }
  update(u: ViewUpdate) { if (u.docChanged || u.viewportChanged) this.decorations = build(u.view) }
}, { decorations: p => p.decorations })

function build(view: EditorView): DecorationSet {
  const ranges: Range<Decoration>[] = []
  const doc = view.state.doc
  for (const { from, to } of view.visibleRanges) {
    for (let pos = from; pos <= to;) {
      const line = doc.lineAt(pos)
      const shape = lineShape(line.text)
      if (shape) ranges.push(Decoration.line({ attributes: { style: `padding-left:${shape.v}ch;text-indent:${shape.s - shape.v}ch` } }).range(line.from))
      for (const m of line.text.matchAll(/\$\{\{.*?\}\}/g)) ranges.push(Decoration.mark({ class: 'cm-wf-expr' }).range(line.from + m.index, line.from + m.index + m[0].length))
      pos = line.to + 1
    }
  }
  return Decoration.set(ranges, true)
}

export default function WorkflowYamlEditor({ value, onChange, readOnly, errorLine, errorMessage, goto, onCursor, label = 'Workflow YAML' }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView>(undefined)
  const callbacks = useRef({ onChange, onCursor })
  const start = useRef({ value, readOnly, label })
  useEffect(() => { callbacks.current = { onChange, onCursor } })

  // Mounted once per workflow: the parent remounts it with a key when another workflow is opened.
  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: start.current.value,
        extensions: [
          lineNumbers(), highlightActiveLine(), highlightActiveLineGutter(), history(), lintGutter(),
          EditorView.lineWrapping, EditorState.tabSize.of(2), indentUnit.of('  '),
          keymap.of([indentWithTab, { key: 'Escape', run: x => { x.contentDOM.blur(); return true } }, ...defaultKeymap, ...historyKeymap]),
          yaml(), syntaxHighlighting(highlight), shapes, theme,
          EditorState.readOnly.of(start.current.readOnly), EditorView.editable.of(!start.current.readOnly),
          EditorView.contentAttributes.of({ 'aria-label': start.current.label, autocorrect: 'off', autocapitalize: 'off', spellcheck: 'false' }),
          EditorView.updateListener.of(u => {
            if (u.docChanged) callbacks.current.onChange(u.state.doc.toString())
            if (u.selectionSet) callbacks.current.onCursor?.(u.state.doc.lineAt(u.state.selection.main.head).number)
          }),
        ],
      }),
    })
    view.current = v
    return () => v.destroy()
  }, [])

  // Pull in text the parent changed (not the user's own typing, which already matches).
  useEffect(() => {
    const v = view.current
    if (v && v.state.doc.toString() !== value) v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } })
  }, [value])

  useEffect(() => {
    const v = view.current
    if (!v) return
    const doc = v.state.doc
    const onLine = errorMessage && errorLine > 0 && errorLine <= doc.lines
    const diagnostics = onLine ? [{ from: doc.line(errorLine).from, to: doc.line(errorLine).to, severity: 'error' as const, message: errorMessage }] : []
    v.dispatch(setDiagnostics(v.state, diagnostics))
  }, [errorLine, errorMessage, value])

  // Scroll to a line (error click, flow card click) without focusing, so the iPad keyboard stays down.
  useEffect(() => {
    const v = view.current
    if (!v || !goto) return
    const doc = v.state.doc
    const from = doc.line(Math.min(Math.max(goto.line, 1), doc.lines)).from
    const to = doc.line(Math.min(Math.max(goto.to ?? goto.line, 1), doc.lines)).to
    v.dispatch({ selection: { anchor: from, head: to }, effects: EditorView.scrollIntoView(from, { y: 'center' }) })
  }, [goto])

  return <div ref={host} className="h-full min-h-0" />
}
