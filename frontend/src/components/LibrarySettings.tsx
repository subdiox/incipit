import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'
import { DirectoryPicker } from './DirectoryPicker'
import { IconFolder } from './icons'
import { useRegisterSave } from './SettingsSaver'

export function LibrarySettings() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['admin-library'], queryFn: api.library })

  const [path, setPath] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)

  useEffect(() => {
    if (data) setPath(data.path)
  }, [data])

  useRegisterSave('library', async () => {
    const label = t('library.cfgTitle')
    const trimmed = path.trim()
    if (!trimmed) return { ok: false, label, error: t('login.libraryPathRequired') }
    if (data && trimmed === data.path) return { ok: true, label } // unchanged
    try {
      const next = await api.updateLibrary(trimmed)
      qc.setQueryData(['admin-library'], next)
      // The library changed underneath us: refresh anything derived from it.
      qc.invalidateQueries({ queryKey: ['books'] })
      qc.invalidateQueries({ queryKey: ['facets'] })
      qc.invalidateQueries({ queryKey: ['stats'] })
      return { ok: true, label }
    } catch (e) {
      return { ok: false, label, error: e instanceof ApiError ? e.message : t('library.failedToSaveCfg') }
    }
  })

  return (
    <div className="card p-5 sm:p-6">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-white">{t('library.cfgTitle')}</h2>
        <p className="mt-0.5 text-sm text-slate-500">{t('library.cfgSubtitle')}</p>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner className="h-6 w-6 text-accent-400" />
        </div>
      ) : (
        <div className="space-y-4">
          <div>
            <label className="label">{t('library.path')}</label>
            <div className="flex gap-2">
              <input
                className="input flex-1"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/library"
              />
              <button
                type="button"
                className="btn-secondary shrink-0"
                onClick={() => setPickerOpen(true)}
              >
                <IconFolder width={16} height={16} />
                {t('picker.browse')}
              </button>
            </div>
            <p className="mt-1 text-xs text-slate-500">{t('library.pathHelp')}</p>
          </div>

          <DirectoryPicker
            open={pickerOpen}
            initialPath={path}
            onClose={() => setPickerOpen(false)}
            onSelect={(p) => setPath(p)}
          />

          {data?.readOnly && (
            <p className="rounded-xl border border-ink-700 bg-ink-900 px-3.5 py-2.5 text-xs text-slate-400">
              {t('library.readOnlyNote')}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
