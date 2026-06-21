import type { ASInfo, GeoIPLookupReport, GeoIPResolveSource } from '@/types/domain'

export function looksLikeIP(value: string | undefined | null): boolean {
  const v = (value ?? '').trim()
  if (!v || v === '—') return false
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(v)) return true
  if (v.includes(':') && /^[0-9a-fA-F:.]+$/.test(v)) return true
  return false
}

export function formatAS(info: ASInfo | undefined): string {
  if (!info?.asn) return '—'
  const org = info.org_name || info.short_name
  return org ? `AS${info.asn} — ${org}` : `AS${info.asn}`
}

export function sourceBadgeVariant(source: GeoIPResolveSource): 'default' | 'secondary' | 'outline' {
  if (source === 'blocks' || source === 'mmdb') return 'default'
  if (source === 'dns') return 'secondary'
  return 'outline'
}

export function formatSourceStatus(
  candidate: GeoIPLookupReport['blocks'],
  t: (key: string) => string,
): string {
  if (!candidate.available) return t('admin.geoip_lookup_not_loaded')
  if (candidate.error) return candidate.error
  if (candidate.matched) return formatAS(candidate.info)
  return t('admin.geoip_lookup_no_match')
}

export function geoipSourceLabel(source: GeoIPResolveSource, t: (key: string) => string): string {
  switch (source) {
    case 'blocks': return t('admin.geoip_source_blocks')
    case 'mmdb': return t('admin.geoip_source_mmdb')
    case 'dns': return t('admin.geoip_source_dns')
    default: return t('admin.geoip_source_none')
  }
}
