# Presets

A preset is a saved bundle of generation parameters that you can apply to any chat or set as your default.

## Types

WuNest has five preset types (SillyTavern-compatible):

- **Samplers** — temperature, top-p, top-k, min-p, max_tokens, penalties, seed, stop strings.
- **Instruct** — turn wrappers: `input_sequence`, `output_sequence`, `system_sequence`. For text-completion models.
- **Context** — `story_string` with `{{char}}`, `{{user}}`, `{{scenario}}` macros; controls how history is formatted.
- **System prompt** — text that replaces the character's default system prompt. Supports post_history_instructions.
- **Reasoning** — prefix/suffix/separator for `<think>` blocks. For o1, Claude thinking, DeepSeek-R1.

## Management

**Library → Presets** — list grouped by type. For each preset:

- Pencil — open the editor
- Code-braces — show raw JSON
- Download — export as ST-compatible JSON
- Star — make it the default for this type (used when a chat hasn't picked one)
- Trash — delete

## Import

**Presets → Import** accepts a JSON file. The type is auto-detected via signature keys:

- `input_sequence`, `output_sequence`, `wrap` → instruct
- `story_string`, `chat_start`, `example_separator` → context
- `temperature`, `top_p`, `max_tokens`, `frequency_penalty` → sampler
- `prefix`, `suffix`, `separator` (without sequence keys) → reasoning
- `openai_model`, `temp_openai` → openai legacy

When the detector's unsure, an expansion panel lets you override the type manually.

## Attaching to a chat

In the generation-settings drawer (`mdi-tune-variant` in the chat header), pick a preset from the dropdown. The pick persists into `chat_metadata.sampler.preset_id` and applies on every turn.

You can also mark a preset as your user default — the star button in the list. The default fires whenever a chat hasn't explicitly picked one.

## Soft validation

On save the server checks that `data` is a JSON object (not null, array, or scalar), and that basic field types match (numbers are numbers, strings are strings, `stop` is `string[]` or a single string). Unknown fields pass through — so ST-specific extensions don't break imports.
