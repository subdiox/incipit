import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { IconArrowUp, IconFolder } from './icons'

interface Props {
  open: boolean
  initialPath?: string
  onClose: () => void
  onSelect: (path: string) => void
}

// DirectoryPicker browses the server's filesystem so an admin can pick the
// Calibre library folder without typing a path.
export function DirectoryPicker({ open, initialPath, onClose, onSelect }: Props) {
  const { t } = useI18n()
  const [path, setPath] = useState(initialPath ?? '')
  const [newName, setNewName] = useState('')

  const { data, isLoading, isError } = useQuery({
    queryKey: ['fs', path],
    queryFn: () => api.browseFs(path || undefined),
    enabled: open,
  })

  const current = data?.path ?? path
  const choose = (p: string) => {
    onSelect(p)
    setNewName('')
    onClose()
  }

  return (
    <Modal open={open} onClose={onClose} title={t('picker.title')} maxWidth="max-w-lg">
      <div className="space-y-3">
        {/* Current path + up */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => data?.parent && setPath(data.parent)}
            disabled={!data?.parent}
            aria-label={t('picker.up')}
            title={t('picker.up')}
            className="shrink-0 rounded-lg border border-ink-700 bg-ink-800 p-2 text-slate-300 transition-colors hover:bg-ink-700 hover:text-white disabled:opacity-40"
          >
            <IconArrowUp width={16} height={16} />
          </button>
          <code className="min-w-0 flex-1 truncate rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-xs text-slate-300">
            {current || '…'}
          </code>
        </div>

        {/* Folder list */}
        <div className="h-64 overflow-y-auto rounded-xl border border-ink-700 bg-ink-900">
          {isLoading ? (
            <div className="flex h-full items-center justify-center">
              <Spinner className="h-6 w-6 text-accent-400" />
            </div>
          ) : isError ? (
            <p className="p-4 text-center text-sm text-red-300">{t('picker.failed')}</p>
          ) : (data?.entries.length ?? 0) === 0 ? (
            <p className="p-4 text-center text-sm text-slate-500">{t('picker.empty')}</p>
          ) : (
            <ul className="divide-y divide-ink-800">
              {data?.entries.map((e) => (
                <li key={e.path}>
                  <button
                    type="button"
                    onClick={() => setPath(e.path)}
                    className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-ink-800"
                  >
                    <IconFolder width={16} height={16} className="shrink-0 text-accentSoft" />
                    <span className="truncate">{e.name}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* New folder */}
        <div className="flex items-center gap-2">
          <input
            className="input flex-1"
            value={newName}
            onChange={(ev) => setNewName(ev.target.value)}
            placeholder={t('picker.newFolderName')}
          />
          <button
            type="button"
            className="btn-secondary shrink-0"
            disabled={!newName.trim() || !current}
            onClick={() => choose(`${current.replace(/\/+$/, '')}/${newName.trim()}`)}
          >
            {t('picker.createHere')}
          </button>
        </div>
        <p className="text-xs text-slate-500">{t('picker.createHint')}</p>

        {/* Footer */}
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-secondary" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-primary"
            disabled={!current}
            onClick={() => choose(current)}
          >
            {t('picker.selectThis')}
          </button>
        </div>
      </div>
    </Modal>
  )
}
