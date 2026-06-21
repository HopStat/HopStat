import { useCallback, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Badge } from '@/components/ui/badge'
import type { CommunityRule } from '@/types/domain'
import { communitySeverityBadgeVariant, normCommunity } from '@/lib/community-severity'

interface TagProps {
  rule: CommunityRule
}

export function CommunityTag({ rule }: TagProps) {
  const label = rule.community
  const message = rule.message_i18n?.trim()
  const anchorRef = useRef<HTMLDivElement>(null)
  const [visible, setVisible] = useState(false)
  const [coords, setCoords] = useState({ x: 0, y: 0 })

  const show = useCallback(() => {
    if (!message) return
    const el = anchorRef.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    setCoords({
      x: rect.left + rect.width / 2,
      y: rect.top - 8,
    })
    setVisible(true)
  }, [message])

  const hide = useCallback(() => setVisible(false), [])

  return (
    <>
      <div
        ref={anchorRef}
        className="inline-flex"
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
      >
        <Badge
          variant={communitySeverityBadgeVariant(rule.severity)}
          className="text-[11px] font-mono normal-case tracking-normal py-0.5 px-2 cursor-pointer"
          title={message || label}
        >
          {label}
        </Badge>
      </div>
      {message && visible && createPortal(
        <div
          role="tooltip"
          className="as-path-tooltip pointer-events-none fixed z-[200] w-max max-w-[20rem] -translate-x-1/2 -translate-y-full px-2.5 py-1.5 text-[11px] font-normal normal-case leading-snug rounded-md"
          style={{ left: coords.x, top: coords.y }}
        >
          {message}
        </div>,
        document.body,
      )}
    </>
  )
}

interface RouteCommunitiesProps {
  communities: string[]
  rules: CommunityRule[]
}

export function RouteCommunities({ communities, rules }: RouteCommunitiesProps) {
  const ruleByCommunity = new Map<string, CommunityRule>()
  for (const r of rules) {
    ruleByCommunity.set(normCommunity(r.community), r)
  }

  const seen = new Set<string>()
  const items: { key: string; community: string; rule?: CommunityRule }[] = []

  for (const c of communities) {
    const key = normCommunity(c)
    if (!key || seen.has(key)) continue
    seen.add(key)
    items.push({ key, community: c, rule: ruleByCommunity.get(key) })
  }

  for (const r of rules) {
    const key = normCommunity(r.community)
    if (!key || seen.has(key)) continue
    seen.add(key)
    items.push({ key, community: r.community, rule: r })
  }

  if (items.length === 0) return null

  return (
    <div className="route-communities flex flex-wrap gap-x-2 gap-y-1.5">
      {items.map(({ key, community, rule }) =>
        rule ? (
          <CommunityTag key={key} rule={rule} />
        ) : (
          <Badge
            key={key}
            variant="outline"
            className="text-[10px] font-mono normal-case tracking-normal py-0.5 px-2 cursor-pointer"
            title={community}
          >
            {community}
          </Badge>
        ),
      )}
    </div>
  )
}

interface Props {
  rules: CommunityRule[]
}

/** @deprecated Use RouteCommunities when route community strings are available */
export function CommunityBadges({ rules }: Props) {
  if (!rules.length) return null

  return (
    <div className="flex flex-wrap gap-1.5">
      {rules.map(rule => (
        <CommunityTag key={rule.id} rule={rule} />
      ))}
    </div>
  )
}
