# Characters

A character card is **the person you're talking to** — name, looks, personality, mannerisms, opening scene. The tighter the card, the less the model "drifts out of role."

This page is about **how to write** a card. Technical import details live at the bottom.

---

## Field map: what goes where

A V3 card (our default format) has eight semantic fields. They're stitched into the system prompt in a fixed order. Knowing "where what goes" prevents duplicating the same content twice.

| Field | What goes in | Length |
|---|---|---|
| **Name** (`name`) | Canonical handle. Substituted into every `{{char}}` macro. | 1 line |
| **Description** (`description`) | Looks, biography, surroundings, key relationships. **The main field.** | 200–800 words |
| **Personality** (`personality`) | Compressed character sheet — labels and traits. Not history, just brushstrokes. | 30–120 words |
| **Scenario** (`scenario`) | Where / when / under what conditions the dialogue takes place. Persistent setting. | 50–200 words |
| **First message** (`first_mes`) | The opening line the character delivers. Sets tone. | 50–300 words |
| **Example dialogue** (`mes_example`) | Sample exchanges in `<START>`-block format. Heavy lever for style. | 1–4 blocks |
| **System prompt** (`system_prompt`) | If set, **replaces** the default system prompt. Used rarely. | — |
| **Post-history instructions** (`post_history_instructions`) | Text inserted at the very end of the prompt. Hard reminders. | 1–3 lines |

> **Rule of thumb:** 90% of cards work with `description` + `personality` + `first_mes`. Everything else is opt-in.

---

## Description (`description`) — the heart

This is the field the model quotes most readily. Put everything that should be **permanently known** about the character here.

### What to write

- **Looks** — face, height, voice, clothes. Specifics, not "beautiful."
- **Biography** — who, where from, how they ended up here. 2–4 anchor points.
- **Habits and mannerisms** — how they walk, sit, look, what they repeat.
- **Relationships** — allies, enemies, who `{{user}}` is to them.
- **World context** — setting, era, tech level.

### Style

- **Present tense.** "Alice wears horn-rimmed glasses," not "wore."
- **Concrete, not evaluative.** "Sleeps in her coat with boots on" beats "messy."
- **Markdown works.** `## Looks` headings help both you and the model parse blocks.
- **Narrative style is yours.** Lists, prose, JSON-ish "fact: value" — all valid. Just stay consistent.

### Anti-patterns

- **Don't write history; write state.** "She lost her parents young, apprenticed to a smith, became a captain…" → drowns in tokens. Better: "Captain in her thirties, orphaned early, smith's burn-scar on her right hand."
- **Don't dupe `personality`.** If "hot-tempered" is here, leave it out of personality.
- **Don't write ad copy.** "The best friend you'll ever meet" plays as fake every time.

---

## Personality (`personality`)

Compressed labels for "how to behave."

### Format

Most reliable — **comma-separated**:

```
sarcastic, distrusting, soft on children, fears the dark, says "as you wish" instead of "yes"
```

Or **OCEAN big-five header**:

```
Openness: high (curious, loves the new)
Conscientiousness: middling (plans, then forgets)
Extraversion: low
Agreeableness: high but doesn't show it
Neuroticism: managed
```

Keep biography out — it lives in `description`. Just the **traits**.

---

## Scenario (`scenario`)

"When does this conversation happen?" Without this, the model invents a setting on the fly, often the wrong one.

### Good scenarios

- "Late night at `The Crow's Foot` tavern. {{char}} is at the bar, {{user}} just walked in from the cold."
- "{{user}} is a fresh recruit on {{char}}'s ship. First watch."
- "Letter #3 in an exchange between {{user}} and {{char}}. They've never met."

### Bad scenarios

- "They meet" — too generic; model defaults to clichés
- "Fantasy world" — that's setting, not scenario (goes in `description`)

---

## First message (`first_mes`)

The most important turn — it **sets the tone**. The model anchors all future replies to this style.

### What it should contain

1. **Action** — `{{char}}` is doing something, not just standing there.
2. **Sensory beat** — sound, smell, texture. At least one.
3. **Hook** — line, question or gesture that demands a response from `{{user}}`.

### Example

```markdown
*The bell over the door rings. {{char}} doesn't look up from her ledger.*

"We're closed," she says, pen still scratching. "Unless you've got something
worth my time, in which case the door swings the other way."

*She finally raises her eyes. Sharp. Grey. Tired.*

"Well? Which is it?"
```

### What doesn't work

- Long world exposition ("It was the year of the great drought…") — doesn't read, eats tokens.
- Full scene without a hook — model "completes" alone and loses the user.
- Many actions for `{{user}}` — breaks player agency.

### Alternate greetings

ST and WuNest support multiple `alternate_greetings`. Pick one when starting a chat. Useful when the same character has several typical entry points (work vs home; formal vs intimate). Open them from the character's library entry.

---

## Example dialogue (`mes_example`)

Most under-rated quality lever. The model **strongly** copies tone from this field.

### Format

Blocks separated by `<START>`. Each block is a mini-exchange between a generic `{{user}}` and `{{char}}`. WuNest normalises `<START>` regardless of casing or whitespace.

```
<START>
{{user}}: Where's that accent from?
{{char}}: *Smiles thinly.* You'll forgive me if I don't share. Old habits — they keep you alive.

<START>
{{user}}: Late again.
{{char}}: *Drops the bag. Doesn't apologise.* The street had questions.
```

### Why it matters

- Teaches the model **format**: italics for action, quotes for speech.
- Teaches the model **tone**: nudge "warmer / colder / sharper" with one edit.
- Often outperforms 200 more words in `personality`.

### Tip

2–3 blocks is enough. More wastes tokens, and the model starts copying lines verbatim.

---

## Macros: dynamic substitution

Any text field accepts macros. They expand at prompt-build time (the model sees the substituted value, not the placeholder).

| Macro | Expands to |
|---|---|
| `{{user}}` | Current persona name (`Library → Personas`) |
| `{{char}}` | Card's name |
| `{{random::a,b,c}}` | Uniform-random pick |
| `{{pick::a,b,c}}` | Alias of `random` |
| `{{roll::2d6}}` | Sum of 2 d6 rolls. Also `{{roll::d20}}`. |
| `{{time}}` | `HH:MM` of the request |
| `{{date}}` | `YYYY-MM-DD` |
| `{{weekday}}` | "Monday" / "Tuesday" / … |
| `{{idle_duration}}` | "3 hours" / "2 days" — gap since previous message |
| `{{lastUserMessage}}` | Most recent `{{user}}` line |
| `{{lastCharMessage}}` | Most recent `{{char}}` line |
| `{{getvar::name}}` | Per-chat variable; empty if unset |
| `{{setvar::name::val}}` | Writes a variable (expands to empty string) |

### Examples

```
*{{char}} glances at the window. It's {{time}} — {{weekday}}, naturally.*

You'll roll 2d6 plus your dexterity. {{roll::2d6}} — that's what fate handed you.

I haven't seen {{user}} for {{idle_duration}}. Long enough to wonder.
```

---

## Token budget

A baseline card (description + personality + scenario + first_mes) usually clocks in at **400–1200 tokens**. That's the floor of every request.

| Length | Profile | Where it works |
|---|---|---|
| 200–500 tokens | Minimalist | 5–20-turn chats on cheap models |
| 500–1500 tokens | Medium | Default. Good for 50+-turn roleplay. |
| 1500–3000 tokens | Detailed | Long-arc stories needing dense canon |
| 3000+ | Too dense | Model starts quoting the card verbatim |

**Long description? Move parts into a [lorebook](lorebooks).** Lorebooks fire on keys; they don't tax every turn.

---

## Import and formats

WuNest accepts cards in two formats:

- **PNG with `tEXt` chunk** — standard SillyTavern. Metadata in base64 under key `ccv3` (V3) or `chara` (V2).
- **JSON** — exports from ST or compatible tools. Supported shapes:
  - V3 wrapper: `{"spec":"chara_card_v3","data":{...}}`
  - V2 wrapper: `{"spec":"chara_card_v2","data":{...}}`
  - V2 flat: `{"name":"...","description":"...",...}`

A BOM at the start of the JSON file is auto-stripped.

**Library → Import** — drop zone for both PNG and JSON. The backend detects format by magic bytes.

An embedded lorebook (`data.character_book.entries`) becomes a standalone lorebook on import and gets attached to the character.

### CHUB

**Library → Browse CHUB** — search chub.ai. Filters: language, gender, genre, target. Separate NSFW toggle. One-click import: fetch PNG, parse, save.

### Creating from scratch

**Library → Create** — form with three sections (identity / profile / scene). Saved as V3, exported losslessly.

---

## Related

- [`lorebooks`](lorebooks) — for dynamic canon that shouldn't be in every turn
- [`personas`](personas) — `{{user}}` from the player's side
- [`presets`](presets) — generation params (temperature, top-p, sysprompt)
- [`memory`](memory) — Author's Note and auto-summary for long chats
