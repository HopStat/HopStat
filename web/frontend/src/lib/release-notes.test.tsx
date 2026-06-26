import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ReleaseNotesContent, ReleaseNotesPanel, formatReleaseVersion, stripReleaseTitleHeadings } from './release-notes'

describe('ReleaseNotesContent', () => {
  it('renders headings and bullet lists from markdown', () => {
    render(
      <ReleaseNotesContent markdown={'## Fixes\n- Standalone ping loopback\n- BGP peering select'} />,
    )
    expect(screen.getByRole('heading', { level: 3, name: 'Fixes' })).toBeTruthy()
    expect(screen.getByText('Standalone ping loopback')).toBeTruthy()
    expect(screen.getByText('BGP peering select')).toBeTruthy()
  })

  it('renders inline links', () => {
    render(<ReleaseNotesContent markdown="See [release notes](https://example.com/release)" />)
    const link = screen.getByRole('link', { name: 'release notes' })
    expect(link.getAttribute('href')).toBe('https://example.com/release')
  })

  it('renders horizontal rules between aggregated sections', () => {
    const { container } = render(<ReleaseNotesContent markdown={'## v2.1.64\n- one\n---\n## v2.1.65\n- two'} />)
    expect(container.querySelectorAll('hr').length).toBe(1)
  })

  it('strips duplicate HopStat version headings from markdown', () => {
    const raw = '# HopStat v2.1.66\n\nSummary line.\n\n## Fixes\n- one'
    expect(stripReleaseTitleHeadings(raw)).toBe('Summary line.\n\n## Fixes\n- one')
    const agg = '# HopStat v2.1.64\n\nA\n\n---\n\n# HopStat v2.1.65\n\nB'
    expect(stripReleaseTitleHeadings(agg)).toBe('A\n\n---\n\nB')
  })

  it('normalizes version labels with a v prefix', () => {
    expect(formatReleaseVersion('2.1.63')).toBe('v2.1.63')
    expect(formatReleaseVersion('v2.1.66')).toBe('v2.1.66')
  })

  it('renders per-version sections for aggregated release notes', () => {
    const markdown = [
      '# HopStat v2.1.64\n\nA\n\n---\n\n# HopStat v2.1.65\n\nB\n\n---\n\n# HopStat v2.1.66\n\n## Fixes\n- BGP fix',
    ].join('')
    render(
      <ReleaseNotesPanel
        markdown={markdown}
        releaseVersions={['v2.1.64', 'v2.1.65', 'v2.1.66']}
      />,
    )
    expect(screen.getByRole('heading', { level: 3, name: 'v2.1.64' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 3, name: 'v2.1.65' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 3, name: 'v2.1.66' })).toBeTruthy()
    expect(screen.getByText('BGP fix')).toBeTruthy()
  })
})
