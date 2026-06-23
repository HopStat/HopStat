import { normalizeVersion } from '@/lib/wait-for-health'

const STORAGE_KEY = 'hopstat_admin_last_seen_version'

function parseParts(version: string): [number, number, number] {
  const parts = normalizeVersion(version).split('.')
  return [Number(parts[0] || 0), Number(parts[1] || 0), Number(parts[2] || 0)]
}

export function compareVersions(a: string, b: string): number {
  const left = parseParts(a)
  const right = parseParts(b)
  for (let i = 0; i < 3; i++) {
    if (left[i] > right[i]) return 1
    if (left[i] < right[i]) return -1
  }
  return 0
}

export function getLastSeenVersion(): string {
  try {
    return localStorage.getItem(STORAGE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function setLastSeenVersion(version: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, version.trim())
  } catch {
    // ignore quota / private mode
  }
}

export function shouldShowPostUpdateWhatsNew(current: string): boolean {
  const lastSeen = getLastSeenVersion()
  if (!lastSeen) return false
  return compareVersions(current, lastSeen) > 0
}
