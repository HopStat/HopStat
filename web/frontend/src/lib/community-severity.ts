import type { CommunityRule } from '@/types/domain'

export const communitySeverityVariant = {
  alert: 'destructive' as const,
  warning: 'warning' as const,
  info: 'info' as const,
  success: 'success' as const,
}

export type CommunitySeverity = keyof typeof communitySeverityVariant

export function normalizeCommunitySeverity(severity: string): CommunitySeverity | string {
  if (severity === 'reject') return 'alert'
  return severity
}

export function communitySeverityBadgeVariant(severity: string) {
  const normalized = normalizeCommunitySeverity(severity) as CommunitySeverity
  return communitySeverityVariant[normalized] ?? 'secondary'
}

const rowAccent: Record<CommunitySeverity, string> = {
  alert: 'border-l-destructive/70 bg-destructive/[0.04]',
  warning: 'border-l-amber-500/80 bg-amber-500/[0.05]',
  info: 'border-l-sky-500/80 bg-sky-500/[0.05]',
  success: 'border-l-emerald-500/80 bg-emerald-500/[0.05]',
}

export function communityRowAccent(severity: string): string {
  const normalized = normalizeCommunitySeverity(severity) as CommunitySeverity
  return rowAccent[normalized] ?? 'border-l-border bg-muted/20'
}

export function communitySeverityLabel(severity: string, t: (key: string) => string): string {
  const normalized = normalizeCommunitySeverity(severity)
  const key = `admin.severity_${normalized}`
  const label = t(key)
  return label !== key ? label : String(normalized)
}

export function normCommunity(c: string): string {
  return c.trim().toLowerCase()
}

export function communityMessage(rule: Pick<CommunityRule, 'message_i18n'>): string {
  return rule.message_i18n?.trim() ?? ''
}
