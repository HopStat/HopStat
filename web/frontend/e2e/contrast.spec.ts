import { test, expect } from '@playwright/test'
import { BRANDS, THEMES, bootAs, mockApi } from './harness'
import { formatViolations, scanContrast } from './contrast-scan'

const PUBLIC_PAGES = ['/', '/communities', '/admin/login', '/no-such-page']
const ADMIN_PAGES = [
  '/admin',
  '/admin/nodes',
  '/admin/audit',
  '/admin/community-rules',
  '/admin/quick-queries',
  '/admin/bgp-neighbors',
  '/admin/geoip',
  '/admin/settings',
]

for (const theme of THEMES) {
  for (const brand of BRANDS) {
    test(`contrast: public pages, ${theme}, brand ${brand}`, async ({ page }) => {
      await mockApi(page, { authenticated: false, brand })
      await bootAs(page, brand, theme)

      let totalExamined = 0
      for (const path of PUBLIC_PAGES) {
        await page.goto(path)
        await page.waitForLoadState('networkidle')
        const { violations, examined } = await scanContrast(page)
        expect(examined, `${path} rendered no measurable text`).toBeGreaterThan(1)
        totalExamined += examined
        expect(
          violations,
          `${path} (${theme}, ${brand}):\n${formatViolations(violations)}`,
        ).toEqual([])
      }
      expect(totalExamined, 'the scan covered too little of the app').toBeGreaterThan(12)
    })

    test(`contrast: admin pages, ${theme}, brand ${brand}`, async ({ page }) => {
      await mockApi(page, { authenticated: true, brand })
      await bootAs(page, brand, theme)

      let totalExamined = 0
      for (const path of ADMIN_PAGES) {
        await page.goto(path)
        await page.waitForLoadState('networkidle')
        const { violations, examined } = await scanContrast(page)
        expect(examined, `${path} rendered no measurable text`).toBeGreaterThan(1)
        totalExamined += examined
        expect(
          violations,
          `${path} (${theme}, ${brand}):\n${formatViolations(violations)}`,
        ).toEqual([])
      }
      expect(totalExamined, 'the scan covered too little of the app').toBeGreaterThan(150)
    })
  }
}
