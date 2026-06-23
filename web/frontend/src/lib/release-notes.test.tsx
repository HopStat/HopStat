import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ReleaseNotesContent } from './release-notes'

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
})
