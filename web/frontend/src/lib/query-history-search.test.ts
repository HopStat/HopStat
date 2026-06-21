import { describe, expect, it } from 'vitest'
import type { QueryHistoryRecord } from '@/lib/query-history-db'
import { highlightMatch, rankQueryHistory } from '@/lib/query-history-search'

const entries: QueryHistoryRecord[] = [
  { key: '1', target: '8.8.8.8', command: 'ping', nodeId: 1, nodeName: 'Istanbul', usedAt: 3000 },
  { key: '2', target: '1.1.1.1', command: 'traceroute', nodeId: 1, nodeName: 'Istanbul', usedAt: 2000 },
  { key: '3', target: '31.223.39.210', command: 'bgp_route', nodeId: 1, nodeName: 'Istanbul', usedAt: 1000 },
  { key: '4', target: '88.8.4.4', command: 'ping', nodeId: 1, nodeName: 'Istanbul', usedAt: 4000 },
]

describe('rankQueryHistory', () => {
  it('prefers prefix matches and current command', () => {
    const ranked = rankQueryHistory(entries, '8.8', 'ping')
    expect(ranked[0]?.target).toBe('8.8.8.8')
  })

  it('returns recent entries for blank query with current command first', () => {
    const ranked = rankQueryHistory(entries, '   ', 'ping')
    expect(ranked.map(entry => entry.target)).toEqual(['88.8.4.4', '8.8.8.8', '1.1.1.1', '31.223.39.210'])
  })
})

describe('highlightMatch', () => {
  it('splits matched segment', () => {
    expect(highlightMatch('8.8.8.8', '8.8')).toEqual({
      before: '',
      match: '8.8',
      after: '.8.8',
    })
  })
})
