import { buildAsInfoMap, compressConsecutiveASPath, displayAsName, asCountryCode, formatASLabel } from '@/lib/as-info'
import type { ASInfo, NodeASPath } from '@/types/domain'

/** Layout geometry. Kept next to the algorithm so the component stays presentational. */
export const COL_W = 140
export const ROW_H = 60
export const BOX_W = 116
export const BOX_H = 36
export const PAD = 18

/** Beyond these the diagram stops being readable, so the tail is dropped rather than drawn. */
export const MAX_COLS = 14
export const MAX_NODES = 24

export interface GraphVertex {
  key: string
  kind: 'node' | 'as'
  label: string
  org: string
  cc: string
  asn?: number
  nodeId?: number
  count: number
  col: number
  row: number
  x: number
  y: number
  /** Which looking-glass nodes traverse this vertex. */
  nodeIds: number[]
  viaDefaultRoute?: boolean
  prefix?: string
}

export interface GraphEdge {
  key: string
  from: string
  to: string
  nodeIds: number[]
  viaDefaultRoute: boolean
  d: string
}

export interface NetworkGraph {
  vertices: GraphVertex[]
  edges: GraphEdge[]
  /** Nodes that see no route at all — shown beside the graph, not in it. */
  unrouted: NodeASPath[]
  width: number
  height: number
}

interface NormalizedEntry {
  entry: NodeASPath
  hops: { asn: number; count: number }[]
}

const nodeKey = (nodeId: number) => `n:${nodeId}`
const asKey = (asn: number) => `as:${asn}`

/**
 * Collapses prepends and drops non-consecutive repeats (AS path poisoning), so each path
 * visits every AS at most once and the graph stays acyclic.
 */
function normalizePath(path: number[]): { asn: number; count: number }[] {
  const compressed = compressConsecutiveASPath(path.filter(asn => asn > 0))
  const seen = new Set<number>()
  const hops: { asn: number; count: number }[] = []
  for (const hop of compressed) {
    if (seen.has(hop.asn)) continue
    seen.add(hop.asn)
    hops.push(hop)
  }
  return hops
}

/**
 * Longest-path layering over the reversed DAG. Using the longest distance to a sink (rather
 * than each path's own hop index) is what keeps every edge pointing left→right when nodes
 * disagree on path length, or share a prefix and then diverge.
 */
function layerVertices(keys: string[], edges: Map<string, Set<string>>): Map<string, number> {
  const outDegree = new Map<string, number>()
  const incoming = new Map<string, string[]>()
  for (const key of keys) {
    outDegree.set(key, 0)
    incoming.set(key, [])
  }
  for (const [from, tos] of edges) {
    for (const to of tos) {
      outDegree.set(from, (outDegree.get(from) ?? 0) + 1)
      incoming.get(to)?.push(from)
    }
  }

  // Kahn from the sinks backwards; iterative, so a hostile path length cannot blow the stack.
  const layer = new Map<string, number>()
  const queue: string[] = []
  for (const key of keys) {
    if ((outDegree.get(key) ?? 0) === 0) {
      layer.set(key, 0)
      queue.push(key)
    }
  }
  const remaining = new Map(outDegree)
  while (queue.length > 0) {
    const key = queue.shift() as string
    for (const from of incoming.get(key) ?? []) {
      layer.set(from, Math.max(layer.get(from) ?? 0, (layer.get(key) ?? 0) + 1))
      const left = (remaining.get(from) ?? 0) - 1
      remaining.set(from, left)
      if (left === 0) queue.push(from)
    }
  }

  // Anything left sits on a cycle the per-path dedupe could not prevent (two nodes
  // disagreeing on hop order). Place it rather than dropping the whole graph.
  for (const key of keys) {
    if (!layer.has(key)) layer.set(key, 0)
  }
  return layer
}

export function buildNetworkGraph(
  entries: NodeASPath[] | undefined,
  enriched: ASInfo[] | undefined,
  opts: { queriedNodeId?: number } = {},
): NetworkGraph | null {
  const routed: NormalizedEntry[] = []
  const unrouted: NodeASPath[] = []

  for (const entry of entries ?? []) {
    const hops = entry.no_route ? [] : normalizePath(entry.as_path ?? [])
    if (hops.length === 0) {
      unrouted.push(entry)
      continue
    }
    routed.push({ entry, hops })
  }

  // With one routed node this only repeats the AS path map above it.
  if (routed.length < 2) return null

  // The queried node leads: it takes the first row and the first colour, and its route is
  // the one highlighted by default.
  if (opts.queriedNodeId !== undefined) {
    const queried = routed.findIndex(r => r.entry.node_id === opts.queriedNodeId)
    if (queried > 0) routed.unshift(...routed.splice(queried, 1))
  }

  const capped = routed.slice(0, MAX_NODES)
  const byAsn = buildAsInfoMap(enriched ?? [])

  const vertices = new Map<string, GraphVertex>()
  const edgeTargets = new Map<string, Set<string>>()
  const edges = new Map<string, GraphEdge>()

  const touchVertex = (key: string, seed: () => GraphVertex, nodeId: number) => {
    const existing = vertices.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      return existing
    }
    const created = seed()
    vertices.set(key, created)
    edgeTargets.set(key, new Set())
    return created
  }

  const linkVertices = (from: string, to: string, nodeId: number, viaDefaultRoute: boolean) => {
    const key = `${from}|${to}`
    const existing = edges.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      existing.viaDefaultRoute = existing.viaDefaultRoute && viaDefaultRoute
      return
    }
    // Never accept an edge that would close a cycle — the reverse pair already exists.
    if (edgeTargets.get(to)?.has(from)) return
    edgeTargets.get(from)?.add(to)
    edges.set(key, { key, from, to, nodeIds: [nodeId], viaDefaultRoute, d: '' })
  }

  capped.forEach(({ entry, hops }) => {
    const id = entry.node_id
    touchVertex(nodeKey(id), () => ({
      key: nodeKey(id),
      kind: 'node',
      label: entry.node_name,
      org: '',
      cc: '',
      nodeId: id,
      count: 1,
      col: 0,
      row: 0,
      x: 0,
      y: 0,
      nodeIds: [id],
      viaDefaultRoute: entry.via_default_route,
      prefix: entry.prefix,
    }), id)

    hops.forEach(hop => {
      const info = byAsn.get(hop.asn)
      touchVertex(asKey(hop.asn), () => ({
        key: asKey(hop.asn),
        kind: 'as',
        label: formatASLabel(hop.asn, hop.count),
        org: displayAsName(info),
        cc: asCountryCode(info),
        asn: hop.asn,
        count: hop.count,
        col: 0,
        row: 0,
        x: 0,
        y: 0,
        nodeIds: [id],
      }), id)
    })

    linkVertices(nodeKey(id), asKey(hops[0].asn), id, Boolean(entry.via_default_route))
    for (let i = 0; i < hops.length - 1; i++) {
      linkVertices(asKey(hops[i].asn), asKey(hops[i + 1].asn), id, false)
    }
  })

  const keys = [...vertices.keys()]
  const layer = layerVertices(keys, edgeTargets)
  const maxLayer = Math.max(...keys.map(key => layer.get(key) ?? 0))

  // Node boxes are pinned to column 0 so the diagram reads as a fan-in.
  for (const vertex of vertices.values()) {
    vertex.col = vertex.kind === 'node' ? 0 : Math.min(maxLayer - (layer.get(vertex.key) ?? 0), MAX_COLS)
    if (vertex.kind === 'as' && vertex.col < 1) vertex.col = 1
  }

  // Rows: node boxes keep input order, AS vertices sit at the mean row of the nodes that
  // traverse them, which keeps shared hops centred between their branches.
  const nodeRow = new Map<number, number>()
  capped.forEach(({ entry }, index) => nodeRow.set(entry.node_id, index))

  const byColumn = new Map<number, GraphVertex[]>()
  for (const vertex of vertices.values()) {
    const list = byColumn.get(vertex.col) ?? []
    list.push(vertex)
    byColumn.set(vertex.col, list)
  }

  let maxRow = 0
  for (const [col, list] of byColumn) {
    if (col === 0) {
      list.forEach(vertex => { vertex.row = nodeRow.get(vertex.nodeId ?? -1) ?? 0 })
    } else {
      const barycenter = (vertex: GraphVertex) => {
        const rows = vertex.nodeIds.map(id => nodeRow.get(id) ?? 0)
        return rows.reduce((sum, row) => sum + row, 0) / (rows.length || 1)
      }
      list.sort((a, b) => barycenter(a) - barycenter(b) || (a.asn ?? 0) - (b.asn ?? 0))
      list.forEach((vertex, index) => { vertex.row = index })
    }
    list.forEach(vertex => { maxRow = Math.max(maxRow, vertex.row) })
  }

  const maxCol = Math.max(...[...vertices.values()].map(vertex => vertex.col))
  for (const vertex of vertices.values()) {
    vertex.x = PAD + vertex.col * COL_W
    vertex.y = PAD + vertex.row * ROW_H + BOX_H / 2
  }

  for (const edge of edges.values()) {
    const from = vertices.get(edge.from)
    const to = vertices.get(edge.to)
    if (!from || !to) continue
    const x1 = from.x + BOX_W
    const x2 = to.x
    if (from.y === to.y) {
      edge.d = `M ${x1} ${from.y} L ${x2} ${to.y}`
    } else {
      const dx = Math.max((x2 - x1) * 0.4, 12)
      edge.d = `M ${x1} ${from.y} C ${x1 + dx} ${from.y}, ${x2 - dx} ${to.y}, ${x2} ${to.y}`
    }
  }

  return {
    vertices: [...vertices.values()],
    edges: [...edges.values()],
    unrouted,
    width: PAD * 2 + maxCol * COL_W + BOX_W,
    height: PAD * 2 + (maxRow + 1) * ROW_H,
  }
}
