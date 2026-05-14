const API_BASE = '/api/v1'

function getToken(): string | null {
  return localStorage.getItem('jwt_token')
}

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers })

  if (res.status === 401) {
    localStorage.removeItem('jwt_token')
    window.location.href = '/admin/login'
    throw new Error('Unauthorized')
  }

  if (res.status === 204) return undefined as T

  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Request failed')
  return data.data as T
}

export const api = {
  get: <T>(path: string) => apiFetch<T>(path),

  post: <T>(path: string, body?: unknown) =>
    apiFetch<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),

  put: <T>(path: string, body: unknown) =>
    apiFetch<T>(path, { method: 'PUT', body: JSON.stringify(body) }),

  patch: <T>(path: string) =>
    apiFetch<T>(path, { method: 'PATCH' }),

  delete: <T>(path: string) =>
    apiFetch<T>(path, { method: 'DELETE' }),
}

export async function login(email: string, password: string): Promise<{ token: string; expires_at: string }> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Login failed')
  return data.data
}

export async function exportAuditCSV(): Promise<void> {
  const token = getToken()
  const res = await fetch(`${API_BASE}/admin/audit/export`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error('Export failed')
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'audit_log.csv'
  a.click()
  URL.revokeObjectURL(url)
}
