import { describe, expect, it } from 'vitest'
import { buildResultText } from './result-export'
import type { BGPResult } from '@/types/domain'

describe('buildResultText', () => {
  it('renders terminal output under a command header', () => {
    expect(buildResultText({
      command: 'ping',
      target: '8.8.8.8',
      nodeName: 'Frankfurt',
      shareUrl: 'https://lg.example/ping/8.8.8.8',
      lines: ['64 bytes from 8.8.8.8: time=12 ms'],
    })).toBe([
      '$ ping 8.8.8.8  @ Frankfurt',
      '',
      '64 bytes from 8.8.8.8: time=12 ms',
      '',
      'https://lg.example/ping/8.8.8.8',
    ].join('\n'))
  })

  it('prefers parsed BGP routes over raw lines', () => {
    const bgp: BGPResult = {
      raw: 'ignored',
      routes: [{
        prefix: '8.8.8.0/24',
        next_hop: '10.0.0.1',
        as_path: [64500, 15169],
        local_pref: 100,
        med: 0,
        origin: 'IGP',
        communities: ['65000:100'],
        status: '',
        protocol: '',
        age: '',
        best: true,
      }],
    }
    const text = buildResultText({ command: 'bgp_route', target: '8.8.8.8', lines: ['raw line'], bgp })
    expect(text).toContain('as_path=64500 15169')
    expect(text).toContain('communities=65000:100')
    expect(text).toContain('best')
    expect(text).not.toContain('raw line')
  })

  it('marks the selected route and its backups in the node list', () => {
    const text = buildResultText({
      command: 'bgp_route',
      target: '8.8.8.8',
      lines: [],
      nodePaths: [
        { node_id: 1, node_name: 'BURSA', prefix: '8.8.8.0/24', as_path: [9121, 15169], best: true },
        { node_id: 1, node_name: 'BURSA', prefix: '8.8.8.0/24', as_path: [9121, 6939, 15169] },
        { node_id: 2, node_name: 'DARK', no_route: true },
      ],
    })
    expect(text).toContain('*BURSA  8.8.8.0/24  9121 15169')
    expect(text).toContain('~BURSA  8.8.8.0/24  9121 6939 15169')
    expect(text).toContain('DARK  -  no route')
  })

  it('falls back to a placeholder when there is nothing to copy', () => {
    expect(buildResultText({ command: 'ping', target: '8.8.8.8', lines: [] })).toContain('(no output)')
  })
})
