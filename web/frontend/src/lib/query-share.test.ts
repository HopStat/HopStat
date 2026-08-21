import { describe, expect, it } from 'vitest'
import { buildQueryPath, parseQueryLocation } from './query-share'

describe('buildQueryPath', () => {
  it('maps bgp_route onto the short slug and keeps prefix slashes readable', () => {
    expect(buildQueryPath({ command: 'bgp_route', target: '1.1.1.0/24', nodeId: 1, isDefaultNode: true }))
      .toBe('/bgp/1.1.1.0/24')
  })

  it('appends the node only when it is not the default one', () => {
    expect(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeId: 3 })).toBe('/ping/8.8.8.8?node=3')
    expect(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeId: 3, isDefaultNode: true }))
      .toBe('/ping/8.8.8.8')
    expect(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeId: null })).toBe('/ping/8.8.8.8')
  })

  it('falls back to the home path for unknown commands or empty targets', () => {
    expect(buildQueryPath({ command: 'shutdown', target: '8.8.8.8' })).toBe('/')
    expect(buildQueryPath({ command: 'ping', target: '  ' })).toBe('/ping/%20%20')
    expect(buildQueryPath({ command: 'ping', target: '/' })).toBe('/')
  })
})

describe('parseQueryLocation', () => {
  it('round-trips a built path', () => {
    expect(parseQueryLocation(buildQueryPath({ command: 'bgp_route', target: '1.1.1.0/24' }), '?node=3'))
      .toEqual({ command: 'bgp_route', target: '1.1.1.0/24', nodeId: 3 })
  })

  it('decodes escaped targets', () => {
    expect(parseQueryLocation('/traceroute/google.com')?.target).toBe('google.com')
    expect(parseQueryLocation('/ping/2001%3Adb8%3A%3A1')?.target).toBe('2001:db8::1')
  })

  it('returns a null node when the id is not a positive integer', () => {
    expect(parseQueryLocation('/ping/8.8.8.8', '?node=abc')?.nodeId).toBeNull()
    expect(parseQueryLocation('/ping/8.8.8.8', '?node=0')?.nodeId).toBeNull()
    expect(parseQueryLocation('/ping/8.8.8.8')?.nodeId).toBeNull()
  })

  it('ignores paths that are not query links', () => {
    expect(parseQueryLocation('/')).toBeNull()
    expect(parseQueryLocation('/communities')).toBeNull()
    expect(parseQueryLocation('/admin/nodes')).toBeNull()
    expect(parseQueryLocation('/ping')).toBeNull()
    expect(parseQueryLocation('/ping/')).toBeNull()
  })

  it('rejects malformed encoding and oversized targets', () => {
    expect(parseQueryLocation('/ping/%E0%A4%A')).toBeNull()
    expect(parseQueryLocation(`/ping/${'a'.repeat(256)}`)).toBeNull()
  })
})
