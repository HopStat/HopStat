/** Release-note text helpers. Kept apart from the components in release-notes.tsx so
 *  that file exports components only and Fast Refresh keeps working. */

export function formatReleaseVersion(v: string): string {
  const core = v.trim().replace(/^v/i, '')
  if (!core) return ''
  return `v${core}`
}

export function stripReleaseTitleHeadings(markdown: string): string {
  const stripSection = (section: string) =>
    section
      .replace(/\r\n/g, '\n')
      .replace(/^#\s+HopStat\s+v[^\n]*\n+/i, '')
      .trim()

  const normalized = markdown.replace(/\r\n/g, '\n').trim()
  if (!normalized.includes('\n\n---\n\n')) {
    return stripSection(normalized)
  }
  return normalized
    .split('\n\n---\n\n')
    .map(stripSection)
    .join('\n\n---\n\n')
}
