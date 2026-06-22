export function isMobileQueryLayout(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(max-width: 639px)').matches
}

/** Chrome-only: avoid invalid viewport token in HTML (Safari warns and ignores it). */
export function enhanceViewportForVirtualKeyboard(): void {
  if (typeof navigator === 'undefined' || !('virtualKeyboard' in navigator)) return
  const meta = document.querySelector('meta[name="viewport"]')
  if (!meta) return
  const content = meta.getAttribute('content') ?? ''
  if (content.includes('interactive-widget')) return
  meta.setAttribute('content', `${content}, interactive-widget=overlays-content`)
}

export function blurActiveFieldPreservingScroll(): void {
  const el = document.activeElement
  if (!(el instanceof HTMLElement)) return
  el.blur()
}

export function scrollResultsBelowSticky(
  stickyEl: HTMLElement | null,
  resultsEl: HTMLElement | null,
): void {
  if (!isMobileQueryLayout() || !resultsEl) return

  const offset = (stickyEl?.offsetHeight ?? 0) + 8
  document.documentElement.style.setProperty('--query-sticky-offset', `${offset}px`)

  const top = resultsEl.getBoundingClientRect().top + window.scrollY - offset
  window.scrollTo({ top: Math.max(0, top), left: 0, behavior: 'auto' })
}
