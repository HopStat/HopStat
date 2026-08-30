import { useEffect, useLayoutEffect, useMemo, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { Clock3, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { highlightMatch } from '@/lib/query-history-search'
import type { RankedQueryHistoryRecord } from '@/lib/query-history-search'
import { useI18n } from '@/contexts/i18n-context'

const commandLabelKey: Record<string, string> = {
  ping: 'cmd.ping',
  traceroute: 'cmd.traceroute',
  bgp_route: 'cmd.bgp_route',
}

interface Props {
  open: boolean
  query: string
  suggestions: RankedQueryHistoryRecord[]
  activeIndex: number
  keyboardNavActive: boolean
  anchorRef: RefObject<HTMLElement | null>
  onSelect: (entry: RankedQueryHistoryRecord) => void
  onDelete: (key: string) => void
  className?: string
}

function useAnchorPosition(
  anchorRef: RefObject<HTMLElement | null>,
  open: boolean,
  itemCount: number,
) {
  const [position, setPosition] = useState<{
    top: number
    left: number
    width: number
    maxHeight: number
  } | null>(null)

  useLayoutEffect(() => {
    // No reset when closed: the list renders nothing without `open`, and reopening
    // recomputes the position in this same layout effect, before the browser paints.
    if (!open) return

    function update() {
      const anchor = anchorRef.current
      if (!anchor) return

      const rect = anchor.getBoundingClientRect()
      const gap = 6
      const viewportPadding = 8
      const estimatedHeight = Math.min(itemCount * 44 + 8, 256)
      const spaceBelow = window.innerHeight - rect.bottom - gap - viewportPadding
      const spaceAbove = rect.top - gap - viewportPadding
      const openUp = spaceBelow < Math.min(estimatedHeight, 160) && spaceAbove > spaceBelow
      const maxHeight = Math.max(
        120,
        Math.min(256, openUp ? spaceAbove : spaceBelow),
      )
      const top = openUp
        ? Math.max(viewportPadding, rect.top - gap - maxHeight)
        : rect.bottom + gap

      setPosition({
        top,
        left: rect.left,
        width: rect.width,
        maxHeight,
      })
    }

    update()
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
    }
  }, [anchorRef, open, itemCount])

  return position
}

export function QueryTargetSuggestions({
  open,
  query,
  suggestions,
  activeIndex,
  keyboardNavActive,
  anchorRef,
  onSelect,
  onDelete,
  className,
}: Props) {
  const { t } = useI18n()
  const listId = useMemo(() => 'query-target-suggestions', [])
  const position = useAnchorPosition(anchorRef, open, suggestions.length)

  useEffect(() => {
    if (!open || !keyboardNavActive || activeIndex < 0) return
    const active = document.getElementById(`${listId}-item-${activeIndex}`)
    active?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex, keyboardNavActive, listId, open])

  if (!open || suggestions.length === 0 || !position) return null

  const panel = (
    <div
      id={listId}
      className={cn(
        'fixed z-[80] overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-elevated',
        className,
      )}
      style={{
        top: position.top,
        left: position.left,
        width: position.width,
      }}
    >
      <ul
        role="listbox"
        aria-label={t('query.history_suggestions')}
        className="overflow-y-auto py-1"
        style={{ maxHeight: position.maxHeight }}
      >
        {suggestions.map((entry, index) => {
          const parts = highlightMatch(entry.target, query)
          const commandKey = commandLabelKey[entry.command]
          const commandLabel = commandKey ? t(commandKey) : entry.command
          const active = keyboardNavActive && index === activeIndex

          return (
            <li key={entry.key} role="presentation" className="group flex items-stretch">
              <button
                id={`${listId}-item-${index}`}
                type="button"
                role="option"
                aria-selected={active}
                onMouseDown={e => {
                  e.preventDefault()
                  onSelect(entry)
                }}
                className={cn(
                  'flex min-w-0 flex-1 items-center gap-3 px-3 py-2 text-left transition-colors',
                  active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/70',
                )}
              >
                <Clock3 className="w-3.5 h-3.5 shrink-0 text-muted-foreground" aria-hidden />
                <span className="min-w-0 flex-1 font-data text-sm truncate">
                  {parts.before}
                  {parts.match && <strong className="font-semibold text-foreground">{parts.match}</strong>}
                  {parts.after}
                </span>
                <span className="shrink-0 text-[11px] text-muted-foreground">
                  {commandLabel}
                  {entry.nodeName ? ` · ${entry.nodeName}` : ''}
                </span>
              </button>
              <button
                type="button"
                aria-label={t('query.history_remove')}
                title={t('query.history_remove')}
                onMouseDown={e => {
                  e.preventDefault()
                  e.stopPropagation()
                  onDelete(entry.key)
                }}
                className={cn(
                  'shrink-0 px-2 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive',
                  active ? 'bg-accent text-muted-foreground hover:text-destructive' : 'opacity-70 group-hover:opacity-100',
                )}
              >
                <X className="w-3.5 h-3.5" aria-hidden />
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )

  return createPortal(panel, document.body)
}
