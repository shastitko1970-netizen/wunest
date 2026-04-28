/**
 * CSP nonce reader (M51 Sprint 3 wave 3).
 *
 * WuNest doesn't currently set a Content-Security-Policy header — but
 * when it does (Sprint 4+ or whenever the security review lands), any
 * `<style>` tag we inject from JS will be silently rejected by strict
 * CSP unless it carries the matching `nonce=…` attribute. To stay
 * forward-compatible, every `<style>` injection site (theme preset,
 * user CSS, admin guard, chat override) reads the per-request nonce
 * from a meta tag and applies it.
 *
 * The flow once CSP is enabled:
 *
 *   1. Server middleware generates a random nonce per request.
 *   2. The HTML response carries:
 *        Content-Security-Policy: ... 'nonce-<base64>' ...
 *        <meta name="csp-nonce" content="<base64>">
 *   3. JS calls `getCSPNonce()` which reads the meta and returns the
 *      value (or empty string when CSP isn't enabled — current state).
 *   4. JS does `style.setAttribute('nonce', nonce)` if non-empty.
 *
 * Until step 1 ships, `getCSPNonce()` returns `''` and `applyCSPNonce`
 * is a no-op. Behaviour is identical to today; we just won't break
 * when CSP gets tightened.
 *
 * The meta is read ONCE on first call (cached) — nonces don't change
 * within a session.
 */

let cachedNonce: string | null = null

/**
 * Reads `<meta name="csp-nonce" content="…">` and returns its value.
 * Returns empty string if no meta is present (no CSP — current default).
 * Result is cached per session — nonces don't change once HTML lands.
 */
export function getCSPNonce(): string {
  if (cachedNonce !== null) return cachedNonce
  if (typeof document === 'undefined') {
    cachedNonce = ''
    return cachedNonce
  }
  const meta = document.querySelector('meta[name="csp-nonce"]') as HTMLMetaElement | null
  cachedNonce = meta?.content ?? ''
  return cachedNonce
}

/**
 * Sets the `nonce` attribute on a freshly-created `<style>` element.
 * Idempotent — calling it twice with the same nonce is fine; calling
 * with empty string removes the attribute (in case of HMR weirdness).
 *
 * Use right after `document.createElement('style')` and before
 * `appendChild` — that's the typical injection pattern in our stores.
 */
export function applyCSPNonce(el: HTMLStyleElement): void {
  const nonce = getCSPNonce()
  if (nonce) {
    el.setAttribute('nonce', nonce)
  } else {
    el.removeAttribute('nonce')
  }
}
