# Theming & CSS

WuNest supports **four** layers of customization:

1. Built-in presets (5 of them)
2. UI toggles under Appearance
3. ST-compatible CSS variables
4. Your own CSS + scope

> See the 5 presets side-by-side → [theme gallery](/themes) — public page, shareable link.

---

## Built-in presets (M42)

**Settings → Appearance → Theme** — five curated themes:

| Preset | Kind | Description |
|--------|:----:|-------------|
| **Nest — dark** | 🌑 | Flagship dark (charcoal + coral accent) |
| **Nest — light** | ☀️ | Paper-light, Dossier-CRM inspired |
| **Cyber neon** | 🌑 | Deep purple + magenta glow |
| **Minimal reader** | ☀️ | Max text density, no decoration |
| **Tavern warm** | 🌑 | Warm amber, roadhouse vibes |

Quick **light ↔ dark** toggle lives in **Settings → Theme** (top section). Pair logic remembers which dark and light preset you actually picked: cyber-neon ↔ minimal-reader round-trips without losing your pick.

---

## Quick settings (UI)

**Settings → Appearance**:

- Font size (0.75× — 1.4×)
- Chat width (40–100%)
- Avatar shape (round / square)
- Message style (bubbles / flat / document)
- Accent color, background, text, borders
- Background image + blur
- Shadows, reduced motion, HTML rendering in messages

Everything here writes CSS custom properties to `:root` inline — applies instantly, no reload.

---

## Import & export

### SillyTavern-compatible JSON

**Appearance → Import ST theme (.json)**. Extracted from the file:

- **Colors**: `main_text_color`, `italics_text_color`, `quote_text_color`, `border_color`
- **Sizes**: `font_scale`, `chat_width`, `blur_strength`
- **Styles**: `avatar_style`, `chat_display`, `noShadows`, `reduced_motion`
- **`custom_css`** — applied as your custom CSS

Scope auto-sets to `chat` — the ST theme's CSS won't break the menu. If the theme contains rules for broad elements (`body`, `textarea`, `input`), we show an info notice.

### Raw CSS (.css)

**Appearance → Import .css file** — loads a bare CSS file directly (M42.4). Handy when themes travel through the community as plain `.css`. Scope auto-set to `chat` too.

**Appearance → Export as .css** — saves your current custom CSS as a standalone `.css`. Easy to share in Discord/forums. Filename derived from the `/* Name: ... */` comment if present.

---

## Custom CSS — full guide

**Appearance → Custom CSS** — hand-written code. Full CSS syntax supported, including `@import`, `@font-face`, `@media`, `@supports`, nesting, `:has()`, `@scope`, etc. Saved to the server (`user.settings.custom_css`) with a 400 ms debounce and injected on load as `<style id="nest-user-css">` in `<head>`.

### Philosophy: token-first

**Main idea:** don't fight variables — override them.

WuNest is built on CSS custom properties. Every color in the UI is a `var(--nest-something, fallback)`. If you change a variable at `:root`, **the whole shell** re-paints automatically, without selectors, without `!important`, without fragility.

Compare:

```css
/* ❌ Fragile: breaks on every refactor */
.nest-msg-body .nest-msg-content p {
  color: #c485ff !important;
}

/* ✅ Stable: works forever */
:root { --SmartThemeBodyColor: #c485ff; }
```

The first approach breaks when we rename a class or add a wrapper. The second — survives every refactor as long as the variable contract lives.

### Scope — "Apply to"

Under the textarea — a two-mode toggle:

| Mode | What it captures | When to use |
|---|---|---|
| **Chat only** (default) | `#chat` and descendants | ST themes, trials, rules for `body`/`textarea`/`input` won't touch menus |
| **Whole app** | The whole shell | `.nest-*`-based themes, when you really need to repaint topbar, sidebar, dialogs |

**How "Chat only" works:**

On modern browsers (Chromium, Safari 18+) your CSS is wrapped in native `@scope (#chat) { ... }`. On Firefox a manual prefixer runs: every rule gets prefixed with `#chat `. Behaviour is identical in 95% of cases.

Differences worth knowing:

- Nesting (`&`) and `:has()` may behave slightly differently in the prefixer fallback. Avoid nesting in public themes.
- `@media` inside user CSS works everywhere — scope applies outside media queries.
- `@import`, `@font-face`, `@keyframes` auto-hoist to document top — they can't be scoped per CSS spec.

**Dangerous-selector audit:** under the textarea, the system counts selectors that would repaint the whole shell (`body`, `html`, `textarea`, `input`, `.menu_button`, `#top-bar`, etc.) in real time. If "Whole app" mode is on and such selectors > 0, a warning appears.

### Five levels of modification

From most stable to most fragile. **Recommendation: stay on levels 1–3.**

---

#### Level 1 — `--SmartTheme*` variables (recommended)

Public API, SillyTavern-compatible. **The most stable level**: works on all WuNest versions, doesn't break on refactors.

```css
:root {
  --SmartThemeBlurTintColor: #0b0818;           /* main background */
  --SmartThemeChatTintColor: #130d15;           /* card and message background */
  --SmartThemeBorderColor:   #3c1e50;           /* borders */
  --SmartThemeBodyColor:     #f1d1ff;           /* main text */
  --SmartThemeQuoteColor:    #c485ff;           /* accent: CTA, focus */
  --SmartThemeBodyFont:      'Inter';           /* body font */
  --SmartThemeEmColor:         #9d4edd;         /* italics / <em> */
  --SmartThemeUnderlineColor:  #7a2fb8;         /* underlines / links */
  --SmartThemeShadowColor:     rgba(0,0,0,.4);  /* shadows under messages */
  --SmartThemeUserMesBlurTintColor: #1c1228;    /* user message bg */
  --SmartThemeBotMesBlurTintColor:  #150c1f;    /* AI message bg */
}
```

The first five cover 80% of theme needs. The rest are for fine-tuning. Save — watch the whole shell repaint.

---

#### Level 2 — `--nest-*` tokens directly

WuNest's internal tokens. Usually no need to touch them directly — they inherit from `--SmartTheme*`. But if you need **fine-grained control** (say, a custom radius for cards separate from bubbles):

```css
:root {
  --nest-bg:             #0a0a0f;
  --nest-surface:        #141018;
  --nest-text:           #e8e0f0;
  --nest-accent:         #9d4edd;
  --nest-border:         #2a1f3a;
  --nest-border-subtle:  #1a1428;
  --nest-radius:         16px;
  --nest-radius-sm:      8px;
  --nest-font-body:      'Inter', sans-serif;
  --nest-font-display:   'Fraunces', serif;
  --nest-font-mono:      'JetBrains Mono', monospace;
  --nest-transition-fast: 0.15s ease;
}
```

⚠️ **Trade-off:** if we ever rename a token, your theme breaks. For max stability, prefer Level 1.

Full token list — in `tokens/colors_and_type.css` and `tokens/customization.css` (under `frontend/src/styles/`).

---

#### Level 3 — `.nest-*` classes

WuNest public anchors (from the **Selector Contract**). These don't change without a changelog.

**Messages:**

| WuNest class | ST alias | What it is |
|---|---|---|
| `.nest-msg` | `.mes` | Message row |
| `.nest-msg-body` | `.mes_block` | Bubble body (bg, border) |
| `.nest-msg-content` | `.mes_text` | Content (text) |
| `.nest-msg-name` | `.mes_name` | Sender name |
| `.nest-msg-avatar` | `.mes .avatar` | Avatar |
| `.nest-msg-time` | — | Timestamp |
| `.nest-msg-actions` | `.mes_buttons` | Row with buttons (edit, swipe, delete) |

**Shell:**

| WuNest class/id | ST alias | What it is |
|---|---|---|
| `.nest-chat-scroll` | `#chat` | History container |
| `.nest-chat-input` | `#send_form` | Composer (input form) |
| input textarea | `#send_textarea` | Message input (ID hook) |
| send button | `#send_but` | Send button (ID hook) |
| `.nest-topbar` | `#top-bar`, `.topbar` | Top bar |
| `.nest-sidebar` | `#leftNavPanel` | Chat list sidebar |

**State modifiers:**

| Class | Where | Active when |
|---|---|---|
| `.is-user` | `.nest-msg` | message is from the user (ST aliases also available: `.mes_user` / `.mes_char`) |
| `.is-streaming` | `.nest-msg` | while message streams |
| `.is-error` | `.nest-msg-body` | when generation errored |
| `.is-favorite` | `.nest-char-card` | when the card is starred |

Example:

```css
/* Fancy left border for library favorites */
.nest-char-card.is-favorite {
  border-left: 3px solid gold;
}

/* Pulse while streaming */
.nest-msg.is-streaming .nest-msg-content {
  animation: nest-pulse 1.2s ease-in-out infinite;
}
@keyframes nest-pulse {
  50% { opacity: 0.7; }
}
```

---

#### Level 4 — ST aliases (`.mes`, `#chat`, ...)

**For SillyTavern theme compatibility**. The same selectors work as in ST. If you're importing a `.json` or `.css` from ST — this layer "just works".

```css
.mes { border-radius: 20px; }
.mes_name { font-style: italic; }
.mes_text { font-size: 16px; }
#send_textarea { background: #1a0f1f; color: #f1d1ff; }
#chat { padding: 24px; }
```

⚠️ Internal structure is **slightly different**: `.mes` doesn't have every great-grandchild from ST. What works — in the tables above. What doesn't — see "Compatibility" below.

---

#### Level 5 — data attributes (reacting to mode)

Advanced technique. Lets you write CSS that **fires only in a specific mode** (document vs bubble, square avatar vs round, etc.).

Attributes live on `<html>` or `#chat`:

| Attribute | Values |
|---|---|
| `data-nest-chat-display` | `bubbles` / `flat` / `document` |
| `data-nest-avatar-style` | `round` / `square` / `portrait` |
| `data-nest-density` | `compact` (others planned) |
| `data-nest-reduced-motion` | present when enabled in settings |

Example: editorial typography only in document mode:

```css
[data-nest-chat-display='document'] .nest-msg-body {
  font-family: 'Fraunces', serif;
  font-size: 17px;
  line-height: 1.75;
  max-width: 68ch;
}

[data-nest-chat-display='document'] .nest-msg-name {
  font-variant-caps: small-caps;
  letter-spacing: 0.08em;
}
```

Example: drop shadow for square avatars:

```css
[data-nest-avatar-style='square'] .nest-msg-avatar {
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
```

---

## Recipes

### Glowing bubble

```css
.nest-msg-body {
  background: rgba(30, 15, 45, 0.65);
  border: 1px solid #3c1e50;
  border-radius: 14px;
  box-shadow:
    0 2px 14px rgba(120, 60, 200, 0.25),
    inset 0 1px 0 rgba(255, 255, 255, 0.03);
  backdrop-filter: blur(6px);
}

.nest-msg-name {
  color: #c485ff;
  text-shadow: 0 0 12px rgba(196, 133, 255, 0.4);
}
```

### Parchment / tavern

```css
:root {
  --SmartThemeBlurTintColor: #1a130a;
  --SmartThemeChatTintColor: #2a1f12;
  --SmartThemeBodyColor:     #f4e4c1;
  --SmartThemeBorderColor:   #6b4a2a;
  --SmartThemeQuoteColor:    #d4904a;
  --SmartThemeBodyFont:      'Fraunces';
}

.nest-msg-body {
  border-radius: 4px;
  box-shadow:
    0 1px 0 rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 220, 160, 0.05);
}

.nest-msg-name {
  font-style: italic;
  letter-spacing: 0.03em;
}
```

### Reader-mode typography

```css
[data-nest-chat-display='document'] .nest-msg-body {
  font-family: 'Fraunces', Georgia, serif;
  font-size: 17px;
  line-height: 1.78;
  max-width: 68ch;
}

[data-nest-chat-display='document'] .nest-msg-body em {
  font-style: italic;
  color: #555;
}

/* Drop-cap for first letter */
[data-nest-chat-display='document'] .nest-msg-content > p:first-child::first-letter {
  font-size: 3.2em;
  font-family: 'Fraunces', serif;
  float: left;
  line-height: 0.9;
  margin: 6px 8px 0 0;
  color: var(--nest-accent);
}
```

### Different accents for user and character

For side-of-dialogue distinction — use the `.is-user` modifier (on user messages; absent on AI messages):

```css
/* You — cool cyan */
.nest-msg.is-user .nest-msg-name {
  color: #4dd0e1;
}

/* AI — warm magenta */
.nest-msg:not(.is-user) .nest-msg-name {
  color: #c485ff;
}

/* Thin left border — colored by side */
.nest-msg.is-user .nest-msg-body {
  border-left: 3px solid #4dd0e1;
}
.nest-msg:not(.is-user) .nest-msg-body {
  border-left: 3px solid #c485ff;
}

/* Alternative: ST aliases .mes_user / .mes_char */
.mes_user .mes_name { color: #4dd0e1; }
.mes_char .mes_name { color: #c485ff; }
```

### Hide timestamps

```css
.nest-msg-time { display: none; }
```

(In `chat` scope, the global topbar stays untouched.)

### Custom font from Google Fonts

```css
@import url('https://fonts.googleapis.com/css2?family=Andika&display=swap');

:root {
  --SmartThemeBodyFont: 'Andika';
}
```

The system auto-hoists `@import` to document top — works even in scope mode.

### Your own font via CDN

```css
@font-face {
  font-family: 'MyFont';
  src: url('https://my-cdn.com/my-font.woff2') format('woff2');
  font-display: swap;
}

:root { --SmartThemeBodyFont: 'MyFont'; }
```

### Refined composer

```css
/* Airier composer */
.nest-chat-input {
  padding: 18px 20px;
  background: rgba(20, 10, 35, 0.5);
  backdrop-filter: blur(10px);
}

#send_textarea {
  font-size: 15px;
  line-height: 1.5;
  color: #f1d1ff;
}

#send_textarea::placeholder {
  color: rgba(241, 209, 255, 0.35);
  font-style: italic;
}
```

---

## What to avoid

### ❌ Rules on `body` / `html` in global mode

```css
/* WILL BREAK the topbar, menus, scroll, everything */
body { background: black !important; }
```

In `chat` scope, such rules are **ignored** (per the `@scope` spec — global elements are hoisted). In global mode they break things. **Alternative:** change variables (Level 1).

### ❌ `!important` without reason

Every `!important` is a nail in future theme maintenance. You compete with yourself on every WuNest update. Use it only when there's no other way (e.g., you need to kill `transition: none` from the system reduced-motion mode).

### ❌ Absolute positions on key elements

```css
/* DON'T: breaks mobile layout, keyboard handling, IME */
#chat { position: absolute !important; top: 100px; }
```

### ❌ Fixed px widths

```css
/* Bad: breaks mobile */
.nest-msg-body { width: 600px !important; }

/* Good */
.nest-msg-body { max-width: 600px; width: 100%; }
```

### ❌ Changing `display` on key containers

Don't set `display: block` / `flex` / `grid` on `#chat`, `#sheld`, `.nest-shell` — it breaks internal height calculations (`100dvh`, flex-based viewport).

### ❌ `vh` / `vw` units

Use `dvh` / `svh` / `lvh` — they correctly handle mobile keyboard. `vh` → on iOS/Android the chat "jumps" when the keyboard appears.

### ❌ Hiding interactive elements

```css
/* Will kill edit / swipe / delete buttons */
.nest-msg-actions { display: none; }
```

If you really want to hide them — keep access via context menu or long-tap. Otherwise users can't edit/regenerate messages.

---

## Debug

1. **DevTools → Console** (F12) — CSS parse errors show up there. WuNest runs offline validation via `CSSStyleSheet.replaceSync()`: if there's an orphan `}` — a red strip appears under the textarea with the error text, **before** saving.

2. **Inspect the injected CSS:** in DevTools find `<style id="nest-user-css">` in `<head>`. You'll see exactly how your CSS was pasted (including scope wrapper).

3. **Specificity wars:** `DevTools → Inspector → Computed` → hover over a rule → see what overrides it.

4. **Theme works locally, breaks after reload:** CSS writes to localStorage + server (400 ms debounce). Wait a second after the last keystroke before F5. Save indicator — "saving…" in the Appearance panel corner.

5. **Different behaviour in Chrome / Firefox:** Chromium/Safari use native `@scope`, Firefox uses a manual prefixer. `&` and CSS nesting can behave differently. For public themes — no nesting.

6. **Mobile:** check at < 960 px window (DevTools → Device toolbar). WuNest's mobile breakpoint is single (960 px). Everything below — one column.

7. **Playground:** `frontend/src/styles/design-system/playground.html` is a standalone page with the full set of components. Open it locally (no WuNest needed) → write CSS in any editor → rapid iteration without server saves.

---

## Safety nets

### Safe mode

Theme broke the shell so badly you can't open Settings? **Add `?safe` to the URL** → your CSS and background image won't apply, a yellow banner will appear: "Safe mode: custom CSS disabled". Click "Clear CSS" or edit — exit safe mode by reloading without the param.

Safe mode is also in the avatar menu: **Avatar → Safe mode**.

See [details](/docs/safe-mode).

### Import validation

When importing ST JSON:

- Scope auto-sets to `chat` (even if the theme has broad selectors — they'll scope).
- Notice shown: "Imported theme with N dangerous selectors — applied to chat only".

Same for `.css` import. The parser checks syntax and warns about problems.

---

## SillyTavern compatibility

| What works | What doesn't |
|---|---|
| `--SmartTheme*` variables | `.drawer-content > *` with ST-proprietary containers |
| `.mes`, `.mes_block`, `.mes_text`, `.mes_name` | `.mes .avatar` with ST-specific nesting |
| `#chat`, `#send_form`, `#send_textarea`, `#send_but` | `#expression-image`, `#bg1` (ST wallpaper system) |
| `#top-bar`, `.topbar`, `#leftNavPanel`, `#sheld` | ST-specific `.drawer-content` layout rules |
| `@import`, `@font-face`, `@keyframes` | Direct `body { background }` edits in `chat` scope |
| `[style*="...."]` attribute selectors | `window.extension_settings.*` (no such API) |

Import/export ST themes — via **Appearance → Import / Export** (ST JSON format).

---

## Publishing a theme

No public gallery yet, but the format is sharing-ready:

1. **Export `.css`** (Appearance → Export as .css) saves your theme as a standalone file.
2. **Or JSON** — saves CSS + all UI settings (font size, chat width, etc.).
3. Share the file — recipient imports via Appearance → Import.

### A good theme:

- **Level 1 or 2** (variables), minimum hard-coded selectors.
- Works in `chat` scope — doesn't touch topbar/sidebar unnecessarily.
- **Explicit name and author** in the first comment:
  ```css
  /* Name: Purple Tavern */
  /* Author: you */
  /* Version: 1.0 */
  /* License: MIT */
  ```
- Tested on **mobile** (< 960 px) and **desktop**.
- Tested on both kinds (if it's a light theme — verify it's dark-content compatible; if dark — vice versa).
- Graceful degradation: if your theme uses `backdrop-filter`, add a fallback for older browsers.
- No heavy external assets (CDN fonts — ok, > 500 KB images — not ok).

### Planned:

- Internal community gallery (M43+) — upload, rating, install in one click.
- Theme CLI — validator, linter, auto-screenshots.
