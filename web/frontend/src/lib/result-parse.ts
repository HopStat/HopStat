import type { BGPRoute, BGPResult, PingResult } from '@/types/domain'

export function bestBGPRoute(routes: BGPRoute[] | undefined): BGPRoute | undefined {
  if (!routes?.length) return undefined
  return routes.find(r => r.best) ?? routes[0]
}

export function extractASPathFromBGP(parsed: BGPResult | null | undefined): number[] {
  const fromParsed = bestBGPRoute(parsed?.routes)?.as_path
  if (fromParsed && fromParsed.length > 0) return fromParsed
  return []
}

/** Build AS path for the BGP map; never synthesize a path from target ASN alone. */
export function buildBgpMapAsPath(asPath: number[], targetAsn?: number | null): number[] {
  const validPath = asPath.filter(asn => asn > 0)
  if (validPath.length === 0) return []
  if (!targetAsn || targetAsn <= 0 || validPath.includes(targetAsn)) return validPath
  return [...validPath, targetAsn]
}

/** Show the AS path map only when BGP returned a real path to visualize. */
export function shouldShowBgpAsPathMap(
  command: string | undefined,
  mapAsPath: number[],
  options?: { hasBgpRoutes?: boolean; hasServerAsPath?: boolean },
): boolean {
  if (command !== 'bgp_route' || mapAsPath.length === 0) return false
  if (options?.hasBgpRoutes || options?.hasServerAsPath) return true
  return false
}

export function extractASPathFromLines(lines: string[]): number[] {
  for (const line of lines) {
    const m = line.match(/AS_PATH:\s*(\[[^\]]+\]|\d+(?:\s+\d+)*)/i)
    if (!m) continue
    const nums = m[1].match(/\d+/g)
    if (nums?.length) return nums.map(Number)
  }
  return []
}

export function isTracerouteHopLine(line: string): boolean {
  const trimmed = line.trim()
  if (!trimmed || trimmed.startsWith('traceroute to') || trimmed.startsWith('Start')) return false
  return /^\d+[\s.]/.test(trimmed)
}

/** Parse ping statistics from streamed or final output lines (mirrors backend generic parser). */
export function parsePingFromLines(lines: string[]): PingResult {
  const result: PingResult = { raw: lines.join('\n') }
  const rtts: number[] = []
  let hasSummary = false
  let minSeq = Infinity
  let maxSeq = -1

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) continue

    if (trimmed.includes('packets transmitted')) {
      hasSummary = true
      const normalized = trimmed.replace(/,/g, '')
      const fields = normalized.split(/\s+/)
      for (let i = 0; i < fields.length; i++) {
        if (fields[i] === 'transmitted' && i >= 2) {
          const n = parseInt(fields[i - 2], 10)
          if (!Number.isNaN(n)) result.packets_sent = n
        }
        if (fields[i] === 'received' && i > 0) {
          let val = fields[i - 1]
          if (val === 'packets' && i >= 2) val = fields[i - 2]
          const n = parseInt(val, 10)
          if (!Number.isNaN(n)) result.packets_recv = n
        }
        if (fields[i] === 'loss' && i > 0) {
          let candidate = fields[i - 1]
          if (candidate === 'packet' && i >= 2) candidate = fields[i - 2]
          if (candidate.endsWith('%')) {
            const n = parseFloat(candidate.slice(0, -1))
            if (!Number.isNaN(n)) result.packet_loss = n
          }
        }
      }
    }

    if (trimmed.includes('rtt') || trimmed.includes('min/avg/max')) {
      const eq = trimmed.indexOf('=')
      if (eq !== -1) {
        let stats = trimmed.slice(eq + 1).trim()
        stats = stats.replace(/^mdev=/, '').replace(/\s*ms\s*$/i, '').trim()
        const parts = stats.split('/')
        if (parts.length >= 3) {
          const min = parseFloat(parts[0])
          const avg = parseFloat(parts[1])
          const max = parseFloat(parts[2])
          if (!Number.isNaN(min)) result.min_rtt = min
          if (!Number.isNaN(avg)) result.avg_rtt = avg
          if (!Number.isNaN(max)) result.max_rtt = max
        }
      }
    }

    const seq = extractPingSeq(trimmed)
    if (seq != null) {
      if (seq < minSeq) minSeq = seq
      if (seq > maxSeq) maxSeq = seq
    }

    const timeMatch = trimmed.match(/time[=<]([\d.]+)\s*ms/i)
    if (timeMatch) {
      const t = parseFloat(timeMatch[1])
      if (!Number.isNaN(t)) rtts.push(t)
    } else if (/(?:^|\s)seq=\d+/i.test(trimmed)) {
      // MikroTik: "seq=1 time=1.2ms" — exclude Linux "icmp_seq=" lines (handled above).
      const mikrotikTime = trimmed.match(/time=([\d.]+)/)
      if (mikrotikTime) {
        const t = parseFloat(mikrotikTime[1])
        if (!Number.isNaN(t)) rtts.push(t)
      }
    }
  }

  if (rtts.length > 0) {
    if (!hasSummary) {
      result.packets_recv = rtts.length
    }
    if (result.min_rtt == null) result.min_rtt = Math.min(...rtts)
    if (result.max_rtt == null) result.max_rtt = Math.max(...rtts)
    if (result.avg_rtt == null) {
      result.avg_rtt = rtts.reduce((sum, v) => sum + v, 0) / rtts.length
    }
  }

  if (!hasSummary && maxSeq >= 0) {
    result.packets_sent = minSeq === 0 ? maxSeq + 1 : maxSeq
  }

  if (
    hasSummary &&
    result.packets_sent != null &&
    result.packets_sent > 0 &&
    result.packets_recv != null &&
    result.packet_loss == null
  ) {
    result.packet_loss = ((result.packets_sent - result.packets_recv) / result.packets_sent) * 100
  }

  return result
}

function extractPingSeq(line: string): number | null {
  const patterns = [
    /icmp_seq=(\d+)/i,
    /icmp_seq[=\s]+(\d+)/i,
    /(?:^|\s)seq=(\d+)/i,
  ]
  for (const pattern of patterns) {
    const match = line.match(pattern)
    if (!match) continue
    const seq = parseInt(match[1], 10)
    return Number.isNaN(seq) ? null : seq
  }
  return null
}

function pickNumber(lineVal?: number, serverVal?: number): number | undefined {
  if (serverVal != null && Number.isFinite(serverVal)) return serverVal
  if (lineVal != null && Number.isFinite(lineVal)) return lineVal
  return undefined
}

export function mergePingResult(fromLines: PingResult, fromServer?: PingResult | null): PingResult {
  return {
    raw: fromServer?.raw || fromLines.raw,
    packets_sent: pickNumber(fromLines.packets_sent, fromServer?.packets_sent),
    packets_recv: pickNumber(fromLines.packets_recv, fromServer?.packets_recv),
    packet_loss: pickNumber(fromLines.packet_loss, fromServer?.packet_loss),
    min_rtt: pickNumber(fromLines.min_rtt, fromServer?.min_rtt),
    avg_rtt: pickNumber(fromLines.avg_rtt, fromServer?.avg_rtt),
    max_rtt: pickNumber(fromLines.max_rtt, fromServer?.max_rtt),
  }
}

export function hasPingData(result: PingResult): boolean {
  return (
    (result.packets_sent ?? 0) > 0 ||
    (result.packets_recv ?? 0) > 0 ||
    result.packet_loss != null ||
    result.min_rtt != null ||
    result.avg_rtt != null ||
    result.max_rtt != null
  )
}

/** Ping command finished streaming (summary line present); backend may still be enriching AS path. */
export function isPingOutputComplete(lines: string[]): boolean {
  return lines.some(line => /packets transmitted/i.test(line))
}
