import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'

export interface SiteSettings {
  site_name: string
  site_description: string
  logo_path: string
  header_color: string
  url_website: string
  url_peeringdb: string
  url_contact: string
  url_terms: string
  url_privacy: string
  ping_count: string
  max_hops: string
  mtr_cycles: string
}

const defaults: SiteSettings = {
  site_name: 'Looking Glass',
  site_description: 'Network Diagnostic Platform',
  logo_path: '',
  header_color: '#1e293b',
  url_website: '',
  url_peeringdb: '',
  url_contact: '',
  url_terms: '',
  url_privacy: '',
  ping_count: '5',
  max_hops: '30',
  mtr_cycles: '10',
}

interface SettingsContextType {
  settings: SiteSettings
  loading: boolean
  reload: () => void
}

const SettingsContext = createContext<SettingsContextType>({
  settings: defaults,
  loading: true,
  reload: () => {},
})

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<SiteSettings>(defaults)
  const [loading, setLoading] = useState(true)

  const load = () => {
    fetch('/api/v1/settings')
      .then(r => r.json())
      .then(json => {
        if (json.data) setSettings({ ...defaults, ...json.data })
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(load, [])
  return <SettingsContext.Provider value={{ settings, loading, reload: load }}>{children}</SettingsContext.Provider>
}

export function useSettings() {
  return useContext(SettingsContext)
}
