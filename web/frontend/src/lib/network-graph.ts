import { buildAsInfoMap, compressConsecutiveASPath, displayAsName, asCountryCode, formatASLabel } from '@/lib/as-info'
import type { ASInfo, NodeASPath } from '@/types/domain'

/** Layout geometry. Kept next to the algorithm so the component stays presentational. */
export interface Layout {
  colW: number
  rowH: number
  boxW: number
  boxH: number
  pad: number
}

export const WIDE_LAYOUT: Layout = { colW: 140, rowH: 60, boxW: 116, boxH: 36, pad: 18 }

/** Narrow screens get tighter geometry so the diagram needs less shrinking to fit. */
export const COMPACT_LAYOUT: Layout = { colW: 108, rowH: 48, boxW: 96, boxH: 30, pad: 10 }

/** Caption font size, and the advance width of IBM Plex Mono at that size. Knowing the
 *  advance lets captions be placed and trimmed without measuring rendered text. */
export const SUB_FONT = 9
export const SUB_ADVANCE = SUB_FONT * 0.6

/**
 * Trims an operator name to what a box can hold, at a word boundary where possible, so a
 * long name never spills past the box that labels it.
 */
export function fitCaption(org: string, available: number): string {
  const maxChars = Math.floor(available / SUB_ADVANCE)
  if (maxChars <= 0) return ''
  if (org.length <= maxChars) return org

  const cut = org.slice(0, maxChars)
  const lastSpace = cut.lastIndexOf(' ')
  return lastSpace > 2 ? cut.slice(0, lastSpace) : cut
}

/** Beyond these the diagram stops being readable, so the tail is dropped rather than drawn. */
export const MAX_COLS = 14
export const MAX_NODES = 24

export interface GraphVertex {
  key: string
  kind: 'node' | 'as'
  label: string
  org: string
  cc: string
  /** Country shown next to the AS name: flag when we have one, otherwise the code. */
  flag: string
  asn?: number
  nodeId?: number
  count: number
  col: number
  row: number
  x: number
  y: number
  /** Which looking-glass nodes traverse this vertex. */
  nodeIds: number[]
  /** Nodes whose selected route passes through this hop, and nodes for which it is only
   *  a backup. */
  selectedFor: number[]
  backupFor: number[]
  viaDefaultRoute?: boolean
  prefix?: string
}

export interface GraphEdge {
  key: string
  from: string
  to: string
  nodeIds: number[]
  /** Nodes whose selected route runs over this edge, and nodes for which it is a backup.
   *  Kept per node because one node's fallback is often another's live path. */
  selectedFor: number[]
  backupFor: number[]
  viaDefaultRoute: boolean
  d: string
}

export interface NetworkGraph {
  vertices: GraphVertex[]
  edges: GraphEdge[]
  layout: Layout
  /** True when paths run top to bottom instead of left to right. */
  vertical: boolean
  width: number
  height: number
}

interface NormalizedEntry {
  entry: NodeASPath
  hops: { asn: number; count: number }[]
}

/** One node with the paths it holds: its selected route first, then any backups. */
interface NodeGroup {
  nodeId: number
  name: string
  paths: NormalizedEntry[]
}

/** Left-to-right connector: out of the right edge, into the left edge. */
function horizontalEdgePath(from: GraphVertex, to: GraphVertex, layout: Layout): string {
  const x1 = from.x + layout.boxW
  const x2 = to.x
  if (from.y === to.y) return `M ${x1} ${from.y} L ${x2} ${to.y}`
  const dx = Math.max((x2 - x1) * 0.4, 12)
  return `M ${x1} ${from.y} C ${x1 + dx} ${from.y}, ${x2 - dx} ${to.y}, ${x2} ${to.y}`
}

/** Top-to-bottom connector: out of the bottom edge, into the top edge. */
function verticalEdgePath(from: GraphVertex, to: GraphVertex, layout: Layout): string {
  const cx1 = from.x + layout.boxW / 2
  const cx2 = to.x + layout.boxW / 2
  const y1 = from.y + layout.boxH / 2
  const y2 = to.y - layout.boxH / 2
  if (cx1 === cx2) return `M ${cx1} ${y1} L ${cx2} ${y2}`
  const dy = Math.max((y2 - y1) * 0.4, 12)
  return `M ${cx1} ${y1} C ${cx1} ${y1 + dy}, ${cx2} ${y2 - dy}, ${cx2} ${y2}`
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

/** True when the element carries only a fallback route for the node currently in focus. */
export function isBackupFor(
  element: { selectedFor: number[]; backupFor: number[] },
  nodeId: number | null,
): boolean {
  if (nodeId === null) {
    // Nothing in focus: only call it a backup when no node routes over it live.
    return element.selectedFor.length === 0 && element.backupFor.length > 0
  }
  return element.backupFor.includes(nodeId) && !element.selectedFor.includes(nodeId)
}

export function buildNetworkGraph(
  entries: NodeASPath[] | undefined,
  enriched: ASInfo[] | undefined,
  opts: { queriedNodeId?: number; compact?: boolean; vertical?: boolean } = {},
): NetworkGraph | null {
  const layout = opts.compact ? COMPACT_LAYOUT : WIDE_LAYOUT
  const vertical = Boolean(opts.vertical)
  // Group by node: one box per node, one branch per path it holds.
  const groups: NodeGroup[] = []
  const groupByNode = new Map<number, NodeGroup>()

  for (const entry of entries ?? []) {
    const hops = entry.no_route ? [] : normalizePath(entry.as_path ?? [])
    if (hops.length === 0) continue

    let group = groupByNode.get(entry.node_id)
    if (!group) {
      group = { nodeId: entry.node_id, name: entry.node_name, paths: [] }
      groupByNode.set(entry.node_id, group)
      groups.push(group)
    }
    group.paths.push({ entry, hops })
  }

  if (groups.length === 0) return null

  // The queried node leads: it takes the first row and the first colour, and its route is
  // the one highlighted by default.
  if (opts.queriedNodeId !== undefined) {
    const queried = groups.findIndex(g => g.nodeId === opts.queriedNodeId)
    if (queried > 0) groups.unshift(...groups.splice(queried, 1))
  }

  const capped = groups.slice(0, MAX_NODES)
  const byAsn = buildAsInfoMap(enriched ?? [])

  const vertices = new Map<string, GraphVertex>()
  const edgeTargets = new Map<string, Set<string>>()
  const edges = new Map<string, GraphEdge>()

  const addRole = (target: { selectedFor: number[]; backupFor: number[] }, nodeId: number, alternate: boolean) => {
    const list = alternate ? target.backupFor : target.selectedFor
    if (!list.includes(nodeId)) list.push(nodeId)
  }

  const touchVertex = (key: string, seed: () => GraphVertex, nodeId: number, alternate = false) => {
    const existing = vertices.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      addRole(existing, nodeId, alternate)
      return existing
    }
    const created = seed()
    addRole(created, nodeId, alternate)
    vertices.set(key, created)
    edgeTargets.set(key, new Set())
    return created
  }

  const linkVertices = (from: string, to: string, nodeId: number, path: NodeASPath) => {
    const alternate = !path.best
    const viaDefaultRoute = Boolean(path.via_default_route)
    const key = `${from}|${to}`
    const existing = edges.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      addRole(existing, nodeId, alternate)
      existing.viaDefaultRoute = existing.viaDefaultRoute && viaDefaultRoute
      return
    }
    // Never accept an edge that would close a cycle — the reverse pair already exists.
    if (edgeTargets.get(to)?.has(from)) return
    edgeTargets.get(from)?.add(to)
    const edge: GraphEdge = {
      key, from, to, nodeIds: [nodeId], selectedFor: [], backupFor: [], viaDefaultRoute, d: '',
    }
    addRole(edge, nodeId, alternate)
    edges.set(key, edge)
  }

  capped.forEach(group => {
    const id = group.nodeId
    const selected = group.paths.find(p => p.entry.best) ?? group.paths[0]

    touchVertex(nodeKey(id), () => ({
      key: nodeKey(id),
      kind: 'node',
      label: group.name,
      org: '',
      cc: '',
      flag: '',
      nodeId: id,
      count: 1,
      col: 0,
      row: 0,
      x: 0,
      y: 0,
      nodeIds: [id],
      selectedFor: [],
      backupFor: [],
      viaDefaultRoute: selected.entry.via_default_route,
      prefix: selected.entry.prefix,
    }), id)

    group.paths.forEach(({ entry, hops }) => {
      const alternate = !entry.best
      hops.forEach(hop => {
        const info = byAsn.get(hop.asn)
        touchVertex(asKey(hop.asn), () => ({
          key: asKey(hop.asn),
          kind: 'as',
          label: formatASLabel(hop.asn, hop.count),
          org: displayAsName(info),
          cc: asCountryCode(info),
          flag: info?.flag_emoji ?? '',
          asn: hop.asn,
          count: hop.count,
          col: 0,
          row: 0,
          x: 0,
          y: 0,
          nodeIds: [id],
          selectedFor: [],
          backupFor: [],
        }), id, alternate)
      })

      linkVertices(nodeKey(id), asKey(hops[0].asn), id, entry)
      for (let i = 0; i < hops.length - 1; i++) {
        linkVertices(asKey(hops[i].asn), asKey(hops[i + 1].asn), id, entry)
      }
    })
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
  capped.forEach((group, index) => nodeRow.set(group.nodeId, index))

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

  // Vertical mode swaps the axes: paths run top to bottom and siblings spread sideways,
  // which is the shape a phone screen actually has room for.
  for (const vertex of vertices.values()) {
    if (vertical) {
      vertex.x = layout.pad + vertex.row * layout.colW
      vertex.y = layout.pad + vertex.col * layout.rowH + layout.boxH / 2
    } else {
      vertex.x = layout.pad + vertex.col * layout.colW
      vertex.y = layout.pad + vertex.row * layout.rowH + layout.boxH / 2
    }
  }

  for (const edge of edges.values()) {
    const from = vertices.get(edge.from)
    const to = vertices.get(edge.to)
    if (!from || !to) continue
    edge.d = vertical ? verticalEdgePath(from, to, layout) : horizontalEdgePath(from, to, layout)
  }

  return {
    vertices: [...vertices.values()],
    edges: [...edges.values()],
    layout,
    vertical,
    width: vertical
      ? layout.pad * 2 + maxRow * layout.colW + layout.boxW
      : layout.pad * 2 + maxCol * layout.colW + layout.boxW,
    height: vertical
      ? layout.pad * 2 + maxCol * layout.rowH + layout.boxH
      : layout.pad * 2 + (maxRow + 1) * layout.rowH,
  }
}
