import type { Page } from '@playwright/test'

/** Brand colours the palette must survive. #e0edd4 is the one that shipped unreadable
 *  buttons; the rest bracket the lightness and saturation range an operator can pick. */
export const BRANDS = ['#e0edd4', '#1e293b', '#ffd700', '#808080'] as const
export const THEMES = ['light', 'dark'] as const

export type Theme = (typeof THEMES)[number]

const NODES = [
  {
    id: 1,
    name: 'Istanbul',
    location: 'Istanbul, TR',
    type: 'standalone',
    active: true,
    is_default: true,
    enabled_cmds: ['ping', 'traceroute', 'bgp_route'],
    asn: 64500,
  },
  {
    id: 2,
    name: 'Sofia',
    location: 'Sofia, BG',
    type: 'lg_node',
    active: true,
    is_default: false,
    enabled_cmds: ['ping', 'traceroute'],
    asn: 64501,
  },
]

const AUDIT = [
  {
    id: 2,
    created_at: '2026-01-02T10:00:00Z',
    source_ip: '192.0.2.10',
    user_id: null,
    node_id: 1,
    node_name: 'Istanbul',
    command: 'traceroute',
    params: '{"target":"192.0.2.1"}',
    duration_ms: 1840,
    success: false,
    error_msg: 'agent unreachable',
  },
  {
    id: 1,
    created_at: '2026-01-02T09:59:00Z',
    source_ip: '192.0.2.11',
    user_id: 1,
    node_id: 2,
    node_name: 'Sofia',
    command: 'ping',
    params: '{"target":"192.0.2.2"}',
    duration_ms: 220,
    success: true,
    error_msg: '',
  },
]

/** Endpoints whose response is not just {data}. The audit list carries paging meta, and a
 *  page that reads meta.total crashes without it. */
const ENVELOPES: Record<string, unknown> = {
  '/admin/audit': { data: AUDIT, meta: { total: 2, today: 2, page: 1, limit: 10 } },
}

const FIXTURES: Record<string, unknown> = {
  '/settings': { site_name: 'Contrast Probe', header_color: '#1e293b', site_description: 'Looking glass' },
  '/nodes': NODES,
  '/admin/nodes': NODES,
  '/myip': { ip: '192.0.2.10', asn: 64496, as_name: 'EXAMPLE-AS', country: 'TR', city: 'Istanbul' },
  '/communities': [],
  '/admin/community-rules': [],
  '/admin/quick-queries': [],
  '/admin/bgp-neighbors': [
    {
      id: 1,
      node_id: 1,
      local_as: 64500,
      remote_as: 64496,
      peering_ip: '192.0.2.20',
      neighbor_ip: '192.0.2.21',
      ipv6_peering_ip: '',
      ipv6_neighbor_ip: '',
      multihop: false,
      peer_type: 'external',
      status: 'established',
      prefixes_received: 1024,
      created_at: '2026-01-01T00:00:00Z',
    },
  ],
  '/admin/account': { email: 'admin@hopstat.local' },
  '/admin/settings': {
    site_name: 'Contrast Probe',
    header_color: '#1e293b',
    site_description: 'Looking glass',
  },
  '/admin/system/status': {
    cpu: { percent: 34, level: 'normal' },
    memory: { percent: 78, level: 'warning' },
    memory_used_bytes: 3_400_000_000,
    memory_total_bytes: 4_300_000_000,
    cpu_cores: 4,
    cpu_load_1: 1.2,
    cpu_available: true,
    collected_at: '2026-01-02T10:00:00Z',
  },
  '/admin/system/addresses': {
    ipv4: [{ ip: '192.0.2.10', interface: 'eth0' }],
    ipv6: [{ ip: '2001:db8::1', interface: 'eth0' }],
  },
  '/admin/geoip/status': {
    configured: true,
    enabled: true,
    update_interval: '168h',
    asn_last_download: '2026-01-01T00:00:00Z',
    city_last_download: '2026-01-01T00:00:00Z',
    last_download: '2026-01-01T00:00:00Z',
    asn_build_date: '2025-12-30T00:00:00Z',
    city_build_date: '2025-12-30T00:00:00Z',
    asn_loaded: true,
    city_loaded: true,
  },
  '/admin/bgp/config': { enabled: false },
  '/admin/update/status': { current: 'test', latest: 'test', update_available: false },
}

/**
 * Answers every API call from fixtures. The scan needs pages that render real content;
 * it does not need a real backend, and a real one would make the run non-deterministic.
 */
export async function mockApi(page: Page, opts: { authenticated: boolean; brand?: string }) {
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace(/^\/api\/v1/, '')

    if (path === '/auth/session') {
      return opts.authenticated
        ? route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { email: 'admin@hopstat.local' } }) })
        : route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }) })
    }

    const envelope = Object.keys(ENVELOPES).find(key => path === key)
    if (envelope) {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ENVELOPES[envelope]),
      })
    }

    const match = Object.keys(FIXTURES).find(key => path === key || path.startsWith(`${key}/`))
    let data = match ? FIXTURES[match] : []
    if (opts.brand && (path === '/settings' || path === '/admin/settings')) {
      data = { ...(data as object), header_color: opts.brand }
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data }),
    })
  })
}

/**
 * Puts the page in the state the boot script expects: a chosen theme and a brand colour
 * delivered the way the server delivers it, with no cached palette to short-circuit it.
 */
export async function bootAs(page: Page, brand: string, theme: Theme) {
  await page.addInitScript(
    ({ brand, theme }: { brand: string; theme: string }) => {
      try {
        localStorage.clear()
        localStorage.setItem('hopstat-theme', theme)
      } catch {
        /* private mode */
      }
      ;(window as unknown as { __HOPSTAT_BOOTSTRAP__: unknown }).__HOPSTAT_BOOTSTRAP__ = {
        header_color: brand,
        site_name: 'Contrast Probe',
        site_description: 'Looking glass',
      }
    },
    { brand, theme },
  )
}
