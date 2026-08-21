import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { NetworkMap } from './network-map'
import type { NodeASPath } from '@/types/domain'

vi.mock('@/contexts/i18n-context', () => ({
  useI18n: () => ({
    locale: 'en',
    t: (key: string) =>
      ({
        'result.network_map': 'Network map',
        'result.via_default_route': 'Via default route',
      })[key] ?? key,
  }),
}))

const entries: NodeASPath[] = [
  { node_id: 1, node_name: 'BURSA', prefix: '8.8.8.0/24', as_path: [9121, 3356, 15169] },
  { node_id: 2, node_name: 'SOFIA', prefix: '8.8.8.0/24', as_path: [8866, 6939, 15169] },
]

function renderMap(props: Partial<React.ComponentProps<typeof NetworkMap>> = {}) {
  return render(<NetworkMap entries={entries} enriched={[]} {...props} />)
}

describe('NetworkMap', () => {
  it('renders nothing without at least two routed nodes', () => {
    const { container } = renderMap({ entries: [entries[0]] })
    expect(container.querySelector('svg')).toBeNull()
  })

  it('draws one box per node plus the merged AS hops', () => {
    renderMap()
    expect(screen.getByText('BURSA')).toBeTruthy()
    expect(screen.getByText('SOFIA')).toBeTruthy()
    // The shared origin appears exactly once.
    expect(screen.getAllByText('AS15169')).toHaveLength(1)
    expect(screen.getByText('AS3356')).toBeTruthy()
  })

  it('highlights only the hovered node path', () => {
    const { container } = renderMap()
    const bursa = screen.getByText('BURSA').closest('g') as SVGGElement

    fireEvent.mouseEnter(bursa)
    const active = container.querySelectorAll('.network-map__edge.is-active')
    expect(active.length).toBeGreaterThan(0)
    // AS8866 belongs to the other node, so its edge must stay neutral.
    active.forEach(edge => expect(edge.getAttribute('class')).not.toContain('nm-c1'))

    fireEvent.mouseLeave(bursa)
    expect(container.querySelectorAll('.network-map__edge.is-active')).toHaveLength(0)
  })

  it('shows the AS org and country on hover', () => {
    renderMap({
      enriched: [{ asn: 15169, org_name: 'Google LLC', short_name: '', country_code: 'US', flag_emoji: '🇺🇸' }],
    })
    fireEvent.mouseEnter(screen.getByText('AS15169').closest('g') as SVGGElement)
    expect(screen.getByRole('tooltip').textContent).toContain('Google')
  })

  it('leaves a node with no route off the diagram', () => {
    renderMap({
      entries: [...entries, { node_id: 3, node_name: 'DARK', no_route: true }],
    })
    expect(screen.queryByText('DARK')).toBeNull()
  })

  it('dashes backup paths and keeps the selected one solid', () => {
    const { container } = renderMap({
      entries: [
        { node_id: 1, node_name: 'BURSA', as_path: [9121, 3356, 15169], best: true },
        { node_id: 1, node_name: 'BURSA', as_path: [9121, 6939, 15169] },
        { node_id: 2, node_name: 'SOFIA', as_path: [8866, 15169], best: true },
      ],
    })
    const alt = container.querySelectorAll('.network-map__edge.is-alt')
    expect(alt.length).toBeGreaterThan(0)
    // The hop shared by the selected and the backup path is not itself a backup.
    expect(container.querySelectorAll('.network-map__edge').length).toBeGreaterThan(alt.length)
  })

  it('describes every node path for screen readers', () => {
    const { container } = renderMap()
    const desc = container.querySelector('desc')?.textContent ?? ''
    expect(desc).toContain('BURSA: AS9121 → AS3356 → AS15169')
    expect(desc).toContain('SOFIA: AS8866 → AS6939 → AS15169')
  })
})
