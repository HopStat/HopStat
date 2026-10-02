import { test, expect } from '@playwright/test'
import { bootAs, mockApi } from './harness'

const SITE_KEY = '1x00000000000000000000AA'

/** Stands in for Cloudflare's api.js: renders nothing and hands out a token shortly after. */
const FAKE_TURNSTILE = `
window.turnstile = {
  ready: cb => cb(),
  render: (box, opts) => { setTimeout(() => opts.callback('fake-token'), 300); return 'w1' },
  reset: () => {},
  remove: () => {},
  getResponse: () => undefined,
}
`

test('a shared query link waits for the Turnstile token before submitting', async ({ page }) => {
  const posted: Record<string, unknown>[] = []

  await mockApi(page, { authenticated: false })
  // Registered after mockApi, so these win for the paths they handle.
  await page.route('**/api/v1/settings', async route => {
    // A first-time visitor has no cached settings, and the site key arrives after /nodes.
    await new Promise(resolve => setTimeout(resolve, 800))
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: { site_name: 'Share Probe', turnstile_site_key: SITE_KEY } }),
    })
  })
  await page.route('**/api/v1/query', async route => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    posted.push(body)
    if (!body['cf-turnstile-response']) {
      return route.fulfill({
        status: 403,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'security check failed', code: 'TURNSTILE' }),
      })
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: { query_id: 'q1' } }),
    })
  })
  await page.route('https://challenges.cloudflare.com/**', route =>
    route.fulfill({ status: 200, contentType: 'application/javascript', body: FAKE_TURNSTILE }),
  )
  await bootAs(page, '#1e293b', 'light')

  await page.goto('/ping/192.0.2.1')

  await expect.poll(() => posted.length, { timeout: 10_000 }).toBe(1)
  expect(posted[0]['cf-turnstile-response']).toBe('fake-token')
  expect(posted[0].target).toBe('192.0.2.1')
  await expect(page.locator('.query-form-error')).toHaveCount(0)
})
