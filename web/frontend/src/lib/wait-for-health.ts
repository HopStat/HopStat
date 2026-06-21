export interface HealthResponse {
  status?: string
  version?: string
}

export interface WaitForHealthOptions {
  expectedVersion?: string
  initialDelayMs?: number
  intervalMs?: number
  maxAttempts?: number
  signal?: AbortSignal
  fetchHealth?: () => Promise<HealthResponse | null>
}

export function normalizeVersion(version: string): string {
  return version.trim().replace(/^v/i, '')
}

function versionsMatch(expected: string, actual: string): boolean {
  const want = normalizeVersion(expected)
  const got = normalizeVersion(actual)
  if (!want || !got) return false
  if (want === 'dev' || got === 'dev') return true
  return want === got
}

async function defaultFetchHealth(): Promise<HealthResponse | null> {
  const res = await fetch('/health', { cache: 'no-store' })
  if (!res.ok) return null
  return res.json() as Promise<HealthResponse>
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException('Aborted', 'AbortError'))
      return
    }
    const timer = window.setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = () => {
      window.clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    }
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

export async function waitForHealth(options: WaitForHealthOptions = {}): Promise<HealthResponse> {
  const {
    expectedVersion,
    initialDelayMs = 1500,
    intervalMs = 2000,
    maxAttempts = 45,
    signal,
    fetchHealth = defaultFetchHealth,
  } = options

  await sleep(initialDelayMs, signal)

  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    if (signal?.aborted) {
      throw new DOMException('Aborted', 'AbortError')
    }

    try {
      const health = await fetchHealth()
      if (health?.status === 'ok') {
        if (!expectedVersion || !health.version || versionsMatch(expectedVersion, health.version)) {
          return health
        }
      }
    } catch {
      // Service still restarting or temporarily unreachable.
    }

    if (attempt < maxAttempts - 1) {
      await sleep(intervalMs, signal)
    }
  }

  throw new Error('service not ready')
}
