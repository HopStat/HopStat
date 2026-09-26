export const TURNSTILE_ACTION = 'query'
export const TURNSTILE_SCRIPT_SRC = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'

export type TurnstileRenderOptions = {
  sitekey: string
  action: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
}

export type TurnstileClient = {
  ready: (callback: () => void) => void
  render: (container: HTMLElement, options: TurnstileRenderOptions) => string
  reset: (widgetId?: string) => void
  remove: (widgetId?: string) => void
  getResponse: (widgetId?: string) => string | undefined
}

declare global {
  interface Window {
    turnstile?: TurnstileClient
  }
}

let scriptPromise: Promise<void> | null = null

export function resetTurnstileScriptForTests() {
  scriptPromise = null
}

const TURNSTILE_READY_WAIT_MS = 50

function wait(ms: number) {
  return new Promise<void>(resolve => {
    setTimeout(resolve, ms)
  })
}

/** The script's load event can fire before window.turnstile exists. Keep polling until the client is ready. */
export function whenTurnstileReady(
  timeoutMs = 10000,
  delay: (ms: number) => Promise<void> = wait,
): Promise<TurnstileClient> {
  return loadTurnstileScript().then(async () => {
    const start = Date.now()
    for (;;) {
      const api = window.turnstile
      if (api?.ready) {
        return await new Promise<TurnstileClient>((resolve, reject) => {
          api.ready(() => {
            if (window.turnstile) resolve(window.turnstile)
            else reject(new Error('turnstile script failed'))
          })
        })
      }
      if (Date.now() - start > timeoutMs) {
        throw new Error('turnstile script failed')
      }
      await delay(TURNSTILE_READY_WAIT_MS)
    }
  })
}

export function loadTurnstileScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve()
  if (scriptPromise) return scriptPromise
  scriptPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = TURNSTILE_SCRIPT_SRC
    script.async = true
    script.defer = true
    script.dataset.turnstile = '1'
    script.onload = () => resolve()
    script.onerror = () => {
      scriptPromise = null
      reject(new Error('turnstile script failed'))
    }
    document.head.appendChild(script)
  })
  return scriptPromise
}

type Waiter = (token: string | null) => void

/** Explicit widget. The page stays up after a query, so each attempt resets the widget. */
export class TurnstileSession {
  private widgetId: string | null = null
  private token = ''
  private waiters: Waiter[] = []

  mount(client: TurnstileClient, container: HTMLElement, siteKey: string) {
    this.widgetId = client.render(container, {
      sitekey: siteKey,
      action: TURNSTILE_ACTION,
      callback: token => this.deliver(token),
      'expired-callback': () => this.deliver(null),
      'error-callback': () => this.deliver(null),
    })
  }

  deliver(token: string | null) {
    this.token = token ?? ''
    const waiters = this.waiters
    this.waiters = []
    for (const waiter of waiters) waiter(token)
  }

  getToken(client: TurnstileClient | undefined, timeoutMs = 15000): Promise<string> {
    const current = this.widgetId && client ? client.getResponse(this.widgetId) : ''
    if (current) {
      this.token = current
      return Promise.resolve(current)
    }
    if (this.token) return Promise.resolve(this.token)
    return new Promise((resolve, reject) => {
      let onToken: Waiter = () => {}
      const timer = setTimeout(() => {
        this.waiters = this.waiters.filter(waiter => waiter !== onToken)
        reject(new Error('turnstile timeout'))
      }, timeoutMs)
      onToken = token => {
        clearTimeout(timer)
        if (!token) reject(new Error('turnstile failed'))
        else resolve(token)
      }
      this.waiters.push(onToken)
    })
  }

  reset(client: TurnstileClient | undefined) {
    this.token = ''
    if (this.widgetId && client) client.reset(this.widgetId)
  }

  remove(client: TurnstileClient | undefined) {
    const id = this.widgetId
    this.widgetId = null
    this.token = ''
    if (id && client) client.remove(id)
    this.deliver(null)
  }
}
