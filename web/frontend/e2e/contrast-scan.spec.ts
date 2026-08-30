import { test, expect } from '@playwright/test'
import { bootAs, mockApi } from './harness'
import { scanContrast } from './contrast-scan'

/**
 * Proves the scanner can fail. Without this, a scan that silently stopped measuring would
 * report every page clean and the gate would mean nothing.
 */
test('the scanner flags text below the threshold', async ({ page }) => {
  await mockApi(page, { authenticated: false })
  await bootAs(page, '#1e293b', 'light')
  await page.goto('/')

  await page.evaluate(() => {
    const el = document.createElement('p')
    el.id = 'planted-low-contrast'
    el.textContent = 'deliberately unreadable'
    el.style.cssText = 'color:#bbbbbb;background:#ffffff;font-size:14px;padding:8px'
    document.body.appendChild(el)
  })

  const { violations } = await scanContrast(page)
  const planted = violations.filter(v => v.path.includes('planted-low-contrast'))
  expect(planted).toHaveLength(1)
  expect(planted[0].ratio).toBeLessThan(4.5)
})

test('the scanner exempts disabled controls and invisible text', async ({ page }) => {
  await mockApi(page, { authenticated: false })
  await bootAs(page, '#1e293b', 'light')
  await page.goto('/')

  await page.evaluate(() => {
    const disabled = document.createElement('button')
    disabled.id = 'planted-disabled'
    disabled.disabled = true
    disabled.textContent = 'inactive control'
    disabled.style.cssText = 'color:#bbbbbb;background:#ffffff'

    // aria-hidden is not an exemption: that text is still on screen. Only what cannot be
    // seen at all is skipped.
    const hidden = document.createElement('p')
    hidden.id = 'planted-hidden'
    hidden.textContent = 'not painted'
    hidden.style.cssText = 'color:#bbbbbb;background:#ffffff;visibility:hidden'

    document.body.append(disabled, hidden)
  })

  const { violations } = await scanContrast(page)
  expect(violations.filter(v => v.path.includes('planted-'))).toEqual([])
})
