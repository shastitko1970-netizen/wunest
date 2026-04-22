// splitContent — walks message text and splits it into markdown chunks
// and JSON-plate chunks. Shared by MessageContent.vue and unit tests.
//
// Two shapes get plated:
//   1. Fenced ```json … ``` blocks — high-confidence; always plated.
//   2. Bare JSON objects at paragraph boundaries — only when the opening `{`
//      starts a line and the matching `}` ends one, with balanced braces
//      and proper string escaping along the way.
//
// A non-trivial threshold (array with >0 items OR object with ≥2 keys)
// prevents "I said {word}" or "{}" from becoming a table.

export type Part =
  | { kind: 'markdown'; body: string }
  | { kind: 'plate'; body: unknown; label?: string }

export function splitContent(text: string, raw = false): Part[] {
  if (!text) return []
  if (raw) return [{ kind: 'markdown', body: text }]

  const matches: Array<{ start: number; end: number; body: string }> = []

  // Fenced json blocks.
  const fenceRe = /```(?:json|JSON)\s*\n([\s\S]*?)\n?```/g
  for (const m of text.matchAll(fenceRe)) {
    if (m.index == null) continue
    matches.push({ start: m.index, end: m.index + m[0].length, body: m[1] })
  }

  // Bare object blocks at paragraph boundaries.
  for (const m of bareJsonBlocks(text)) {
    if (matches.some(x => m.start < x.end && m.end > x.start)) continue
    matches.push(m)
  }
  matches.sort((a, b) => a.start - b.start)

  const parts: Part[] = []
  let cursor = 0
  for (const m of matches) {
    let parsed: unknown
    try {
      parsed = JSON.parse(m.body)
    } catch {
      continue
    }
    if (!isPlateWorthy(parsed)) continue

    if (m.start > cursor) {
      const before = text.slice(cursor, m.start)
      const { text: trimmed, label } = extractLabel(before)
      parts.push({ kind: 'markdown', body: trimmed })
      parts.push({ kind: 'plate', body: parsed, label })
    } else {
      parts.push({ kind: 'plate', body: parsed })
    }
    cursor = m.end
  }

  if (cursor < text.length) {
    parts.push({ kind: 'markdown', body: text.slice(cursor) })
  }
  return parts
}

// ─── helpers ──────────────────────────────────────────────────────────

function isPlateWorthy(v: unknown): boolean {
  if (v == null) return false
  if (Array.isArray(v)) return v.length > 0
  if (typeof v === 'object') return Object.keys(v as object).length >= 2
  return false
}

function* bareJsonBlocks(text: string): Generator<{ start: number; end: number; body: string }> {
  let i = 0
  const n = text.length
  while (i < n) {
    const at = text.indexOf('{', i)
    if (at < 0) return
    const lineStart = text.lastIndexOf('\n', at - 1) + 1
    const prefix = text.slice(lineStart, at).trim()
    if (prefix !== '') { i = at + 1; continue }

    let depth = 0
    let j = at
    let inStr = false
    let esc = false
    while (j < n) {
      const ch = text[j]
      if (inStr) {
        if (esc) { esc = false }
        else if (ch === '\\') { esc = true }
        else if (ch === '"') { inStr = false }
      } else {
        if (ch === '"') inStr = true
        else if (ch === '{') depth++
        else if (ch === '}') { depth--; if (depth === 0) { j++; break } }
      }
      j++
    }
    if (depth !== 0) return

    const nextNl = text.indexOf('\n', j)
    const trailing = text.slice(j, nextNl < 0 ? n : nextNl).trim()
    if (trailing !== '') { i = j; continue }

    yield { start: at, end: j, body: text.slice(at, j) }
    i = j
  }
}

function extractLabel(before: string): { text: string; label: string | undefined } {
  const trimmed = before.replace(/\s+$/, '')
  const lines = trimmed.split('\n')
  const last = lines[lines.length - 1]?.trim() ?? ''
  if (/[:：]\s*$/.test(last) || /^#+\s+\S/.test(last)) {
    return {
      text: lines.slice(0, -1).join('\n'),
      label: last.replace(/^#+\s*/, '').replace(/[:：]\s*$/, '').trim() || undefined,
    }
  }
  return { text: before, label: undefined }
}
