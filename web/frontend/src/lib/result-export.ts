import type { BGPResult, BGPRoute } from '@/types/domain'

interface ResultTextInput {
  command: string
  target: string
  nodeName?: string
  shareUrl?: string
  lines: string[]
  bgp?: BGPResult | null
}

function formatBgpRoute(route: BGPRoute): string {
  const fields = [
    `prefix=${route.prefix || '-'}`,
    `next_hop=${route.next_hop || '-'}`,
    `as_path=${route.as_path?.length ? route.as_path.join(' ') : '-'}`,
    `origin=${route.origin || '-'}`,
    `local_pref=${route.local_pref ?? 0}`,
    `med=${route.med ?? 0}`,
  ]
  if (route.communities?.length) fields.push(`communities=${route.communities.join(',')}`)
  if (route.best) fields.push('best')
  return fields.join('  ')
}

/** Plain-text rendering of a finished query, suitable for pasting into a chat or ticket. */
export function buildResultText({ command, target, nodeName, shareUrl, lines, bgp }: ResultTextInput): string {
  let header = `$ ${command} ${target}`.trim()
  if (nodeName) header += `  @ ${nodeName}`

  const routes = bgp?.routes ?? []
  const body = routes.length > 0 ? routes.map(formatBgpRoute) : lines

  const parts = [header, '', ...(body.length > 0 ? body : ['(no output)'])]
  if (shareUrl) parts.push('', shareUrl)
  return parts.join('\n')
}
