# Chat memory

In long chats the model forgets the beginning. WuNest gives you three parallel "memory" mechanisms. Use any one, or combine them.

All three live in the **chat drawer (`mdi-tune-variant` icon in the header) → Memory tab**.

| Mechanism | When it fires | What lands in the prompt |
|---|---|---|
| **Author's Note** | Unconditionally, every request | Free-form text in the position you pick |
| **Auto-summary** | When the prompt sends ≥ `threshold` tokens | A compact recap of older history |
| **Notes & pinned facts** | Unconditionally, every request | Manual "don't forget X" lines in the system `[Memory]` block |

---

## Author's Note

Free-form text **mixed into every request**. The main lever for genre nudges and quick fixes ("the model keeps doing X — tell it to stop").

In WuNest (post-M53) Author's Note can be edited in **two** places:

- **Memory tab** — primary editor (added in M53 for discoverability)
- **Generation tab** — same editor, handy when you're already there

Both stay in sync. Source of truth is `chat_metadata.authors_note`.

### Fields

| Field | Meaning |
|---|---|
| **Text** | Body of the note. Markdown works. Macros (`{{user}}`, `{{char}}`, `{{time}}`, …) expand. |
| **Depth** | How many messages from the end to splice it. `0` = right after the last user message. `1` = one message back. Etc. |
| **Role** | `system` (default), `user`, or `assistant`. Controls how the model "hears" the note. |

### Where it lands

At `depth = 0` the Author's Note is spliced **at the very end of the history, just before the model writes**. The loudest position — last thing the model sees.

At `depth = 1, 2, …` — N messages earlier. Useful for "persistent context, but not the very last word."

### Picking a role

- **`system`** (default) — hard rules and instructions. "This is noir. Don't break the fourth wall."
- **`assistant`** — in-character guidance. "Remember Alice's left leg is broken." Reads like the character's own insight.
- **`user`** — rarer. For a "fake" user reminder ("by the way, I have no money") — the model takes it as canon.

### What to put in

- **Hard scene constraints** — "This is grim noir, no jokes," "Never break character"
- **Long-running facts** that won't fit the card — "Alice's left leg is broken, she uses a cane"
- **Writing style** — "Reply in Hemingway style: short sentences, no flourishes"
- **Genre and tone** — "This is RPG in Disco Elysium style: inner thoughts in italics"

### What NOT to put in

- **Changing facts** — you'd edit the note every turn. Use a [lorebook](lorebooks).
- **Big text blocks** — this goes into **every** request. Token budget evaporates.
- **Character biography** — that lives in the card.

### Token budget

200–400 chars ≈ 50–100 tokens. Per request. On a long chat — tens of thousands of tokens cumulatively. Keep it terse.

---

## Auto-summary (rolling)

**Memory tab → Auto-summary**.

Toggle ON → when a request's prompt hits ≥ `threshold_tokens`, a background LLM call collapses older history into a **summary** and mixes it into the system prompt.

### How it works

1. After each successful response, `tokens_in` (input tokens of the last request) is measured.
2. If `tokens_in >= threshold_tokens`, a goroutine calls the chosen summary model with `[Previous summary] + [recent messages] → updated summary`.
3. The summary is saved with role `auto`. It overwrites the previous one.
4. On the next request the summary is spliced into the system block as `[Memory] / ## Rolling summary of earlier events`.

### Parameters

| Field | Meaning |
|---|---|
| **Enabled** | On/off |
| **Threshold (tokens)** | At what `tokens_in` volume to fire. Default ~2000–4000. Too low → every turn. Too high → summary updates rarely. |
| **Model** | Summary model — separate from your main. Pick something cheap (Gemini 2.5 Flash, Haiku, gpt-4o-mini). |
| **BYOK** | Use your own key for the summary call instead of the WuApi pool. |

### Per-chat mutex

One background summary per chat at a time. Trigger that fires while one is running gets dropped (the running one will catch up).

### Retry-and-silent-fail

If the summary call errors (rate limit, network) — back off 5s → one retry → fail-silent in slog. We don't spam toasts and we don't auto-disable the feature.

### When it helps

- Chat >50 turns on an 8K-context model — without summary the model forgets the start.
- Long roleplay where "remember our deal back in turn 5" matters.
- Cost-saving: cheap Haiku compresses, expensive Sonnet runs on the compressed history.

### When you don't need it

- Up to ~30 turns on 1M-context models — the window isn't even close to full.
- Single-turn tests (trying out a card).
- When verbatim memory of every turn matters — summaries are lossy by definition.

---

## Notes and pinned facts

**Memory tab → Notes**.

Free-form notes you write yourself; **always** spliced into the prompt. The auto-summariser doesn't compress them.

### Two kinds

| Role | Where in the prompt | Use for |
|---|---|---|
| **Pinned** (starred) | `[Memory] / ## Key facts (always-on)` block — loudest spot | Lifelong canon |
| **Manual** | `[Memory] / ## Notes` block — after the summary | Semi-canon, episodic reminders |

Both fire **every request**, no condition. Unlike a lorebook, where an entry waits for a key.

### When to use which

- **Pinned** — "Morgan has only one leg," "Set on Mars," "Alice's daughter is named Lina." Lifelong canon.
- **Manual** — "They had a fight recently," "It's been raining for three days." Current circumstances, may change.

### Where in the UI

In the Memory tab:

1. **Pinned** — separate panel (with a star)
2. **Manual** — list below
3. **Add** — input with a "regular / pinned" toggle

Edit, re-pin (star), delete. Pinned always sit above manual in the assembled prompt.

### Interaction with auto-summary

When the auto-summary fires, it **sees** your manual/pinned notes (they're part of the summarisation template). But it **doesn't rewrite** them: your notes stay verbatim, the auto-summary lives in parallel.

If you edit the auto-generated summary, its role flips to `manual` so the next auto run won't overwrite your edits.

---

## What to use when

| Goal | Use |
|---|---|
| "Don't break character" | Author's Note (`system`, depth 0) |
| "Remember this canon forever" | Pinned note |
| "Alice has a cat named Murka, may come up" | Lorebook entry with key `Murka, cat` |
| "Character always wears a dark coat" | Card → description |
| "Chat got long, trim the tokens" | Auto-summary + cheap summary model |
| "It's raining now" | Manual note (edit when weather changes) |
| "Roll a d20 for a check" | Macros in card (`{{roll::d20}}`) |

---

## Token cost

| Mechanism | Typical size | Tokens/turn |
|---|---|---|
| Author's Note (200 chars) | small | ~50 |
| Auto-summary (1K chars) | medium | ~250 |
| Pinned facts (5×100 chars) | small | ~125 |
| Manual notes (10×100 chars) | medium | ~250 |

All four together usually add **+500–1000 tokens** per request. Invisible on 1M-context models; noticeable on 8K (old GPT-4, Claude Haiku 1).

---

## Related

- [`lorebooks`](lorebooks) — **conditional** memory (fires on keyword match)
- [`characters`](characters) — permanent traits belong in the card
- [`presets`](presets) — `post_history_instructions` is a separate channel for hard reminders
