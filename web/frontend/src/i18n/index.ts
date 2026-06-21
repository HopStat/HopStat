import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './en.json'
import tr from './tr.json'
import ru from './ru.json'
import de from './de.json'
import fr from './fr.json'

export const SUPPORTED_LOCALES = ['tr', 'en', 'ru', 'de', 'fr'] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]

export const LOCALE_LABEL_KEYS: Record<Locale, string> = {
  tr: 'lang.tr',
  en: 'lang.en',
  ru: 'lang.ru',
  de: 'lang.de',
  fr: 'lang.fr',
}

const STORAGE_KEY = 'hopstat-locale'

const resources = {
  en: { translation: en },
  tr: { translation: tr },
  ru: { translation: ru },
  de: { translation: de },
  fr: { translation: fr },
}

function isLocale(value: string): value is Locale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

export function parseActiveLanguages(raw: string | undefined): Locale[] {
  const fallback: Locale[] = ['en', 'tr']
  if (!raw?.trim()) return fallback
  const parsed = raw.split(',').map(s => s.trim().toLowerCase()).filter(isLocale)
  return parsed.length > 0 ? parsed : fallback
}

function browserLanguages(): string[] {
  if (typeof navigator === 'undefined') return ['en']
  return [...(navigator.languages ?? []), navigator.language]
    .filter(Boolean)
    .map(language => language.toLowerCase())
}

function matchBrowserLocale(active: Locale[]): Locale | null {
  for (const nav of browserLanguages()) {
    for (const locale of active) {
      if (nav === locale || nav.startsWith(`${locale}-`)) return locale
    }
  }
  return null
}

export function detectSystemLocale(active: Locale[]): Locale {
  if (active.length === 0) return 'en'
  const matched = matchBrowserLocale(active)
  if (matched) return matched
  if (active.includes('en')) return 'en'
  return active[0]
}

export function readPersistedLocale(): Locale | null {
  if (typeof window === 'undefined') return null
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && isLocale(saved)) return saved
  } catch {
    // ignore storage errors
  }
  return null
}

/** Saved user choice wins; otherwise browser language if active, else English. */
export function resolveInitialLocale(active: Locale[]): Locale {
  const saved = readPersistedLocale()
  if (saved && active.includes(saved)) return saved
  return detectSystemLocale(active)
}

export function persistLocale(locale: Locale) {
  if (typeof window === 'undefined') return
  localStorage.setItem(STORAGE_KEY, locale)
}

function bootstrapLocale(): Locale {
  const saved = readPersistedLocale()
  if (saved) return saved
  return detectSystemLocale([...SUPPORTED_LOCALES])
}

void i18n.use(initReactI18next).init({
  resources,
  lng: bootstrapLocale(),
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
  react: { useSuspense: false },
})

export default i18n
