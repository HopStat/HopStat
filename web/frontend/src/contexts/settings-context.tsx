import { createContext, useContext } from 'react'

export interface SiteSettings {
  site_name: string
  site_description: string
  logo_path: string
  header_color: string
  local_as: string
  active_languages: string
  url_website: string
  url_peeringdb: string
  url_contact: string
  url_terms: string
  url_privacy: string
  ping_count: string
  max_hops: string
}

export const settingsDefaults: SiteSettings = {
  site_name: 'Looking Glass',
  site_description: 'Network Diagnostic Platform',
  logo_path: '',
  header_color: '#1e293b',
  local_as: '',
  active_languages: 'en,tr',
  url_website: '',
  url_peeringdb: '',
  url_contact: '',
  url_terms: '',
  url_privacy: '',
  ping_count: '5',
  max_hops: '30',
}

export interface SettingsContextType {
  settings: SiteSettings
  loading: boolean
  reload: () => void
}

export const SettingsContext = createContext<SettingsContextType>({
  settings: settingsDefaults,
  loading: true,
  reload: () => {},
})

export function useSettings() {
  return useContext(SettingsContext)
}
