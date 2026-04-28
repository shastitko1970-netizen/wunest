# Presets

A preset is a saved bundle of generation parameters. One preset of each type can be "applied" (used by default in every chat) or set per-chat. Imports from SillyTavern are compatible.

WuNest has five preset types — each governs a different layer of the request. Knowing "what affects what" beats turning knobs blindly.

---

## Types and what they do

| Type | What it controls | When to touch it |
|---|---|---|
| **Sampler** | Style and creativity (temperature, top-p, max_tokens, penalties) | Often. The main "originality vs predictability" knob |
| **Instruct** | Turn wrappers for text-completion APIs (`<|im_start|>`, `[INST]`, etc.) | Only for local models on a completion endpoint. Not needed for chat-completion (OpenAI, Claude) |
| **Context** | Story/context assembly template (`{{description}}`, `{{personality}}`, history shape) | Rarely. Touch if you want a custom system-prompt architecture |
| **Sysprompt** | The system prompt text that replaces the card's default | Often. Genre cues, RP rules, jailbreaks |
| **Reasoning** | Thinking-block markers (`<think>...</think>`) | Only for thinking models (Claude 3.7+, o1, DeepSeek-R1) |

---

## Sampler — the main knob

Parameters are forwarded upstream as-is (via WuApi or BYOK). The provider decides exactly how to interpret them. Below — neutral meanings.

### Temperature

The single most important parameter. Controls "randomness" of next-token choice.

| Value | Effect |
|---|---|
| `0.0` | Deterministic. Same prompt → same output. |
| `0.3–0.6` | Conservative. Less variation, less wildness. |
| `0.7–0.9` | Most models' default. Balanced. |
| `1.0–1.3` | Creative. More surprises, often "good surprises." |
| `>1.5` | Often noise. On some models, a lottery. |

**Tip:** for long roleplay, 0.85–1.0 usually beats the 0.7 default. The model repeats itself less.

### Top-P (nucleus sampling)

Trims the long tail of unlikely tokens. `top_p=0.9` keeps tokens whose cumulative probability ≤ 90%.

- `1.0` — no trim (most creative)
- `0.9` — soft filter
- `0.7` — strict, "safer" output

Usually you turn either `temperature` or `top_p`, not both.

### Top-K

Keeps only the N most-likely tokens. `top_k=40` is a local-model standard.

`0` or unset — disabled (not all providers honor it).

### Min-P

A newer alternative to top-p. Trims tokens below a relative-probability floor. `min_p=0.05` is a sensible default.

Often **better** than top-p for roleplay — it cuts less aggressively into interesting branches.

### Max tokens

Hard ceiling on response length.

| Value | When |
|---|---|
| 200–400 | Short replies, chat format |
| 600–1200 | Default. Full scenes. |
| 2000+ | Long-form, narrative scenes |
| Unset | Model's choice (often 4096) |

### Penalties

All three lower the probability of repetition. Effects overlap — **don't stack all three at once**.

- **Frequency penalty** — penalises tokens proportional to how often they've appeared. `0.0`–`2.0`. `0.3–0.7` is normal for roleplay.
- **Presence penalty** — penalises **the fact of having appeared**, regardless of count. `0.0`–`2.0`.
- **Repetition penalty** — multiplicative penalty (local models, llama.cpp / koboldcpp). `1.0`–`1.3`. `1.0` = off.

**Tip:** if the model "loops," try `frequency 0.5` or `presence 0.3`.

### Seed

Pins the random source. Same `prompt` + `seed` + `temperature 0` → same output. Useful for debugging.

`-1` or unset — fresh seed every request.

### Stop strings

Strings that, when the model emits them, end generation. Example: `["{{user}}:", "\nUser:", "\n\n"]` — to stop the model from playing `{{user}}`'s line.

WuNest accepts an array of strings (`string[]`) or a single string.

---

## Instruct — for text-completion models

The distinction:

- **Chat-completion** (OpenAI, Claude API, OpenRouter chat) — the model already knows what `role:user` and `role:assistant` mean. No instruct wrappers needed.
- **Text-completion** (local llama.cpp, KoboldCpp, raw OpenRouter completion) — the model sees raw text. The instruct preset says "this is what end-of-user looks like, this is what start-of-assistant looks like."

### Fields

- **input_sequence** — inserted before each user message (e.g. `<|im_start|>user\n`)
- **output_sequence** — inserted before each assistant message
- **system_sequence** — wraps the system message
- **stop_sequence** — where the model should stop
- **wrap** — whether to glue everything into one string or keep newline separators

### When to touch it

Only when using a local provider via a `/completion` endpoint (BYOK with `base_url` pointing at kobold/llama.cpp/text-generation-webui). Cloud providers don't apply instruct.

### Stock templates

Most ST imports already include `ChatML`, `Llama3 Instruct`, `Mistral Instruct`. Import and apply — don't write from scratch.

---

## Context — history assembly format

The skeleton WuNest uses to build the final system prompt.

### Fields

- **story_string** — template with macros `{{description}}`, `{{personality}}`, `{{scenario}}`, `{{persona}}`, `{{wiBefore}}`, `{{wiAfter}}`, etc. This is "how the card is glued."
- **chat_start** — separator between the storyString and the message history
- **example_separator** — separator between `mes_example` blocks

### Default

The default `story_string` looks roughly like:

```
{{description}}

{{personality}}

[Scenario: {{scenario}}]

{{wiBefore}}

{{persona}}

{{wiAfter}}
```

Touch only for an exotic architecture (e.g. wrapping everything in `[INST]…[/INST]` for local models).

---

## Sysprompt — the system prompt text

Replaces the default "You are {{char}}, a chat companion…". The most-used preset type.

### When you need it

- **Genre guidance** — "This is noir. 1940s LA atmosphere."
- **Model role** — "You are a Game Master. React to {{user}}'s actions; don't roleplay {{char}} directly."
- **Writing style** — "Respond in third person, past tense, descriptive prose."
- **Jailbreak / refusal-removal** for NSFW etc. (at your own risk)

### Structure

The reliable shape:

```
You are {{char}}. {{char}}'s description, personality and scenario follow:
{{description}}
{{personality}}
{{scenario}}

[Format]
- Reply in third person, past tense.
- Use markdown for actions: *italics for actions, dialogue in quotes*.
- {{char}} is autonomous. {{user}} controls only their own character.

[Don't]
- Don't break the fourth wall.
- Don't speak or act for {{user}}.
```

### Post-history instructions

Separate field on the preset — `post_history_instructions`. Spliced at the very end of the prompt (after the full history). Use for hard reminders:

```
Stay in character. Do not speak for {{user}}. Reply in third person.
```

Effective when the model "drifts" toward the end of long chats.

---

## Reasoning — for thinking models

Modern thinking models (Claude 3.7+, o1, DeepSeek-R1) emit a hidden reasoning block before the answer. This preset describes how to delimit that block.

### Fields

- **prefix** — start marker (e.g. `<think>`)
- **suffix** — end marker (`</think>`)
- **separator** — what sits between thinking and the final answer

WuNest strips the thinking block from the visible response, keeping only the final part. The thinking itself lives in message metadata; access it via **long-press on a message → "Show reasoning"**.

### When to touch it

Only when using a thinking model and WuNest can't tell where reasoning ends (you see `<think>` in the visible reply). The default works for Claude and o1.

---

## Managing presets

**Library → Presets** — list grouped by type. For each preset:

- **Pencil** — open the form editor
- **Code-braces** — show raw JSON
- **Download** — export as ST-compatible JSON
- **Star** — make it "applied" for this type (used in every chat with no per-chat override)
- **Trash** — delete

### Per-chat override

In the chat drawer (`mdi-tune-variant` in the header), pick a preset for this chat specifically. Stored in `chat_metadata.sampler.preset_id`. Overrides the "applied" default.

### Import

**Presets → Import** accepts a JSON file. The type is auto-detected by signature keys:

- `input_sequence`, `output_sequence`, `wrap` → instruct
- `story_string`, `chat_start`, `example_separator` → context
- `temperature`, `top_p`, `max_tokens`, `frequency_penalty` → sampler
- `prefix`, `suffix`, `separator` (no sequence keys) → reasoning
- `openai_model`, `temp_openai` → openai legacy

When the detector isn't sure, a dropdown lets you choose manually.

### Soft validation

On save the server checks that `data` is a JSON object (not null/array/scalar) and that basic field types match (numbers are numbers, strings are strings, `stop` is `string[]` or a single string). Unknown fields pass through — ST extensions round-trip cleanly.

---

## Related

- [`byok`](byok) — your own API keys and custom base URLs for local models
- [`memory`](memory) — Author's Note and summary, applied on top of any preset
- [`characters`](characters) — a card's `system_prompt` field overrides the sysprompt preset
