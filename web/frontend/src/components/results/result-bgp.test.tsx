import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ResultBGP } from './result-bgp'
import type { ASInfo, BGPResult } from '@/types/domain'
import '@/globals.css'

vi.mock('@/contexts/i18n-context', () => ({
  useI18n: () => ({
    t: (key: string) =>
      ({
        'result.best': 'Best',
        'result.age': 'Age',
        'result.prefix': 'Prefix',
        'result.as_path': 'AS Path',
        'result.local_pref': 'Local Pref',
        'result.med': 'MED',
        'result.communities': 'Communities',
        'result.best_route': 'Active route',
        'result.via_default_route': 'Via default route',
      })[key] ?? key,
  }),
}))

const enriched: ASInfo[] = [
  { asn: 43260, org_name: 'DGN TEKNOLOJI', short_name: 'DGN', country_code: 'TR', flag_emoji: '🇹🇷' },
  { asn: 9121, org_name: 'TURK TELEKOM', short_name: 'Türk', country_code: 'TR', flag_emoji: '🇹🇷' },
]

const result185: BGPResult = {
  raw: '',
  routes: [
    {
      prefix: '185.203.171.1/32',
      next_hop: '',
      as_path: [43260, 9121],
      local_pref: 0,
      med: 0,
      origin: '',
      communities: [],
      status: '',
      protocol: '',
      age: '—',
      via_default_route: true,
      best: true,
      node_name: 'BURSA',
    },
  ],
}

describe('ResultBGP communities column alignment', () => {
  it('centers route table headers horizontally', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('min-width'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    })

    render(<ResultBGP result={result185} enriched={enriched} />)

    for (const label of ['Age', 'Prefix', 'AS Path', 'Communities']) {
      const header = screen.getByText(label).closest('th')
      expect(header).toBeTruthy()
      expect(header?.className).toContain('text-center')
    }
  })

  it('keeps Communities header vertically centered like other headers', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('min-width'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    })

    render(<ResultBGP result={result185} enriched={enriched} />)

    const communitiesHeader = screen.getByText('Communities').closest('th')
    const prefixHeader = screen.getByText('Prefix').closest('th')
    expect(communitiesHeader).toBeTruthy()
    expect(prefixHeader).toBeTruthy()

    expect(getComputedStyle(communitiesHeader!).verticalAlign).toBe('middle')
    expect(getComputedStyle(prefixHeader!).verticalAlign).toBe('middle')
  })

  it('top-aligns Communities body cells in the rendered desktop table markup', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('min-width'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    })

    render(<ResultBGP result={result185} enriched={enriched} />)

    const communitiesCell = document.querySelector('.result-bgp-table tbody td.result-bgp-table__communities')
    expect(communitiesCell).toBeTruthy()
    expect(communitiesCell?.className).toContain('result-bgp-table__communities')
  })

  it('fits long route age values in the Age column without clipping', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('min-width'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    })

    const longAgeResult: BGPResult = {
      ...result185,
      routes: [{ ...result185.routes[0], age: '13h52m54s' }],
    }

    render(<ResultBGP result={longAgeResult} enriched={enriched} />)

    expect(screen.getByText('13h52m54s')).toBeTruthy()
    const ageCell = document.querySelector('.result-bgp-table tbody td.result-bgp-table__age')
    expect(ageCell).toBeTruthy()
    expect(ageCell?.className).toContain('tabular-nums')
    expect(ageCell?.className).toContain('whitespace-nowrap')
  })

  it('renders full IPv4 /32 prefixes in the Prefix column', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('min-width'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }),
    })

    const hostResult: BGPResult = {
      ...result185,
      routes: [{ ...result185.routes[0], prefix: '213.146.165.165/32', via_default_route: true }],
    }

    render(<ResultBGP result={hostResult} enriched={enriched} />)

    const prefixCell = document.querySelector('.result-bgp-table tbody td.result-bgp-table__prefix')
    expect(prefixCell?.textContent).toBe('213.146.165.165/32')
    expect(prefixCell?.className).toContain('whitespace-nowrap')
  })
})
