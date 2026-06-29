import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'
import { useRegisterSave } from './SettingsSaver'

export function ServerSettings() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['site'], queryFn: api.site })

  const [title, setTitle] = useState('')

  useEffect(() => {
    if (data) setTitle(data.title)
  }, [data])

  useRegisterSave('server', async () => {
    const label = t('server.title')
    const trimmed = title.trim()
    if (!trimmed) return { ok: false, label, error: t('server.titleRequired') }
    if (data && trimmed === data.title) return { ok: true, label } // unchanged
    try {
      const next = await api.updateSite(trimmed)
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
      )}
    </div>
  )
}
