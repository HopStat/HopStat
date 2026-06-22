import { describe, expect, it } from 'vitest'
import type { ASInfo } from '@/types/domain'

function mergeAsPathEnriched(
  data: { as_path?: number[]; as_path_enriched?: ASInfo[] },
  prev: { as_path_enriched?: ASInfo[] } | null,
): ASInfo[] {
  if (data.as_path_enriched?.length) {
    return data.as_path_enriched
  }
  if (data.as_path?.length) {
    return []
  }
  return prev?.as_path_enriched ?? []
}

describe('mergeAsPathEnriched', () => {
  it('clears stale enrichment when a new AS path arrives without enriched data', () => {
    const stale = [{ asn: 9121, org_name: 'Turk Telekom', short_name: 'Turk', country_code: 'TR', flag_emoji: '🇹🇷' }]
    expect(
      mergeAsPathEnriched(
        { as_path: [43260, 204457, 15169] },
        { as_path_enriched: stale },
      ),
    ).toEqual([])
  })

  it('keeps enrichment when the AS path has not changed yet', () => {
    const enriched = [{ asn: 43260, org_name: 'DGN', short_name: 'DGN', country_code: 'TR', flag_emoji: '🇹🇷' }]
    expect(mergeAsPathEnriched({}, { as_path_enriched: enriched })).toEqual(enriched)
  })
})
