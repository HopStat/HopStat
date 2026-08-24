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
  it('renders nothing without a routed node', () => {
    const { container } = renderMap({ entries: [] })
    expect(container.querySelector('svg')).toBeNull()
  })

  it('draws a single node on its own', () => {
    renderMap({ entries: [entries[0]] })
    expect(screen.getByText('BURSA')).toBeTruthy()
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

  it('lights the route running through a hovered hop', () => {
    const { container } = renderMap()
    const hop = screen.getByText('AS6939').closest('g') as SVGGElement

    // AS6939 sits only on SOFIA's path, so hovering it traces SOFIA end to end.
    fireEvent.mouseEnter(hop)
    const active = container.querySelectorAll('.network-map__edge.is-active')
    expect(active.length).toBeGreaterThan(0)
    active.forEach(edge => expect(edge.getAttribute('class')).toContain('nm-c1'))

    fireEvent.mouseLeave(hop)
    expect(container.querySelectorAll('.network-map__edge.is-active')).toHaveLength(0)
  })

  it('keeps the focused route when hovering a hop it shares', () => {
    const { container } = renderMap({ queriedNodeId: 2 })

    // AS15169 is shared by both nodes; with SOFIA in focus its route must not jump to BURSA.
    fireEvent.mouseEnter(screen.getByText('AS15169').closest('g') as SVGGElement)
    const active = container.querySelectorAll('.network-map__edge.is-active')
    expect(active.length).toBeGreaterThan(0)
    active.forEach(edge => expect(edge.getAttribute('class')).toContain('nm-c0'))
    active.forEach(edge => expect(edge.getAttribute('class')).not.toContain('nm-c1'))
  })

  it('prints the flag and AS name on the label instead of in a tooltip', () => {
    const { container } = renderMap({
      enriched: [{ asn: 15169, org_name: 'Google LLC', short_name: '', country_code: 'US', flag_emoji: '🇺🇸' }],
    })
    expect(screen.getByText('GOOGLE')).toBeTruthy()
    // The same flag source the rest of the app uses — emoji do not render everywhere.
    const flag = container.querySelector('image[href*="flagcdn.com"]')
    expect(flag?.getAttribute('href')).toContain('/us.png')
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('drops the flag when the lookup gave no country', () => {
    const { container } = renderMap({
      enriched: [{ asn: 15169, org_name: 'Google LLC', short_name: '', country_code: '', flag_emoji: '' }],
    })
    expect(screen.getByText('GOOGLE')).toBeTruthy()
    expect(container.querySelector('image')).toBeNull()
  })

  it('shows no popup for a node box or an AS box', () => {
    const { container } = renderMap()
    fireEvent.mouseEnter(screen.getByText('BURSA').closest('g') as SVGGElement)
    expect(screen.queryByRole('tooltip')).toBeNull()

    // Only node boxes are reachable by keyboard; an AS box activates nothing.
    expect(container.querySelectorAll('g[tabindex]')).toHaveLength(2)
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

describe('NetworkMap caption fitting', () => {
  const twoNodes = [
    { node_id: 1, node_name: 'A', as_path: [43260, 15169], best: true },
    { node_id: 2, node_name: 'B', as_path: [8866, 15169], best: true },
  ]
  const longOrg = [
    { asn: 43260, org_name: 'DGN Teknoloji Hizmetleri', short_name: '', country_code: 'TR', flag_emoji: '' },
  ]

  it('trims a long operator name so the caption stays inside its box', () => {
    const { container } = render(<NetworkMap entries={twoNodes} enriched={longOrg} />)

    const flag = container.querySelector('image')!
    const text = container.querySelector('.network-map__sub')!
    const captionLeft = Number(flag.getAttribute('x'))
    const captionRight =
      Number(text.getAttribute('x')) + (text.textContent?.length ?? 0) * 9 * 0.6

    const boxes = [...container.querySelectorAll('.network-map__box')].map(b => ({
      x: Number(b.getAttribute('x')),
      w: Number(b.getAttribute('width')),
    }))
    const owner = boxes.find(b => captionLeft >= b.x && captionLeft <= b.x + b.w)

    expect(owner).toBeTruthy()
    expect(captionRight).toBeLessThanOrEqual(owner!.x + owner!.w)
  })
})

describe('NetworkMap node selection', () => {
  const entries = [
    { node_id: 1, node_name: 'BURSA', as_path: [9121, 15169], best: true },
    { node_id: 2, node_name: 'SOFIA', as_path: [8866, 15169], best: true },
  ]

  it('re-runs the query from a clicked node', () => {
    const onNodeSelect = vi.fn()
    render(<NetworkMap entries={entries} enriched={[]} onNodeSelect={onNodeSelect} />)

    fireEvent.click(screen.getByText('SOFIA').closest('g') as SVGGElement)
    expect(onNodeSelect).toHaveBeenCalledWith(2)
  })

  it('answers the keyboard too', () => {
    const onNodeSelect = vi.fn()
    render(<NetworkMap entries={entries} enriched={[]} onNodeSelect={onNodeSelect} />)

    const box = screen.getByText('BURSA').closest('g') as SVGGElement
    fireEvent.keyDown(box, { key: 'Enter' })
    expect(onNodeSelect).toHaveBeenCalledWith(1)

    fireEvent.keyDown(box, { key: 'a' })
    expect(onNodeSelect).toHaveBeenCalledTimes(1)
  })

  it('leaves AS hops inert', () => {
    const onNodeSelect = vi.fn()
    const { container } = render(
      <NetworkMap entries={entries} enriched={[]} onNodeSelect={onNodeSelect} />,
    )

    fireEvent.click(screen.getByText('AS15169').closest('g') as SVGGElement)
    expect(onNodeSelect).not.toHaveBeenCalled()
    // Only the node boxes advertise themselves as actionable.
    expect(container.querySelectorAll('g[role="button"]')).toHaveLength(2)
  })
})
