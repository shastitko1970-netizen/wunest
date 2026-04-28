# Personas

A persona is **you in the chat** — the name, description and avatar that substitute for `{{user}}` in the character card and the system prompt.

If a card answers "**who is in front of you**," a persona answers "**who are you**."

---

## Why bother

Cards often reference `{{user}}`: "`{{user}}` is in the doorway," "`{{user}}` is a detective on duty." Without a persona, `{{user}}` is your WuApi `first_name` — which usually sounds out of place ("Morgan stands in the doorway… and Morgan's name is Ivan? Ivan's a detective?").

Set up "Alex — detective, 32, cynical" — and `{{user}}` in any chat is now Alex. You can keep several personas for different genres: a detective for crime, an elf for fantasy, a neutral "you" for contemporary.

---

## Where they live

**Library → Personas** — a separate tab in the library.

Each persona stores:

- **Name** — what gets substituted into `{{user}}` and `{{user_name}}`
- **Description** — short bio / character text. Mixed into the system prompt.
- **Avatar** — optional URL or uploaded image. Used on "your side" of the chat.
- **Default flag** — one persona can be the default, used in every new chat with no explicit pick.

---

## What to put in the name

Easy case — your character's actual operating name.

| If | Name |
|---|---|
| One protagonist across genres | Universal: `Morgan`, `Lex`, `River` |
| Genre-split | Tie to genre: `Detective Carver`, `Talieth the Elf` |
| Nameless observer | `Stranger`, `Traveler`, `you` (careful — model may get confused) |

A few rules:

- **Don't put `{{user}}` as the name** — recursive substitution.
- **Use a name the model can quote easily** — short, no diacritics, no emoji.
- **If the name already appears in the card (`scenario`, `description`), match it.** Otherwise the model mixes "{{user}}" and "that name from the scene."

---

## What to put in the description

The persona's description is **your character sheet**. It mixes into the system prompt at build time (much like the character description, but on the player side).

### Good descriptions

- **Concrete and short.** 2–6 lines. Longer is a card.
- **Looks + one personality beat.** The model just needs to know what you look like and one anchor trait.
- **No long biographies.** Lore drifts into the chat naturally as you talk.

### Example

```
Alex — detective, 32. Short, in a battered coat
that never comes off, even at the precinct.
Cynical, but always trusts kids. Smokes when nervous. Left-handed.
```

### What NOT to put in

- **Plot.** "Investigating his wife's disappearance" — that's a scene, not a persona. Personas are persistent.
- **Relationships with specific characters.** That goes in the card or lorebook.
- **Massive biographies.** Every chat will spend the same tokens on it.

---

## Avatar

Optional. If set, shows on your messages in chat. You can upload a file (PNG/JPG, up to 5MB) or supply a URL.

Uploaded files land in WuNest's MinIO storage and get a public URL.

URLs are used as-is, no proxy.

> **Privacy:** avatars are public (URL-accessible without auth). Don't put sensitive imagery there.

---

## Default persona

You can mark **one** persona as default (star button in the list). Resolution order:

1. **Per-chat override** (`chat_metadata.persona_id`) — wins if a chat has explicitly picked.
2. **User's default persona** — when no override.
3. **WuApi `first_name`** — fallback when neither is set. `{{user}}` becomes your WuApi name with no description.

---

## Pinning to a chat

Each chat can pin its own persona via **chat header → profile icon** or **Chat settings drawer → Persona**.

Changes apply from the next message. Already-sent messages keep the name they had at send time.

---

## Macros

You can use these inside the card and the system prompt:

- `{{user}}` — current persona's name
- `{{user_name}}` — alias of `{{user}}` for ST compat

These expand at prompt-build time — the model sees the concrete name, not the placeholder.

`{{user.description}}` is **not implemented** as a macro. The description still mixes into the system prompt automatically, but through a different mechanism.

---

## ST import

There's no direct importer: ST keeps personas in the `power_user.personas` segment of `secrets.json`, which isn't a clean JSON table. Most users have 1–3 personas, not 50 — copy them by hand via **Library → Personas → Create**.

---

## When you don't need a persona

- **First-person chat with no name.** If all replies are `{{char}}`'s inner monologue and `{{user}}` rarely shows up, the WuApi-name fallback works.
- **One-off card test.** Spinning up a card to see how it sounds — persona's not critical.
- **Cards that don't reference `{{user}}`.** Rare, but exists (e.g. narrator cards).

---

## Privacy

Personas are stored on the server as part of your user settings. Access — yours only. They are **not** included in chat exports (JSONL): `{{user}}` in the export stays as it was at send time, with no back-reference to the persona.

When you delete the account, personas are deleted with the rest of your user data.

---

## Deletion

Deleting a persona doesn't touch chats pinned to it — `chat_metadata.persona_id` becomes an orphan. On the next message, the resolver sees the persona's gone and falls back to default or WuApi name. No noise, no errors.

---

## Related

- [`characters`](characters) — the character card: "who is in front of you"
- [`memory`](memory) — Author's Note for hard reminders to the model
- [`getting-started`](getting-started) — your first chat in 5 minutes
