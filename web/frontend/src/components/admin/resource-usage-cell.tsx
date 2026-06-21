import { formatResourcePercent, resourceBarClass } from '@/lib/resource-level'
import type { ResourceLevel, SystemResource } from '@/types/domain'

interface ResourceUsageCellProps {
  resource: SystemResource
  available?: boolean
  level: ResourceLevel
}

export function ResourceUsageCell({ resource, available = true, level }: ResourceUsageCellProps) {
  if (!available) {
    return <span className="text-xs text-muted-foreground">—</span>
  }

  const width = Math.max(0, Math.min(100, resource.percent))

  return (
    <div className="admin-resource-usage">
      <div className="admin-resource-bar" aria-hidden="true">
        <div className={`admin-resource-bar__fill ${resourceBarClass(level)}`} style={{ width: `${width}%` }} />
      </div>
      <span className="admin-resource-usage__percent font-data tabular-nums">{formatResourcePercent(resource, true)}</span>
    </div>
  )
}
