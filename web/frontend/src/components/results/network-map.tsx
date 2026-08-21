import { useMemo, useState } from 'react'
import { useI18n } from '@/contexts/i18n-context'
import { buildNetworkGraph, BOX_H, BOX_W, type GraphVertex } from '@/lib/network-graph'
import type { ASInfo, NodeASPath } from '@/types/domain'

interface Props {
  entries: NodeASPath[] | undefined
  enriched: ASInfo[] | undefined
  prefix?: string
  /** Node the query ran on — its route stays highlighted when nothing is hovered. */
  queriedNodeId?: number
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
  const graph = useMemo(
    () => buildNetworkGraph(entries, enriched, { queriedNodeId }),
    [entries, enriched, queriedNodeId],
  )
  const [hoveredNode, setHoveredNode] = useState<number | null>(null)

  const nodeOrder = useMemo(() => {
    const order = new Map<number, number>()
    graph?.vertices.filter(v => v.kind === 'node').forEach((v, i) => order.set(v.nodeId ?? -1, i))
    return order
  }, [graph])

  if (!graph) return null

  const title = t('result.network_map')
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

      <div className="result-surface--path__scroll">
        <svg
          role="img"
          aria-label={title}
          width={graph.width}
          height={graph.height}
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
                  y={vertex.y - BOX_H / 2}
                  width={BOX_W}
                  height={BOX_H}
                  rx={8}
                />
                <text
                  className="network-map__label"
                  x={vertex.x + BOX_W / 2}
                  y={vertex.y - (asCaption(vertex) ? 6 : 0)}
                >
                  {vertex.label}
                </text>
                {asCaption(vertex) && (
                  <text className="network-map__sub" x={vertex.x + BOX_W / 2} y={vertex.y + 8}>
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
