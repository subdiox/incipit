import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { Collection } from '@/types'
import { Modal } from './Modal'
import { Spinner, FullPageSpinner } from './Spinner'
import { TagPicker } from './TagPicker'
import { IconEdit, IconGrip, IconPlus, IconTrash } from './icons'

function CollectionModal({ collection, open, onClose }: { collection: Collection | null; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const { t } = useI18n()
  const [name, setName] = useState(collection?.name ?? '')
  const [tagIds, setTagIds] = useState<number[]>(collection?.tagIds ?? [])
  const [matchAny, setMatchAny] = useState<boolean>(collection?.matchAny ?? false)
  const [error, setError] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: async () => {
      if (collection) await api.updateCollection(collection.id, { name: name.trim(), tagIds, matchAny, position: collection.position })
      else await api.createCollection(name.trim(), tagIds, matchAny)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['collections'] })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('common.genericError')),
  })

  return (
    <Modal open={open} onClose={onClose} title={collection ? t('collections.editTitle') : t('collections.newTitle')}>
      <div className="space-y-4">
        <div>
          <label className="label">{t('collections.name')}</label>
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('collections.namePlaceholder')}
            autoFocus
          />
        </div>
        <div>
          <label className="label">{t('collections.tags')}</label>
          <p className="mb-2 text-xs text-slate-500">{t('collections.tagsHelp')}</p>
          <TagPicker value={tagIds} onChange={setTagIds} />
        </div>
        <div>
          <label className="label">{t('collections.matchMode')}</label>
          <div className="mt-1 inline-flex rounded-xl border border-ink-700 bg-ink-800 p-0.5">
            <button
              type="button"
              onClick={() => setMatchAny(false)}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                !matchAny ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
              }`}
            >
              {t('collections.matchAll')}
            </button>
            <button
              type="button"
              onClick={() => setMatchAny(true)}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                matchAny ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
              }`}
            >
              {t('collections.matchAny')}
            </button>
          </div>
          <p className="mt-1.5 text-xs text-slate-500">
            {matchAny ? t('collections.matchAnyHelp') : t('collections.matchAllHelp')}
          </p>
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

export function CollectionsSettings() {
  const qc = useQueryClient()
  const { t } = useI18n()
  const { data: collections, isLoading } = useQuery({ queryKey: ['collections'], queryFn: api.collections })
  const tags = useQuery({ queryKey: ['facets', 'tags'], queryFn: api.tags, staleTime: 300_000 }).data ?? []
  const tagName = useMemo(() => new Map(tags.map((f) => [f.id, f.name])), [tags])

  const [editing, setEditing] = useState<Collection | null>(null)
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<Collection | null>(null)

  // Local order for drag-and-drop; kept in sync with the server list, and
  // reordered live as the user drags, then persisted on drop.
  const [ordered, setOrdered] = useState<Collection[]>([])
  useEffect(() => {
    if (collections) setOrdered(collections)
  }, [collections])
  const dragIndex = useRef<number | null>(null)

  const del = useMutation({
    mutationFn: (id: number) => api.deleteCollection(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['collections'] })
      setConfirmDelete(null)
    },
  })

  const reorder = useMutation({
    mutationFn: (ids: number[]) => api.reorderCollections(ids),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['collections'] }),
  })

  const onDragEnter = (i: number) => {
    const from = dragIndex.current
    if (from === null || from === i) return
    setOrdered((cur) => {
      const next = [...cur]
      const [moved] = next.splice(from, 1)
      next.splice(i, 0, moved)
      return next
    })
    dragIndex.current = i
  }
  const onDrop = () => {
    dragIndex.current = null
    const ids = ordered.map((p) => p.id)
    if (collections && ids.join(',') !== collections.map((p) => p.id).join(',')) reorder.mutate(ids)
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-3">
        <p className="text-sm text-slate-500">{t('collections.subtitle')}</p>
        <button type="button" className="btn-primary" onClick={() => setCreating(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">{t('collections.new')}</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : !collections || collections.length === 0 ? (
        <div className="card p-8 text-center text-sm text-slate-500">{t('collections.empty')}</div>
      ) : (
        <div className="card divide-y divide-ink-800">
          {ordered.map((p, i) => (
            <div
              key={p.id}
              draggable
              onDragStart={(e) => {
                dragIndex.current = i
                e.dataTransfer.effectAllowed = 'move'
              }}
              onDragEnter={() => onDragEnter(i)}
              onDragOver={(e) => e.preventDefault()}
              onDrop={onDrop}
              onDragEnd={onDrop}
              className={`flex items-center gap-3 p-4 ${
                dragIndex.current === i ? 'opacity-50' : ''
              }`}
            >
              <span
                className="shrink-0 cursor-grab text-slate-600 hover:text-slate-300 active:cursor-grabbing"
                title={t('collections.reorder')}
                aria-hidden
              >
                <IconGrip width={18} height={18} />
              </span>
              <div className="min-w-0 flex-1">
                <p className="font-medium text-slate-100">{p.name}</p>
                <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
                  {p.tagIds.length === 0 ? (
                    t('collections.noTags')
                  ) : (
                    <>
                      {p.matchAny && p.tagIds.length > 1 && (
                        <span className="text-accentSoft/80">{t('collections.anyBadge')} </span>
                      )}
                      {p.tagIds
                        .map((id) => tagName.get(id) ?? `#${id}`)
                        .join(p.matchAny ? ' / ' : ' · ')}
                    </>
                  )}
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

      {creating && <CollectionModal collection={null} open={creating} onClose={() => setCreating(false)} />}
      {editing && <CollectionModal collection={editing} open={!!editing} onClose={() => setEditing(null)} />}

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title={t('collections.deleteTitle')}>
        <p className="text-sm text-slate-300">
          {t('collections.deleteConfirm', { name: confirmDelete?.name ?? '' })}
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
