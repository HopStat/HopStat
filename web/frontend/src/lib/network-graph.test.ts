import { describe, expect, it } from 'vitest'
import { buildNetworkGraph, COMPACT_LAYOUT, WIDE_LAYOUT } from './network-graph'
import type { NodeASPath } from '@/types/domain'

function entry(id: number, name: string, path: number[], extra: Partial<NodeASPath> = {}): NodeASPath {
  return { node_id: id, node_name: name, as_path: path, prefix: '8.8.8.0/24', ...extra }
}

const colOf = (g: ReturnType<typeof buildNetworkGraph>, key: string) =>
  g!.vertices.find(v => v.key === key)!.col

describe('buildNetworkGraph', () => {
  it('needs at least two routed nodes', () => {
    expect(buildNetworkGraph(undefined, [])).toBeNull()
    expect(buildNetworkGraph([], [])).toBeNull()
    expect(buildNetworkGraph([entry(1, 'A', [1, 2])], [])).toBeNull()
    expect(buildNetworkGraph([entry(1, 'A', [1, 2]), entry(2, 'B', [], { no_route: true })], [])).toBeNull()
  })

  it('merges a shared hop into one vertex and keeps both branches', () => {
    const graph = buildNetworkGraph([
      entry(1, 'BURSA', [9121, 3356, 15169]),
      entry(2, 'SOFIA', [8866, 6939, 15169]),
    ], [])!

    const origin = graph.vertices.filter(v => v.asn === 15169)
    expect(origin).toHaveLength(1)
    expect(origin[0].nodeIds.sort()).toEqual([1, 2])

    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(2)
    // Two 3-hop paths that only share the origin: 6 distinct edges.
    expect(graph.edges).toHaveLength(6)
    expect(graph.edges.every(e => e.nodeIds.length === 1 || e.nodeIds.length === 2)).toBe(true)
  })

  it('shares a common prefix then diverges', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 3356, 15169]),
      entry(2, 'B', [9121, 3356, 20940]),
    ], [])!

    const shared = graph.edges.find(e => e.from === 'as:9121' && e.to === 'as:3356')!
    expect(shared.nodeIds.sort()).toEqual([1, 2])
    expect(graph.vertices.filter(v => v.asn === 9121)).toHaveLength(1)
    // Two distinct origins, both terminal.
    expect(graph.edges.filter(e => e.from === 'as:3356')).toHaveLength(2)
  })

  it('keeps every edge pointing left to right when paths differ in length', () => {
    const graph = buildNetworkGraph([
      entry(1, 'short', [9121, 15169]),
      entry(2, 'long', [8866, 6939, 3356, 174, 15169]),
      entry(3, 'mid', [8866, 1299, 15169]),
    ], [])!

    for (const edge of graph.edges) {
      expect(colOf(graph, edge.from)).toBeLessThan(colOf(graph, edge.to))
    }
    // Node boxes are pinned to the left, the shared origin lands rightmost.
    expect(graph.vertices.filter(v => v.kind === 'node').every(v => v.col === 0)).toBe(true)
    const maxCol = Math.max(...graph.vertices.map(v => v.col))
    expect(colOf(graph, 'as:15169')).toBe(maxCol)
  })

  it('collapses prepends and drops non-consecutive repeats', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 9121, 9121, 3356, 15169]),
      entry(2, 'B', [8866, 3356, 15169]),
    ], [])!

    const prepended = graph.vertices.find(v => v.asn === 9121)!
    expect(prepended.count).toBe(3)
    expect(prepended.label).toBe('3×AS9121')

    // A poisoned path revisiting an AS must not create a second vertex or a cycle.
    const poisoned = buildNetworkGraph([
      entry(1, 'A', [9121, 3356, 9121, 15169]),
      entry(2, 'B', [8866, 15169]),
    ], [])!
    expect(poisoned.vertices.filter(v => v.asn === 9121)).toHaveLength(1)
    for (const edge of poisoned.edges) {
      expect(colOf(poisoned, edge.from)).toBeLessThan(colOf(poisoned, edge.to))
    }
  })

  it('refuses an edge that would close a cycle across two nodes', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [3356, 6939, 15169]),
      entry(2, 'B', [6939, 3356, 15169]),
    ], [])!

    const forward = graph.edges.some(e => e.from === 'as:3356' && e.to === 'as:6939')
    const backward = graph.edges.some(e => e.from === 'as:6939' && e.to === 'as:3356')
    expect(forward && backward).toBe(false)
    expect(graph.vertices.length).toBeGreaterThan(0)
  })

  it('leaves nodes with no route off the diagram', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 15169]),
      entry(2, 'B', [8866, 15169]),
      entry(3, 'DARK', [], { no_route: true }),
      entry(4, 'EMPTY', []),
    ], [])!

    expect(graph.vertices.filter(v => v.kind === 'node').map(v => v.label)).toEqual(['A', 'B'])
  })

  it('carries enrichment and default-route context onto the vertices', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 15169], { via_default_route: true, prefix: '8.8.8.8/32' }),
      entry(2, 'B', [8866, 15169]),
    ], [
      { asn: 15169, org_name: 'Google LLC', short_name: '', country_code: 'US', flag_emoji: '🇺🇸' },
    ])!

    const origin = graph.vertices.find(v => v.asn === 15169)!
    expect(origin.org).toBe('GOOGLE')
    expect(origin.cc).toBe('US')

    const box = graph.vertices.find(v => v.kind === 'node' && v.nodeId === 1)!
    expect(box.viaDefaultRoute).toBe(true)
    expect(box.prefix).toBe('8.8.8.8/32')
    expect(graph.edges.find(e => e.from === 'n:1')!.viaDefaultRoute).toBe(true)
  })

  it('produces stable geometry across repeated calls', () => {
    const build = () => buildNetworkGraph([
      entry(1, 'A', [9121, 3356, 15169]),
      entry(2, 'B', [8866, 6939, 15169]),
      entry(3, 'C', [8866, 3356, 15169]),
    ], [])!

    const first = build()
    const second = build()
    expect(second.vertices.map(v => [v.key, v.col, v.row])).toEqual(first.vertices.map(v => [v.key, v.col, v.row]))
    expect(second.width).toBe(first.width)
    expect(second.height).toBe(first.height)
    expect(first.width).toBeGreaterThan(WIDE_LAYOUT.boxW)
  })

  it('caps the node count so the diagram stays a diagram', () => {
    const many = Array.from({ length: 30 }, (_, i) => entry(i + 1, `N${i}`, [1000 + i, 15169]))
    const graph = buildNetworkGraph(many, [])!
    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(24)
  })
})

describe('buildNetworkGraph queried node', () => {
  it('puts the queried node first so it leads the rows and colours', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 15169]),
      entry(2, 'B', [8866, 15169]),
      entry(3, 'C', [1299, 15169]),
    ], [], { queriedNodeId: 3 })!

    const boxes = graph.vertices.filter(v => v.kind === 'node').sort((a, b) => a.row - b.row)
    expect(boxes.map(v => v.label)).toEqual(['C', 'A', 'B'])
  })

  it('ignores a queried node that has no route on the map', () => {
    const graph = buildNetworkGraph([
      entry(1, 'A', [9121, 15169]),
      entry(2, 'B', [8866, 15169]),
    ], [], { queriedNodeId: 99 })!

    const boxes = graph.vertices.filter(v => v.kind === 'node').sort((a, b) => a.row - b.row)
    expect(boxes.map(v => v.label)).toEqual(['A', 'B'])
  })
})

describe('buildNetworkGraph backup paths', () => {
  it('draws a node’s backup path and marks only its own edges as alternate', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [9121, 3356, 15169], best: true },
      { node_id: 1, node_name: 'A', as_path: [9121, 6939, 15169] },
      { node_id: 2, node_name: 'B', as_path: [8866, 15169], best: true },
    ], [])!

    // One box per node, not per path.
    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(2)
    // The backup's own hop is present.
    expect(graph.vertices.some(v => v.asn === 6939)).toBe(true)

    const shared = graph.edges.find(e => e.from === 'n:1' && e.to === 'as:9121')!
    expect(shared.alternate).toBe(false)
    const backup = graph.edges.find(e => e.from === 'as:9121' && e.to === 'as:6939')!
    expect(backup.alternate).toBe(true)
    const selected = graph.edges.find(e => e.from === 'as:9121' && e.to === 'as:3356')!
    expect(selected.alternate).toBe(false)
  })

  it('keeps a node whose only path is a backup on the map', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [9121, 15169], best: true },
      { node_id: 2, node_name: 'B', as_path: [8866, 15169] },
    ], [])!

    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(2)
    expect(graph.edges.find(e => e.from === 'n:2')!.alternate).toBe(true)
  })

  it('counts nodes, not paths, against the node cap', () => {
    const many = Array.from({ length: 30 }, (_, i) => ([
      { node_id: i + 1, node_name: `N${i}`, as_path: [1000 + i, 15169], best: true },
      { node_id: i + 1, node_name: `N${i}`, as_path: [2000 + i, 15169] },
    ])).flat()
    const graph = buildNetworkGraph(many, [])!
    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(24)
  })
})

describe('buildNetworkGraph layout', () => {
  const entries = [
    { node_id: 1, node_name: 'A', as_path: [9121, 3356, 15169], best: true },
    { node_id: 2, node_name: 'B', as_path: [8866, 6939, 15169], best: true },
  ]

  it('narrows the geometry in compact mode so less shrinking is needed', () => {
    const wide = buildNetworkGraph(entries, [])!
    const compact = buildNetworkGraph(entries, [], { compact: true })!

    expect(wide.layout).toEqual(WIDE_LAYOUT)
    expect(compact.layout).toEqual(COMPACT_LAYOUT)
    expect(compact.width).toBeLessThan(wide.width)
    expect(compact.height).toBeLessThan(wide.height)
  })

  it('keeps the same graph, only smaller', () => {
    const wide = buildNetworkGraph(entries, [])!
    const compact = buildNetworkGraph(entries, [], { compact: true })!

    expect(compact.vertices.map(v => v.key)).toEqual(wide.vertices.map(v => v.key))
    expect(compact.edges.map(e => e.key)).toEqual(wide.edges.map(e => e.key))
    expect(compact.vertices.map(v => [v.col, v.row])).toEqual(wide.vertices.map(v => [v.col, v.row]))
  })
})
