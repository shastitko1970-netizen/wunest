# Safe mode

If your custom CSS broke the UI badly enough that you can't get to Settings — there's an escape hatch.

## How to enable

Append `?safe` to the URL:

```
https://nest.wusphere.ru/?safe
```

Reload. Custom CSS is automatically disabled **for this session** — not deleted, just suppressed, so your rules stay in place.

An amber banner appears at the top with two buttons:

- **"Clear CSS"** — wipes your saved custom CSS from the server. After that you can drop `?safe`.
- **"Exit"** — strips `?safe` from the URL and reloads. The CSS comes back.

## What else is disabled in safe mode

- Custom CSS isn't injected as a `<style>` tag
- The body background image isn't set (an unreachable / oversized image could stall painting)

## What stays on

- ST-compatible variables from the UI (accent, background color) — applied as normal
- System theme (light / dark)
- Font scale, blur, shadows — every inline-style variable

## Banner is override-proof

The safe mode banner is `position: fixed`, `z-index: 100000`, with inline `!important` on its styles. Even a hostile stylesheet couldn't hide it. Plus, in safe mode the custom CSS is suppressed entirely, so this is belt-and-braces.
