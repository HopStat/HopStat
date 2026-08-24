import { describe, expect, it } from 'vitest'
import { buildNetworkGraph, fitCaption, isBackupFor, COMPACT_LAYOUT, SUB_ADVANCE, WIDE_LAYOUT } from './network-graph'
import type { NodeASPath } from '@/types/domain'

function entry(id: number, name: string, path: number[], extra: Partial<NodeASPath> = {}): NodeASPath {
  return { node_id: id, node_name: name, as_path: path, prefix: '8.8.8.0/24', ...extra }
}

const colOf = (g: ReturnType<typeof buildNetworkGraph>, key: string) =>
  g!.vertices.find(v => v.key === key)!.col

describe('buildNetworkGraph', () => {
  it('needs at least one routed node', () => {
    expect(buildNetworkGraph(undefined, [])).toBeNull()
    expect(buildNetworkGraph([], [])).toBeNull()
    expect(buildNetworkGraph([entry(1, 'A', [], { no_route: true })], [])).toBeNull()

    // The map replaced the single-path AS map, so one node is a valid diagram.
    const single = buildNetworkGraph([entry(1, 'A', [9121, 15169])], [])!
    expect(single.vertices.filter(v => v.kind === 'node')).toHaveLength(1)
    expect(single.edges).toHaveLength(2)
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

    // Both of node 1's paths leave through this edge, so it is not a fallback for it.
    const shared = graph.edges.find(e => e.from === 'n:1' && e.to === 'as:9121')!
    expect(isBackupFor(shared, 1)).toBe(false)
    const backup = graph.edges.find(e => e.from === 'as:9121' && e.to === 'as:6939')!
    expect(isBackupFor(backup, 1)).toBe(true)
    const selected = graph.edges.find(e => e.from === 'as:9121' && e.to === 'as:3356')!
    expect(isBackupFor(selected, 1)).toBe(false)
  })

  it('keeps a node whose only path is a backup on the map', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [9121, 15169], best: true },
      { node_id: 2, node_name: 'B', as_path: [8866, 15169] },
    ], [])!

    expect(graph.vertices.filter(v => v.kind === 'node')).toHaveLength(2)
    expect(isBackupFor(graph.edges.find(e => e.from === 'n:2')!, 2)).toBe(true)
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

describe('buildNetworkGraph vertical layout', () => {
  const entries = [
    { node_id: 1, node_name: 'A', as_path: [9121, 3356, 15169], best: true },
    { node_id: 2, node_name: 'B', as_path: [8866, 3356, 15169], best: true },
  ]

  it('runs paths downwards and spreads siblings sideways', () => {
    const across = buildNetworkGraph(entries, [])!
    const down = buildNetworkGraph(entries, [], { vertical: true })!

    expect(across.vertical).toBe(false)
    expect(down.vertical).toBe(true)
    // Same graph, transposed: taller than it is wide, where the horizontal one is wider.
    expect(down.height).toBeGreaterThan(across.height)
    expect(down.width).toBeLessThan(across.width)

    // Depth now increases downwards, and the shared hop sits below both node boxes.
    const box = down.vertices.find(v => v.key === 'n:1')!
    const shared = down.vertices.find(v => v.asn === 3356)!
    const origin = down.vertices.find(v => v.asn === 15169)!
    expect(shared.y).toBeGreaterThan(box.y)
    expect(origin.y).toBeGreaterThan(shared.y)
  })

  it('connects boxes bottom to top', () => {
    const down = buildNetworkGraph(entries, [], { vertical: true })!
    const edge = down.edges.find(e => e.from === 'n:1')!
    const from = down.vertices.find(v => v.key === edge.from)!
    // The path starts at the horizontal centre of the box, not its right edge.
    expect(edge.d.startsWith(`M ${from.x + down.layout.boxW / 2} `)).toBe(true)
  })
})

describe('fitCaption', () => {
  it('keeps a name that already fits', () => {
    expect(fitCaption('GOOGLE', 100)).toBe('GOOGLE')
  })

  it('trims at a word boundary when the box is too narrow', () => {
    // COMPACT_LAYOUT box minus the flag leaves room for about twelve characters.
    expect(fitCaption('DGN TEKNOLOJI', 12 * SUB_ADVANCE)).toBe('DGN')
  })

  it('cuts mid-word only when there is no earlier boundary', () => {
    expect(fitCaption('TEKNOLOJIHIZMET', 6 * SUB_ADVANCE)).toBe('TEKNOL')
    expect(fitCaption('A LONGNAME', 6 * SUB_ADVANCE)).toBe('A LONG')
  })

  it('gives up when there is no room at all', () => {
    expect(fitCaption('GOOGLE', 2)).toBe('')
  })
})

describe('buildNetworkGraph backup hops', () => {
  it('marks a hop only the backup route passes through', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [9121, 3356, 15169], best: true },
      { node_id: 1, node_name: 'A', as_path: [9121, 6939, 15169] },
    ], [])!

    const onlyBackup = graph.vertices.find(v => v.asn === 6939)!
    const onSelected = graph.vertices.find(v => v.asn === 3356)!
    const shared = graph.vertices.find(v => v.asn === 15169)!
    const box = graph.vertices.find(v => v.kind === 'node')!

    expect(isBackupFor(onlyBackup, 1)).toBe(true)
    expect(isBackupFor(onSelected, 1)).toBe(false)
    // The origin is reached by both, so it is not a backup hop.
    expect(isBackupFor(shared, 1)).toBe(false)
    expect(isBackupFor(box, 1)).toBe(false)
  })

  it('clears the backup mark when another node selects that hop', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [9121, 6939, 15169] },
      { node_id: 2, node_name: 'B', as_path: [8866, 6939, 15169], best: true },
    ], [])!

    // For B it is the live path; for A it is only a fallback.
    const hop = graph.vertices.find(v => v.asn === 6939)!
    expect(isBackupFor(hop, 2)).toBe(false)
    expect(isBackupFor(hop, 1)).toBe(true)
  })
})

describe('isBackupFor with a shared edge', () => {
  // The live shape that made this necessary: ESENYURT falls back over the very hop BURSA
  // uses as its live path.
  const entries = [
    { node_id: 1, node_name: 'BURSA', as_path: [43260, 204457, 15169], best: true },
    { node_id: 2, node_name: 'ESENYURT', as_path: [43260, 44901, 15169], best: true },
    { node_id: 2, node_name: 'ESENYURT', as_path: [43260, 204457, 15169] },
  ]

  it('calls the same edge live for one node and a fallback for the other', () => {
    const graph = buildNetworkGraph(entries, [])!
    const contested = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:204457')!

    expect(isBackupFor(contested, 1)).toBe(false)
    expect(isBackupFor(contested, 2)).toBe(true)

    const esenyurtLive = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:44901')!
    expect(isBackupFor(esenyurtLive, 2)).toBe(false)
  })

  it('treats a hop as live when nothing is in focus and any node routes over it', () => {
    const graph = buildNetworkGraph(entries, [])!
    const contested = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:204457')!
    expect(isBackupFor(contested, null)).toBe(false)
  })
})

describe('isBackupFor outside the focused node', () => {
  // The live shape behind "why is BELCLOUD on this map": three nodes reach the target
  // directly, and only SOFIA also holds a fallback through AS44901.
  const entries = [
    { node_id: 1, node_name: 'ESENYURT', as_path: [43260, 201178], best: true },
    { node_id: 2, node_name: 'BURSA', as_path: [43260, 201178], best: true },
    { node_id: 3, node_name: 'SOFIA', as_path: [43260, 201178], best: true },
    { node_id: 3, node_name: 'SOFIA', as_path: [43260, 44901, 201178] },
  ]

  it('marks a hop no node routes over live as a fallback, whoever is in focus', () => {
    const graph = buildNetworkGraph(entries, [], { queriedNodeId: 1 })!
    const intoBelcloud = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:44901')!
    const belcloud = graph.vertices.find(v => v.asn === 44901)!

    // Focused on ESENYURT, which never touches this hop.
    expect(isBackupFor(intoBelcloud, 1)).toBe(true)
    expect(isBackupFor(belcloud, 1)).toBe(true)
    // And for the node that actually holds it as a fallback.
    expect(isBackupFor(intoBelcloud, 3)).toBe(true)
  })

  it('still calls the shared live hop a live one', () => {
    const graph = buildNetworkGraph(entries, [], { queriedNodeId: 1 })!
    const shared = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:201178')!

    expect(isBackupFor(shared, 1)).toBe(false)
    expect(isBackupFor(shared, 3)).toBe(false)
    expect(isBackupFor(shared, null)).toBe(false)
  })
})

describe('edges that skip a column', () => {
  it('runs straight when the row it crosses is clear', () => {
    // The live shape: four nodes reach the target directly, so the fallback hop settles on
    // its own row and the live edge has nothing to avoid.
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'ESENYURT', as_path: [43260, 201178], best: true },
      { node_id: 2, node_name: 'BURSA', as_path: [43260, 201178], best: true },
      { node_id: 3, node_name: 'LEVENT', as_path: [43260, 201178], best: true },
      { node_id: 4, node_name: 'SOFIA', as_path: [43260, 201178], best: true },
      { node_id: 4, node_name: 'SOFIA', as_path: [43260, 44901, 201178] },
    ], [])!

    const direct = graph.edges.find(e => e.from === 'as:43260' && e.to === 'as:201178')!
    const from = graph.vertices.find(v => v.key === 'as:43260')!
    const to = graph.vertices.find(v => v.key === 'as:201178')!
    const belcloud = graph.vertices.find(v => v.asn === 44901)!

    expect(to.col - from.col).toBeGreaterThan(1)
    expect(belcloud.row).not.toBe(from.row)
    expect(direct.d).toContain(' L ')
  })

  it('bows only around a box standing in its way', () => {
    // Two nodes share the origin, and the middle hop lands on the same row as the edge
    // that skips it.
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [100, 300, 900], best: true },
      { node_id: 2, node_name: 'B', as_path: [100, 900], best: true },
    ], [])!

    const skipping = graph.edges.find(e => e.from === 'as:100' && e.to === 'as:900')!
    const middle = graph.vertices.find(v => v.asn === 300)!
    const from = graph.vertices.find(v => v.asn === 100)!

    if (middle.row === from.row) {
      expect(skipping.d).toContain('C')
      expect(skipping.d).not.toContain(' L ')
    } else {
      expect(skipping.d).toContain(' L ')
    }
  })
})

describe('hop placement', () => {
  it('drops a hop only the last node uses down beside it', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'ESENYURT', as_path: [43260, 201178], best: true },
      { node_id: 2, node_name: 'BURSA', as_path: [43260, 201178], best: true },
      { node_id: 3, node_name: 'LEVENT', as_path: [43260, 201178], best: true },
      { node_id: 4, node_name: 'SOFIA', as_path: [43260, 201178], best: true },
      { node_id: 4, node_name: 'SOFIA', as_path: [43260, 44901, 201178] },
    ], [], { queriedNodeId: 1 })!

    const sofia = graph.vertices.find(v => v.nodeId === 4)!
    const belcloud = graph.vertices.find(v => v.asn === 44901)!
    const hub = graph.vertices.find(v => v.asn === 43260)!

    // Only SOFIA reaches it, so it sits on SOFIA's row rather than at the top.
    expect(belcloud.row).toBe(sofia.row)
    // The hub every node crosses stays in the middle of the fan.
    expect(hub.row).toBeGreaterThan(0)
    expect(hub.row).toBeLessThan(sofia.row)
  })

  it('pushes a colliding hop to the next free row instead of overlapping', () => {
    const graph = buildNetworkGraph([
      { node_id: 1, node_name: 'A', as_path: [100, 999], best: true },
      { node_id: 2, node_name: 'B', as_path: [200, 999], best: true },
    ], [])!

    const first = graph.vertices.find(v => v.asn === 100)!
    const second = graph.vertices.find(v => v.asn === 200)!
    expect(first.col).toBe(second.col)
    expect(first.row).not.toBe(second.row)
  })
})

describe('layout spacing', () => {
  const entries = [
    { node_id: 1, node_name: 'A', as_path: [43260, 201178], best: true },
    { node_id: 2, node_name: 'B', as_path: [43260, 201178], best: true },
    { node_id: 3, node_name: 'C', as_path: [43260, 201178], best: true },
    { node_id: 4, node_name: 'D', as_path: [43260, 201178], best: true },
  ]

  it('centres a hop every node crosses on the middle of the fan', () => {
    const graph = buildNetworkGraph(entries, [])!
    const boxes = graph.vertices.filter(v => v.kind === 'node')
    const hub = graph.vertices.find(v => v.asn === 43260)!

    const middle = (Math.min(...boxes.map(v => v.y)) + Math.max(...boxes.map(v => v.y))) / 2
    expect(hub.y).toBe(middle)
  })

  it('leaves room between the node column and the first hop', () => {
    const graph = buildNetworkGraph(entries, [])!
    const box = graph.vertices.find(v => v.kind === 'node')!
    const hub = graph.vertices.find(v => v.asn === 43260)!

    const gap = hub.x - (box.x + graph.layout.boxW)
    expect(gap).toBe(graph.layout.colW - graph.layout.boxW + graph.layout.nodeGap)
    expect(graph.layout.nodeGap).toBeGreaterThan(0)
  })
})
