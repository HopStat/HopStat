import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AsPathInline } from './as-path-inline'
import type { ASInfo } from '@/types/domain'

vi.mock('@/contexts/i18n-context', () => ({
  useI18n: () => ({ locale: 'en' }),
}))

const enriched8888: ASInfo[] = [
  { asn: 43260, org_name: 'Dgn Teknoloji A.s.', short_name: 'Dgn', country_code: 'TR', flag_emoji: '🇹🇷' },
  { asn: 204457, org_name: 'Atlantis Telekomunikasyon Bilisim Hizmetleri San. ve Tic. Ltd. Sti.', short_name: 'Atlantis', country_code: 'TR', flag_emoji: '🇹🇷' },
  { asn: 15169, org_name: 'Google LLC', short_name: 'Google', country_code: 'US', flag_emoji: '🇺🇸' },
]

describe('AsPathInline tooltips', () => {
  it('enables tooltips for every hop when enrichment is present', () => {
    render(<AsPathInline path={[43260, 204457, 15169]} enriched={enriched8888} />)

    for (const label of ['AS43260', 'AS204457', 'AS15169']) {
      const el = screen.getByText(label)
      expect(el.className).toContain('cursor-pointer')
      expect(el.closest('.inline-flex')).toBeTruthy()
    }
  })
})
