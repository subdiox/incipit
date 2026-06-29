import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useAuth } from '@/auth/AuthContext'
import { api } from '@/lib/api'
import { setFormatLocale } from '@/lib/format'
import { en, type TranslationKey } from './en'
import { ja } from './ja'

export type Lang = 'en' | 'ja'

export const LANGUAGES: { value: Lang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
]

const STORAGE_KEY = 'incipit.lang'
const localeMap: Record<Lang, string> = { en: 'en-US', ja: 'ja-JP' }

function isLang(v: unknown): v is Lang {
  return v === 'en' || v === 'ja'
}

function detectInitial(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (isLang(saved)) return saved
  } catch {
    // ignore unavailable storage
  }
  if (typeof navigator !== 'undefined' && navigator.language?.toLowerCase().startsWith('ja')) {
    return 'ja'
  }
  return 'en'
}

interface I18nValue {
  lang: Lang
  setLang: (l: Lang) => void
  t: (key: TranslationKey, vars?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nValue | undefined>(undefined)

export function I18nProvider({ children }: { children: ReactNode }) {
  const { user, setUser } = useAuth()
  const [lang, setLangState] = useState<Lang>(detectInitial)

  // Apply the chosen language to date/number formatting + <html lang> and
  // remember it locally so it survives reloads and the login screen.
  useEffect(() => {
    setFormatLocale(localeMap[lang])
    try {
      localStorage.setItem(STORAGE_KEY, lang)
    } catch {
      // ignore unavailable storage
    }
    if (typeof document !== 'undefined') document.documentElement.lang = lang
  }, [lang])

  // Adopt the signed-in user's saved preference (e.g. right after login).
  useEffect(() => {
    if (user && isLang(user.language) && user.language !== lang) {
      setLangState(user.language)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.language])

  const setLang = useCallback(
    (l: Lang) => {
      setLangState(l)
      if (user && user.language !== l) {
        // Persist per-account; keep the local change even if the request fails.
        api
          .setLanguage(l)
          .then((updated) => setUser(updated))
          .catch(() => {})
      }
    },
    [user, setUser],
  )

  const t = useCallback(
    (key: TranslationKey, vars?: Record<string, string | number>) => {
      const dict = lang === 'ja' ? ja : en
      let s: string = dict[key] ?? en[key] ?? key
      if (vars) {
        for (const [k, v] of Object.entries(vars)) {
          s = s.replace(new RegExp(`{{\\s*${k}\\s*}}`, 'g'), String(v))
        }
      }
      return s
    },
    [lang],
  )

  const value = useMemo<I18nValue>(() => ({ lang, setLang, t }), [lang, setLang, t])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within I18nProvider')
  return ctx
}
