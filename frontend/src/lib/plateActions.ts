/**
 * plateActions — the "safe interactivity bridge" for HTML plates in
 * chat messages.
 *
 * Character authors can emit `<button data-nest-action="...">` (or any
 * element) in their message HTML. Our message renderer runs a delegated
 * click listener that:
 *
 *   1. Walks up from the click target to find a `[data-nest-action]`.
 *   2. Parses the action name against KNOWN_ACTIONS (whitelist).
 *   3. Runs the resolver — which returns a PlateAction result the
 *      caller (MessageContent + MessageBubble) dispatches to the app.
 *
 * We never eval author-supplied JS or accept arbitrary onclick. The
 * whitelist is the ONLY attack surface, and each entry is a function we
 * wrote — no evaluation of user strings.
 *
 * Security rules:
 *   - Attributes are already DOMPurify-scrubbed (no scripts, no JS URIs).
 *   - We read string data off `dataset.*`, never eval.
 *   - Clipboard writes happen via navigator.clipboard with the current
 *     origin — no cross-origin leakage.
 *   - Dice rolls use crypto.getRandomValues (fallback Math.random) on
 *     the client — never sent upstream.
 */

// ─── Action descriptors ───────────────────────────────────────────

/** Result of resolving a click — tells the caller what to do. */
export type PlateAction =
  // Caller should show a brief confirmation to the user.
  | { kind: 'toast'; level: 'info' | 'success' | 'error'; text: string }
  // Caller should emit up to the MessageBubble so it can trigger
  // parent-level chat mutations (swipe, regenerate).
  | { kind: 'bubble'; name: 'swipe-prev' | 'swipe-next' | 'regenerate' | 'edit' | 'delete' }
  // Caller should insert text into the composer draft (as if the user
  // typed it). Useful for "speak-as-narrator" style suggestion buttons.
  | { kind: 'draft'; text: string; send?: boolean }
  // Caller should update the DOM target's attribute / class (safe
  // because we write to the same sanitized element tree).
  | {
      kind: 'dom-update'
      target: HTMLElement
      update:
        | { type: 'toggle-attr'; attr: string }
        | { type: 'toggle-class'; className: string }
        | { type: 'set-text'; text: string }
    }
  // No-op — used for actions that silently fail validation (e.g. bad
  // dice spec) — we still swallow the click so the browser doesn't do
  // anything weird like submit a form.
  | { kind: 'noop' }

/**
 * KNOWN_ACTIONS is the complete whitelist of resolvers. Each handler
 * takes the trigger element + optional parent root (for target queries)
 * and returns what the caller should do.
 */
type ActionHandler = (el: HTMLElement, root: HTMLElement | null) => PlateAction | null

const KNOWN_ACTIONS: Record<string, ActionHandler> = {
  // ── Clipboard ─────────────────────────────────────────────────
  // <button data-nest-action="copy" data-nest-text="Hello">Copy</button>
  // <button data-nest-action="copy" data-nest-target="#stat-block">Copy</button>
  // Falls back to the trigger's innerText if neither attr set.
  'copy': (el, root) => {
    const text = resolveTextSource(el, root)
    if (!text) {
      return { kind: 'toast', level: 'error', text: 'Nothing to copy' }
    }
    // navigator.clipboard is secure-context only. On file:// or HTTP
    // fallback to a throwaway textarea select+execCommand; we're
    // always HTTPS in prod but stay portable.
    void writeClipboard(text)
    return { kind: 'toast', level: 'success', text: `Скопировано: ${clip(text, 40)}` }
  },

  // ── Dice ─────────────────────────────────────────────────────
  // <button data-nest-action="dice" data-nest-dice="2d6">Roll 2d6</button>
  // <button data-nest-action="dice" data-nest-sides="20">Roll d20</button>
  // Result written into data-nest-result-to target (if present) or
  // shown as a toast.
  'dice': (el, root) => {
    const spec = el.dataset.nestDice
      || (el.dataset.nestSides ? `1d${el.dataset.nestSides}` : '1d20')
    const roll = rollDice(spec)
    if (!roll.ok) {
      return { kind: 'toast', level: 'error', text: `Неверный dice spec: ${spec}` }
    }
    const text = `🎲 ${roll.spec} → ${roll.rolls.join(' + ')} = ${roll.total}`

    // Optional result target — write into a specific element so authors
    // can render "You rolled: ___" inline.
    const targetSel = el.dataset.nestResultTo
    if (targetSel && root) {
      const target = root.querySelector(targetSel) as HTMLElement | null
      if (target) {
        return { kind: 'dom-update', target, update: { type: 'set-text', text } }
      }
    }
    return { kind: 'toast', level: 'info', text }
  },

  // Alias: `reroll` is less precise but what character authors type
  // when they mean "roll again" — we treat it exactly like dice.
  'reroll': (el, root) => KNOWN_ACTIONS['dice']!(el, root),

  // ── Swipe / regenerate (bubble up) ────────────────────────────
  // These need MessageBubble context — we emit up and let it invoke
  // the store action.
  'swipe-prev':  () => ({ kind: 'bubble', name: 'swipe-prev' }),
  'swipe-next':  () => ({ kind: 'bubble', name: 'swipe-next' }),
  'regenerate':  () => ({ kind: 'bubble', name: 'regenerate' }),

  // ── Composer (suggest text for the user) ─────────────────────
  // <button data-nest-action="say" data-nest-text="I approach cautiously.">Approach</button>
  // `say`  → fills composer with the text (user can edit before send)
  // `send` → fills and immediately sends (one-click suggestions)
  'say': (el) => {
    const text = (el.dataset.nestText || el.innerText || '').trim()
    if (!text) return { kind: 'noop' }
    return { kind: 'draft', text, send: false }
  },
  'send': (el) => {
    const text = (el.dataset.nestText || el.innerText || '').trim()
    if (!text) return { kind: 'noop' }
    return { kind: 'draft', text, send: true }
  },

  // ── DOM toggles (self-contained UI state) ────────────────────
  // <button data-nest-action="toggle-attr" data-nest-target="#more" data-nest-attr="hidden">…</button>
  // <button data-nest-action="toggle-class" data-nest-target="#card" data-nest-class="expanded">…</button>
  // These mutate the SAME sanitized subtree — safe because there's no
  // privileged info there, it's all author-supplied content.
  'toggle-attr': (el, root) => {
    const target = resolveTarget(el, root)
    const attr = (el.dataset.nestAttr || 'hidden').trim()
    if (!target || !attr) return { kind: 'noop' }
    return { kind: 'dom-update', target, update: { type: 'toggle-attr', attr } }
  },
  'toggle-class': (el, root) => {
    const target = resolveTarget(el, root)
    const className = (el.dataset.nestClass || '').trim()
    if (!target || !className) return { kind: 'noop' }
    return { kind: 'dom-update', target, update: { type: 'toggle-class', className } }
  },
}

/**
 * dispatchAction — the entry point the rendered-HTML click listener
 * calls with its `click` Event.
 *
 * - Walks up from event.target to find the nearest [data-nest-action]
 *   element (event delegation — one listener per message).
 * - Looks the action up in the whitelist.
 * - Calls the handler and returns the result, or null if nothing to do.
 * - Also returns `prevent: true` when the handler fired so the caller
 *   can suppress default browser behavior (form submit, link follow).
 */
export function dispatchAction(
  event: Event,
  root: HTMLElement | null,
): { action: PlateAction; prevent: boolean } | null {
  const target = event.target as HTMLElement | null
  if (!target) return null
  const trigger = target.closest<HTMLElement>('[data-nest-action]')
  if (!trigger) return null

  const name = (trigger.dataset.nestAction || '').trim()
  const handler = KNOWN_ACTIONS[name]
  if (!handler) {
    // Unknown action — silently ignore. Don't throw, don't log loudly
    // (some cards might ship future-action names we'll support later).
    return { action: { kind: 'noop' }, prevent: true }
  }
  const result = handler(trigger, root)
  if (!result) return null
  return { action: result, prevent: true }
}

// ─── Helpers ────────────────────────────────────────────────────

function resolveTarget(el: HTMLElement, root: HTMLElement | null): HTMLElement | null {
  const sel = (el.dataset.nestTarget || '').trim()
  if (!sel || !root) return null
  try {
    return root.querySelector<HTMLElement>(sel)
  } catch {
    return null
  }
}

function resolveTextSource(el: HTMLElement, root: HTMLElement | null): string {
  if (el.dataset.nestText) return el.dataset.nestText
  const targetSel = (el.dataset.nestTarget || '').trim()
  if (targetSel && root) {
    try {
      const target = root.querySelector<HTMLElement>(targetSel)
      if (target) return target.innerText
    } catch { /* invalid selector — fall through */ }
  }
  return (el.innerText || '').trim()
}

async function writeClipboard(text: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch { /* permission denied — fall back */ }
  // Fallback: invisible textarea + execCommand. Works on older browsers
  // and when clipboard perms are denied but document has focus.
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  try { document.execCommand('copy') } finally { ta.remove() }
}

/** Abbreviate long text for toast notifications. */
function clip(text: string, max: number): string {
  const trimmed = text.trim().replace(/\s+/g, ' ')
  return trimmed.length > max ? trimmed.slice(0, max - 1) + '…' : trimmed
}

/**
 * rollDice parses `NdM` or `dM` or plain integer-sides and returns the
 * result. Uses crypto.getRandomValues when available (browsers do in
 * secure contexts).
 */
export function rollDice(spec: string): { ok: boolean; spec: string; rolls: number[]; total: number } {
  const m = /^(\d+)?d(\d+)$/i.exec(spec.trim())
  if (!m) return { ok: false, spec, rolls: [], total: 0 }
  const count = Math.max(1, Math.min(100, parseInt(m[1] || '1', 10)))
  const sides = Math.max(1, Math.min(10000, parseInt(m[2]!, 10)))
  const rolls: number[] = []
  for (let i = 0; i < count; i++) rolls.push(cryptoRandomInt(sides) + 1)
  const total = rolls.reduce((a, b) => a + b, 0)
  return { ok: true, spec: `${count}d${sides}`, rolls, total }
}

function cryptoRandomInt(max: number): number {
  if (max <= 0) return 0
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const buf = new Uint32Array(1)
    crypto.getRandomValues(buf)
    return buf[0] % max
  }
  return Math.floor(Math.random() * max)
}

/**
 * applyDomUpdate — the resolver returns `dom-update` actions as a data
 * object; the caller runs THIS to actually mutate. Kept here (not in
 * the resolver) so the resolver stays pure/testable and the caller
 * can choose to no-op the update (e.g. if the target element was
 * unmounted between click and dispatch).
 */
export function applyDomUpdate(update: Extract<PlateAction, { kind: 'dom-update' }>['update'], target: HTMLElement) {
  switch (update.type) {
    case 'toggle-attr':
      if (target.hasAttribute(update.attr)) target.removeAttribute(update.attr)
      else target.setAttribute(update.attr, '')
      break
    case 'toggle-class':
      target.classList.toggle(update.className)
      break
    case 'set-text':
      target.textContent = update.text
      break
  }
}
