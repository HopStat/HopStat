import { useMemo, useEffect, useRef } from 'react'
import { useQueryStream } from '@/hooks/use-sse'
import { useI18n } from '@/contexts/i18n-context'
import { saveSuccessfulQuery } from '@/lib/query-history-db'
import { OutputTerminal } from './output-terminal'
import { AsPathMap } from './as-path-map'
import { ResultBGP } from './result-bgp'
import { ResultPing } from './result-ping'
import { QueryErrorAlert } from './query-error-alert'
import { extractASPathFromBGP, extractASPathFromLines, parsePingFromLines, mergePingResult, hasPingData, isPingOutputComplete, bestBGPRoute } from '@/lib/result-parse'
import { translateQueryError, translateBgpRawMessage } from '@/lib/query-errors'
import type { BGPResult, PingResult } from '@/types/domain'

interface Props {
  queryId: string | null
  command?: string
  historyContext?: {
    target: string
    nodeId: number
    nodeName: string
  }
  onHistorySaved?: () => void
}

export function ResultContainer({ queryId, command, historyContext, onHistorySaved }: Props) {
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
  const enrichedForDisplay = !targetAS?.asn || enrichedBase.some(entry => entry.asn === targetAS.asn)
    ? enrichedBase
    : [...enrichedBase, targetAS]
  const mapAsPath = !targetAS?.asn || asPath.includes(targetAS.asn)
    ? asPath
    : [...asPath, targetAS.asn]

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

  const showAsPathMap = command === 'bgp_route' && (mapAsPath.length > 0)

  const selectedRoute = bestBGPRoute(bgpParsed?.routes)

  const routesForMap: BGPResult['routes'] = [{
    prefix: result?.as_path_prefix?.trim() || selectedRoute?.prefix?.trim() || bgpParsed?.routes?.[0]?.prefix?.trim() || '',
    next_hop: selectedRoute?.next_hop ?? bgpParsed?.routes?.[0]?.next_hop ?? '',
    as_path: mapAsPath,
    local_pref: selectedRoute?.local_pref ?? bgpParsed?.routes?.[0]?.local_pref ?? 0,
    med: selectedRoute?.med ?? bgpParsed?.routes?.[0]?.med ?? 0,
    origin: selectedRoute?.origin ?? bgpParsed?.routes?.[0]?.origin ?? '',
    communities: selectedRoute?.communities ?? bgpParsed?.routes?.[0]?.communities ?? [],
    status: selectedRoute?.status ?? bgpParsed?.routes?.[0]?.status ?? '',
    protocol: selectedRoute?.protocol ?? bgpParsed?.routes?.[0]?.protocol ?? '',
    age: selectedRoute?.age ?? bgpParsed?.routes?.[0]?.age ?? '',
  }]

  const showBgpRoutes = Boolean(command === 'bgp_route' && bgpParsed?.routes?.length)
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

  return (
    <div className="space-y-3 pb-1">
      {showAsPathMap && (
        <AsPathMap routes={routesForMap} enriched={enrichedForDisplay} />
      )}

      {showPingStats && (
        <ResultPing result={pingDisplay} loading={pingLoading} running={pingRunning} />
      )}

      {showBgpRoutes && bgpParsed && (
        <ResultBGP result={bgpParsed} enriched={enrichedForDisplay} />
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
        />
      )}

      {errorMessage && (
        <QueryErrorAlert message={errorMessage} className="mt-1 mb-2" />
      )}
    </div>
  )
}
