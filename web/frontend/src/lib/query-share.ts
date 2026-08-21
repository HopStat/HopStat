/** URL slug ↔ backend command name. Slugs keep shared links readable: /bgp/1.1.1.0/24 */
const COMMAND_SLUGS: Record<string, string> = {
  ping: 'ping',
  traceroute: 'traceroute',
  bgp: 'bgp_route',
}

const SLUG_BY_COMMAND: Record<string, string> = Object.fromEntries(
  Object.entries(COMMAND_SLUGS).map(([slug, command]) => [command, slug]),
)

export const QUERY_PATH_SLUGS = Object.keys(COMMAND_SLUGS)

const MAX_TARGET_LENGTH = 255

export interface SharedQuery {
  command: string
  target: string
  nodeId: number | null
}

export interface ShareableQuery {
  command: string
  target: string
  nodeId?: number | null
  /** Default-node queries drop the ?node= suffix — single-node installs get a bare path. */
  isDefaultNode?: boolean
}

/** Builds the shareable location for a query, e.g. `/bgp/1.1.1.0/24` or `/ping/8.8.8.8?node=3`. */
export function buildQueryPath({ command, target, nodeId, isDefaultNode }: ShareableQuery): string {
  const slug = SLUG_BY_COMMAND[command]
  if (!slug) return '/'

  const encodedTarget = target
    .split('/')
    .filter(segment => segment.length > 0)
    .map(encodeURIComponent)
    .join('/')
  if (!encodedTarget) return '/'

  const path = `/${slug}/${encodedTarget}`
  return nodeId && nodeId > 0 && !isDefaultNode ? `${path}?node=${nodeId}` : path
}

/** Reads a shareable query off a location. Returns null when the path is not a query link. */
export function parseQueryLocation(pathname: string, search = ''): SharedQuery | null {
  const [slug, ...targetSegments] = pathname.replace(/^\/+/, '').split('/')
  const command = COMMAND_SLUGS[slug]
  if (!command) return null

  let target: string
  try {
    target = targetSegments.filter(segment => segment.length > 0).map(decodeURIComponent).join('/').trim()
  } catch {
    return null // malformed percent-encoding
  }
  if (!target || target.length > MAX_TARGET_LENGTH) return null

  const nodeId = Number.parseInt(new URLSearchParams(search).get('node') ?? '', 10)
  return {
    command,
    target,
    nodeId: Number.isInteger(nodeId) && nodeId > 0 ? nodeId : null,
  }
}

export function buildShareUrl(query: ShareableQuery): string {
  return `${window.location.origin}${buildQueryPath(query)}`
}
