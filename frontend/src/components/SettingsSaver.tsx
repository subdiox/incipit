import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useI18n } from '@/i18n'
import { useLatest } from '@/lib/hooks'
import { Spinner } from './Spinner'

export interface SaveResult {
  ok: boolean
  label: string
  error?: string
}
type SaveFn = () => Promise<SaveResult>

interface Ctx {
  register: (id: string, fn: SaveFn) => void
  unregister: (id: string) => void
}
const SettingsCtx = createContext<Ctx | null>(null)

/**
 * Registers a section's save function with the surrounding <SettingsContainer>,
 * so its single "Save settings" button persists every section at once. The
 * callback should be a no-op (returning { ok: true }) when nothing changed.
 */
// eslint-disable-next-line react-refresh/only-export-components
export function useRegisterSave(id: string, fn: SaveFn) {
  const ctx = useContext(SettingsCtx)
  const latest = useLatest(fn)
  useEffect(() => {
    if (!ctx) return
    ctx.register(id, () => latest.current())
    return () => ctx.unregister(id)
  }, [ctx, id, latest])
}

export function SettingsContainer({
  children,
  showSaveBar = true,
}: {
  children: ReactNode
  showSaveBar?: boolean
}) {
  const { t } = useI18n()
  const registry = useRef(new Map<string, SaveFn>())
  const [saving, setSaving] = useState(false)
  const [results, setResults] = useState<SaveResult[] | null>(null)

  const register = useCallback((id: string, fn: SaveFn) => {
    registry.current.set(id, fn)
  }, [])
  const unregister = useCallback((id: string) => {
    registry.current.delete(id)
  }, [])

  const onSave = useCallback(async () => {
    setSaving(true)
    setResults(null)
    const fns = [...registry.current.values()]
    const res = await Promise.all(
      fns.map((f) =>
        f().catch((e): SaveResult => ({ ok: false, label: '', error: String(e) })),
      ),
    )
    setResults(res)
    setSaving(false)
  }, [])

  const failed = results?.filter((r) => !r.ok) ?? []
  const allOk = !!results && failed.length === 0

  return (
    <SettingsCtx.Provider value={{ register, unregister }}>
      {children}
      <div
        className={`sticky bottom-0 z-10 mt-6 flex flex-wrap items-center justify-end gap-3 border-t border-ink-800 bg-ink-950/85 py-4 backdrop-blur ${
          showSaveBar ? '' : 'hidden'
        }`}
      >
        {allOk && <span className="text-sm text-emerald-300">{t('settings.saved')}</span>}
        {failed.length > 0 && (
          <span className="text-sm text-red-300">
            {failed
              .map((r) => (r.label ? `${r.label}: ${r.error}` : r.error))
              .join('  /  ')}
          </span>
        )}
        <button type="button" className="btn-primary" onClick={onSave} disabled={saving}>
          {saving && <Spinner className="h-4 w-4" />}
          {t('settings.save')}
        </button>
      </div>
    </SettingsCtx.Provider>
  )
}
