/** Page buttons for the audit log. A long log keeps the first, last, and nearby pages, with gaps between. */
export function auditPageWindow(current: number, total: number): Array<number | 'gap'> {
  if (total <= 1) return total === 1 ? [1] : []
  if (total <= 10) return Array.from({ length: total }, (_, i) => i + 1)

  const keep = new Set<number>([1, total])
  for (let n = current - 2; n <= current + 2; n++) {
    if (n >= 1 && n <= total) keep.add(n)
  }
  const sorted = [...keep].sort((a, b) => a - b)
  const pages: Array<number | 'gap'> = []
  for (let i = 0; i < sorted.length; i++) {
    if (i > 0 && sorted[i] - sorted[i - 1] > 1) pages.push('gap')
    pages.push(sorted[i])
  }
  return pages
}
