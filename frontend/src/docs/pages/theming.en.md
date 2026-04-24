# Theming & CSS

WuNest supports **four** layers of customization: 5 built-in presets, UI toggles, ST-compatible CSS variables, and your own CSS.

> See the 5 presets side-by-side → [theme gallery](/themes) (public page, shareable link).

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

## Quick settings

**Settings → Appearance**:

- Font size (0.75× — 1.4×)
- Chat width (40–100%)
- Avatar shape (round / square)
- Message style (bubbles / flat / document)
- Accent color, background, text, borders
- Background image + blur
- Shadows, reduced motion, HTML rendering in messages

Everything here writes CSS custom properties to `:root` inline — applies instantly, no reload.

## Import & export

### SillyTavern-compatible JSON

**Appearance → Import ST theme (.json)** — load a `.json`. Extracted:

- Colors: `main_text_color`, `italics_text_color`, `quote_text_color`, `border_color`
- Sizes: `font_scale`, `chat_width`, `blur_strength`
- Styles: `avatar_style`, `chat_display`, `noShadows`, `reduced_motion`
- `custom_css` — applied as your custom CSS

Scope auto-sets to `chat` — the ST theme's CSS won't break the menu. If the theme contains rules for broad elements (`body`, `textarea`, `input`), we show an info notice.

### Raw CSS (.css)

**Appearance → Import .css file** — loads a bare CSS file directly (M42.4). Handy when themes travel through the community as plain `.css` without the JSON envelope. Scope auto-set to `chat` too.

**Appearance → Export as .css** — saves your current custom CSS as a standalone `.css` file. Easy to share in Discord/forums without JSON noise. Filename derived from the `/* Name: ... */` comment if present.

## Custom CSS

**Appearance → Custom CSS** — hand-written code. Full CSS syntax supported.

### Scope — "Apply to"

Under the textarea — toggle with two modes:

- **Chat only** (default) — CSS is wrapped in `@scope (#chat) { ... }` on modern browsers, or prefixed with `#chat ` manually on Firefox. Rules only reach elements inside the chat.
- **Whole app** — CSS applies as-is to the entire UI. Rules for `body`/`input`/`textarea` will repaint fields in every menu.

Recommended: **Chat only** for ST themes. **Whole app** — for WuNest-native themes where you write selectors against `.nest-*`.

### Theme variables

The cleanest way to recolor the app is overriding ST variables:

```css
:root {
  --SmartThemeBodyColor: #f0f0f0;       /* main text */
  --SmartThemeBorderColor: #3c1e50;     /* borders */
  --SmartThemeQuoteColor: #ef4444;      /* accent */
  --SmartThemeBlurTintColor: #0b0818;   /* primary bg */
  --SmartThemeChatTintColor: #130d15;   /* panel surfaces */
  --SmartThemeBodyFont: 'Andika';       /* body font */
}
```

WuNest reads these via a fallback chain — they propagate everywhere via CSS inheritance.

### WuNest classes

For message styling:

| WuNest class        | ST alias       | What it is                   |
|---------------------|----------------|------------------------------|
| `.nest-msg`         | `.mes`         | Message row                  |
| `.nest-msg-body`    | `.mes_block`   | Bubble body (bg, border)     |
| `.nest-msg-content` | `.mes_text`    | Message content              |
| `.nest-msg-name`    | `.mes_name`    | Sender name                  |

ID hooks:

| WuNest class/id    | ST alias          | What it is                     |
|--------------------|-------------------|--------------------------------|
| `.nest-chat-scroll`| `#chat`           | History container              |
| `.nest-chat-input` | `#send_form`      | Composer (input form)          |
| input textarea     | `#send_textarea`  | The message input              |
| Send button        | `#send_but`       | The Send button                |
| Topbar             | `#top-bar`, `.topbar` | The top bar                |

Use either — both hit the same element.

### Example

```css
/* Message bubble with soft glow */
.mes {
  background: rgba(30, 15, 45, 0.65);
  border: 1px solid #3c1e50;
  border-radius: 14px;
  box-shadow: 0 2px 10px rgba(120, 60, 200, 0.15);
}

/* Accent for sender name */
.mes_name { color: #c485ff; letter-spacing: 0.02em; }

/* Composer text color */
#send_textarea { color: #f1d1ff; }
```

## If something breaks

If your custom CSS broke the UI — flip the scope to "Chat only", or use [safe mode](/docs/safe-mode) (`?safe` in the URL).
