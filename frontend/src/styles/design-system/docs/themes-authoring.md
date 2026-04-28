# Authoring a new theme for WuNest

This guide is for contributors / third-party authors who want to ship a new built-in theme. End users editing colours through the Appearance panel don't need this — see [`CSS_MODS_GUIDE.md`](./CSS_MODS_GUIDE.md).

## Architecture (M51 Sprint 3 wave 1)

WuNest's theme registry is **data-driven**: each theme is two sibling files in `frontend/src/styles/themes/`:

```
themes/
├── nest-default-dark.theme.json   ← manifest
├── nest-default-dark.css          ← actual CSS overrides
├── cyber-neon.theme.json
├── cyber-neon.css
└── ...
```

`THEME_PRESETS` in `frontend/src/stores/theme.ts` is built at compile time via Vite's `import.meta.glob('@/styles/themes/*.theme.json', { eager: true })`. Sorted by `order` (ascending), tiebreak by `id`.

The CSS file is NOT auto-discovered — there's an explicit map `THEME_LOADERS` keyed by the manifest's `css` field. This is a Vite limitation: `?raw` glob with full dynamism isn't supported. **Adding a 6th theme requires editing 3 things:**
1. Create `my-theme.theme.json` (manifest)
2. Create `my-theme.css` (actual CSS)
3. Add **one line** to `THEME_LOADERS` in `theme.ts`:
   ```ts
   'my-theme.css': () => import('@/styles/themes/my-theme.css?raw'),
   ```

## Manifest schema

```jsonc
{
  // Required: stable id used in localStorage, server appearance.themePreset,
  // chat_metadata.theme_preset, and as the lookup key in THEME_LOADERS.
  // Lowercase + dashes. Don't change after release — would orphan existing
  // user picks.
  "id": "my-theme",

  // Required: short label shown in the gallery card and the picker.
  "label": "My Theme",

  // Required: 1-2 sentence description shown in the gallery card.
  "description": "Описание в одном-двух предложениях.",

  // Required: dark | light. Determines which Vuetify base palette
  // (nestDark / nestLight) lights up alongside this preset, and which
  // system pref kind it satisfies.
  "kind": "dark",

  // Required: brand colour mirrored from the CSS file's
  // --SmartThemeQuoteColor. Used:
  //   1. To sync Vuetify's `primary` palette slot when this preset
  //      is active (so <v-btn color="primary"> matches the theme).
  //   2. As one of the swatches rendered in the gallery / picker preview.
  "accent": "#c485ff",

  // Required: stable sort key for gallery / picker ordering.
  // Lower goes first. Spread bundled themes at 10/20/30/40/50 so a third
  // party can wedge one between (e.g. 25) without bumping everything.
  "order": 30,

  // Required: filename of the sibling .css file. Must match the key in
  // THEME_LOADERS (see "Adding a new theme" above).
  "css": "my-theme.css",

  // Optional: id of the same-family theme in the opposite kind.
  // Used by:
  //   - The "follow system theme" feature — when the OS flips dark↔light,
  //     applyForSystemPref tries `pair` first, then falls back to
  //     nest-default-{dark|light} if no pair or wrong-kind.
  //   - The gallery's "pair" badge.
  // Omit when there's no honest sibling.
  "pair": "minimal-reader",

  // Required: 6-color palette for static previews (gallery cards,
  // mini-picker swatches). NOT what gets applied at runtime — that's
  // the .css file's job. These are just for the visual preview.
  // Roles:
  //   bg       — page backdrop
  //   surface  — cards, sidebar, elevated chrome
  //   border   — frame strokes
  //   text     — primary readable colour
  //   accent   — same as top-level `accent`
  //   accentOn — readable colour over accent (CTA text)
  "swatches": {
    "bg": "#0a0612",
    "surface": "#150a20",
    "border": "#3c1e50",
    "text": "#f1d1ff",
    "accent": "#c485ff",
    "accentOn": "#1a0a24"
  }
}
```

## CSS file shape

The `.css` file should override the **`--SmartTheme*`** variable family on `:root`. Our token cascade in `tokens/colors_and_type.css` maps every `--nest-*` token through a `--SmartTheme*` fallback, so theming through SmartTheme propagates across the whole shell automatically:

```css
/* my-theme.css */
:root {
  --SmartThemeBlurTintColor:    #0a0612;  /* → --nest-bg */
  --SmartThemeChatTintColor:    #150a20;  /* → --nest-surface, --nest-bg-elevated */
  --SmartThemeBorderColor:      #3c1e50;  /* → --nest-border */
  --SmartThemeBodyColor:        #f1d1ff;  /* → --nest-text */
  --SmartThemeQuoteColor:       #c485ff;  /* → --nest-accent (mirror in manifest) */
  --SmartThemeBodyFont:         'Outfit'; /* → --nest-font-body */
}

/* Optional: scoped tweaks for ST-alias selectors */
.mes_block,
.nest-msg-body {
  /* magenta glow on hover */
  transition: box-shadow var(--nest-transition-fast);
}
.mes_block:hover,
.nest-msg-body:hover {
  box-shadow: 0 0 12px rgba(196, 133, 255, 0.4);
}
```

## Testing locally

1. `cd frontend && npm run dev` — Vite hot-reloads on `.theme.json` changes.
2. Open `http://localhost:5173/themes` — gallery should show your new card.
3. Click → applies; verify the gallery preview swatches match what you actually see in the chat.
4. Test the pair/follow-system flow if you set `pair`: change OS dark/light, watch it flip.

## Style conventions

- **Don't** override `--nest-*` directly — go through the `--SmartTheme*` indirection. `--nest-*` names are public contract; `--SmartTheme*` fallback chain absorbs your changes through the cascade.
- **Don't** add `!important` unless absolutely necessary — themes shouldn't fight Vuetify.
- **Don't** target Vuetify internals (`.v-card__overlay`, `.v-overlay__scrim` etc.) — they break across Vuetify minor versions.
- **Don't** use `body { background: ... }` — it bleeds into admin surfaces (Settings, Account, Docs). Use `:root { --SmartThemeBlurTintColor: ... }` so only the chat surface picks it up.
- **Do** name your accent honestly in the manifest (`accent`) AND in the CSS (`--SmartThemeQuoteColor`) — they should be the same colour.

## Pair conventions

Themes intended to be flipped by the OS dark/light setting need a `pair`:
- `cyber-neon` (dark, magenta) ↔ `minimal-reader` (light, grey-pencil) — both prioritise narrow reading column
- `nest-default-dark` ↔ `nest-default-light` — both are "the brand"

If your theme has no honest light/dark sibling, omit `pair`. The system-prefs listener will fall back to `nest-default-{dark|light}` when the OS kind doesn't match yours, which is graceful.

## Submitting a theme

WuNest doesn't have a public marketplace yet (M53+). For now, fork the repo and PR the two files + the THEME_LOADERS line. Reviewers check:
- Manifest valid JSON, all required fields, `id` unique
- CSS doesn't add `!important` salvage hacks
- Gallery preview looks like the actual theme (swatches match runtime)
- Pair (if set) goes both ways — the sibling has a `pair` back to you

## See also

- [`SELECTOR_CONTRACT.md`](./SELECTOR_CONTRACT.md) — what classes / IDs / data-attrs are public contract you can target
- [`VISUAL_FOUNDATIONS.md`](./VISUAL_FOUNDATIONS.md) — design principles
- [`CSS_MODS_GUIDE.md`](./CSS_MODS_GUIDE.md) — end-user theming via Appearance panel
