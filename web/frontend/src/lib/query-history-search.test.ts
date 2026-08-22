import { describe, expect, it } from 'vitest'
import type { QueryHistoryRecord } from '@/lib/query-history-db'
import { dedupeQueryHistory, highlightMatch, rankQueryHistory } from '@/lib/query-history-search'

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

describe('dedupeQueryHistory', () => {
  const entry = (key: string, command: string, target: string, nodeName: string, usedAt: number) =>
    ({ key, command, target, nodeId: 1, nodeName, usedAt })

  it('shows the same question once, keeping the most recent run', () => {
    const out = dedupeQueryHistory([
      entry('a', 'bgp_route', '8.8.8.8', 'BURSA', 30),
      entry('b', 'bgp_route', '8.8.8.8', 'SOFIA', 20),
      entry('c', 'bgp_route', '8.8.8.8', 'ESENYURT', 10),
      entry('d', 'ping', '8.8.8.8', 'BURSA', 5),
    ], 8)

    expect(out.map(e => e.key)).toEqual(['a', 'd'])
  })

  it('treats casing and padding as the same question', () => {
    const out = dedupeQueryHistory([
      entry('a', 'bgp_route', 'Google.com', 'BURSA', 20),
      entry('b', 'BGP_ROUTE', ' google.com ', 'SOFIA', 10),
    ], 8)

    expect(out).toHaveLength(1)
  })

  it('stops at the limit', () => {
    const many = Array.from({ length: 20 }, (_, i) => entry(`k${i}`, 'ping', `10.0.0.${i}`, 'A', i))
    expect(dedupeQueryHistory(many, 4)).toHaveLength(4)
  })
})
