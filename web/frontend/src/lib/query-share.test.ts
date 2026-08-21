import { describe, expect, it } from 'vitest'
import { buildQueryPath, isCommandSlug, parseQueryLocation, slugifyNodeName } from './query-share'

describe('slugifyNodeName', () => {
  it('folds names down to ASCII path segments', () => {
    expect(slugifyNodeName('SOFIA')).toBe('sofia')
    expect(slugifyNodeName('Smoke Node')).toBe('smoke-node')
    expect(slugifyNodeName('ŞİŞLİ')).toBe('sisli')
    expect(slugifyNodeName('Frankfurt / DE-CIX')).toBe('frankfurt-de-cix')
  })

  it('returns an empty slug when nothing usable is left', () => {
    expect(slugifyNodeName('—')).toBe('')
  })
})

describe('buildQueryPath', () => {
  it('keeps the default node out of the path', () => {
    expect(buildQueryPath({ command: 'bgp_route', target: '1.1.1.0/24' })).toBe('/bgp/1.1.1.0/24')
    expect(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeSlug: null })).toBe('/ping/8.8.8.8')
  })

  it('puts a named node in front of the command', () => {
    expect(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeSlug: 'sofia' })).toBe('/sofia/ping/8.8.8.8')
    expect(buildQueryPath({ command: 'bgp_route', target: '1.1.1.0/24', nodeSlug: '4' }))
      .toBe('/4/bgp/1.1.1.0/24')
  })

  it('falls back to the home path for unknown commands or empty targets', () => {
    expect(buildQueryPath({ command: 'shutdown', target: '8.8.8.8' })).toBe('/')
    expect(buildQueryPath({ command: 'ping', target: '/' })).toBe('/')
  })
})

describe('parseQueryLocation', () => {
  it('round-trips both link shapes', () => {
    expect(parseQueryLocation(buildQueryPath({ command: 'bgp_route', target: '1.1.1.0/24' })))
      .toEqual({ command: 'bgp_route', target: '1.1.1.0/24', node: null })
    expect(parseQueryLocation(buildQueryPath({ command: 'ping', target: '8.8.8.8', nodeSlug: 'sofia' })))
      .toEqual({ command: 'ping', target: '8.8.8.8', node: 'sofia' })
  })

  it('still accepts the v2.1.76 ?node= links', () => {
    expect(parseQueryLocation('/bgp/8.8.8.8', '?node=4'))
      .toEqual({ command: 'bgp_route', target: '8.8.8.8', node: '4' })
  })

  it('prefers the path node over the query parameter', () => {
    expect(parseQueryLocation('/sofia/ping/8.8.8.8', '?node=9')?.node).toBe('sofia')
  })

  it('decodes escaped segments', () => {
    expect(parseQueryLocation('/ping/2001%3Adb8%3A%3A1')?.target).toBe('2001:db8::1')
    expect(parseQueryLocation('/smoke%20node/ping/8.8.8.8')?.node).toBe('smoke node')
  })

  it('ignores paths that are not query links', () => {
    expect(parseQueryLocation('/')).toBeNull()
    expect(parseQueryLocation('/communities')).toBeNull()
    expect(parseQueryLocation('/admin/nodes')).toBeNull()
    expect(parseQueryLocation('/ping')).toBeNull()
    expect(parseQueryLocation('/sofia/ping')).toBeNull()
    expect(parseQueryLocation('/foo/bar/8.8.8.8')).toBeNull()
  })

  it('rejects malformed encoding and oversized targets', () => {
    expect(parseQueryLocation('/ping/%E0%A4%A')).toBeNull()
    expect(parseQueryLocation(`/ping/${'a'.repeat(256)}`)).toBeNull()
  })
})

describe('isCommandSlug', () => {
  it('flags slugs that would be read back as a command', () => {
    expect(isCommandSlug('ping')).toBe(true)
    expect(isCommandSlug('bgp')).toBe(true)
    expect(isCommandSlug('sofia')).toBe(false)
  })
})
