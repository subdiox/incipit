import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'

export type ThemeMode = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'incipit.theme'
const prefersDark = () =>
  typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches

function isDark(mode: ThemeMode): boolean {
  return mode === 'dark' || (mode === 'system' && prefersDark())
}

function apply(mode: ThemeMode) {
  document.documentElement.classList.toggle('dark', isDark(mode))
}

interface ThemeValue {
  mode: ThemeMode
  setMode: (m: ThemeMode) => void
}
const ThemeCtx = createContext<ThemeValue | undefined>(undefined)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => {
    try {
      const v = localStorage.getItem(STORAGE_KEY)
      if (v === 'light' || v === 'dark' || v === 'system') return v
    } catch {
      // ignore
    }
    return 'system'
  })

  useEffect(() => {
    apply(mode)
  }, [mode])

  // Follow the OS when in "system" mode.
  useEffect(() => {
    if (mode !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => apply('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [mode])

  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m)
    try {
      localStorage.setItem(STORAGE_KEY, m)
    } catch {
      // ignore
    }
  }, [])

  return <ThemeCtx.Provider value={{ mode, setMode }}>{children}</ThemeCtx.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme(): ThemeValue {
  const ctx = useContext(ThemeCtx)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}

export const THEME_OPTIONS: { value: ThemeMode; labelKey: 'theme.system' | 'theme.light' | 'theme.dark' }[] = [
  { value: 'system', labelKey: 'theme.system' },
  { value: 'light', labelKey: 'theme.light' },
  { value: 'dark', labelKey: 'theme.dark' },
]
