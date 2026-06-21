const API_BASE = '/api/v1'

export class ApiError extends Error {
  code?: string

  constructor(message: string, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

const defaultFetchOptions: RequestInit = {
  credentials: 'include',
}

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...defaultFetchOptions,
    ...options,
    headers,
  })

  if (res.status === 401) {
    window.location.href = '/admin/login'
    throw new ApiError('Unauthorized', 'UNAUTHORIZED')
  }

  if (res.status === 204) return undefined as T

  const data = await res.json()
  if (!res.ok) {
    throw new ApiError(data.error || 'Request failed', data.error_code)
  }
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

export async function login(email: string, password: string): Promise<{ expires_at: string }> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  const data = await res.json()
  if (!res.ok) throw new ApiError(data.error || 'Login failed', data.error_code)
  return data.data
}

export async function checkSession(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/auth/session`, { credentials: 'include' })
    return res.ok
  } catch {
    return false
  }
}

export async function exportAuditCSV(): Promise<void> {
  const res = await fetch(`${API_BASE}/admin/audit/export`, { credentials: 'include' })
  if (!res.ok) throw new ApiError('Export failed')
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'audit_log.csv'
  a.click()
  URL.revokeObjectURL(url)
}
