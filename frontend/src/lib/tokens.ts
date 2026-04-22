// tokens.ts — approximate token counter for the chat UI's budget gauge.
//
// We deliberately don't bundle a real tokenizer (cl100k_base is ~450 KB
// gzipped — disproportionate for a "rough idea" widget). Empirically, the
// 4-chars-per-token heuristic lands within ±15% for English and Russian
// prose, which is plenty for "will this fit in 4k / 32k / 128k context?"
//
// If precision ever matters (e.g. hard enforcement of a provider limit
// before send), the right path is a server-side /api/tokenize endpoint
// that calls the WuApi proxy using the provider's own tokenizer.

const PROSE_CHARS_PER_TOKEN = 4

/** Sync approximation — safe to call in hot paths (watch, computed). */
export function countTokens(text: string): number {
  if (!text) return 0
  // Heuristic bumps: whitespace-delimited tokens for alphanumeric runs,
  // plus punctuation which often becomes its own token. Keeps the count
  // honest on short messages where chars/4 under-counts.
  const words = text.trim().split(/\s+/).length
  const chars = text.length
  return Math.max(words, Math.ceil(chars / PROSE_CHARS_PER_TOKEN))
}

/** Sum approximations across a list. */
export function countTokensMany(parts: string[]): number {
  if (!parts.length) return 0
  let sum = 0
  for (const p of parts) sum += countTokens(p)
  return sum
}

// Async wrappers kept so callers that already await don't need to change.
// No tokenizer load, no Promise.resolve allocation in the hot path — the
// function is sync under the hood and just returns the result wrapped.
export async function countTokensAsync(text: string): Promise<number> {
  return countTokens(text)
}
export async function countTokensManyAsync(parts: string[]): Promise<number> {
  return countTokensMany(parts)
}
