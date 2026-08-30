import { useState, useEffect, type ReactNode } from 'react'
import { cachedSiteSettings } from '@/lib/appearance-cache'
import { SettingsContext, settingsDefaults, type SiteSettings } from './settings-context'

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<SiteSettings>(() => ({
    ...settingsDefaults,
    ...cachedSiteSettings(),
  }))
  const [loading, setLoading] = useState(true)

  const load = () => {
    fetch('/api/v1/settings')
      .then(r => r.json())
      .then(json => {
        if (json.data) setSettings({ ...settingsDefaults, ...json.data, active_languages: json.data.active_languages || settingsDefaults.active_languages })
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(load, [])
  return <SettingsContext.Provider value={{ settings, loading, reload: load }}>{children}</SettingsContext.Provider>
}
