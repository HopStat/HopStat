import { useEffect, useState, type ReactNode } from 'react'
import {
  applyThemeClass,
  getInitialTheme,
  THEME_STORAGE_KEY,
  type Theme,
} from '@/lib/appearance-cache'
import { ThemeContext } from './theme-context'

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    const initial = getInitialTheme()
    applyThemeClass(initial)
    return initial
  })

  useEffect(() => {
    applyThemeClass(theme)
  }, [theme])

  const toggleTheme = () => {
    setTheme(prev => {
      const next: Theme = prev === 'dark' ? 'light' : 'dark'
      localStorage.setItem(THEME_STORAGE_KEY, next)
      return next
    })
  }

  return (
    <ThemeContext.Provider value={{ theme, toggleTheme }}>
      {children}
    </ThemeContext.Provider>
  )
}
