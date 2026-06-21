import { useEffect } from 'react'
import { useSettings } from '@/contexts/settings-context'
import { useTheme } from '@/contexts/theme-context'
import { saveAppearanceCache } from '@/lib/appearance-cache'

export function BrandStyleInjector() {
  const { settings } = useSettings()
  const { theme } = useTheme()

  useEffect(() => {
    saveAppearanceCache(
      {
        header_color: settings.header_color,
        site_name: settings.site_name,
        site_description: settings.site_description,
      },
      theme,
    )
  }, [settings.header_color, settings.site_name, settings.site_description, theme])

  return null
}
