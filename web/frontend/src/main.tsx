import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { AuthProvider } from '@/contexts/auth-context'
import { ThemeProvider } from '@/contexts/theme-context'
import { I18nProvider } from '@/contexts/i18n-context'
import { SettingsProvider } from '@/contexts/settings-context'
import { BrandStyleInjector } from '@/components/brand-style-injector'
import { enhanceViewportForVirtualKeyboard } from '@/lib/mobile-viewport'
import App from './App'
import './globals.css'
import '@/i18n/index'

enhanceViewportForVirtualKeyboard()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <SettingsProvider>
        <I18nProvider>
          <AuthProvider>
            <BrandStyleInjector />
            <App />
          </AuthProvider>
        </I18nProvider>
      </SettingsProvider>
    </ThemeProvider>
  </StrictMode>,
)
