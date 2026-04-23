# Lorebooks (World Info)

A lorebook is a set of entries dynamically spliced into the system prompt when their keys show up in the conversation.

## Creating & importing

**Library → Lorebooks → New** or **Import** for SillyTavern JSON files. Both array (`entries: [...]`) and object (`entries: { "0": {}, "1": {} }`) shapes are supported.

## Entries

Each entry has:

- **Keys** — OR-matched. Any single hit activates.
- **Secondary keys** — only consulted when the "Requires secondary key" (selective) flag is on. Then both primary AND secondary must match.
- **Content** — the text spliced into the prompt.
- **Position** — before or after the character card. `at_depth`, `before_an`, `after_an` are also persisted for ST round-trip but not yet enforced by the activator.

## Activation controls (M21)

- **Always active** — fires every turn, no keys needed.
- **Probability** (0-100) — random gate. 0 or 100 = always, 1-99 = random roll.
- **Whole words** — word-boundary matching. `cat` won't fire on `concatenate`.
- **Case sensitive** — respect case when comparing.
- **Group** — within a single group, only one entry fires per pass (lowest InsertionOrder wins). Useful for variant greetings.
- **Bypass group cap** — override flag; the entry always fires even when its group is "claimed".

## Recursion

- **No outbound recursion** (`exclude_recursion`) — this entry's content is NOT added to the next recursion pass's scan window. Useful for flavor text that shouldn't cascade.
- **No inbound recursion** (`prevent_recursion`) — the entry only fires on the initial history scan, never as a result of another entry activating.

The activator runs up to 3 recursive passes and stops early when a pass adds nothing.

## Order & depth

- **Order** (InsertionOrder) — lower first in the prompt. Ties break on book index → entry index.
- **Depth** — how many recent messages are scanned for keys. 0 = book-level default (usually 4).

## Character attachment

From the character card → "Lorebooks" icon → multi-select. At generation time only attached books contribute to the scan.
