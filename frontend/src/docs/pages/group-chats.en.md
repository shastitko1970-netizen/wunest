# Group chats

WuNest supports group chats — multiple characters in one conversation. Useful for scenes with pairs / triples of characters where each should speak in their own voice, instead of one character playing every role.

## Creating one

**Library → "Group chat" button** opens GroupChatSetupDialog:

1. Pick 2+ characters (multi-select)
2. Optional chat name (defaults to "Group: Alice, Bob, Cyril")
3. Rotation style:
   - **Round-robin** — each in turn
   - **Manual address** — the model only replies on @-mention of a specific character
   - **Auto** — the model decides based on context (needs a stronger model)

After creation — standard chat view, with extra mechanics.

## What changes in chat

### Several `assistant` messages in a row
Each turn the model replies as **one** character from the roster. Attribution lives in `Message.character_id` — the UI shows the avatar + name of who's speaking.

### Per-character swipes
When you regenerate, swipes are pinned to that specific character — `swipe_character_ids` runs parallel to `swipes`. So if Alice has 3 alternative replies, swiping doesn't switch the speaker.

### Addressing
The `{{char}}` macro in a group chat expands **based on who's replying this turn**. If Alice's card has "{{char}} notices {{user}} in the corner" — that expands with Alice's name.

### Per-character lorebooks
Every character in the group brings their own lorebook (if attached). All of them activate into the shared prompt — ordering is by InsertionOrder, same as single chats.

## Limits & gotchas

- **Cost scales linearly** with the number of participants — more system context = more tokens per turn
- **Streaming is fast** only for one persona per turn — don't try to "have everyone reply at once", at best the model picks one, at worst it mixes voices
- **Context window** — several cards + several lorebooks easily eat 10-20K tokens of system prompt alone. For long group scenes you want a model with 128K+ window

## Removing a participant

Chat settings drawer → Tags lists participants. You can remove — `character_ids` updates in the DB. But **past messages remain** — their `character_id` references the removed participant. The UI will render them with a placeholder avatar.

## See also

- [`characters`](characters) — creating / editing the cards themselves
- [`personas`](personas) — `{{user}}` substitution works identically in group and single chats
- [`lorebooks`](lorebooks) — recursive keyword activations
