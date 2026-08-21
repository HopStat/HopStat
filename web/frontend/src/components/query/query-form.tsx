import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { CornerDownLeft, Info, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api, ApiError } from '@/lib/api-client'
import { translateQueryError } from '@/lib/query-errors'
import { useI18n } from '@/contexts/i18n-context'
import { useSettings } from '@/contexts/settings-context'
import type { Node, QuerySubmitMeta } from '@/types/domain'
import { isCommandSlug, slugifyNodeName, type SharedQuery } from '@/lib/query-share'
import { QueryTargetSuggestions } from '@/components/query/query-target-suggestions'
import { listQueryHistory, deleteQueryHistory, type QueryHistoryRecord } from '@/lib/query-history-db'
import { rankQueryHistory, type RankedQueryHistoryRecord } from '@/lib/query-history-search'
import { blurActiveFieldPreservingScroll } from '@/lib/mobile-viewport'
import { QueryErrorAlert } from '@/components/results/query-error-alert'
const commands = [
  { value: 'ping', labelKey: 'cmd.ping' },
  { value: 'traceroute', labelKey: 'cmd.traceroute' },
  { value: 'bgp_route', labelKey: 'cmd.bgp_route' },
]

export interface QueryFormHandle {
  runQuery: (command: string, target: string, nodeIdOverride?: string) => Promise<void>
  refreshHistory: () => Promise<void>
}

interface Props {
  onQuerySubmit: (meta: QuerySubmitMeta) => void
  showNodeSelect?: boolean
  showFormHint?: boolean
  /** Query decoded from a shared link — run once as soon as the node list is available. */
  initialQuery?: SharedQuery | null
}

function isTypingBlocked(el: Element | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false
  if (el.isContentEditable) return true
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (el.closest('[role="dialog"]') || el.closest('[data-radix-popper-content-wrapper]')) return true
  return false
}

function pickDefaultNodeId(nodes: Node[]): string {
  if (!nodes.length) return ''
  const marked = nodes.find(n => n.is_default === true)
  if (marked) return String(marked.id)
  return String(nodes[0].id)
}

/** Resolves the node token from a shared link — its slug, or its id as a fallback. */
function resolveLinkedNodeId(nodes: Node[], token: string | null): string {
  if (!token) return pickDefaultNodeId(nodes)
  if (nodes.some(n => String(n.id) === token)) return token

  const slug = slugifyNodeName(token)
  const match = nodes.find(n => slugifyNodeName(n.name) === slug)
  return match ? String(match.id) : pickDefaultNodeId(nodes)
}

/**
 * Slug used in the shareable link. Null for the default node (keeps the link short) and
 * for anything whose name would be ambiguous — those fall back to the numeric id.
 */
function shareSlugForNode(nodes: Node[], nodeId: string): string | null {
  if (nodeId === pickDefaultNodeId(nodes)) return null
  const node = nodes.find(n => String(n.id) === nodeId)
  if (!node) return null

  const slug = slugifyNodeName(node.name)
  if (!slug || isCommandSlug(slug)) return nodeId
  const sameSlug = nodes.filter(n => slugifyNodeName(n.name) === slug)
  return sameSlug.length === 1 ? slug : nodeId
}

export const QueryForm = forwardRef<QueryFormHandle, Props>(function QueryForm(
  { onQuerySubmit, showNodeSelect = false, showFormHint = true, initialQuery = null },
  ref,
) {
  const { t } = useI18n()
  const { settings } = useSettings()
  const [nodes, setNodes] = useState<Node[]>([])
  const [nodesLoaded, setNodesLoaded] = useState(false)
  const [nodeId, setNodeId] = useState('')
  const [command, setCommand] = useState('bgp_route')
  const [target, setTarget] = useState('')
  const selectedNodeId = useMemo(() => {
    if (nodeId && nodes.some(n => String(n.id) === nodeId)) return nodeId
    return pickDefaultNodeId(nodes)
  }, [nodeId, nodes])
  const availableCmds = useMemo(() => commands.filter(c => {
    if (!selectedNodeId) return true
    const node = nodes.find(n => n.id === parseInt(selectedNodeId))
    if (!node || !node.enabled_cmds?.length) return true
    return node.enabled_cmds.includes(c.value)
  }), [selectedNodeId, nodes])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [history, setHistory] = useState<QueryHistoryRecord[]>([])
  const [suggestionsOpen, setSuggestionsOpen] = useState(false)
  const [activeSuggestion, setActiveSuggestion] = useState(-1)
  const [suggestionKeyboardNav, setSuggestionKeyboardNav] = useState(false)
  const nodeSelectRef = useRef<HTMLButtonElement>(null)
  const commandSelectRef = useRef<HTMLButtonElement>(null)
  const targetInputRef = useRef<HTMLInputElement>(null)
  const targetAnchorRef = useRef<HTMLDivElement>(null)
  const showNodeRef = useRef(false)
  const initialQueryRanRef = useRef(false)

  const pingCount = parseInt(settings.ping_count as string) || 5
  const maxHops = parseInt(settings.max_hops as string) || 30

  useEffect(() => {
    api.get<Node[]>('/nodes').then(loaded => {
      setNodes(loaded)
      setNodeId(pickDefaultNodeId(loaded))
      setNodesLoaded(true)
    }).catch(() => setNodesLoaded(true))
  }, [])

  useEffect(() => {
    listQueryHistory()
      .then(setHistory)
      .catch(() => {})
  }, [])

  const suggestions = useMemo(
    () => rankQueryHistory(history, target, command),
    [history, target, command],
  )

  useEffect(() => {
    setActiveSuggestion(-1)
    setSuggestionKeyboardNav(false)
  }, [target, command])

  useEffect(() => {
    if (!suggestionsOpen) return
    function onPointerDown(e: MouseEvent) {
      const target = e.target
      if (!(target instanceof Node)) return
      const root = targetAnchorRef.current
      if (root?.contains(target)) return
      if (target instanceof Element && target.closest('#query-target-suggestions')) return
      setSuggestionsOpen(false)
    }
    document.addEventListener('mousedown', onPointerDown)
    return () => document.removeEventListener('mousedown', onPointerDown)
  }, [suggestionsOpen])

  const refreshHistory = useCallback(async () => {
    try {
      const entries = await listQueryHistory()
      setHistory(entries)
      return entries
    } catch {
      return []
    }
  }, [])

  async function applySuggestion(entry: RankedQueryHistoryRecord) {
    const nextNodeId =
      showNodeRef.current && entry.nodeId > 0 ? String(entry.nodeId) : selectedNodeId
    setTarget(entry.target)
    setCommand(entry.command)
    if (showNodeRef.current && entry.nodeId > 0) {
      setNodeId(String(entry.nodeId))
    }
    setSuggestionsOpen(false)
    blurActiveFieldPreservingScroll()
    await submitQuery(entry.command, entry.target, nextNodeId)
  }

  function handleTargetChange(value: string) {
    setTarget(value)
    setSuggestionKeyboardNav(false)
    setActiveSuggestion(-1)
    if (document.activeElement === targetInputRef.current) {
      setSuggestionsOpen(rankQueryHistory(history, value, command).length > 0)
    }
  }

  async function handleTargetFocus(e: React.FocusEvent<HTMLInputElement>) {
    targetAnchorRef.current = e.currentTarget.closest('[data-query-target-root]') as HTMLDivElement | null
    const entries = await refreshHistory()
    setSuggestionsOpen(rankQueryHistory(entries, target, command).length > 0)
  }

  function handleTargetKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown' && suggestionsOpen && suggestions.length > 0) {
      e.preventDefault()
      setSuggestionKeyboardNav(true)
      setActiveSuggestion(index => {
        if (index < suggestions.length - 1) return index + 1
        return 0
      })
      return
    }
    if (e.key === 'ArrowUp' && suggestionsOpen && suggestions.length > 0) {
      e.preventDefault()
      setSuggestionKeyboardNav(true)
      setActiveSuggestion(index => {
        if (index <= 0) return suggestions.length - 1
        return index - 1
      })
      return
    }
    if (e.key === 'Enter') {
      if (
        suggestionKeyboardNav &&
        activeSuggestion >= 0 &&
        suggestions[activeSuggestion]
      ) {
        e.preventDefault()
        void applySuggestion(suggestions[activeSuggestion])
      }
      return
    }
    if (e.key === 'Escape' && suggestionsOpen) {
      e.preventDefault()
      setSuggestionsOpen(false)
      setSuggestionKeyboardNav(false)
      setActiveSuggestion(-1)
    }
  }

  async function handleDeleteHistory(key: string) {
    try {
      await deleteQueryHistory(key)
      const entries = await refreshHistory()
      const nextSuggestions = rankQueryHistory(entries, target, command)
      setSuggestionsOpen(nextSuggestions.length > 0)
      setSuggestionKeyboardNav(false)
      setActiveSuggestion(-1)
    } catch {
      /* ignore */
    }
  }

  useEffect(() => {
    if (command && !availableCmds.find(c => c.value === command)) {
      setCommand(availableCmds[0]?.value ?? '')
    }
  }, [selectedNodeId, command, availableCmds])

  function queryTabFields(): HTMLElement[] {
    const fields: HTMLElement[] = []
    if (showNodeRef.current && nodeSelectRef.current) fields.push(nodeSelectRef.current)
    if (commandSelectRef.current) fields.push(commandSelectRef.current)
    if (targetInputRef.current) fields.push(targetInputRef.current)
    return fields
  }

  function focusQueryTabField(backwards: boolean) {
    const fields = queryTabFields()
    if (!fields.length) return

    const active = document.activeElement as HTMLElement | null
    const idx = active ? fields.indexOf(active) : -1

    if (idx === -1) {
      ;(backwards ? fields.at(-1) : fields[0])?.focus()
      return
    }

    if (!backwards && active === targetInputRef.current) {
      fields[0].focus()
      return
    }
    if (backwards && active === fields[0]) {
      targetInputRef.current?.focus()
      return
    }

    const next = idx + (backwards ? -1 : 1)
    fields[next]?.focus()
  }

  function handleFormKeyDown(e: React.KeyboardEvent<HTMLFormElement>) {
    if (e.key !== 'Tab' || e.defaultPrevented) return
    const active = document.activeElement
    const fields = queryTabFields()
    if (!fields.length) return
    if (active && !fields.includes(active as HTMLElement)) return

    e.preventDefault()
    focusQueryTabField(e.shiftKey)
  }

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return

      const active = document.activeElement
      if (active?.closest('[role="dialog"]')) return

      if (e.key === 'Escape') {
        if (suggestionsOpen) {
          setSuggestionsOpen(false)
          setSuggestionKeyboardNav(false)
          setActiveSuggestion(-1)
          e.preventDefault()
          return
        }
        setTarget('')
        setError('')
        targetInputRef.current?.focus()
        setSuggestionKeyboardNav(false)
        setActiveSuggestion(-1)
        setSuggestionsOpen(rankQueryHistory(history, '', command).length > 0)
        e.preventDefault()
        return
      }

      if (e.key === 'Tab' && active === document.body) {
        e.preventDefault()
        focusQueryTabField(e.shiftKey)
        return
      }

      if (isTypingBlocked(active) && active !== targetInputRef.current) return

      if (e.key.length === 1 && !loading) {
        if (active !== targetInputRef.current) {
          e.preventDefault()
          targetInputRef.current?.focus()
          setTarget(prev => {
            const next = prev + e.key
            setSuggestionKeyboardNav(false)
            setActiveSuggestion(-1)
            setSuggestionsOpen(rankQueryHistory(history, next, command).length > 0)
            return next
          })
        }
      }
    }

    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [loading, suggestionsOpen, history, command])

  const submitQuery = useCallback(async (cmd: string, tgt: string, nodeIdOverride?: string) => {
    const trimmed = tgt.trim()
    const effectiveNodeId = nodeIdOverride ?? selectedNodeId
    if (!effectiveNodeId || !cmd || !trimmed || loading) return
    setLoading(true)
    setError('')
    setSuggestionsOpen(false)
    try {
      const options: Record<string, number> = {}
      if (cmd === 'ping') options.ping_count = pingCount
      if (cmd === 'traceroute') options.max_hops = maxHops

      const res = await api.post<{ query_id: string }>('/query', {
        node_id: parseInt(effectiveNodeId),
        command: cmd,
        target: trimmed,
        options,
      })
      const node = nodes.find(n => n.id === parseInt(effectiveNodeId))
      onQuerySubmit({
        queryId: res.query_id,
        command: cmd,
        bgpActive: node?.bgp_active ?? false,
        target: trimmed,
        nodeId: parseInt(effectiveNodeId),
        nodeName: node?.name ?? '',
        nodeSlug: shareSlugForNode(nodes, effectiveNodeId),
      })
    } catch (err: unknown) {
      const code = err instanceof ApiError ? err.code : undefined
      const raw = err instanceof Error ? err.message : ''
      setError(translateQueryError(t, code, raw || undefined))
    } finally {
      setLoading(false)
    }
  }, [selectedNodeId, loading, pingCount, maxHops, nodes, onQuerySubmit, t])

  useEffect(() => {
    if (!initialQuery || initialQueryRanRef.current || !nodesLoaded) return
    initialQueryRanRef.current = true

    const linkedNodeId = resolveLinkedNodeId(nodes, initialQuery.node)
    if (!linkedNodeId) return

    setCommand(initialQuery.command)
    setTarget(initialQuery.target)
    setNodeId(linkedNodeId)
    void submitQuery(initialQuery.command, initialQuery.target, linkedNodeId)
  }, [initialQuery, nodesLoaded, nodes, submitQuery])

  useImperativeHandle(ref, () => ({
    runQuery: async (cmd: string, tgt: string, nodeIdOverride?: string) => {
      setCommand(cmd)
      setTarget(tgt)
      if (nodeIdOverride) {
        setNodeId(nodeIdOverride)
      }
      blurActiveFieldPreservingScroll()
      await submitQuery(cmd, tgt, nodeIdOverride)
    },
    refreshHistory: () => refreshHistory().then(() => undefined),
  }), [submitQuery, refreshHistory])

  // Radix Select echoes onValueChange('') while its item list is still empty; letting that
  // through would wipe a node picked from a shared link right after we set it.
  function handleNodeChange(value: string) {
    if (!value) return
    setNodeId(value)
  }

  function blurFocusedField() {
    blurActiveFieldPreservingScroll()
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    blurFocusedField()
    await submitQuery(command, target)
  }

  const targetPlaceholder = command === 'bgp_route'
    ? '8.8.8.8 or 1.1.1.0/24'
    : t('query.target_placeholder')

  const showNode = showNodeSelect && nodes.length > 0
  showNodeRef.current = showNode

  const selectFieldClass = 'h-10 py-0 flex items-center rounded-md'
  const inputFieldClass = 'h-10 py-0 px-3 leading-[2.5rem] rounded-md'
  const submitDisabled = !selectedNodeId || !command || !target.trim() || loading

  const commandSelect = (
    <Select value={command} onValueChange={setCommand}>
      <SelectTrigger
        ref={commandSelectRef}
        className={`query-form-field__command w-[4.75rem] sm:w-[5.25rem] shrink-0 rounded-none sm:rounded-none border-0 sm:border-0 ${selectFieldClass} text-base sm:text-xs font-semibold justify-center`}
      >
        <SelectValue placeholder={t('query.select_command')} />
      </SelectTrigger>
      <SelectContent>
        {availableCmds.map(c => (
          <SelectItem key={c.value} value={c.value}>{t(c.labelKey)}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  )

  const targetField = (
    <div className="relative min-w-0 flex-1" data-query-target-root>
      <Input
        ref={targetInputRef}
        value={target}
        onChange={e => handleTargetChange(e.target.value)}
        onFocus={handleTargetFocus}
        onKeyDown={handleTargetKeyDown}
        placeholder={targetPlaceholder}
        role="combobox"
        aria-expanded={suggestionsOpen && suggestions.length > 0}
        aria-controls="query-target-suggestions"
        aria-activedescendant={
          suggestionKeyboardNav && activeSuggestion >= 0
            ? `query-target-suggestions-item-${activeSuggestion}`
            : undefined
        }
        aria-autocomplete="list"
        autoComplete="off"
        inputMode="url"
        autoCapitalize="off"
        autoCorrect="off"
        spellCheck={false}
        enterKeyHint="go"
        className={`query-form-field__target w-full rounded-none sm:rounded-none border-0 sm:border-0 bg-transparent ${inputFieldClass} text-[16px] sm:text-sm sm:font-data focus-visible:ring-0 focus-visible:ring-offset-0`}
      />
    </div>
  )

  const historySuggestions = (
    <QueryTargetSuggestions
      anchorRef={targetAnchorRef}
      open={suggestionsOpen && suggestions.length > 0}
      query={target}
      suggestions={suggestions}
      activeIndex={activeSuggestion}
      keyboardNavActive={suggestionKeyboardNav}
      onSelect={entry => { void applySuggestion(entry) }}
      onDelete={key => { void handleDeleteHistory(key) }}
    />
  )

  const submitButton = (
    <Button
      type="submit"
      disabled={submitDisabled}
      aria-label={t('query.submit')}
      className="h-10 w-10 shrink-0 rounded-md p-0 sm:w-[4.75rem] sm:px-2 flex items-center justify-center text-white text-xs font-semibold bg-brand hover:bg-brand/90 disabled:opacity-45"
    >
      {loading ? (
        <Loader2 className="w-4 h-4 animate-spin" />
      ) : (
        <>
          <CornerDownLeft className="w-4 h-4 sm:hidden" aria-hidden />
          <span className="hidden sm:inline">{t('query.submit')}</span>
        </>
      )}
    </Button>
  )

  return (
    <form onSubmit={handleSubmit} onKeyDown={handleFormKeyDown} className="query-form-controls">
      <div className="query-form-panel">
        <div className="query-form-mobile-stack flex flex-col sm:hidden min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            {showNode && (
              <div className="min-w-0 flex-1">
                <Select value={selectedNodeId} onValueChange={handleNodeChange} disabled={!nodesLoaded}>
                  <SelectTrigger
                    ref={nodeSelectRef}
                    className={`query-form-select query-form-select__node w-full bg-transparent ${selectFieldClass} text-sm [&>span]:truncate`}
                  >
                    <SelectValue placeholder={t('query.select_node')} />
                  </SelectTrigger>
                  <SelectContent>
                    {nodes.map(n => (
                      <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="shrink-0">
              <Select value={command} onValueChange={setCommand}>
                <SelectTrigger
                  ref={commandSelectRef}
                  className={`query-form-select query-form-command-mobile w-auto max-w-none ${selectFieldClass} text-sm font-semibold [&>span]:line-clamp-none [&>span]:whitespace-nowrap`}
                >
                  <SelectValue placeholder={t('query.select_command')} />
                </SelectTrigger>
                <SelectContent>
                  {availableCmds.map(c => (
                    <SelectItem key={c.value} value={c.value}>{t(c.labelKey)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="query-form-mobile-target-row flex items-center gap-2 min-w-0">
            <div className="query-form-field flex flex-1 min-w-0 relative">
              {targetField}
            </div>
            {submitButton}
          </div>
        </div>

        <div className="hidden sm:flex sm:flex-row sm:items-center sm:gap-2 min-w-0">
          {showNode && (
            <Select value={selectedNodeId} onValueChange={handleNodeChange} disabled={!nodesLoaded}>
              <SelectTrigger
                ref={nodeSelectRef}
                className={`query-form-select query-form-select__node w-36 shrink-0 bg-transparent ${selectFieldClass} text-sm`}
              >
                <SelectValue placeholder={t('query.select_node')} />
              </SelectTrigger>
              <SelectContent>
                {nodes.map(n => (
                  <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <div className="query-form-field flex flex-1 min-w-0 flex-row items-center relative">
            {commandSelect}
            {targetField}
          </div>

          {submitButton}
        </div>
      </div>

      {showFormHint && (
        <p className="query-form-hint">
          <Info className="query-form-hint__icon" aria-hidden="true" />
          <span>{t('query.form_hint')}</span>
        </p>
      )}

      {error && (
        <div className="query-form-error">
          <QueryErrorAlert message={error} />
        </div>
      )}

      {historySuggestions}
    </form>
  )
})
