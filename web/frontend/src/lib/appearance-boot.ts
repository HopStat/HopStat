import { applyBrandColor } from '@/lib/brand-color'
import {
  applyAppearanceCache,
  applyThemeClass,
  getInitialTheme,
  readAppearanceCache,
  saveAppearanceCache,
  type AppearanceSettings,
} from '@/lib/appearance-cache'

declare global {
  interface Window {
    __HOPSTAT_BOOTSTRAP__?: Partial<AppearanceSettings>
  }
}

function applyDocumentTitle(siteName?: string) {
  const name = siteName?.trim()
  if (name) document.title = name
}

function loadSettingsSync(): Partial<AppearanceSettings> | null {
  try {
    const xhr = new XMLHttpRequest()
    xhr.open('GET', '/api/v1/settings', false)
    xhr.send()
    if (xhr.status < 200 || xhr.status >= 300) return null
    const json = JSON.parse(xhr.responseText) as { data?: Partial<AppearanceSettings> }
    return json.data ?? null
  } catch {
    return null
  }
}

export function runAppearanceBoot() {
  const theme = getInitialTheme()
  applyThemeClass(theme)

  const cached = readAppearanceCache()
  if (cached) {
    applyAppearanceCache(cached)
    applyDocumentTitle(cached.site_name)
    return
  }

  const bootstrap = window.__HOPSTAT_BOOTSTRAP__
  if (bootstrap?.header_color) {
    applyBrandColor(bootstrap.header_color, theme)
    applyDocumentTitle(bootstrap.site_name)
    saveAppearanceCache(
      {
        header_color: bootstrap.header_color,
        site_name: bootstrap.site_name ?? 'Looking Glass',
        site_description: bootstrap.site_description,
      },
      theme,
    )
    return
  }

  const settings = loadSettingsSync()
  if (!settings?.header_color) return

  applyBrandColor(settings.header_color, theme)
  applyDocumentTitle(settings.site_name)
  saveAppearanceCache(
    {
      header_color: settings.header_color,
      site_name: settings.site_name ?? 'Looking Glass',
      site_description: settings.site_description,
    },
    theme,
  )
}
