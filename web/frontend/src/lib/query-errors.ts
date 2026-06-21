type TranslateFn = (key: string) => string

const ERROR_CODE_KEYS: Record<string, string> = {
  COMMAND_DISABLED: 'error.command_disabled',
  NODE_NOT_FOUND: 'error.node_not_found',
  INVALID_TARGET: 'error.invalid_target',
  DNS_NOT_FOUND: 'error.dns_not_found',
  COMMAND_TIMEOUT: 'error.timeout',
  TIMEOUT: 'error.timeout',
  RATE_LIMITED: 'error.rate_limited',
  QUERY_POOL_FULL: 'error.busy',
  POOL_FULL: 'error.busy',
  NODE_UNAVAILABLE: 'error.node_unavailable',
  INTERNAL: 'error.internal',
  INTERNAL_ERROR: 'error.internal',
  DRIVER_ERROR: 'error.internal',
  UNAUTHORIZED: 'login.invalid',
}

const ERROR_MSG_KEYS: Record<string, string> = {
  'command not enabled for this node': 'error.command_disabled',
  'node not found': 'error.node_not_found',
  'invalid target': 'error.invalid_target',
  'dns resolution failed': 'error.dns_not_found',
  'command timed out': 'error.timeout',
  'rate limit exceeded': 'error.rate_limited',
  'server is busy, try again later': 'error.busy',
  'command execution failed': 'error.internal',
  'invalid credentials': 'login.invalid',
  'timeout': 'error.timeout',
  'failed to fetch result': 'query.fetch_failed',
}

export function translateQueryError(
  t: TranslateFn,
  errorCode?: string,
  errorMsg?: string,
): string {
  const code = errorCode?.trim().toUpperCase()
  if (code && ERROR_CODE_KEYS[code]) {
    return t(ERROR_CODE_KEYS[code])
  }

  const normalized = errorMsg?.trim().toLowerCase()
  if (normalized && ERROR_MSG_KEYS[normalized]) {
    return t(ERROR_MSG_KEYS[normalized])
  }

  if (errorMsg?.trim()) {
    return errorMsg.trim()
  }

  return t('query.failed')
}

const BGP_NO_ROUTES_RE = /^no matching BGP routes for (.+)$/i

export function translateBgpRawMessage(t: TranslateFn, raw: string): string {
  return raw
    .split('\n')
    .map(line => {
      const trimmed = line.trim()
      const match = BGP_NO_ROUTES_RE.exec(trimmed)
      if (match) {
        return t('result.bgp_no_routes').replace('{{target}}', match[1])
      }
      return line
    })
    .join('\n')
}
