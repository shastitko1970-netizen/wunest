# Interactive plates (plate actions)

Character authors can embed clickable buttons in messages via
`data-nest-action` — without JavaScript and without XSS risk. WuNest
scans the message DOM and attaches handlers only for actions in the
allow-list.

## How it works

Add `data-nest-action="name"` to any `<button>` (or any element) in
your message HTML. Pass extra parameters via `data-nest-*` attributes.
No `onclick` or `<script>` — those get stripped by DOMPurify
regardless.

```html
<!-- Simple — copy to clipboard -->
<button data-nest-action="copy" data-nest-text="Hello, world!">📋 Copy</button>

<!-- Dice roll -->
<button data-nest-action="dice" data-nest-dice="2d6">🎲 Roll 2d6</button>
```

## Available actions

### Toast-style (confirmation feedback)

| Action | Params | What it does |
|---|---|---|
| `copy` | `data-nest-text` (literal) or `data-nest-target` (CSS selector) | Writes to clipboard; defaults to the trigger's innerText |
| `dice` | `data-nest-dice="2d6"` or `data-nest-sides="20"` | Rolls dice; result in toast or in `data-nest-result-to` selector |
| `reroll` | Same as `dice` | Alias |

### Message-context (bubble up)

| Action | What it does |
|---|---|
| `swipe-prev` | Go to previous swipe of this message (handy for alternate_greetings) |
| `swipe-next` | Next swipe |
| `regenerate` | Re-generate this message (only for the latest assistant turn) |
| `edit` | Open edit mode |
| `delete` | Remove the message |

### Composer (inject text)

| Action | Params | What it does |
|---|---|---|
| `say` | `data-nest-text` or innerText | Fills the input box (user can edit & send) |
| `send` | `data-nest-text` or innerText | Fills and sends immediately |

### Local DOM toggles

| Action | Params | What it does |
|---|---|---|
| `toggle-attr` | `data-nest-target`, `data-nest-attr="hidden"` | Toggles an attribute on the target |
| `toggle-class` | `data-nest-target`, `data-nest-class="expanded"` | Toggles a class |

## Complete examples

### Action menu

```html
<div class="char-actions">
  <button data-nest-action="say" data-nest-text="I step back and listen.">
    👂 Listen
  </button>
  <button data-nest-action="say" data-nest-text="I draw my blade and advance.">
    ⚔️ Attack
  </button>
  <button data-nest-action="say" data-nest-text="I try to flee.">
    🏃 Flee
  </button>
</div>
```

### Stat block with a roll

```html
<div class="stat-panel">
  <h3>Dexterity check</h3>
  <p>Mod: +3, DC 15</p>
  <button data-nest-action="dice" data-nest-dice="1d20" data-nest-result-to="#roll-result">
    🎲 Roll d20
  </button>
  <div id="roll-result" class="roll-result">(press to roll)</div>
</div>
```

### Copy a stat block

```html
<div id="stat-block">
  <p>HP: 50/50 · MP: 30/30 · STR 14 · DEX 16</p>
</div>
<button data-nest-action="copy" data-nest-target="#stat-block">📋 Copy stats</button>
```

### Collapsible without `<details>`

```html
<button data-nest-action="toggle-attr" data-nest-target="#lore-extra" data-nest-attr="hidden">
  📖 Toggle extras
</button>
<div id="lore-extra" hidden>
  <p>Extra lore…</p>
</div>
```

### "Next greeting" button

```html
<button data-nest-action="swipe-next">→ Another greeting</button>
```

## Safety and limits

- `<script>`, `onclick=`, `javascript:` and friends are always removed
  by DOMPurify — no bypass
- Action whitelist is fixed; unknown names fail silently
- `data-nest-target` is a regular CSS selector — scoped to the
  same message subtree
- Bubble-up actions (`swipe-next` etc.) only work on assistant
  messages with that capability
- Dice rolls are client-side (`crypto.getRandomValues`) — nothing
  goes to the server

## For more complex interactivity

Full JavaScript (state machines, API calls) is intentionally not
supported and isn't planned — too much XSS surface. Use the
documented whitelist, and for state use CSS-only techniques
(`:checked ~ sibling`, `:target`, `<details>`) — they work without
any opt-in.
