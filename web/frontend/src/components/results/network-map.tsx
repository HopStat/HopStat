import { useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '@/contexts/i18n-context'
import { buildAsInfoMap, asTooltipLines } from '@/lib/as-info'
import { buildNetworkGraph, BOX_H, BOX_W, type GraphVertex } from '@/lib/network-graph'
import type { ASInfo, NodeASPath } from '@/types/domain'

interface Props {
  entries: NodeASPath[] | undefined
  enriched: ASInfo[] | undefined
  prefix?: string
  /** Node the query ran on — its route stays highlighted when nothing is hovered. */
  queriedNodeId?: number
}

interface Tip {
  text: string
  x: number
  y: number
}

/** Colour token for a node, cycling through the eight theme-aware classes. */
function colorClass(nodeId: number | null | undefined, order: Map<number, number>): string {
  const index = nodeId === undefined || nodeId === null ? undefined : order.get(nodeId)
  return index === undefined ? '' : `nm-c${index % 8}`
}

export function NetworkMap({ entries, enriched, prefix, queriedNodeId }: Props) {
  const { t, locale } = useI18n()
  const graph = useMemo(
    () => buildNetworkGraph(entries, enriched, { queriedNodeId }),
    [entries, enriched, queriedNodeId],
  )
  const byAsn = useMemo(() => buildAsInfoMap(enriched ?? []), [enriched])
  const [hoveredNode, setHoveredNode] = useState<number | null>(null)
  const [tip, setTip] = useState<Tip | null>(null)

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

  function showTip(event: React.MouseEvent | React.FocusEvent, text: string) {
    if (!text) return
    const rect = (event.currentTarget as SVGGElement).getBoundingClientRect()
    setTip({ text, x: rect.left + rect.width / 2, y: rect.top - 6 })
  }

  function vertexTip(vertex: GraphVertex): string {
    if (vertex.kind === 'node') {
      return [vertex.prefix, vertex.viaDefaultRoute ? t('result.via_default_route') : '']
        .filter(Boolean)
        .join(' · ')
    }
    return asTooltipLines(byAsn.get(vertex.asn ?? 0), vertex.asn ?? 0, vertex.count, locale)[0] ?? ''
  }

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
            {graph.vertices.map(vertex => (
              <g
                key={vertex.key}
                tabIndex={0}
                role="button"
                aria-label={`${vertex.label}${vertex.org ? ` ${vertex.org}` : ''}`}
                className={[
                  'network-map__vertex',
                  vertex.kind === 'node' ? 'network-map__vertex--node' : '',
                  chainClass(vertex.nodeId, vertex.nodeIds),
                  isActive(vertex.nodeIds) ? 'is-active' : '',
                ].filter(Boolean).join(' ')}
                onMouseEnter={e => { setHoveredNode(vertex.nodeIds[0] ?? null); showTip(e, vertexTip(vertex)) }}
                onMouseLeave={() => { setHoveredNode(null); setTip(null) }}
                onFocus={e => { setHoveredNode(vertex.nodeIds[0] ?? null); showTip(e, vertexTip(vertex)) }}
                onBlur={() => { setHoveredNode(null); setTip(null) }}
              >
                <rect
                  className="network-map__box"
                  x={vertex.x}
                  y={vertex.y - BOX_H / 2}
                  width={BOX_W}
                  height={BOX_H}
                  rx={8}
                />
                <text className="network-map__label" x={vertex.x + BOX_W / 2} y={vertex.y - (vertex.org ? 6 : 0)}>
                  {vertex.label}
                </text>
                {vertex.org && (
                  <text className="network-map__sub" x={vertex.x + BOX_W / 2} y={vertex.y + 8}>
                    {vertex.org}
                  </text>
                )}
              </g>
            ))}
          </g>
        </svg>
      </div>

      {tip && createPortal(
        <div
          role="tooltip"
          className="as-path-tooltip pointer-events-none fixed z-[200] w-max max-w-[24rem] -translate-x-1/2 -translate-y-full truncate whitespace-nowrap rounded-md px-2.5 py-1 text-[11px] font-normal normal-case leading-none"
          style={{ left: tip.x, top: tip.y }}
        >
          {tip.text}
        </div>,
        document.body,
      )}
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
