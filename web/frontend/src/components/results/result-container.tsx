import { useMemo, useEffect, useRef } from 'react'
import { useQueryStream } from '@/hooks/use-sse'
import { useI18n } from '@/contexts/i18n-context'
import { saveSuccessfulQuery } from '@/lib/query-history-db'
import { OutputTerminal } from './output-terminal'
import { ResultBGP } from './result-bgp'
import { ResultPing } from './result-ping'
import { QueryErrorAlert } from './query-error-alert'
import { ResultShareBar } from './result-share-bar'
import { NetworkMap } from './network-map'
import { buildBgpMapAsPath, extractASPathFromBGP, extractASPathFromLines, parsePingFromLines, mergePingResult, hasPingData, isPingOutputComplete, bestBGPRoute, shouldShowBgpAsPathMap } from '@/lib/result-parse'
import { translateQueryError, translateBgpRawMessage } from '@/lib/query-errors'
import { buildResultText } from '@/lib/result-export'
import type { BGPResult, NodeASPath, PingResult } from '@/types/domain'

interface Props {
  queryId: string | null
  command?: string
  historyContext?: {
    target: string
    nodeId: number
    nodeName: string
  }
  shareUrl?: string
  onHistorySaved?: () => void
  /** Re-runs the query on another node, from the network map. */
  onNodeSelect?: (nodeId: number) => void
}

/** The label the query form shows for a command, so the result names it the same way the
 *  user picked it rather than echoing the API's identifier. */
function commandLabel(t: (key: string) => string, command: string): string {
  const key = `cmd.${command}`
  const label = t(key)
  return label === key ? command : label
}

export function ResultContainer({ queryId, command, historyContext, shareUrl, onHistorySaved, onNodeSelect }: Props) {
  const { t } = useI18n()
  const { result, lines, error, outputComplete } = useQueryStream(queryId)
  const savedHistoryRef = useRef<string | null>(null)

  useEffect(() => {
    if (!queryId || !command || !historyContext) return
    if (result?.status !== 'done') return
    if (error || result.error_msg || result.error_code) return
    if (savedHistoryRef.current === queryId) return

    savedHistoryRef.current = queryId
    saveSuccessfulQuery({
      target: historyContext.target,
      command,
      nodeId: historyContext.nodeId,
      nodeName: historyContext.nodeName,
    })
      .then(() => onHistorySaved?.())
      .catch(() => {})
  }, [queryId, command, historyContext, result, error, onHistorySaved])

  const pingParsed = command === 'ping' ? (result?.parsed as PingResult | null | undefined) : null
  const rawLines = result?.raw?.split('\n').filter(line => line.length > 0) ?? []
  const displayLines = lines.length > 0 ? lines : rawLines

  const pingFromLines = useMemo(
    () => (command === 'ping' ? parsePingFromLines(displayLines) : null),
    [command, displayLines],
  )
  const pingDisplay = useMemo(() => {
    return command === 'ping' ? mergePingResult(pingFromLines ?? {}, pingParsed) : {}
  }, [pingFromLines, pingParsed, command])

  const bgpParsed = result?.parsed as BGPResult | null | undefined

  const asPathFromParsed = extractASPathFromBGP(bgpParsed)
  const asPathFromResult = result?.as_path ?? []
  const asPathFromLines = extractASPathFromLines(lines)
  const asPath = asPathFromResult.length > 0
    ? asPathFromResult
    : asPathFromParsed.length > 0
      ? asPathFromParsed
      : asPathFromLines

  const targetAS = bgpParsed?.target_as ?? null
  const enrichedBase = result?.as_path_enriched ?? []
  const hasRouteAsPath = asPath.length > 0
  const enrichedForDisplay = hasRouteAsPath || !targetAS?.asn || enrichedBase.some(entry => entry.asn === targetAS.asn)
    ? enrichedBase
    : [...enrichedBase, targetAS]
  const mapAsPath = buildBgpMapAsPath(asPath, targetAS?.asn)

  if (!queryId) return null

  if (error && lines.length === 0) {
    return (
      <QueryErrorAlert
        message={translateQueryError(t, undefined, error)}
        className="mb-2"
      />
    )
  }

  const isRunning = !result || result.status === 'pending' || result.status === 'running'
  const isError = result?.status === 'error'

  const hasBgpRoutes = Boolean(bgpParsed?.routes?.length)
  const showNetworkMap = shouldShowBgpAsPathMap(command, mapAsPath, {
    hasBgpRoutes,
    hasServerAsPath: asPathFromResult.length > 0,
  })

  const selectedRoute = bestBGPRoute(bgpParsed?.routes)

  const showBgpRoutes = Boolean(command === 'bgp_route' && hasBgpRoutes)
  const showBgpRawMessage = Boolean(
    command === 'bgp_route' &&
    !showBgpRoutes &&
    (result?.raw?.trim() || displayLines.length > 0)
  )
  const bgpRawMessage = translateBgpRawMessage(
    t,
    result?.raw?.trim() || displayLines.join('\n').trim(),
  )
  const showPingStats = command === 'ping'
  const pingOutputComplete = command === 'ping' && isPingOutputComplete(displayLines)
  const pingRunning = isRunning && !pingOutputComplete
  const pingLoading = pingRunning && !hasPingData(pingDisplay)
  const tracerouteOutputComplete = command === 'traceroute' && outputComplete
  const commandOutputComplete = pingOutputComplete || tracerouteOutputComplete
  const terminalRunning = isRunning && !commandOutputComplete
  const errorMessage = isError && result
    ? translateQueryError(t, result.error_code, result.error_msg)
    : null

  // Without a per-node map — a node answering from its own router, or a target only GeoIP
  // could place — fall back to the single path the result already carries.
  const mapPrefix = result?.as_path_prefix?.trim() || selectedRoute?.prefix?.trim() || ''
  const mapEntries: NodeASPath[] | undefined = result?.as_path_nodes?.length
    ? result.as_path_nodes
    : historyContext && mapAsPath.length > 0
      ? [{
          node_id: historyContext.nodeId,
          node_name: historyContext.nodeName,
          prefix: mapPrefix,
          as_path: mapAsPath,
          best: true,
          via_default_route: selectedRoute?.via_default_route,
        }]
      : undefined

  const copyResultText = () => buildResultText({
    command: command ?? '',
    target: historyContext?.target ?? '',
    nodeName: historyContext?.nodeName,
    shareUrl,
    lines: displayLines,
    bgp: command === 'bgp_route' ? bgpParsed : null,
    nodePaths: command === 'bgp_route' ? result?.as_path_nodes : null,
  })

  const shareContext = command && historyContext && shareUrl ? { command, shareUrl, ...historyContext } : null
  const shareSummary = shareContext
    ? [commandLabel(t, shareContext.command), shareContext.target, shareContext.nodeName]
        .filter(Boolean)
        .join(' · ')
    : ''

  return (
    <div className="space-y-3 pb-1">
      {shareContext && (
        <ResultShareBar
          summary={shareSummary}
          shareUrl={shareContext.shareUrl}
        />
      )}

      {showNetworkMap && (
        <NetworkMap
          entries={mapEntries}
          enriched={enrichedForDisplay}
          queriedNodeId={historyContext?.nodeId}
          onNodeSelect={onNodeSelect}
        />
      )}

      {showPingStats && (
        <ResultPing result={pingDisplay} loading={pingLoading} running={pingRunning} />
      )}

      {showBgpRoutes && bgpParsed && (
        <ResultBGP result={bgpParsed} enriched={enrichedForDisplay} copyText={copyResultText} />
      )}

      {showBgpRawMessage && bgpRawMessage && (
        <div className="result-surface result-surface--message px-4 py-3 text-sm text-muted-foreground font-data animate-fade-up">
          {bgpRawMessage}
        </div>
      )}

      {((displayLines.length > 0) || (terminalRunning && !showBgpRawMessage)) && command !== 'bgp_route' && (
        <OutputTerminal
          lines={displayLines}
          isRunning={terminalRunning}
          animateHops={command === 'traceroute'}
          copyText={copyResultText}
        />
      )}

      {errorMessage && (
        <QueryErrorAlert message={errorMessage} className="mt-1 mb-2" />
      )}
    </div>
  )
}
