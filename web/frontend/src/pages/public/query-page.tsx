import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { QueryForm, type QueryFormHandle } from '@/components/query/query-form'
import { QueryHeader } from '@/components/query/query-header'
import { QueryHomeEmpty } from '@/components/query/query-home-empty'
import { CommunitiesPanel } from '@/components/communities/communities-panel'
import { ResultContainer } from '@/components/results/result-container'
import { useSettings } from '@/contexts/settings-context'
import { scrollResultsBelowSticky } from '@/lib/mobile-viewport'
import { buildQueryPath, buildShareUrl, parseQueryLocation } from '@/lib/query-share'
import type { QuerySubmitMeta } from '@/types/domain'

export function QueryPage() {
  const [queryMeta, setQueryMeta] = useState<QuerySubmitMeta | null>(null)
  // Captured once: a query link opened directly should run, later URL updates must not re-run it.
  const [initialQuery] = useState(() =>
    parseQueryLocation(window.location.pathname, window.location.search),
  )
  const formRef = useRef<QueryFormHandle>(null)
  const stickyRef = useRef<HTMLDivElement>(null)
  const resultsRef = useRef<HTMLDivElement>(null)
  const { settings } = useSettings()
  const location = useLocation()
  const navigate = useNavigate()
  const showCommunities = location.pathname === '/communities'

  const siteName = settings.site_name || 'Looking Glass'

  useEffect(() => {
    if (showCommunities) setQueryMeta(null)
  }, [showCommunities])

  // A query path with no usable target (/ping, /bgp/) is not a real link — fall back to home.
  useEffect(() => {
    if (queryMeta || showCommunities || location.pathname === '/') return
    if (parseQueryLocation(location.pathname, location.search)) return
    navigate('/', { replace: true })
  }, [queryMeta, showCommunities, location.pathname, location.search, navigate])

  function goHome() {
    navigate('/', { replace: true })
    setQueryMeta(null)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function closeCommunities() {
    navigate('/', { replace: true })
  }

  function handleTraceroute(ip: string) {
    if (showCommunities) closeCommunities()
    formRef.current?.runQuery('traceroute', ip)
  }

  function handleQuickStart(command: string, target: string, nodeId?: number | null) {
    if (showCommunities) closeCommunities()
    void formRef.current?.runQuery(command, target, nodeId ? String(nodeId) : undefined)
  }

  function handleQuerySubmit(meta: QuerySubmitMeta) {
    // Reflect the query in the address bar so the result can be linked to someone else.
    navigate(buildQueryPath(meta), { replace: true })
    setQueryMeta(meta)
  }

  useLayoutEffect(() => {
    if (!queryMeta) return
    const alignResults = () => scrollResultsBelowSticky(stickyRef.current, resultsRef.current)
    alignResults()
    requestAnimationFrame(() => requestAnimationFrame(alignResults))
  }, [queryMeta])

  const showResultsPanel = showCommunities || queryMeta

  return (
    <div className={`query-page-shell${showResultsPanel ? ' query-page-shell--results' : ''}`}>
      <div className="query-page-atmosphere" aria-hidden="true">
        <div className="query-page-glow" />
        <div className="query-page-grid" />
      </div>

      <div className="query-sticky-shell" ref={stickyRef}>
        <div className="query-sticky-inner">
          <div className="query-console">
            <div className="query-header-band">
              <QueryHeader
                siteName={siteName}
                siteDescription={settings.site_description}
                logoPath={settings.logo_path}
                onTraceroute={handleTraceroute}
                onHomeClick={goHome}
              />
            </div>
            <div className="query-form-band">
              <QueryForm
                ref={formRef}
                onQuerySubmit={handleQuerySubmit}
                initialQuery={initialQuery}
                showNodeSelect
                showFormHint={!showResultsPanel}
              />
            </div>
          </div>
        </div>
      </div>

      <div className="query-sticky-inner">
        {!showResultsPanel && (
          <QueryHomeEmpty onQuickStart={handleQuickStart} />
        )}

        {showResultsPanel && (
          <div ref={resultsRef} className="query-results-stage animate-fade-up">
            {showCommunities ? (
              <CommunitiesPanel onClose={closeCommunities} />
            ) : queryMeta ? (
              <div className="query-results-shell">
                <div className="query-results-body">
                  <ResultContainer
                    queryId={queryMeta.queryId}
                    command={queryMeta.command}
                    historyContext={{
                      target: queryMeta.target,
                      nodeId: queryMeta.nodeId,
                      nodeName: queryMeta.nodeName,
                    }}
                    shareUrl={buildShareUrl(queryMeta)}
                    onHistorySaved={() => { void formRef.current?.refreshHistory() }}
                  />
                </div>
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
