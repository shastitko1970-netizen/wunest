# ST → WuNest converter

`/converter` is an LLM-helper that takes your SillyTavern theme and rewrites it into WuNest-native form: ST selectors (`.mes`, `.mes_block`, `#chat`) become our classes (`.nest-msg`, `.nest-msg-body`, `.nest-chat-scroll`), rules targeting ST-only containers (`.drawer-content`, `#expression-image`) are dropped, variables get normalised.

## When to use it

- You imported an ST `.json` and the theme looks broken (avatars not round, swipe buttons in odd places)
- You grabbed a pretty `.css` from Discord/CHUB and want to apply it as a WuNest theme
- You're authoring a theme yourself and want to auto-rewrite selectors before publishing

If the theme **already uses** `.nest-*` classes or `--SmartTheme*` variables, you don't need the converter — **Appearance → Import** works directly.

## Inputs

Two modes via tabs at the top:

### File
`.json` or `.css` file, drag-and-drop or click. Max 500 KB.

- **`.json`** — full ST theme export (`{name, main_text_color, ..., custom_css}`). Sent to the LLM as-is.
- **`.css`** — bare CSS without the JSON envelope. The frontend auto-wraps it as `{name: <filename without ext>, custom_css: <bytes>}` before submit.

### Paste text
Monospaced textarea. Auto-detect:
- First non-whitespace char `{` → JSON. `JSON.parse` validates on the frontend — malformed JSON blocks submit (saves a 30-180s LLM wait on garbage)
- Otherwise → CSS. Auto-wrapped as `{name: 'Untitled CSS', custom_css: <text>}`

Same 500 KB cap as file mode.

## Model & source

### Source
**WuApi pool** or **BYOK** (your own key). For light themes (50-200 lines of CSS), a cheap model is enough. For 1000+ line boilerplate, prefer Claude Sonnet / GPT-4o / Gemini Pro — they miss fewer edge cases.

### Model
The picker loads the catalogue per source:
- WuApi pool → all models on your tier (Sonnet 4.5, GPT-4o, Wu-named tunes)
- BYOK → your provider's catalogue (OpenAI / Anthropic / etc.)

## Conversion

The **Convert** button kicks off a stream to the selected model. **Tokens are billed against your key** (or wu-gold for WuApi pool):
- Input: ~5K tokens for the system prompt + your theme
- Output: rewritten JSON, usually similar size

The LLM call runs in a **detached context** server-side — even if you close the tab, the conversion finishes and the result lives 24h.

## Limits

- **3 conversions per hour** per user (includes retries and errored jobs — errors count to deter spam)
- 500 KB max input
- 180s LLM call timeout (hard)

On rate-limit you get 429, the UI shows a countdown to next try.

## Result preview

After a success — the "What's inside" block:
- **Selectors rewritten to WuNest style: N** — count of `.nest-*` matches in the output `custom_css`. 0 = suspicious (the model probably echoed the input back)
- **First 30 lines of CSS** — visual snippet to eyeball. `<pre>` with monospace font, max-height 280px scrollable
- **Notes** (`_converter_notes`) — what the model did: "Replaced `.mes` with `.nest-msg` in 12 places", "Removed `.drawer-content` rule (ST-only)"

## Actions

- **Download .json** — saves the result as `wunest-theme-<hash>.json`. Share, commit to Git, send to a friend.
- **Apply to me** — merges the result into your `Appearance.customCss` via the same path as manual import (`fromST`). Before apply, a runtime guard `isPlausibleSTTheme` checks the shape — if the result doesn't look like a valid ST theme (field types off), Apply is blocked with a warning but Download still works.
- **Try another model** — opens a picker dialog with the current model + source pre-selected. Confirm → POST `/api/convert/{id}/retry` with the same input + a new model. Counts toward the hourly limit.

## Recent conversions strip

Below the result — the last 24h of jobs. Per row:
- Status icon (green = done, red = error, yellow = running)
- Model + age (`5m ago` / `2h ago`)
- Download button (done only)
- Retry button (done or error) — opens the same model picker

After 24h the job is auto-deleted by the reaper, the link stops working. If you need it permanently, download the `.json` while it's live.

## What makes a "good" ST input

If the converter struggles with your input, it's usually one of:

1. **Too many `body` rules** — aggressive `body { background: ...; color: ... }` in the ST theme confuses the model about how to rewrite. Better to go through `:root { --SmartThemeBlurTintColor: ... }`.
2. **JS-only functionality** — ST plugins using `window.extension_settings.*`. We don't have that API, can't be rewritten. Drop those rules.
3. **Very long themes (>1000 lines)** — even strong models start skipping in the middle. Slice into chunks and run each separately.

## See also

- [`theming`](theming) — how to write a theme by hand from scratch
- [`safe-mode`](safe-mode) — what to do if the applied theme broke the UI
