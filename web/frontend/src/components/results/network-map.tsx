import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useI18n } from '@/contexts/i18n-context'
import { buildNetworkGraph, type GraphVertex } from '@/lib/network-graph'
import type { ASInfo, NodeASPath } from '@/types/domain'

interface Props {
  entries: NodeASPath[] | undefined
  enriched: ASInfo[] | undefined
  prefix?: string
  /** Node the query ran on — its route stays highlighted when nothing is hovered. */
  queriedNodeId?: number
}

/**
 * Below this the labels stop being readable, so a very wide graph scrolls instead of
 * shrinking further.
 */
const MIN_SCALE = 0.4

/** Below this the wide geometry no longer earns its extra width. */
const COMPACT_BELOW = 520

/** Tracks the usable width so the diagram can be scaled down to fit narrow screens. */
function useAvailableWidth(): [React.RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    setWidth(el.clientWidth)
  }, [])

  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(entries => {
      setWidth(entries[0].contentRect.width)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return [ref, width]
}

/** AS name with its country beside it — the flag when the lookup gave us one. */
function asCaption(vertex: GraphVertex): string {
  const country = vertex.flag || vertex.cc
  return [country, vertex.org].filter(Boolean).join(' ')
}

/** Colour token for a node, cycling through the eight theme-aware classes. */
function colorClass(nodeId: number | null | undefined, order: Map<number, number>): string {
  const index = nodeId === undefined || nodeId === null ? undefined : order.get(nodeId)
  return index === undefined ? '' : `nm-c${index % 8}`
}

export function NetworkMap({ entries, enriched, prefix, queriedNodeId }: Props) {
  const { t } = useI18n()
  const [hoveredNode, setHoveredNode] = useState<number | null>(null)
  const [wrapRef, availableWidth] = useAvailableWidth()
  const compact = availableWidth > 0 && availableWidth < COMPACT_BELOW
  const graph = useMemo(
    () => buildNetworkGraph(entries, enriched, { queriedNodeId, compact }),
    [entries, enriched, queriedNodeId, compact],
  )

  const nodeOrder = useMemo(() => {
    const order = new Map<number, number>()
    graph?.vertices.filter(v => v.kind === 'node').forEach((v, i) => order.set(v.nodeId ?? -1, i))
    return order
  }, [graph])

  if (!graph) return null

  const title = t('result.network_map')
  // Fit the diagram to the panel rather than making the reader scroll, but never scale it
  // up past 1:1 and never past the point where the labels stop being legible.
  const scale = availableWidth > 0
    ? Math.max(MIN_SCALE, Math.min(1, availableWidth / graph.width))
    : 1
  // Falls back to the queried node so its route reads as the active one at rest.
  const activeNode = hoveredNode ?? queriedNodeId ?? null
  const isActive = (nodeIds: number[]) => activeNode !== null && nodeIds.includes(activeNode)

  // A highlighted chain takes the hovered node's colour end to end, including the hops it
  // shares with other nodes — otherwise the traced line changes colour halfway.
  const chainClass = (vertexNodeId: number | undefined, nodeIds: number[]) =>
    isActive(nodeIds) ? colorClass(activeNode, nodeOrder) : colorClass(vertexNodeId, nodeOrder)

  return (
    <div className="result-surface result-surface--path network-map animate-fade-up px-3 py-3 sm:px-5 sm:py-4">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="font-data text-sm font-bold sm:text-base">{title}</span>
        {prefix && <span className="font-data text-[11px] text-muted-foreground">{prefix}</span>}
      </div>

      <div className="result-surface--path__scroll network-map__canvas" ref={wrapRef}>
        <svg
          role="img"
          aria-label={title}
          width={graph.width * scale}
          height={graph.height * scale}
          viewBox={`0 0 ${graph.width} ${graph.height}`}
        >
          <desc>
            {graph.vertices
              .filter(v => v.kind === 'node')
              .map(v => `${v.label}: ${describePath(graph, v)}`)
              .join('; ')}
          </desc>

          <g>
            {graph.edges.map(edge => (
              <path
                key={edge.key}
                d={edge.d}
                className={[
                  'network-map__edge',
                  chainClass(undefined, edge.nodeIds),
                  edge.alternate ? 'is-alt' : '',
                  edge.viaDefaultRoute ? 'is-default' : '',
                  isActive(edge.nodeIds) ? 'is-active' : '',
                ].filter(Boolean).join(' ')}
              />
            ))}
          </g>

          <g>
            {graph.vertices.map(vertex => {
              const isNodeBox = vertex.kind === 'node'
              const trace = isNodeBox
                ? {
                    tabIndex: 0,
                    onMouseEnter: () => setHoveredNode(vertex.nodeId ?? null),
                    onMouseLeave: () => setHoveredNode(null),
                    onFocus: () => setHoveredNode(vertex.nodeId ?? null),
                    onBlur: () => setHoveredNode(null),
                  }
                : {}
              return (
              <g
                key={vertex.key}
                aria-label={[vertex.label, vertex.org, vertex.cc].filter(Boolean).join(' ')}
                className={[
                  'network-map__vertex',
                  isNodeBox ? 'network-map__vertex--node' : '',
                  chainClass(vertex.nodeId, vertex.nodeIds),
                  isActive(vertex.nodeIds) ? 'is-active' : '',
                ].filter(Boolean).join(' ')}
                {...trace}
              >
                <rect
                  className="network-map__box"
                  x={vertex.x}
                  y={vertex.y - graph.layout.boxH / 2}
                  width={graph.layout.boxW}
                  height={graph.layout.boxH}
                  rx={8}
                />
                <text
                  className="network-map__label"
                  x={vertex.x + graph.layout.boxW / 2}
                  y={vertex.y - (asCaption(vertex) ? 6 : 0)}
                >
                  {vertex.label}
                </text>
                {asCaption(vertex) && (
                  <text className="network-map__sub" x={vertex.x + graph.layout.boxW / 2} y={vertex.y + 8}>
                    {asCaption(vertex)}
                  </text>
                )}
              </g>
              )
            })}
          </g>
        </svg>
      </div>
    </div>
  )
}

/** Flattens one node's branch for the SVG description, so the map is readable without sight. */
function describePath(graph: NonNullable<ReturnType<typeof buildNetworkGraph>>, node: GraphVertex): string {
  const id = node.nodeId ?? -1
  return graph.vertices
    .filter(v => v.kind === 'as' && v.nodeIds.includes(id))
    .sort((a, b) => a.col - b.col)
    .map(v => v.label)
    .join(' → ')
}
