import type { ResourceLevel, SystemResource, SystemStatus } from '@/types/domain'

export function resourceBadgeVariant(level: ResourceLevel): 'success' | 'warning' | 'destructive' {
  switch (level) {
    case 'critical':
      return 'destructive'
    case 'warning':
      return 'warning'
    default:
      return 'success'
  }
}

export function resourceBarClass(level: ResourceLevel): string {
  switch (level) {
    case 'critical':
      return 'admin-resource-bar__fill--critical'
    case 'warning':
      return 'admin-resource-bar__fill--warning'
    default:
      return 'admin-resource-bar__fill--ok'
  }
}

export function formatResourcePercent(resource: SystemResource, available = true): string {
  if (!available) return '—'
  return `${resource.percent.toFixed(1)}%`
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—'
  const gb = bytes / (1024 ** 3)
  if (gb >= 1) return `${gb.toFixed(1)} GB`
  const mb = bytes / (1024 ** 2)
  if (mb >= 1) return `${Math.round(mb)} MB`
  return `${Math.round(bytes / 1024)} KB`
}

export function formatMemoryDetail(status: SystemStatus): string {
  const used = status.memory_used_bytes
  const total = status.memory_total_bytes
  if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return '—'

  const gb = (bytes: number) => bytes / (1024 ** 3)
  if (total >= 1024 ** 3) {
    return `${gb(used).toFixed(1)}/${gb(total).toFixed(1)} GB`
  }
  const mb = (bytes: number) => bytes / (1024 ** 2)
  if (total >= 1024 ** 2) {
    return `${Math.round(mb(used))}/${Math.round(mb(total))} MB`
  }
  return `${formatBytes(used)}/${formatBytes(total)}`
}

export function formatCpuDetail(status: SystemStatus, t: (key: string) => string): string {
  if (!status.cpu_available) return '—'

  const parts: string[] = []
  if (status.cpu_cores > 0) {
    parts.push(`${status.cpu_cores} ${t('admin.resource_cores')}`)
  }
  if (Number.isFinite(status.cpu_load_1) && status.cpu_load_1 >= 0) {
    parts.push(`${t('admin.resource_load')} ${status.cpu_load_1.toFixed(2)}`)
  }
  return parts.length > 0 ? parts.join(' · ') : '—'
}

export function resourceLevelLabel(level: ResourceLevel, t: (key: string) => string): string {
  switch (level) {
    case 'critical':
      return t('admin.resource_level_critical')
    case 'warning':
      return t('admin.resource_level_warning')
    default:
      return t('admin.resource_level_ok')
  }
}
