function formatLegacyExtra(key: string, value: unknown): string | null {
  if (value == null) return null
  const text = String(value).trim()
  if (!text) return null

  switch (key) {
    case 'ping_count':
      return `count ${text}`
    case 'max_hops':
      return `max hops ${text}`
    case 'protocol':
      return text
    default:
      return `${key} ${text}`
  }
}

/** Extract the queried target from audit params JSON. */
export function extractAuditTarget(raw: string | undefined | null): string {
  const trimmed = (raw ?? '').trim()
  if (!trimmed) return '—'
  if (!trimmed.startsWith('{')) return trimmed

  try {
    const obj = JSON.parse(trimmed) as Record<string, unknown>
    const target = String(obj.target ?? obj.Target ?? '').trim()
    return target || '—'
  } catch {
    const match = trimmed.match(/"target"\s*:\s*"([^"]+)"/i)
    return match?.[1] ?? trimmed
  }
}

export function commandBadgeLabel(command: string, t: (key: string) => string): string {
  const key = `cmd.${command}`
  const label = t(key)
  return label !== key ? label : command
}

/** Normalize audit params for admin display (plain target, no JSON braces). */
export function formatAuditParams(raw: string | undefined | null): string {
  const trimmed = (raw ?? '').trim()
  if (!trimmed) return '—'
  if (!trimmed.startsWith('{')) return trimmed

  try {
    const obj = JSON.parse(trimmed) as Record<string, unknown>
    const target = String(obj.target ?? obj.Target ?? '').trim()
    if (!target) return trimmed

    const extras: string[] = []
    for (const [key, value] of Object.entries(obj)) {
      if (key === 'target' || key === 'Target') continue
      const extra = formatLegacyExtra(key, value)
      if (extra) extras.push(extra)
    }

    return extras.length > 0 ? `${target} · ${extras.join(' · ')}` : target
  } catch {
    const match = trimmed.match(/"target"\s*:\s*"([^"]+)"/i)
    if (match?.[1]) return match[1]
    return trimmed
  }
}
