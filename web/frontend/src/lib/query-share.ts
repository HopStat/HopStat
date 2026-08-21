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
  /** Node slug or id as written in the link; null means "whatever the default node is". */
  node: string | null
}

export interface ShareableQuery {
  command: string
  target: string
  /** Slug for a non-default node; null keeps the link short for the default one. */
  nodeSlug?: string | null
}

/**
 * Node names become path segments, so fold them down to ASCII: "ŞİŞLİ" → "sisli",
 * "Smoke Node" → "smoke-node". Returns '' when nothing usable is left.
 */
export function slugifyNodeName(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u0131\u0130]/g, 'i')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** True for slugs that would be read back as a command, so they must not be used as node slugs. */
export function isCommandSlug(slug: string): boolean {
  return slug in COMMAND_SLUGS
}

/** Builds the shareable location, e.g. `/bgp/1.1.1.0/24` or `/sofia/ping/8.8.8.8`. */
export function buildQueryPath({ command, target, nodeSlug }: ShareableQuery): string {
  const slug = SLUG_BY_COMMAND[command]
  if (!slug) return '/'

  const encodedTarget = target
    .split('/')
    .filter(segment => segment.length > 0)
    .map(encodeURIComponent)
    .join('/')
  if (!encodedTarget) return '/'

  const prefix = nodeSlug ? `/${encodeURIComponent(nodeSlug)}` : ''
  return `${prefix}/${slug}/${encodedTarget}`
}

/** Reads a shareable query off a location. Returns null when the path is not a query link. */
export function parseQueryLocation(pathname: string, search = ''): SharedQuery | null {
  const segments = pathname.split('/').filter(segment => segment.length > 0)

  let node: string | null = null
  let command = COMMAND_SLUGS[segments[0]]
  let targetSegments = segments.slice(1)

  if (!command && segments.length > 1 && COMMAND_SLUGS[segments[1]]) {
    node = decodeSegment(segments[0])
    command = COMMAND_SLUGS[segments[1]]
    targetSegments = segments.slice(2)
  }
  if (!command) return null

  let target: string
  try {
    target = targetSegments.map(decodeURIComponent).join('/').trim()
  } catch {
    return null // malformed percent-encoding
  }
  if (!target || target.length > MAX_TARGET_LENGTH) return null

  // v2.1.76 links carried the node as ?node=<id>; keep them working.
  if (!node) node = new URLSearchParams(search).get('node')?.trim() || null

  return { command, target, node }
}

function decodeSegment(segment: string): string | null {
  try {
    return decodeURIComponent(segment)
  } catch {
    return null
  }
}

export function buildShareUrl(query: ShareableQuery): string {
  return `${window.location.origin}${buildQueryPath(query)}`
}
