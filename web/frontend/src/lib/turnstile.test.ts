import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  TURNSTILE_ACTION,
  TURNSTILE_SCRIPT_SRC,
  TurnstileSession,
  loadTurnstileScript,
  resetTurnstileScriptForTests,
  whenTurnstileReady,
  type TurnstileClient,
} from './turnstile'

function client(partial: Partial<TurnstileClient> = {}): TurnstileClient {
  return {
    ready: cb => cb(),
    render: () => 'widget-1',
    reset: () => {},
    remove: () => {},
    getResponse: () => undefined,
    ...partial,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  resetTurnstileScriptForTests()
  delete window.turnstile
  document.head.querySelectorAll('script[data-turnstile]').forEach(node => node.remove())
})

describe('loadTurnstileScript', () => {
  it('resolves immediately when the api is already present', async () => {
    window.turnstile = client()
    await expect(loadTurnstileScript()).resolves.toBeUndefined()
    expect(document.head.querySelector('script[data-turnstile]')).toBeNull()
  })

  it('loads the script once and retries after a failure', async () => {
    const first = loadTurnstileScript()
    const again = loadTurnstileScript()
    const script = document.head.querySelector('script[data-turnstile]') as HTMLScriptElement
    expect(script.src).toContain(TURNSTILE_SCRIPT_SRC)
    expect(script.async).toBe(false)
    expect(script.defer).toBe(false)
    script.dispatchEvent(new Event('error'))
    await expect(first).rejects.toThrow('turnstile script failed')
    await expect(again).rejects.toThrow('turnstile script failed')

    const retry = loadTurnstileScript()
    const scripts = document.head.querySelectorAll('script[data-turnstile]')
    const next = scripts[scripts.length - 1] as HTMLScriptElement
    next.dispatchEvent(new Event('load'))
    await expect(retry).resolves.toBeUndefined()
  })
})

describe('whenTurnstileReady', () => {
  it('waits until the client appears after the script load event', async () => {
    let polls = 0
    const pending = whenTurnstileReady(1000, async () => {
      polls += 1
      if (polls === 2) window.turnstile = client()
    })
    const script = document.head.querySelector('script[data-turnstile]') as HTMLScriptElement
    script.dispatchEvent(new Event('load'))
    const api = await pending
    expect(polls).toBe(2)
    expect(api.render).toBeTypeOf('function')
  })

  it('times out when the client never appears', async () => {
    let now = 0
    vi.spyOn(Date, 'now').mockImplementation(() => now)
    const pending = whenTurnstileReady(10, async () => {
      now += 20
    })
    const script = document.head.querySelector('script[data-turnstile]') as HTMLScriptElement
    script.dispatchEvent(new Event('load'))
    await expect(pending).rejects.toThrow('turnstile script failed')
  })

  it('rejects when the client disappears inside ready', async () => {
    window.turnstile = client({
      ready: callback => {
        delete window.turnstile
        callback()
      },
    })
    await expect(whenTurnstileReady(1000, async () => {})).rejects.toThrow('turnstile script failed')
  })

  it('uses the client when ready rejects an async script tag', async () => {
    const api = client({
      ready: () => {
        throw new Error('Remove async/defer from the Turnstile api.js script tag')
      },
    })
    window.turnstile = api
    await expect(whenTurnstileReady(1000, async () => {})).resolves.toBe(api)
  })
})

describe('TurnstileSession', () => {
  it('renders with the query action and returns a fresh token', async () => {
    const el = document.createElement('div')
    let renderedAction = ''
    const api = client({
      render: (_container, options) => {
        renderedAction = options.action
        options.callback('token-1')
        return 'widget-1'
      },
    })
    const session = new TurnstileSession()
    session.mount(api, el, 'site-key')
    expect(renderedAction).toBe(TURNSTILE_ACTION)
    await expect(session.getToken(api)).resolves.toBe('token-1')
  })

  it('prefers the widget response and waits for the next token after reset', async () => {
    const el = document.createElement('div')
    let callback: (token: string) => void = () => {}
    let response = 'stored'
    const reset = vi.fn(() => {
      response = ''
    })
    const api = client({
      render: (_container, options) => {
        callback = options.callback
        return 'widget-1'
      },
      getResponse: () => response,
      reset,
    })
    const session = new TurnstileSession()
    session.mount(api, el, 'site-key')
    await expect(session.getToken(api)).resolves.toBe('stored')

    session.reset(api)
    expect(reset).toHaveBeenCalledWith('widget-1')
    const pending = session.getToken(api, 20)
    callback('token-2')
    await expect(pending).resolves.toBe('token-2')
  })

  it('rejects waiters when the widget expires or times out', async () => {
    const el = document.createElement('div')
    let expire: () => void = () => {}
    const api = client({
      render: (_container, options) => {
        expire = options['expired-callback']!
        return 'widget-1'
      },
      getResponse: () => '',
    })
    const session = new TurnstileSession()
    session.mount(api, el, 'site-key')
    const expired = session.getToken(api, 1000)
    expire()
    await expect(expired).rejects.toThrow('turnstile failed')

    const timed = session.getToken(undefined, 5)
    await expect(timed).rejects.toThrow('turnstile timeout')
  })

  it('removes the widget and fails an in-flight token wait', async () => {
    const el = document.createElement('div')
    const remove = vi.fn()
    const api = client({
      render: () => 'widget-1',
      getResponse: () => '',
      remove,
    })
    const session = new TurnstileSession()
    session.mount(api, el, 'site-key')
    const pending = session.getToken(api, 1000)
    session.remove(api)
    expect(remove).toHaveBeenCalledWith('widget-1')
    await expect(pending).rejects.toThrow('turnstile failed')
  })
})
