import type { QueryHistoryRecord } from '@/lib/query-history-db'

export interface RankedQueryHistoryRecord extends QueryHistoryRecord {
  score: number
}

function normalize(value: string): string {
  return value.trim().toLowerCase()
}

export function rankQueryHistory(
  entries: QueryHistoryRecord[],
  query: string,
  command: string,
  limit = 8,
): RankedQueryHistoryRecord[] {
  const needle = normalize(query)
  const currentCommand = normalize(command)

  if (!needle) {
    return entries
      .slice()
      .sort((a, b) => {
        const aCmd = normalize(a.command) === currentCommand ? 1 : 0
        const bCmd = normalize(b.command) === currentCommand ? 1 : 0
        if (bCmd !== aCmd) return bCmd - aCmd
        return b.usedAt - a.usedAt
      })
      .slice(0, limit)
      .map(entry => ({ ...entry, score: entry.usedAt }))
  }

  return entries
    .map(entry => {
      const target = normalize(entry.target)
      let score = 0

      if (target === needle) score += 120
      else if (target.startsWith(needle)) score += 90
      else if (target.includes(needle)) score += 60
      else return null

      if (normalize(entry.command) === currentCommand) score += 20
      score += Math.min(entry.usedAt / 1_000_000_000_000, 10)

      return { ...entry, score }
    })
    .filter((entry): entry is RankedQueryHistoryRecord => entry !== null)
    .sort((a, b) => b.score - a.score || b.usedAt - a.usedAt)
    .slice(0, limit)
}

export function highlightMatch(text: string, query: string): { before: string; match: string; after: string } {
  const trimmed = query.trim()
  if (!trimmed) return { before: text, match: '', after: '' }

  const lowerText = text.toLowerCase()
  const lowerQuery = trimmed.toLowerCase()
  const index = lowerText.indexOf(lowerQuery)
  if (index < 0) return { before: text, match: '', after: '' }

  return {
    before: text.slice(0, index),
    match: text.slice(index, index + trimmed.length),
    after: text.slice(index + trimmed.length),
  }
}
