import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'

type Msg = { type: 'success' | 'error'; text: string } | null

export function LibrarySettings() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['admin-library'], queryFn: api.library })

  const [path, setPath] = useState('')
  const [msg, setMsg] = useState<Msg>(null)

  useEffect(() => {
    if (data) setPath(data.path)
  }, [data])

  const saveM = useMutation({
    mutationFn: () => api.updateLibrary(path.trim()),
    onSuccess: (next) => {
      qc.setQueryData(['admin-library'], next)
      // The library changed underneath us: refresh anything derived from it.
      qc.invalidateQueries({ queryKey: ['books'] })
      qc.invalidateQueries({ queryKey: ['facets'] })
      qc.invalidateQueries({ queryKey: ['stats'] })
      setMsg({ type: 'success', text: t('library.saved') })
    },
    onError: (e) =>
      setMsg({ type: 'error', text: e instanceof ApiError ? e.message : t('library.failedToSaveCfg') }),
  })

  return (
    <div className="card mt-8 p-5 sm:p-6">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-white">{t('library.cfgTitle')}</h2>
        <p className="mt-0.5 text-sm text-slate-500">{t('library.cfgSubtitle')}</p>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner className="h-6 w-6 text-accent-400" />
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            setMsg(null)
            if (path.trim()) saveM.mutate()
          }}
          className="space-y-4"
        >
          <div>
            <label className="label">{t('library.path')}</label>
            <input
              className="input"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="/library"
            />
            <p className="mt-1 text-xs text-slate-500">{t('library.pathHelp')}</p>
          </div>

          {data?.readOnly && (
            <p className="rounded-xl border border-ink-700 bg-ink-900 px-3.5 py-2.5 text-xs text-slate-400">
              {t('library.readOnlyNote')}
            </p>
          )}

          {msg && (
            <div
              className={`rounded-xl border px-3.5 py-2.5 text-sm ${
                msg.type === 'success'
                  ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'
                  : 'border-red-500/30 bg-red-500/10 text-red-300'
              }`}
            >
              {msg.text}
            </div>
          )}

          <button type="submit" className="btn-primary" disabled={saveM.isPending || !path.trim()}>
            {saveM.isPending && <Spinner className="h-4 w-4" />}
            {t('library.save')}
          </button>
        </form>
      )}
    </div>
  )
}
