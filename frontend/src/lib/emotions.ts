// Emotion detection — maps the last-known assistant message to a
// sprite name (from the character's data.assets expression set).
//
// Two-tier detection:
//
//   1. Explicit tag — author wrote `<happy>` / `[sad]` / `:angry:` in
//      the message. Exact names win.
//   2. Keyword dictionary — scan for common verbs/expressions (smiles,
//      frowns, grins) and map to emotion categories. Fallback when
//      the author didn't tag explicitly.
//
// Returns the expression NAME as stored in the card's assets (e.g.
// "happy", "angry"). Caller resolves the URL via the character's
// assets array. Returns "" when no emotion detected — UI either
// keeps the previous sprite or shows none.

/** Keyword → emotion map. English + common Russian stems. Expand
 *  freely as new characters hit the wild; all lowercase, keyword
 *  matching is case-insensitive.
 *
 *  Keep keys generic (emotion categories that cards commonly name).
 *  Card authors pick their own expression names; we expose a UI in
 *  the editor to map custom names, but this dictionary gives a
 *  reasonable "out of the box" for standard emotion sets. */
const KEYWORDS: Record<string, string[]> = {
  happy: [
    'smile', 'smiled', 'smiling', 'grin', 'grinned', 'laugh', 'laughed', 'chuckle',
    'beam', 'cheerful', 'joyful', 'delighted',
    'улыбнул', 'улыбк', 'смеё', 'засмея', 'рассмея', 'радост', 'весел',
  ],
  sad: [
    'frown', 'tear', 'cried', 'crying', 'weep', 'sob', 'sobbed', 'mourn',
    'melancholy', 'sorrow', 'gloomy',
    'грустн', 'заплак', 'плака', 'слёз', 'слез', 'печал', 'тоск',
  ],
  angry: [
    'angry', 'furious', 'rage', 'raged', 'shout', 'yell', 'glare', 'glared',
    'snarl', 'snap', 'scowl', 'snap',
    'зло', 'злит', 'разозл', 'гнев', 'ярост', 'кричал', 'рявкн',
  ],
  surprised: [
    'gasp', 'gasped', 'astonish', 'startled', 'surprised', 'shock',
    'ахнул', 'удивл', 'потряс', 'пораж',
  ],
  embarrassed: [
    'blush', 'blushed', 'flush', 'flushed', 'shy',
    'покрасн', 'застенч', 'смущ',
  ],
  scared: [
    'fear', 'afraid', 'terrified', 'scared', 'tremble', 'trembled', 'shudder',
    'боя', 'испуг', 'ужас', 'дрож',
  ],
  confused: [
    'confused', 'puzzled', 'bewildered', "didn't understand",
    'смущ', 'недоум', 'озадач',
  ],
  neutral: [
    // Fallback; intentionally tiny list — we prefer keeping the
    // previous sprite over force-defaulting.
  ],
}

/** explicitTagRegex matches `<name>` or `[name]` or `:name:` (with
 *  word-boundaries around the name so we don't false-positive on
 *  real punctuation in narration). */
const explicitTagRegex = /[<\[:]\s*(?<name>[a-zA-Zа-яА-Я_-]{2,24})\s*[>\]:]/gi

export interface DetectInput {
  /** Last assistant message content (trimmed, markdown as-is). */
  content: string
  /** Available expression names for this character (from data.assets
   *  filtered to type="expression"). Used to validate tag hits. */
  available: string[]
}

/** detectEmotion runs both tiers and returns the best match, or empty
 *  string when nothing was found. Guarantees the return is either an
 *  empty string or a value from `available`. */
export function detectEmotion(input: DetectInput): string {
  const content = (input.content ?? '').trim()
  if (!content || input.available.length === 0) return ''
  const availLower = new Set(input.available.map(a => a.toLowerCase()))

  // Tier 1: explicit tag. Scan LAST match (latest in message — if the
  // author wrote `<happy> ... <sad>`, the sad wins because that's
  // where the character's emotion ended).
  let lastTagHit = ''
  const matches = content.matchAll(explicitTagRegex)
  for (const m of matches) {
    const name = (m.groups?.name ?? '').toLowerCase()
    if (availLower.has(name)) {
      lastTagHit = name
    }
  }
  if (lastTagHit) {
    // Return the canonical-cased name from available list.
    return input.available.find(a => a.toLowerCase() === lastTagHit) ?? lastTagHit
  }

  // Tier 2: keyword dictionary. Find all matching categories +
  // count occurrences; category with most hits wins. Ties favor
  // the one that appears latest in the text (same "latest = current
  // emotion" logic as tags).
  const lower = content.toLowerCase()
  const scores: Record<string, number> = {}
  const lastSeen: Record<string, number> = {}
  for (const [emotion, keywords] of Object.entries(KEYWORDS)) {
    if (!availLower.has(emotion)) continue
    for (const kw of keywords) {
      let idx = lower.indexOf(kw)
      while (idx >= 0) {
        scores[emotion] = (scores[emotion] ?? 0) + 1
        lastSeen[emotion] = Math.max(lastSeen[emotion] ?? -1, idx)
        idx = lower.indexOf(kw, idx + kw.length)
      }
    }
  }
  const candidates = Object.keys(scores)
  if (candidates.length === 0) return ''
  candidates.sort((a, b) => {
    if (scores[b] !== scores[a]) return scores[b] - scores[a]
    return (lastSeen[b] ?? -1) - (lastSeen[a] ?? -1)
  })
  const pick = candidates[0]
  // Return canonical case.
  return input.available.find(a => a.toLowerCase() === pick) ?? pick
}
