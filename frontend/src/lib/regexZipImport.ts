import { unzip } from 'fflate'
import type { RegexScript } from '@/api/presets'

export class RegexImportError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'RegexImportError'
  }
}

function cryptoID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return 'r-' + Math.random().toString(36).slice(2, 12)
}

/** Normalize one ST regex export object for our bundle. */
export function normalizeRegexScript(raw: unknown, fallbackName?: string): RegexScript | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Record<string, unknown>
  const findRegex = typeof o.findRegex === 'string' ? o.findRegex : ''
  if (!findRegex.trim()) return null

  const replaceString = typeof o.replaceString === 'string' ? o.replaceString : ''
  const placement = Array.isArray(o.placement)
    ? o.placement.filter((p): p is number => typeof p === 'number')
    : [2]

  return {
    id: typeof o.id === 'string' && o.id ? o.id : cryptoID(),
    scriptName:
      (typeof o.scriptName === 'string' && o.scriptName) ||
      fallbackName ||
      'imported',
    findRegex,
    replaceString,
    trimStrings: Array.isArray(o.trimStrings)
      ? o.trimStrings.filter((t): t is string => typeof t === 'string')
      : [],
    placement: placement.length ? placement : [2],
    disabled: o.disabled === true,
    markdownOnly: o.markdownOnly === true,
    promptOnly: o.promptOnly === true,
    runOnEdit: o.runOnEdit === true,
    substituteRegex: typeof o.substituteRegex === 'number' ? o.substituteRegex : 0,
    minDepth: o.minDepth === null || typeof o.minDepth === 'number' ? (o.minDepth as number | null) : undefined,
    maxDepth: o.maxDepth === null || typeof o.maxDepth === 'number' ? (o.maxDepth as number | null) : undefined,
  }
}

/** Parse ST per-file JSON or a bundle fragment. */
export function parseRegexJsonText(text: string, sourceName?: string): RegexScript[] {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new RegexImportError(`Invalid JSON${sourceName ? ` (${sourceName})` : ''}`)
  }

  if (Array.isArray(parsed)) {
    return parsed
      .map((item, i) => normalizeRegexScript(item, `${sourceName ?? 'script'}_${i + 1}`))
      .filter((s): s is RegexScript => s != null)
  }

  if (parsed && typeof parsed === 'object') {
    const o = parsed as Record<string, unknown>
    const ext = o.extensions as { regex_scripts?: unknown } | undefined
    if (ext && Array.isArray(ext.regex_scripts)) {
      return parseRegexJsonText(JSON.stringify(ext.regex_scripts), sourceName)
    }
    if (Array.isArray(o.regex_scripts)) {
      return parseRegexJsonText(JSON.stringify(o.regex_scripts), sourceName)
    }
    const one = normalizeRegexScript(parsed, sourceName)
    return one ? [one] : []
  }

  return []
}

function basename(path: string): string {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || path
}

function scriptNameFromPath(path: string): string {
  const base = basename(path)
  const dot = base.lastIndexOf('.')
  return dot > 0 ? base.slice(0, dot) : base
}

async function unzipToEntries(file: File): Promise<Record<string, Uint8Array>> {
  const buf = new Uint8Array(await file.arrayBuffer())
  return new Promise((resolve, reject) => {
    unzip(buf, (err, data) => {
      if (err) reject(new RegexImportError(err.message || 'ZIP unpack failed'))
      else resolve(data)
    })
  })
}

/** Import regex scripts from a .zip of ST JSON files (one script per file). */
export async function importRegexScriptsFromZip(file: File): Promise<RegexScript[]> {
  const name = file.name.toLowerCase()
  if (!name.endsWith('.zip')) {
    throw new RegexImportError('Expected a .zip file')
  }

  const entries = await unzipToEntries(file)
  const paths = Object.keys(entries)
    .filter(p => !p.endsWith('/'))
    .filter(p => !p.includes('__MACOSX'))
    .filter(p => p.toLowerCase().endsWith('.json'))
    .sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))

  if (paths.length === 0) {
    throw new RegexImportError('No .json files found in the archive')
  }

  const scripts: RegexScript[] = []
  for (const path of paths) {
    const bytes = entries[path]
    if (!bytes?.length) continue
    const text = new TextDecoder('utf-8').decode(bytes)
    const label = scriptNameFromPath(path)
    const chunk = parseRegexJsonText(text, label)
    for (const s of chunk) {
      if (!s.scriptName || s.scriptName === 'imported') {
        s.scriptName = label
      }
      scripts.push(s)
    }
  }

  if (scripts.length === 0) {
    throw new RegexImportError('No valid regex scripts in the archive')
  }

  return scripts
}

/** Single .json file (one script, array, or bundle with regex_scripts). */
export async function importRegexScriptsFromJsonFile(file: File): Promise<RegexScript[]> {
  const text = await file.text()
  const scripts = parseRegexJsonText(text, file.name)
  if (scripts.length === 0) {
    throw new RegexImportError('No regex scripts in this JSON file')
  }
  return scripts
}

export async function importRegexScriptsFromFile(file: File): Promise<RegexScript[]> {
  const lower = file.name.toLowerCase()
  if (lower.endsWith('.zip')) return importRegexScriptsFromZip(file)
  if (lower.endsWith('.json')) return importRegexScriptsFromJsonFile(file)
  throw new RegexImportError('Use .zip or .json')
}

/** Append imports; skip duplicates by id, then scriptName. */
export function mergeRegexScripts(
  existing: RegexScript[],
  imported: RegexScript[],
  mode: 'replace' | 'append',
): RegexScript[] {
  if (mode === 'replace') return [...imported]

  const seenId = new Set(existing.map(s => s.id).filter(Boolean) as string[])
  const seenName = new Set(existing.map(s => s.scriptName).filter(Boolean) as string[])
  const out = [...existing]
  for (const s of imported) {
    if (s.id && seenId.has(s.id)) continue
    if (s.scriptName && seenName.has(s.scriptName)) continue
    if (s.id) seenId.add(s.id)
    if (s.scriptName) seenName.add(s.scriptName)
    out.push(s)
  }
  return out
}
