# Lorebooks (World Info)

A lorebook is **conditional canon**. Instead of stuffing the whole universe into the character card (where it eats tokens every turn), you split it into **entries** that load only when mentioned.

If the character card answers "**who** is in front of you," a lorebook answers "**what** is in this world and **when** does it become relevant."

---

## How it works

On every request, the activator:

1. Scans the last N messages (see **depth**).
2. For every entry in attached lorebooks, checks whether its **key** falls inside the scan window.
3. If yes — splices the entry's content into the system prompt.
4. Runs up to **3 recursive passes** — content from activated entries expands the scan window (unless the entry is flagged `exclude_recursion`).

The model ends up seeing card + activated entries + message history.

---

## When to use a lorebook (vs the card)

| Goes in card | Goes in lorebook |
|---|---|
| Name, looks, voice | Places the character occasionally passes through |
| Core relationships (intimate) | Secondary NPCs, names |
| Habits and mannerisms | Items / artifacts (only when mentioned) |
| Canonical backstory (1-2 anchors) | Detailed world history, factions, events |
| Things to know **always** | Things to know **sometimes** |

Heuristic: if an entry fires less than 1 turn in 5, it belongs in a lorebook.

---

## Creating

**Library → Lorebooks → New** or **Import** for ST JSON files. Both array (`entries: [...]`) and object (`entries: { "0": {}, "1": {} }`) shapes are supported.

After creation, attach the book from a character card: **Library → Characters → card → Lorebooks icon** → multi-select. Only attached books activate for that character.

---

## Entry: fields and meaning

### Keys

OR-matched against the scan window. Any single hit activates.

#### Writing keys

- **A key is a token, not a concept.** `king` won't fire on `kingdom`. If both matter, add both.
- **Include variants.** `Alice, Alice's, Alicia` — the model spells the name several ways.
- **Don't over-split.** If 90% of the time the model writes "Morgan", a single `Morgan` key is enough.
- **Avoid super-generic keys.** `he`, `said`, `time` will fire on almost every turn — entry might as well live in the card.

#### Whole words (`match_whole_words`)

Without the flag, `cat` fires on `concatenate`. With it, the boundary is enforced — but `cat` also stops firing on `cats`. Turn it on for short keys that often appear as substrings of unrelated words.

### Content

Text spliced into the system prompt when the entry activates.

#### Style

- **Third person.** "Morgan is a smith…", not "I'm a smith…".
- **Specifics.** "Wields a hammer with a redwood haft" beats "great smith."
- **Brevity.** 50–250 words per entry. Beyond that, split into two.
- **No plot.** A lorebook is a reference, not a chapter. The chapter is the chat.
- **Markdown works.** `## Looks` headings, lists.

#### Length

Reasonable target — total activated content stays under ~30% of the card's length. Otherwise the entries start drowning out the character.

### Selective + secondary keys

When the **"Requires secondary key"** flag (`selective`) is on, both primary AND secondary keys must match. Useful when the primary key is too generic:

- Primary: `Morgan`
- Secondary: `past, history, youth, parents`

The entry "backstory of Morgan" then only fires when **the past** is being discussed — not on every name drop.

### Position

Where the activated content lands in the final prompt:

| Position | Where |
|---|---|
| `before_char` | Before the character description (default). Good for world / context. |
| `after_char` | After the description. For character-tied clarifications. |
| `at_depth N` | N messages back from the end. For "remind me right before the model thinks." |
| `before_an` / `after_an` | Around the Author's Note. Fine-grained priority tuning. |

All four positions **work in v1**. Need an entry to land "right before the model thinks"? Use `at_depth 1`.

### Depth

How many recent messages are scanned for keys. `0` falls back to the book-level default (usually 4). Larger = longer key memory, but more false positives.

### Insertion order

Lower = earlier in the prompt. Ties break on book index → entry index. Useful when several entries share a topic and order matters.

### Probability

Random gate from 0 to 100.

- `0` or `100` — always fires (when keys match)
- `1`–`99` — random roll: fires with probability N%

Useful for variation (e.g. weather flavor variants).

### Group

Group name (any string). Within one group, **at most one entry per pass fires** (lowest `insertion_order` wins).

#### When to use

- Multiple "tavern greeting" variants — one fires, others suppressed.
- Alternative descriptions of the same object.
- Weather / time-of-day flavor.

#### Bypass group cap (`group_override`)

A flag on the entry. When on, the entry fires regardless of the group cap (as if it had no group).

### Constant

Entry fires **every turn**, no key check. Good for "always remind X." Effect-wise like Author's Note, but tied to the lorebook → travels with the character.

### Recursion

The activator runs up to **3 recursive passes**: content from activated entries gets fed back into the scan window so related entries can fire.

- **No outbound recursion** (`exclude_recursion`) — this entry's content is NOT added to the next pass's scan window. Useful for flavor text where mentioned names shouldn't cascade.
- **No inbound recursion** (`prevent_recursion`) — this entry only fires on the initial history scan, never because another entry fired. Cascade guard.

### Case sensitive

`Alice` ≠ `alice` when on. Off by default.

---

## Advanced fields (not applied in v1)

The entry editor has a collapsed "Advanced" section. Fields are **stored** with the entry (and round-trip to JSON), but the **v1 activator ignores them**:

- **Sticky** (turns) — "stay active for N turns after first activation"
- **Cooldown** (turns) — "min N turns between activations"
- **Delay** (turns) — "skip the first N turns"

These three need state across turns (per-entry counters), which v1 doesn't keep. The UI marks the section with a "not applied in v1" badge. Implementation is on the roadmap.

---

## Importing from ST

The ST JSON format is compatible. Both array and object `entries` shapes are accepted. Fields that ST understands but WuNest doesn't yet (book-level `recursive_scan` toggle, e.g.) are silently dropped.

Importing a PNG card auto-promotes its embedded `data.character_book` into a standalone lorebook attached to that character.

---

## Pattern: typical layouts

### World reference

One lorebook, ~20 entries: factions, cities, artifacts. Attach to every character in that universe.

### Character-folio

One lorebook per character (even if you have a single card). 5–10 entries: backstory, secondary NPCs tied to them, locations they own. The card is "here and now"; the book is "everything else."

### Game mechanics

Entries fire on keys like `roll, check, initiative, save`. Content is rules. Useful for "I want a D&D chat, but I don't want rules in the card."

---

## Related

- [`characters`](characters) — what goes in the card vs the lorebook
- [`memory`](memory) — Author's Note for unconditional memory; lorebook for conditional
- [`group-chats`](group-chats) — group chats load lorebooks from **all** participants
