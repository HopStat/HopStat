export function isMobileQueryLayout(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(max-width: 639px)').matches
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
