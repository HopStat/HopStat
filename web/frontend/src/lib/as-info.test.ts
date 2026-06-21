import { describe, expect, it } from 'vitest'
import { asTooltipLines, compactAsMapName } from './as-info'
import type { ASInfo } from '@/types/domain'

describe('compactAsMapName', () => {
  it('keeps short multi-word names whole', () => {
    expect(compactAsMapName('DGN Teknoloji')).toBe('DGN TEKNOLOJI')
  })

  it('uses only whole words that fit within 15 chars', () => {
    expect(compactAsMapName('Euronet Telekomunikasyon')).toBe('EURONET')
  })

  it('adds second word when both fit', () => {
    expect(compactAsMapName('Türk Telekom')).toBe('TÜRK TELEKOM')
  })

  it('does not append ellipsis when truncating by words', () => {
    expect(compactAsMapName('International Business Machines')).toBe('INTERNATIONAL')
    expect(compactAsMapName('International Business Machines')).not.toContain('...')
  })

  it('strips legal suffixes before fitting words', () => {
    expect(compactAsMapName('Acme Fiber LLC')).toBe('ACME FIBER')
  })
})

describe('asTooltipLines', () => {
  const info: ASInfo = {
    asn: 43260,
    org_name: 'AS43260 - DGN TEKNOLOJI A.S.',
    short_name: 'DGN',
    country_code: 'TR',
    flag_emoji: '🇹🇷',
  }

  it('shows full AS name and country on a single line', () => {
    expect(asTooltipLines(info, 43260, 1, 'en')).toEqual(['DGN TEKNOLOJI A.S. - Türkiye'])
  })

  it('shows name only when country is unavailable', () => {
    expect(asTooltipLines({ ...info, country_code: '' }, 43260)).toEqual(['DGN TEKNOLOJI A.S.'])
  })

  it('shows country only when name is unavailable', () => {
    expect(asTooltipLines({ ...info, org_name: '', short_name: '' }, 43260, 1, 'en')).toEqual(['Türkiye'])
  })

  it('returns no tooltip text when enrichment is missing', () => {
    expect(asTooltipLines(undefined, 43260)).toEqual([])
  })
})
