import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useI18n } from '@/contexts/i18n-context'
import { buildNetworkGraph, fitCaption, isBackupFor, SUB_ADVANCE, type GraphVertex } from '@/lib/network-graph'
import type { ASInfo, NodeASPath } from '@/types/domain'

interface Props {
  entries: NodeASPath[] | undefined
  enriched: ASInfo[] | undefined
  /** Node the query ran on — its route stays highlighted when nothing is hovered. */
  queriedNodeId?: number
  /** Re-runs the query from the clicked node. */
  onNodeSelect?: (nodeId: number) => void
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

const FLAG_W = 12
const FLAG_H = 9
const FLAG_GAP = 3
/** Keeps the caption off the rounded corners. */
const CAPTION_INSET = 4

/** Flag image from the same source the rest of the app uses — emoji flags do not render on
 *  every platform, and at this size they are unreadable where they do. */
function flagSrc(cc: string): string | null {
  const code = cc.trim().toLowerCase()
  return /^[a-z]{2}$/.test(code) ? `https://flagcdn.com/16x12/${code}.png` : null
}

/** Colour token for a node, cycling through the eight theme-aware classes. */
function colorClass(nodeId: number | null | undefined, order: Map<number, number>): string {
  const index = nodeId === undefined || nodeId === null ? undefined : order.get(nodeId)
  return index === undefined ? '' : `nm-c${index % 8}`
}

/** What the pointer rests on: a whole node's traffic, or just one of its chains. Hovering
 *  the node box shows everything the node holds; hovering a hop shows only the chain the
 *  hop is actually part of, so a fallback hop does not light the live route beside it. */
interface HoverTarget {
  nodeId: number
  scope: 'all' | 'live' | 'backup'
}

export function NetworkMap({ entries, enriched, queriedNodeId, onNodeSelect }: Props) {
  const { t } = useI18n()
  const [hovered, setHovered] = useState<HoverTarget | null>(null)
  const [wrapRef, availableWidth] = useAvailableWidth()
  const compact = availableWidth > 0 && availableWidth < COMPACT_BELOW
  // A phone has height to spare and no width, so the diagram turns to run downwards.
  const graph = useMemo(
    () => buildNetworkGraph(entries, enriched, { queriedNodeId, compact, vertical: compact }),
    [entries, enriched, queriedNodeId, compact],
  )

  const nodeOrder = useMemo(() => {
    const order = new Map<number, number>()
    graph?.vertices.filter(v => v.kind === 'node').forEach((v, i) => order.set(v.nodeId ?? -1, i))
    return order
  }, [graph])

  if (!graph) return null

  // No visible heading: the result panel already names the query above the map. The label
  // stays for assistive tech, which has no such context.
  const title = t('result.network_map')
  // Fit the diagram to the panel rather than making the reader scroll, but never scale it
  // up past 1:1 and never past the point where the labels stop being legible.
  const scale = availableWidth > 0
    ? Math.max(MIN_SCALE, Math.min(1, availableWidth / graph.width))
    : 1
  // Falls back to the queried node so its route reads as the active one at rest.
  const active: HoverTarget | null = hovered
    ?? (queriedNodeId !== undefined ? { nodeId: queriedNodeId, scope: 'all' } : null)
  const activeNode = active?.nodeId ?? null

  const isActive = (el: { nodeIds: number[]; selectedFor: number[]; backupFor: number[]; kind?: string }) => {
    if (!active || !el.nodeIds.includes(active.nodeId)) return false
    // The node box anchors every chain it holds, so it lights whichever one is traced.
    if (active.scope === 'all' || el.kind === 'node') return true
    const carrier = active.scope === 'live' ? el.selectedFor : el.backupFor
    return carrier.includes(active.nodeId)
  }

  // A highlighted chain takes the hovered node's colour end to end, including the hops it
  // shares with other nodes — otherwise the traced line changes colour halfway.
  const chainClass = (el: Parameters<typeof isActive>[0], ownColor: number | undefined) =>
    isActive(el) ? colorClass(activeNode, nodeOrder) : colorClass(ownColor, nodeOrder)

  // Hovering a hop lights the chain that runs through it — and only that chain: a hop
  // carrying nothing but a node's fallback traces the fallback, not the live route beside
  // it. A shared hop keeps the node already in focus, so tracing a path hop by hop never
  // jumps to a sibling's route; elsewhere the first node routing over the hop live wins.
  const hopTarget = (vertex: GraphVertex): HoverTarget | null => {
    const nodeId = activeNode !== null && vertex.nodeIds.includes(activeNode)
      ? activeNode
      : vertex.selectedFor[0] ?? vertex.nodeIds[0]
    if (nodeId === undefined) return null
    return { nodeId, scope: vertex.selectedFor.includes(nodeId) ? 'live' : 'backup' }
  }

  return (
    <div className="result-surface result-surface--path network-map animate-fade-up px-3 py-3 sm:px-5 sm:py-4">
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
                  chainClass(edge, undefined),
                  isBackupFor(edge, activeNode) ? 'is-alt' : '',
                  edge.viaDefaultRoute ? 'is-default' : '',
                  isActive(edge) ? 'is-active' : '',
                ].filter(Boolean).join(' ')}
              />
            ))}
          </g>

          <g>
            {graph.vertices.map(vertex => {
              const isNodeBox = vertex.kind === 'node'
              const selectable = isNodeBox && onNodeSelect && vertex.nodeId !== undefined
              const run = () => { if (selectable) onNodeSelect(vertex.nodeId as number) }
              const trace = isNodeBox
                ? {
                    tabIndex: 0,
                    role: selectable ? 'button' : undefined,
                    onMouseEnter: () => setHovered(vertex.nodeId === undefined ? null : { nodeId: vertex.nodeId, scope: 'all' }),
                    onMouseLeave: () => setHovered(null),
                    onFocus: () => setHovered(vertex.nodeId === undefined ? null : { nodeId: vertex.nodeId, scope: 'all' }),
                    onBlur: () => setHovered(null),
                    onClick: run,
                    onKeyDown: (e: React.KeyboardEvent) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        run()
                      }
                    },
                  }
                : {
                    onMouseEnter: () => setHovered(hopTarget(vertex)),
                    onMouseLeave: () => setHovered(null),
                  }
              return (
              <g
                key={vertex.key}
                aria-label={[vertex.label, vertex.org, vertex.cc].filter(Boolean).join(' ')}
                className={[
                  'network-map__vertex',
                  isNodeBox ? 'network-map__vertex--node' : '',
                  isNodeBox ? '' : isBackupFor(vertex, activeNode) ? 'is-alt' : '',
                  chainClass(vertex, vertex.nodeId),
                  isActive(vertex) ? 'is-active' : '',
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
                  y={vertex.y - (vertex.org || vertex.cc ? 6 : 0)}
                >
                  {vertex.label}
                </text>
                {(vertex.org || vertex.cc) && (() => {
                  // Centre flag and name as one unit; the caption font is monospace, so its
                  // width is known without measuring.
                  const flag = flagSrc(vertex.cc)
                  const flagW = flag ? FLAG_W + FLAG_GAP : 0
                  const label = fitCaption(
                    vertex.org || vertex.cc,
                    graph.layout.boxW - CAPTION_INSET * 2 - flagW,
                  )
                  const textW = label.length * SUB_ADVANCE
                  const total = textW + flagW
                  const startX = vertex.x + graph.layout.boxW / 2 - total / 2
                  return (
                    <>
                      {flag && (
                        <image
                          href={flag}
                          x={startX}
                          y={vertex.y + 8 - FLAG_H / 2}
                          width={FLAG_W}
                          height={FLAG_H}
                          preserveAspectRatio="xMidYMid slice"
                        />
                      )}
                      <text
                        className="network-map__sub"
                        x={startX + flagW}
                        y={vertex.y + 8}
                        textAnchor="start"
                      >
                        {label}
                      </text>
                    </>
                  )
                })()}
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
