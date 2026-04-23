# Characters

## Import formats

WuNest accepts two card formats:

- **PNG with a tEXt chunk** — the standard SillyTavern format. Metadata is base64-encoded under the `ccv3` key (V3) or `chara` (V2).
- **JSON** — ST exports and anything compatible. Three shapes supported:
  - V3 wrapper: `{"spec":"chara_card_v3","data":{...}}`
  - V2 wrapper: `{"spec":"chara_card_v2","data":{...}}`
  - V2 flat: `{"name":"...","description":"...",...}`

A leading UTF-8 BOM is stripped automatically.

## Importing

**Library → Import** — drop zone, accepts PNG or JSON. The backend sniffs the magic bytes (`89 50 4E 47` for PNG, `{` for JSON).

An embedded lorebook (`data.character_book.entries`) is promoted to a standalone lorebook and attached to the character automatically.

## CHUB library

**Library → CHUB library** — search public cards on chub.ai. Filters: language, gender, genre, purpose. NSFW has its own toggle. Pagination via Prev/Next arrows.

One-click import — we fetch the PNG, parse it, persist, and drop it into your library.

## Creating from scratch

**Library → Create** — three-section form:

- **Identity:** name, tags, avatar URL
- **Profile:** description, personality
- **Scene:** scenario, first message

Saved as a V3 `data` structure; round-trips to JSON without loss.

## Editing

Open a character to edit fields. Lorebooks are attached separately — via the "Lorebooks" icon on the card.
