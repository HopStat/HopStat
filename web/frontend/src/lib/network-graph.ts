import { buildAsInfoMap, compressConsecutiveASPath, displayAsName, asCountryCode, formatASLabel } from '@/lib/as-info'
import type { ASInfo, NodeASPath } from '@/types/domain'

/** Layout geometry. Kept next to the algorithm so the component stays presentational. */
export interface Layout {
  colW: number
  rowH: number
  boxW: number
  boxH: number
  pad: number
  /** Extra room between the node column and the first hop, so the fan has space to open. */
  nodeGap: number
}

export const WIDE_LAYOUT: Layout = { colW: 140, rowH: 60, boxW: 116, boxH: 36, pad: 18, nodeGap: 28 }

/** Narrow screens get tighter geometry so the diagram needs less shrinking to fit. */
export const COMPACT_LAYOUT: Layout = { colW: 108, rowH: 48, boxW: 96, boxH: 30, pad: 10, nodeGap: 16 }

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
  /** The individual chains running through this vertex (see pathId): a node's fallbacks
   *  are distinct chains, so one can be traced without lighting its siblings. */
  paths: string[]
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
  /** The individual chains running over this edge (see pathId). */
  paths: string[]
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

/**
 * Left-to-right connector: out of the right edge, into the left edge. Bowed only when a box
 * actually stands in the way — drawn straight through one, the edge would read as though
 * the path went via that hop.
 */
function horizontalEdgePath(from: GraphVertex, to: GraphVertex, layout: Layout, blocked: boolean): string {
  const x1 = from.x + layout.boxW
  const x2 = to.x

  if (from.y === to.y) {
    if (!blocked) return `M ${x1} ${from.y} L ${x2} ${to.y}`
    // Up into the gap between this row and the one above, along it, and back down: a bow
    // only clears the boxes at its peak and clips the ones near either end.
    const lane = from.y - layout.rowH / 2
    const k = LANE_TURN
    return `M ${x1} ${from.y} C ${x1 + k} ${from.y}, ${x1 + k} ${lane}, ${x1 + 2 * k} ${lane} L ${x2 - 2 * k} ${lane} C ${x2 - k} ${lane}, ${x2 - k} ${to.y}, ${x2} ${to.y}`
  }

  const dx = Math.max((x2 - x1) * 0.4, 12)
  return `M ${x1} ${from.y} C ${x1 + dx} ${from.y}, ${x2 - dx} ${to.y}, ${x2} ${to.y}`
}

/** Top-to-bottom connector: out of the bottom edge, into the top edge, bowed only around a
 *  box that actually stands in the way. */
function verticalEdgePath(from: GraphVertex, to: GraphVertex, layout: Layout, blocked: boolean): string {
  const cx1 = from.x + layout.boxW / 2
  const cx2 = to.x + layout.boxW / 2
  const y1 = from.y + layout.boxH / 2
  const y2 = to.y - layout.boxH / 2

  if (cx1 === cx2) {
    if (!blocked) return `M ${cx1} ${y1} L ${cx2} ${y2}`
    const lane = cx1 - layout.colW / 2
    const k = LANE_TURN
    return `M ${cx1} ${y1} C ${cx1} ${y1 + k}, ${lane} ${y1 + k}, ${lane} ${y1 + 2 * k} L ${lane} ${y2 - 2 * k} C ${lane} ${y2 - k}, ${cx2} ${y2 - k}, ${cx2} ${y2}`
  }

  const dy = Math.max((y2 - y1) * 0.4, 12)
  return `M ${cx1} ${y1} C ${cx1} ${y1 + dy}, ${cx2} ${y2 - dy}, ${cx2} ${y2}`
}

/** How far a lane-routed edge travels along the path axis while it turns into its lane. */
const LANE_TURN = 10

/** Clearance kept between a skipping edge and a box it passes, in pixels. */
const EDGE_CLEARANCE = 6

/**
 * Whether the connector between two vertices would pass through a box at (col, row). Both
 * connectors are the same cubic along the path axis, so this works in path-axis ("main")
 * and sibling-axis ("cross") coordinates and serves either orientation. Same-row edges are
 * left out: those already bow around whatever stands in their way.
 */
function edgeCrossesBox(
  from: GraphVertex,
  to: GraphVertex,
  col: number,
  row: number,
  layout: Layout,
  vertical: boolean,
): boolean {
  if (from.row === to.row) return false
  const mainSpacing = vertical ? layout.rowH : layout.colW
  const crossSpacing = vertical ? layout.colW : layout.rowH
  const mainExtent = vertical ? layout.boxH : layout.boxW
  const crossExtent = vertical ? layout.boxW : layout.boxH
  const mainStart = (c: number) => layout.pad + c * mainSpacing + (c > 0 ? layout.nodeGap : 0)
  const crossCentre = (r: number) => layout.pad + r * crossSpacing + crossExtent / 2

  const m1 = mainStart(from.col) + mainExtent
  const m2 = mainStart(to.col)
  const c1 = crossCentre(from.row)
  const c2 = crossCentre(to.row)
  const d = Math.max((m2 - m1) * 0.4, 12)

  const boxStart = mainStart(col)
  const boxEnd = boxStart + mainExtent
  const half = crossExtent / 2 + EDGE_CLEARANCE
  const centre = crossCentre(row)

  for (let i = 0; i <= 64; i++) {
    const t = i / 64
    const u = 1 - t
    const m = u * u * u * m1 + 3 * u * u * t * (m1 + d) + 3 * u * t * t * (m2 - d) + t * t * t * m2
    if (m < boxStart || m > boxEnd) continue
    const c = u * u * u * c1 + 3 * u * u * t * c1 + 3 * u * t * t * c2 + t * t * t * c2
    if (Math.abs(c - centre) < half) return true
  }
  return false
}

const nodeKey = (nodeId: number) => `n:${nodeId}`
const asKey = (asn: number) => `as:${asn}`

/** Names one chain of one node — its live route or one specific fallback — so hovering a
 *  hop can light just the chain it lies on. The `|` keeps node 1 distinct from node 12. */
export const pathId = (nodeId: number, live: boolean, index: number) =>
  `${nodeId}|${live ? 'live' : `alt${index}`}`

/** The pathId prefix selecting every chain of one node. */
export const pathNodePrefix = (nodeId: number) => `${nodeId}|`

/** True for the pathId of a live (selected) route. */
export const isLivePath = (id: string) => id.endsWith('|live')

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

/** True when the element carries only a fallback route rather than a live one. */
export function isBackupFor(
  element: { selectedFor: number[]; backupFor: number[] },
  nodeId: number | null,
): boolean {
  // On the focused node's own path the question is per node: one node's fallback is
  // often another's live route.
  if (nodeId !== null && (element.selectedFor.includes(nodeId) || element.backupFor.includes(nodeId))) {
    return !element.selectedFor.includes(nodeId)
  }
  // Elsewhere on the map, judge it on its own merits — a hop no node routes over live is
  // a fallback no matter which node happens to be in focus.
  return element.selectedFor.length === 0 && element.backupFor.length > 0
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

  const addRole = (
    target: { selectedFor: number[]; backupFor: number[]; paths: string[] },
    nodeId: number,
    alternate: boolean,
    path: string,
  ) => {
    const list = alternate ? target.backupFor : target.selectedFor
    if (!list.includes(nodeId)) list.push(nodeId)
    if (!target.paths.includes(path)) target.paths.push(path)
  }

  const touchVertex = (key: string, seed: () => GraphVertex, nodeId: number, alternate: boolean, path: string) => {
    const existing = vertices.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      addRole(existing, nodeId, alternate, path)
      return existing
    }
    const created = seed()
    addRole(created, nodeId, alternate, path)
    vertices.set(key, created)
    edgeTargets.set(key, new Set())
    return created
  }

  const linkVertices = (from: string, to: string, nodeId: number, path: NodeASPath, id: string) => {
    const alternate = !path.best
    const viaDefaultRoute = Boolean(path.via_default_route)
    const key = `${from}|${to}`
    const existing = edges.get(key)
    if (existing) {
      if (!existing.nodeIds.includes(nodeId)) existing.nodeIds.push(nodeId)
      addRole(existing, nodeId, alternate, id)
      existing.viaDefaultRoute = existing.viaDefaultRoute && viaDefaultRoute
      return
    }
    // Never accept an edge that would close a cycle — the reverse pair already exists.
    if (edgeTargets.get(to)?.has(from)) return
    edgeTargets.get(from)?.add(to)
    const edge: GraphEdge = {
      key, from, to, nodeIds: [nodeId], selectedFor: [], backupFor: [], paths: [], viaDefaultRoute, d: '',
    }
    addRole(edge, nodeId, alternate, id)
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
      paths: [],
      viaDefaultRoute: selected.entry.via_default_route,
      prefix: selected.entry.prefix,
    }), id, false, pathId(id, Boolean(selected.entry.best), group.paths.indexOf(selected)))

    group.paths.forEach(({ entry, hops }, index) => {
      const alternate = !entry.best
      const chain = pathId(id, !alternate, index)
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
          paths: [],
        }), id, alternate, chain)
      })

      linkVertices(nodeKey(id), asKey(hops[0].asn), id, entry, chain)
      for (let i = 0; i < hops.length - 1; i++) {
        linkVertices(asKey(hops[i].asn), asKey(hops[i + 1].asn), id, entry, chain)
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
      // Sit each hop level with the nodes that use it rather than packing from the top:
      // a hop every node crosses lands centred on the fan, one only the last node reaches
      // belongs down beside it. Rows are fractional for that reason; collisions push down
      // to the next clear row, which keeps the sorted order.
      let nextFree = 0
      list.forEach(vertex => {
        vertex.row = Math.max(barycenter(vertex), nextFree)
        nextFree = vertex.row + 1
      })
    }
    list.forEach(vertex => { maxRow = Math.max(maxRow, vertex.row) })
  }

  // An edge that skips a column is drawn straight through it, and the rows above never saw
  // it: a hop could land right on the line and read as though that path ran through it.
  // Move such a hop down until every skipping edge clears its box.
  const columns = [...byColumn.keys()].filter(col => col > 0).sort((a, b) => a - b)
  const skipping = [...edges.values()]
  for (let pass = 0; pass < 3; pass++) {
    let moved = false
    for (const col of columns) {
      const crossing = skipping.flatMap(edge => {
        const from = vertices.get(edge.from)
        const to = vertices.get(edge.to)
        return from && to && from.col < col && col < to.col ? [{ from, to }] : []
      })
      if (crossing.length === 0) continue
      let nextFree = 0
      for (const vertex of [...(byColumn.get(col) ?? [])].sort((a, b) => a.row - b.row)) {
        let row = Math.max(vertex.row, nextFree)
        for (let tries = 0; tries < 4 * MAX_NODES && crossing.some(e => edgeCrossesBox(e.from, e.to, col, row, layout, vertical)); tries++) {
          row += 0.5
        }
        if (row !== vertex.row) {
          vertex.row = row
          moved = true
        }
        nextFree = row + 1
        maxRow = Math.max(maxRow, row)
      }
    }
    if (!moved) break
  }

  const maxCol = Math.max(...[...vertices.values()].map(vertex => vertex.col))

  // Vertical mode swaps the axes: paths run top to bottom and siblings spread sideways,
  // which is the shape a phone screen actually has room for.
  for (const vertex of vertices.values()) {
    const depthGap = vertex.col > 0 ? layout.nodeGap : 0
    if (vertical) {
      vertex.x = layout.pad + vertex.row * layout.colW
      vertex.y = layout.pad + vertex.col * layout.rowH + depthGap + layout.boxH / 2
    } else {
      vertex.x = layout.pad + vertex.col * layout.colW + depthGap
      vertex.y = layout.pad + vertex.row * layout.rowH + layout.boxH / 2
    }
  }

  // Whether a box stands in the way of a straight same-row edge. Rows are fractional, so a
  // box half a row off still overlaps the line; compare bands, not exact rows.
  const crossExtent = vertical ? layout.boxW : layout.boxH
  const crossSpacing = vertical ? layout.colW : layout.rowH
  const band = (crossExtent / 2 + EDGE_CLEARANCE) / crossSpacing
  const blockedBetween = (from: GraphVertex, to: GraphVertex) =>
    [...vertices.values()].some(v => v.col > from.col && v.col < to.col && Math.abs(v.row - from.row) < band)

  for (const edge of edges.values()) {
    const from = vertices.get(edge.from)
    const to = vertices.get(edge.to)
    if (!from || !to) continue
    const blocked = from.row === to.row && blockedBetween(from, to)
    edge.d = vertical
      ? verticalEdgePath(from, to, layout, blocked)
      : horizontalEdgePath(from, to, layout, blocked)
  }

  return {
    vertices: [...vertices.values()],
    edges: [...edges.values()],
    layout,
    vertical,
    width: vertical
      ? layout.pad * 2 + maxRow * layout.colW + layout.boxW
      : layout.pad * 2 + maxCol * layout.colW + layout.nodeGap + layout.boxW,
    height: vertical
      ? layout.pad * 2 + maxCol * layout.rowH + layout.nodeGap + layout.boxH
      : layout.pad * 2 + (maxRow + 1) * layout.rowH,
  }
}
