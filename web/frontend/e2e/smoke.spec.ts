import { test, expect } from '@playwright/test'
import { bootAs, mockApi } from './harness'

const PAGES = ['/', '/communities', '/admin/login', '/no-such-page']
const ADMIN_PAGES = ['/admin', '/admin/nodes', '/admin/settings', '/admin/bgp-neighbors']

/** Errors a page emits that say something is actually broken. Failed fetches from routes
 *  the harness does not model are noise, not defects. */
function isRealError(text: string): boolean {
  return !/Failed to load resource|net::ERR_|favicon/i.test(text)
}

test('public pages render without console errors', async ({ page }) => {
  const errors: string[] = []
  let current = ''
  page.on('console', msg => {
    if (msg.type() === 'error' && isRealError(msg.text())) errors.push(`${current}: ${msg.text()}`)
  })
  page.on('pageerror', err => errors.push(`${current}: ${err.message}`))

  await mockApi(page, { authenticated: false })
  await bootAs(page, '#1e293b', 'light')

  for (const path of PAGES) {
    current = path
    await page.goto(path)
    await page.waitForLoadState('networkidle')
    await expect(page.locator('body')).not.toBeEmpty()
  }
  expect(errors, errors.join('\n')).toEqual([])
})

test('admin pages render without console errors', async ({ page }) => {
  const errors: string[] = []
  let current = ''
  page.on('console', msg => {
    if (msg.type() === 'error' && isRealError(msg.text())) errors.push(`${current}: ${msg.text()}`)
  })
  page.on('pageerror', err => errors.push(`${current}: ${err.message}`))

  await mockApi(page, { authenticated: true })
  await bootAs(page, '#1e293b', 'dark')

  for (const path of ADMIN_PAGES) {
    current = path
    await page.goto(path)
    await page.waitForLoadState('networkidle')
    await expect(page.locator('body')).not.toBeEmpty()
  }
  expect(errors, errors.join('\n')).toEqual([])
})

test('the brand colour reaches the document', async ({ page }) => {
  await mockApi(page, { authenticated: false, brand: '#e0edd4' })
  await bootAs(page, '#e0edd4', 'light')
  await page.goto('/')

  const brand = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue('--brand').trim(),
  )
  expect(brand.toLowerCase()).toBe('#e0edd4')
})

test('the theme preference reaches the document', async ({ page }) => {
  await mockApi(page, { authenticated: false })
  await bootAs(page, '#1e293b', 'dark')
  await page.goto('/')
  await expect(page.locator('html')).toHaveClass(/dark/)
})

for (const path of ['/', '/admin/login']) {
  test(`no horizontal overflow at 375px: ${path}`, async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await mockApi(page, { authenticated: false })
    await bootAs(page, '#1e293b', 'light')
    await page.goto(path)
    await page.waitForLoadState('networkidle')

    const overflow = await page.evaluate(() => {
      const doc = document.documentElement
      return doc.scrollWidth - doc.clientWidth
    })
    expect(overflow).toBeLessThanOrEqual(1)
  })
}
