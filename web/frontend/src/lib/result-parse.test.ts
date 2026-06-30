import { describe, expect, it } from 'vitest'
import {
  buildBgpMapAsPath,
  isPingOutputComplete,
  mergePingResult,
  parsePingFromLines,
  shouldShowBgpAsPathMap,
} from './result-parse'

describe('buildBgpMapAsPath', () => {
  it('returns empty when there is no route or GeoIP target ASN', () => {
    expect(buildBgpMapAsPath([], null)).toEqual([])
  })

  it('uses GeoIP target ASN when there is no route AS path', () => {
    expect(buildBgpMapAsPath([], 9121)).toEqual([9121])
  })

  it('keeps route AS path without appending GeoIP target ASN', () => {
    expect(buildBgpMapAsPath([9121, 174], 15169)).toEqual([9121, 174])
  })

  it('keeps path unchanged when target ASN is already present', () => {
    expect(buildBgpMapAsPath([9121, 174], 174)).toEqual([9121, 174])
  })

  it('drops invalid zero ASNs and falls back to GeoIP target', () => {
    expect(buildBgpMapAsPath([0, 0], 9121)).toEqual([9121])
  })
})

describe('shouldShowBgpAsPathMap', () => {
  it('hides the map when there is no AS path', () => {
    expect(shouldShowBgpAsPathMap('bgp_route', [], { hasBgpRoutes: false })).toBe(false)
  })

  it('shows the map when routes include a path', () => {
    expect(shouldShowBgpAsPathMap('bgp_route', [9121, 174], { hasBgpRoutes: true })).toBe(true)
  })

  it('hides the map for target-only enrichment without BGP routes', () => {
    expect(shouldShowBgpAsPathMap('bgp_route', [9121], { hasBgpRoutes: false })).toBe(false)
  })
})

describe('parsePingFromLines', () => {
  it('parses transmitted and received counts', () => {
    const result = parsePingFromLines([
      'PING 8.8.8.8 (8.8.8.8): 56 data bytes',
      '64 bytes from 8.8.8.8: icmp_seq=0 ttl=118 time=10.2 ms',
      '--- 8.8.8.8 ping statistics ---',
      '5 packets transmitted, 5 received, 0% packet loss',
      'rtt min/avg/max/mdev = 9.1/10.2/11.0/0.5 ms',
    ])
    expect(result.packets_sent).toBe(5)
    expect(result.packets_recv).toBe(5)
    expect(result.packet_loss).toBe(0)
    expect(result.avg_rtt).toBeCloseTo(10.2)
  })

  it('updates sent and received live without premature loss', () => {
    const result = parsePingFromLines([
      'PING 8.8.8.8 (8.8.8.8): 56 data bytes',
      '64 bytes from 8.8.8.8: icmp_seq=0 ttl=118 time=10.2 ms',
      '64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=9.8 ms',
    ])
    expect(result.packets_sent).toBe(2)
    expect(result.packets_recv).toBe(2)
    expect(result.packet_loss).toBeUndefined()
  })

  it('counts timeout lines toward sent probes', () => {
    const result = parsePingFromLines([
      '64 bytes from 8.8.8.8: icmp_seq=0 ttl=118 time=10.2 ms',
      'Request timeout for icmp_seq 1',
      '64 bytes from 8.8.8.8: icmp_seq=2 ttl=118 time=11.0 ms',
    ])
    expect(result.packets_sent).toBe(3)
    expect(result.packets_recv).toBe(2)
    expect(result.packet_loss).toBeUndefined()
  })
})

describe('isPingOutputComplete', () => {
  it('detects summary line', () => {
    expect(isPingOutputComplete(['5 packets transmitted, 5 received'])).toBe(true)
    expect(isPingOutputComplete(['64 bytes from 8.8.8.8'])).toBe(false)
  })
})

describe('mergePingResult', () => {
  it('prefers server values when present', () => {
    const merged = mergePingResult(
      { packets_sent: 5, packets_recv: 2 },
      { packets_sent: 5, packets_recv: 5, avg_rtt: 12 },
    )
    expect(merged.packets_recv).toBe(5)
    expect(merged.avg_rtt).toBe(12)
  })
})
