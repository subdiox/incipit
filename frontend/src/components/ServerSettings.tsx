import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'
import { useRegisterSave } from './SettingsSaver'

export function ServerSettings() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['site'], queryFn: api.site })

  // Page-index progress: poll while a scan is running (once the filter is saved
  // on). refetchInterval stops when idle to avoid needless requests.
  const index = useQuery({
    queryKey: ['pageindex'],
    queryFn: api.pageIndexStatus,
    enabled: !!data?.pageFilter,
    refetchInterval: (q) => (q.state.data?.running ? 1500 : false),
  })
  const rescan = useMutation({
    mutationFn: api.reindexPages,
    onSuccess: (s) => {
      qc.setQueryData(['pageindex'], s)
      qc.invalidateQueries({ queryKey: ['pageindex'] })
    },
  })

  const [title, setTitle] = useState('')
  const [pageFilter, setPageFilter] = useState(false)

  useEffect(() => {
    if (data) {
      setTitle(data.title)
      setPageFilter(data.pageFilter)
    }
  }, [data])

  useRegisterSave('server', async () => {
    const label = t('server.title')
    const trimmed = title.trim()
    if (!trimmed) return { ok: false, label, error: t('server.titleRequired') }
    if (data && trimmed === data.title && pageFilter === data.pageFilter) return { ok: true, label }
    try {
      const next = await api.updateSite({ title: trimmed, pageFilter })
      // Shared query key: sidebar, login screen and tab title update at once.
      qc.setQueryData(['site'], next)
      return { ok: true, label }
    } catch (e) {
      return { ok: false, label, error: e instanceof ApiError ? e.message : t('server.failedToSave') }
    }
  })

  return (
    <div className="card p-5 sm:p-6">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-white">{t('server.title')}</h2>
        <p className="mt-0.5 text-sm text-slate-500">{t('server.subtitle')}</p>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner className="h-6 w-6 text-accent-400" />
        </div>
      ) : (
        <div className="space-y-5">
          <div>
            <label className="label">{t('server.siteTitle')}</label>
            <input
              className="input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              maxLength={80}
              placeholder="Incipit"
            />
            <p className="mt-1 text-xs text-slate-500">{t('server.siteTitleHelp')}</p>
          </div>

          <label className="flex cursor-pointer items-start gap-3 border-t border-ink-700 pt-5">
            <input
              type="checkbox"
              className="mt-0.5 h-4 w-4 accent-accent-500"
              checked={pageFilter}
              onChange={(e) => setPageFilter(e.target.checked)}
            />
            <span>
              <span className="text-sm font-medium text-slate-200">{t('server.pageFilter')}</span>
              <span className="mt-0.5 block text-xs text-slate-500">{t('server.pageFilterHelp')}</span>
            </span>
          </label>

          {/* Page-count index progress (only meaningful once the filter is saved on). */}
          {data?.pageFilter && index.data && (
            <div className="rounded-xl border border-ink-700 bg-ink-900 p-3">
              {(() => {
                const { running, done, total } = index.data
                const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0
                const complete = !running && total > 0 && done >= total
                return (
                  <>
                    <div className="mb-2 flex items-center justify-between gap-3">
                      <span className="text-xs font-medium text-slate-300">
                        {running
                          ? t('server.indexRunning')
                          : complete
                            ? t('server.indexDone')
                            : t('server.indexIdle')}
                      </span>
                      <span className="flex items-center gap-2">
                        <span className="text-xs tabular-nums text-slate-500">
                          {done.toLocaleString()} / {total.toLocaleString()}
                          {total > 0 ? ` · ${pct}%` : ''}
                        </span>
                        <button
                          type="button"
                          className="btn-secondary px-2 py-1 text-xs"
                          onClick={() => rescan.mutate()}
                          disabled={running || rescan.isPending}
                        >
                          {(running || rescan.isPending) && <Spinner className="h-3.5 w-3.5" />}
                          {t('server.indexRescan')}
                        </button>
                      </span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-ink-700">
                      <div
                        className={`h-full rounded-full ${complete ? 'bg-emerald-500' : 'bg-accent-500'} transition-[width] duration-500`}
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </>
                )
              })()}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
