/**
 * ST regex cards ship `<script>` blocks that build UI from captured text.
 * Vue `v-html` does not run them; re-insert clones so they execute like
 * SillyTavern (document.currentScript.previousElementSibling, etc.).
 */
export function runEmbeddedScripts(root: HTMLElement | null): void {
  if (!root) return
  for (const old of Array.from(root.querySelectorAll('script'))) {
    const prev = old as HTMLScriptElement
    if (prev.src?.trim()) continue
    const type = (prev.type || 'text/javascript').toLowerCase()
    if (type && type !== 'text/javascript' && type !== 'application/javascript' && type !== 'module') {
      continue
    }
    const neu = document.createElement('script')
    if (prev.type) neu.type = prev.type
    neu.textContent = prev.textContent ?? ''
    prev.replaceWith(neu)
  }
}
