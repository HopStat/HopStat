import type { ASInfo } from '@/types/domain'

const MAX_NAME_LEN = 15

const LEGAL_SUFFIXES = new Set([
  'inc', 'llc', 'ltd', 'corp', 'corporation',
  'co', 'as', 'sa', 'gmbh', 'ag',
  'plc', 'limited', 'bv', 'nv', 'spa', 'pty',
])

export function buildAsInfoMap(enriched: ASInfo[]): Map<number, ASInfo> {
  return new Map(enriched.map(e => [Number(e.asn), e]))
}

function normalizeOrgForDisplay(org: string): string {
  org = org.trim()
  if (!org) return org
  const dash = org.indexOf(' - ')
  if (dash > 0) {
    const before = org.slice(0, dash).trim()
    const after = org.slice(dash + 3).trim()
    if (/^AS\d+/i.test(before) && after) org = after
    else return before
  }
  const comma = org.indexOf(', ')
  if (comma > 0) return org.slice(0, comma).trim()
  return org
}

function trimLegalSuffixWords(words: string[]): string[] {
  const out = [...words]
  while (out.length > 0) {
    const last = out[out.length - 1].replace(/\./g, '').toLowerCase()
    if (LEGAL_SUFFIXES.has(last)) {
      out.pop()
      continue
    }
    break
  }
  return out
}

/** Compact org label for the AS path map: whole word parts only, up to 15 chars (no ellipsis). */
export function compactAsMapName(raw: string): string {
  let name = normalizeOrgForDisplay(raw)
  if (!name) return ''

  const words = trimLegalSuffixWords(name.split(/\s+/).filter(Boolean))
  if (words.length === 0) return ''

  let result = ''
  for (const word of words) {
    const candidate = result ? `${result} ${word}` : word
    if (candidate.length <= MAX_NAME_LEN) {
      result = candidate
      continue
    }
    break
  }

  if (result) return result.toUpperCase()

  // Single word longer than the limit — show the start without an ellipsis suffix.
  return words[0].slice(0, MAX_NAME_LEN).toUpperCase()
}

export function displayAsName(info: ASInfo | undefined): string {
  const org = info?.org_name?.trim()
  const short = info?.short_name?.trim()
  if (org) return compactAsMapName(org)
  if (short) return compactAsMapName(short)
  return ''
}

function stripOrgCountrySuffix(org: string, cc: string): string {
  if (!org || !cc) return org
  const code = cc.trim().toUpperCase()
  let trimmed = org.trim()
  for (const sep of [', ', ' - ']) {
    const suffix = `${sep}${code}`
    if (trimmed.toUpperCase().endsWith(suffix)) {
      trimmed = trimmed.slice(0, -suffix.length).trim()
      break
    }
  }
  return trimmed
}

function tooltipAsName(info: ASInfo | undefined): string {
  const cc = asCountryLabel(info)
  let org = stripOrgCountrySuffix(info?.org_name?.trim() ?? '', cc)
  org = normalizeOrgForDisplay(org)
  if (org && org.toUpperCase() !== cc) return org
  const short = info?.short_name?.trim()
  if (short) return short
  return ''
}

function asCountryDisplayName(code: string, locale?: string): string {
  const cc = code.trim().toUpperCase()
  if (!cc) return ''
  const locales = locale ? [locale, 'en'] : ['en']
  for (const loc of locales) {
    try {
      const name = new Intl.DisplayNames([loc], { type: 'region' }).of(cc)
      if (name) return name
    } catch {
      continue
    }
  }
  return cc
}

function asCountryLabel(info: ASInfo | undefined): string {
  return info?.country_code?.trim().toUpperCase() ?? ''
}

export function asCountryCode(info: ASInfo | undefined): string {
  return asCountryLabel(info)
}

export function asTooltipLines(
  info: ASInfo | undefined,
  _asn: number | string,
  _count = 1,
  locale?: string,
): string[] {
  const name = tooltipAsName(info)
  const country = asCountryDisplayName(asCountryLabel(info), locale)
  if (name && country) return [`${name} - ${country}`]
  if (name) return [name]
  if (country) return [country]
  return []
}

export interface CompressedASHop {
  asn: number
  count: number
}

/** Collapse consecutive identical AS numbers in a path. */
export function compressConsecutiveASPath(path: number[]): CompressedASHop[] {
  if (path.length === 0) return []
  const out: CompressedASHop[] = []
  for (const asn of path) {
    const last = out[out.length - 1]
    if (last && last.asn === asn) {
      last.count++
    } else {
      out.push({ asn, count: 1 })
    }
  }
  return out
}

export function formatASLabel(asn: number, count: number): string {
  if (count > 1) return `${count}×AS${asn}`
  return `AS${asn}`
}
