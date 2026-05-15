/**
 * SillyTavern-compatible regex script runner for the chat UI.
 *
 * Server applies the same rules on save (placement 2). We re-run on
 * display so imported presets and older messages still render card HTML
 * without forcing users to regenerate.
 */
import type { OpenAIBundleData, RegexScript } from '@/api/presets'

/** ST regex_placement — keep numeric values aligned with presets + backend. */
export const REGEX_PLACEMENT = {
  MD_DISPLAY: 0,
  USER_INPUT: 1,
  AI_OUTPUT: 2,
  WORLD_INFO: 5,
  REASONING: 6,
} as const

export type RegexPlacement = (typeof REGEX_PLACEMENT)[keyof typeof REGEX_PLACEMENT]

export interface RegexApplyOptions {
  isMarkdown?: boolean
  isPrompt?: boolean
}

const regexCache = new Map<string, RegExp>()

function compileSTRegex(pattern: string): RegExp | null {
  const cached = regexCache.get(pattern)
  if (cached) {
    cached.lastIndex = 0
    return cached
  }
  const re = regexFromString(pattern)
  if (!re) return null
  regexCache.set(pattern, re)
  return re
}

/** Port of ST regexFromString — "/body/flags" or plain pattern. */
export function regexFromString(regexString: string): RegExp | null {
  if (!regexString) return null
  if (regexString.length >= 2 && regexString[0] === '/') {
    const last = regexString.lastIndexOf('/')
    if (last > 0) {
      const body = regexString.slice(1, last)
      const flags = regexString.slice(last + 1)
      try {
        return new RegExp(body, flags)
      } catch {
        return null
      }
    }
  }
  try {
    return new RegExp(regexString)
  } catch {
    return null
  }
}

function placementMatches(placements: number[] | undefined, want: RegexPlacement): boolean {
  if (!placements?.length) return false
  for (const p of placements) {
    if (p === want) return true
    if (want === REGEX_PLACEMENT.AI_OUTPUT && p === REGEX_PLACEMENT.MD_DISPLAY) return true
  }
  return false
}

function scriptMatchesContext(script: RegexScript, opts: RegexApplyOptions): boolean {
  const md = !!script.markdownOnly
  const pr = !!script.promptOnly
  if (md && pr) return false
  if (md && opts.isMarkdown) return true
  if (pr && opts.isPrompt) return true
  if (!md && !pr && !opts.isMarkdown && !opts.isPrompt) return true
  return false
}

function applyTrimStrings(value: string, trimStrings?: string[]): string {
  let out = value
  for (const t of trimStrings ?? []) {
    if (!t) continue
    out = out.split(t).join('')
  }
  return out
}

/** ST runRegexScript — replace callback with $1 / $10 / {{match}}. */
export function runRegexScript(script: RegexScript, raw: string): string {
  if (!script || script.disabled || !script.findRegex || !raw) return raw

  const find = compileSTRegex(script.findRegex)
  if (!find) return raw

  return raw.replace(find, (...args) => {
    const groups = args.slice(0, -2)
    const match = String(groups[0] ?? '')
    let replace = script.replaceString.replace(/{{match}}/gi, match)
    replace = replace.replace(/\$(\d+)|\$<([^>]+)>/g, (_, num: string, groupName: string) => {
      let captured = ''
      if (num) {
        captured = String(groups[Number(num)] ?? '')
      } else if (groupName) {
        const named = args[args.length - 1] as Record<string, string> | undefined
        captured = named && typeof named === 'object' ? String(named[groupName] ?? '') : ''
      }
      return applyTrimStrings(captured, script.trimStrings)
    })
    return replace
  })
}

export function applyRegexScripts(
  bundle: OpenAIBundleData | null | undefined,
  content: string,
  placement: RegexPlacement,
  opts: RegexApplyOptions = {},
): string {
  if (!bundle?.extensions?.regex_scripts?.length || !content) return content

  let out = content
  for (const script of bundle.extensions.regex_scripts) {
    if (script.disabled) continue
    if (!placementMatches(script.placement, placement)) continue
    if (!scriptMatchesContext(script, opts)) continue
    out = runRegexScript(script, out)
  }
  return out
}

/** Active sampler preset bundle (regex_scripts live in ST OpenAI bundle JSON). */
export function bundleFromPresetData(data: unknown): OpenAIBundleData | null {
  if (!data || typeof data !== 'object') return null
  const d = data as OpenAIBundleData
  if (Array.isArray(d.extensions?.regex_scripts) && d.extensions.regex_scripts.length > 0) {
    return d
  }
  if (Array.isArray(d.prompts) && d.prompts.length > 0) {
    return d
  }
  return d.extensions?.regex_scripts ? d : null
}
