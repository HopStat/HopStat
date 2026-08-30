import { useState, useCallback, useEffect, type ReactNode } from 'react'
import { login as apiLogin, checkSession } from '@/lib/api-client'
import { AuthContext } from './auth-context'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    checkSession()
      .then(setIsAuthenticated)
      .finally(() => setReady(true))
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    await apiLogin(email, password)
    setIsAuthenticated(true)
  }, [])

  const logout = useCallback(async () => {
    setIsAuthenticated(false)
    try {
      await fetch('/api/v1/auth/logout', {
        method: 'POST',
        credentials: 'include',
      })
    } catch {
      // Session cleared locally; ignore network errors.
    }
  }, [])

  return (
    <AuthContext.Provider value={{ isAuthenticated, ready, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
