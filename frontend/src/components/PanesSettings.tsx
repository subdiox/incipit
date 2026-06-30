import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { Pane } from '@/types'
import { Modal } from './Modal'
import { Spinner, FullPageSpinner } from './Spinner'
import { TagPicker } from './TagPicker'
import { IconEdit, IconPlus, IconTrash } from './icons'

function PaneModal({ pane, open, onClose }: { pane: Pane | null; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const { t } = useI18n()
  const [name, setName] = useState(pane?.name ?? '')
  const [tagIds, setTagIds] = useState<number[]>(pane?.tagIds ?? [])
  const [error, setError] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: async () => {
      if (pane) await api.updatePane(pane.id, { name: name.trim(), tagIds, position: pane.position })
      else await api.createPane(name.trim(), tagIds)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['panes'] })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('common.genericError')),
  })

  return (
    <Modal open={open} onClose={onClose} title={pane ? t('panes.editTitle') : t('panes.newTitle')}>
      <div className="space-y-4">
        <div>
          <label className="label">{t('panes.name')}</label>
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('panes.namePlaceholder')}
            autoFocus
          />
        </div>
        <div>
          <label className="label">{t('panes.tags')}</label>
          <p className="mb-2 text-xs text-slate-500">{t('panes.tagsHelp')}</p>
          <TagPicker value={tagIds} onChange={setTagIds} />
        </div>
        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-secondary" onClick={onClose} disabled={save.isPending}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-primary"
            disabled={save.isPending || !name.trim()}
            onClick={() => {
              setError(null)
              save.mutate()
            }}
          >
            {save.isPending && <Spinner className="h-4 w-4" />}
            {t('common.save')}
          </button>
        </div>
      </div>
    </Modal>
  )
}

export function PanesSettings() {
  const qc = useQueryClient()
  const { t } = useI18n()
  const { data: panes, isLoading } = useQuery({ queryKey: ['panes'], queryFn: api.panes })
  const tags = useQuery({ queryKey: ['facets', 'tags'], queryFn: api.tags, staleTime: 300_000 }).data ?? []
  const tagName = useMemo(() => new Map(tags.map((f) => [f.id, f.name])), [tags])

  const [editing, setEditing] = useState<Pane | null>(null)
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<Pane | null>(null)

  const del = useMutation({
    mutationFn: (id: number) => api.deletePane(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['panes'] })
      setConfirmDelete(null)
    },
  })

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-3">
        <p className="text-sm text-slate-500">{t('panes.subtitle')}</p>
        <button type="button" className="btn-primary" onClick={() => setCreating(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">{t('panes.new')}</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : !panes || panes.length === 0 ? (
        <div className="card p-8 text-center text-sm text-slate-500">{t('panes.empty')}</div>
      ) : (
        <div className="card divide-y divide-ink-800">
          {panes.map((p) => (
            <div key={p.id} className="flex items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <p className="font-medium text-slate-100">{p.name}</p>
                <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
                  {p.tagIds.length === 0
                    ? t('panes.noTags')
                    : p.tagIds.map((id) => tagName.get(id) ?? `#${id}`).join(' · ')}
                </p>
              </div>
              <button
                type="button"
                className="btn-secondary shrink-0 px-3"
                onClick={() => setEditing(p)}
                title={t('common.edit')}
              >
                <IconEdit width={16} height={16} />
              </button>
              <button
                type="button"
                className="btn-danger shrink-0 px-3"
                onClick={() => setConfirmDelete(p)}
                title={t('common.delete')}
              >
                <IconTrash width={16} height={16} />
              </button>
            </div>
          ))}
        </div>
      )}

      {creating && <PaneModal pane={null} open={creating} onClose={() => setCreating(false)} />}
      {editing && <PaneModal pane={editing} open={!!editing} onClose={() => setEditing(null)} />}

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title={t('panes.deleteTitle')}>
        <p className="text-sm text-slate-300">
          {t('panes.deleteConfirm', { name: confirmDelete?.name ?? '' })}
        </p>
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(null)}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-danger"
            disabled={del.isPending}
            onClick={() => confirmDelete && del.mutate(confirmDelete.id)}
          >
            {del.isPending && <Spinner className="h-4 w-4" />}
            {t('common.delete')}
          </button>
        </div>
      </Modal>
    </div>
  )
}
