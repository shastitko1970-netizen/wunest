# On mobile

WuNest runs in any phone browser. There's no separate mobile app — just a regular website, but a few things are worth calling out.

## Navigation

On screens >960px there's a sidebar with your chat list on the left. On phones it's hidden — the main area takes the whole width. To reach the chat list:

- **Hamburger in the chat header** (left of the title) — slide-in drawer with all chats
- Or tap the WuNest logo → `/chat` — with no active chat, the list renders as the main content

## Top menu

The burger left of the logo opens the main menu: Chat / Library / Account / Settings / Docs. On wider viewports these live as a horizontal row inside the top bar.

## Generation drawer

The `mdi-tune-variant` icon in the chat header opens a generation-parameters drawer. On mobile it goes fullscreen. Two-column rows (temperature / top_p, freq / pres) collapse to single-column — every field gets the full width.

The top of the drawer has a "Wait for full response" toggle — disables per-token streaming. The message renders in one shot when the model finishes. Useful on flaky internet or if token-flicker bothers you.

## Message swipes

On an assistant message — `< 2/3 >` at the bottom. Left / right arrows step between stored variants. `+` generates a new alternative.

## Dialogs

Dialogs expand to fullscreen on mobile automatically (Import Character, BYOK add, Preset editor, Character Worlds, etc.). Dismiss via the close icon in the dialog header, `Esc`, or tap outside. When the dialog would clip viewport height, it scrolls its own content (except for short confirms).

## Themes on mobile

If you imported a SillyTavern theme — by default it only applies to the chat, not menus / settings (see [Theming & CSS](/docs/theming)).

If a theme still breaks the UI — `?safe` in the URL toggles [safe mode](/docs/safe-mode).

## Narrow chat width

`appearance.chatWidth` is a percentage of the window. At 50%, on a phone that's ~180px of readable text — unusable. So at viewports ≤600px we ignore the setting and let the chat fill the available width. Your desktop choice is preserved when you switch back to a bigger screen.
