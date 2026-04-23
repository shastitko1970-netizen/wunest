import type { PresetType } from '@/api/presets'

// Heuristic detector for SillyTavern-style preset JSON. Given a parsed
// object, returns the most likely preset type — or null if we can't tell.
//
// Why heuristic: ST ships many preset shapes from different years of the
// project (textgen vs openai samplers, instruct templates with/without
// first_input_sequence, etc). Rather than maintain a closed allow-list of
// keys, we score each type by how many of its "signature" keys appear in
// the object and pick the winner.
//
// Keys are ordered from most-specific (strong signal) to least — the
// first matching key in a category tips the score even if siblings are
// absent, so a bare `{"story_string": "..."}` still classifies as
// context.

type Bag = Record<string, unknown>

const SIGNATURES: Record<PresetType, { strong: string[]; weak: string[] }> = {
  // Instruct templates wrap user/assistant turns with sequences.
  // Strong: sequence keys; weak: wrap flag, activation regex.
  instruct: {
    strong: [
      'input_sequence',
      'output_sequence',
      'system_sequence',
      'first_input_sequence',
      'last_input_sequence',
      'first_output_sequence',
      'last_output_sequence',
      'system_sequence_prefix',
      'system_sequence_suffix',
    ],
    weak: ['wrap', 'activation_regex', 'user_alignment_message', 'stop_sequence'],
  },
  // Context templates drive the story_string + splice points.
  context: {
    strong: ['story_string', 'chat_start', 'example_separator'],
    weak: [
      'trim_sentences',
      'single_line',
      'use_stop_strings',
      'names_as_stop_strings',
      'always_force_name2',
    ],
  },
  // System prompt — content + optional post_history hint.
  sysprompt: {
    strong: ['post_history', 'post_history_instructions'],
    weak: ['content', 'prompt'],
  },
  // Reasoning template — prefix/suffix wrapping a think block.
  // Strong: the combo of prefix+suffix+separator that isn't an instruct
  // sequence set; weak: any one of them standalone.
  reasoning: {
    strong: ['prefix', 'suffix', 'separator'],
    weak: ['auto_parse', 'reasoning_effort'],
  },
  // Legacy OpenAI preset — dozens of keys, pick a few distinctive ones.
  openai: {
    strong: [
      'openai_model',
      'openai_max_context',
      'temp_openai',
      'top_p_openai',
      'chat_completion_source',
    ],
    weak: ['freq_pen_openai', 'pres_pen_openai', 'count_pen'],
  },
  // Generic sampler (textgen or openai-numeric). Catch-all for numeric
  // sampler fields that don't fit the other buckets.
  sampler: {
    strong: ['temperature', 'top_p', 'top_k', 'min_p', 'max_tokens', 'frequency_penalty'],
    weak: [
      'temp',
      'repetition_penalty',
      'presence_penalty',
      'stop',
      'seed',
      'reasoning_enabled',
    ],
  },
}

const STRONG_WEIGHT = 3
const WEAK_WEIGHT = 1

export function detectPresetType(data: unknown): PresetType | null {
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    return null
  }
  const bag = data as Bag

  // Score every type by matched signature keys. The type with the highest
  // score wins; ties go to the more specific type via the `tieBreaker`
  // order (instruct/context/sysprompt/reasoning/openai beat generic sampler).
  const tieBreaker: PresetType[] = [
    'instruct',
    'context',
    'sysprompt',
    'reasoning',
    'openai',
    'sampler',
  ]
  const scores: Record<PresetType, number> = {
    sampler: 0, instruct: 0, context: 0, sysprompt: 0, reasoning: 0, openai: 0,
  }

  for (const [type, { strong, weak }] of Object.entries(SIGNATURES) as [
    PresetType, { strong: string[]; weak: string[] },
  ][]) {
    for (const k of strong) if (k in bag) scores[type] += STRONG_WEIGHT
    for (const k of weak)   if (k in bag) scores[type] += WEAK_WEIGHT
  }

  // OpenAI presets almost always also look like samplers (they carry
  // `temperature` etc). If OpenAI scored strong, prefer it over sampler.
  if (scores.openai >= STRONG_WEIGHT && scores.openai >= scores.sampler) {
    scores.sampler = 0
  }

  // Reasoning has prefix/suffix/separator — instruct templates DON'T use
  // those three together (they use *_sequence). Disambiguate: if we matched
  // reasoning's strong three AND not a single instruct strong, lock in
  // reasoning. Conversely an instruct match always wins over a lonely
  // prefix/suffix pair.
  if (scores.instruct > 0) {
    scores.reasoning = 0
  }

  // Sysprompt needs content-ish field to be meaningful.
  if (scores.sysprompt > 0 && !('content' in bag) && !('prompt' in bag) && !('post_history' in bag) && !('post_history_instructions' in bag)) {
    scores.sysprompt = 0
  }

  let best: PresetType | null = null
  let bestScore = 0
  for (const type of tieBreaker) {
    if (scores[type] > bestScore) {
      best = type
      bestScore = scores[type]
    }
  }
  return bestScore > 0 ? best : null
}
