import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import type { Shelf, ShelfSeriesCard } from '@/types'
import { BookCard, BookGrid } from '@/components/BookCard'
import { Cover } from '@/components/Cover'
import { Modal } from '@/components/Modal'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconChevronLeft, IconClose, IconHeart, IconPlus, IconShelf, IconTrash } from '@/components/icons'

function CreateShelfModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const [name, setName] = useState('')
  const [isPublic, setIsPublic] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: () => api.createShelf(name.trim(), isPublic),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
      setName('')
      setIsPublic(false)
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('shelves.failedToCreate')),
  })

  return (
    <Modal open={open} onClose={onClose} title={t('shelves.createTitle')}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (name.trim()) mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">{t('shelves.name')}</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
        </div>
        <label className="flex items-center gap-2.5 text-sm text-slate-300">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={(e) => setIsPublic(e.target.checked)}
            className="h-4 w-4 rounded border-ink-600 bg-ink-800 text-accent-500 focus:ring-accent-500/40"
          />
          {t('shelves.makePublic')}
        </label>
        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending || !name.trim()}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.create')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

// SeriesShelfCard renders a whole series as a single "stacked" card that links
// to the series' volumes.
function SeriesShelfCard({
  card,
  canRemove,
  onRemove,
}: {
  card: ShelfSeriesCard
  canRemove: boolean
  onRemove: () => void
}) {
  const { t } = useI18n()
  return (
    <div className="group relative">
      <Link to={`/?series=${card.id}`} className="block">
        {/* Offset layers behind the cover imply a stack of volumes. */}
        <div className="relative">
          <div className="absolute inset-0 translate-x-1.5 translate-y-1.5 rounded-xl bg-ink-800 ring-1 ring-ink-700" aria-hidden />
          <div className="absolute inset-0 translate-x-[3px] translate-y-[3px] rounded-xl bg-ink-850 ring-1 ring-ink-700" aria-hidden />
          <div className="relative overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all group-hover:ring-accent-500/50 group-hover:shadow-glow">
            {card.cover ? (
              <Cover
                bookId={card.cover.id}
                title={card.name}
                hasCover={card.cover.hasCover}
                version={card.cover.lastModified}
                rounded="rounded-none"
                className="transition-transform duration-300 group-hover:scale-[1.03]"
              />
            ) : (
              <div className="flex aspect-[2/3] w-full items-center justify-center bg-ink-800 text-slate-600">
                <IconShelf width={28} height={28} />
              </div>
            )}
            <span className="absolute bottom-1.5 left-1.5 rounded-full bg-black/70 px-2 py-0.5 text-[11px] font-medium text-white backdrop-blur">
              {t('shelves.volumeCount', { count: card.bookCount })}
            </span>
          </div>
        </div>
      </Link>
      <h3
        title={card.name}
        className="mt-2.5 break-words px-0.5 text-sm font-medium text-accentSoft transition-colors group-hover:text-accent-300"
      >
        {card.name}
      </h3>
      {canRemove && (
        <button
          type="button"
          onClick={onRemove}
          className="absolute right-2 top-2 z-10 rounded-lg bg-black/60 p-1.5 text-slate-200 opacity-0 backdrop-blur transition-opacity hover:bg-red-500/80 hover:text-white group-hover:opacity-100"
          aria-label={t('shelves.removeSeries')}
          title={t('shelves.removeSeries')}
        >
          <IconClose width={14} height={14} />
        </button>
      )}
    </div>
  )
}

function ShelfDetail({ shelf, onBack }: { shelf: Shelf; onBack: () => void }) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const { user } = useAuth()
  const owned = shelf.userId === user?.id

  const { data, isLoading } = useQuery({
    queryKey: ['shelf-contents', shelf.id],
    queryFn: () => api.shelfContents(shelf.id),
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['shelf-contents', shelf.id] })
    queryClient.invalidateQueries({ queryKey: ['shelves'] })
  }
  const removeBook = useMutation({
    mutationFn: (bookId: number) => api.removeFromShelf(shelf.id, bookId),
    onSuccess: invalidate,
  })
  const removeSeries = useMutation({
    mutationFn: (seriesId: number) => api.removeSeriesFromShelf(shelf.id, seriesId),
    onSuccess: invalidate,
  })
  const visibility = useMutation({
    mutationFn: (isPublic: boolean) => api.updateShelf(shelf.id, { isPublic }),
    onSuccess: invalidate,
  })

  const series = data?.series ?? []
  const books = data?.books ?? []
  const empty = series.length === 0 && books.length === 0

  return (
    <div>
      <button type="button" className="btn-ghost mb-4 -ml-2 inline-flex" onClick={onBack}>
        <IconChevronLeft width={18} height={18} />
        {t('shelves.title')}
      </button>

      <div className="mb-5 flex flex-wrap items-center gap-2">
        <h1 className="text-2xl font-semibold tracking-tight text-white">
          {shelf.isDefault ? t('shelves.favorites') : shelf.name}
        </h1>
        {shelf.isPublic && (
          <span className="rounded-full bg-accent-500/15 px-2.5 py-0.5 text-xs font-medium text-accentSoft">
            {t('shelves.public')}
          </span>
        )}
        {owned && !shelf.isDefault && (
          <button
            type="button"
            onClick={() => visibility.mutate(!shelf.isPublic)}
            disabled={visibility.isPending}
            className="btn-secondary ml-auto px-3 py-1.5 text-xs"
          >
            {visibility.isPending && <Spinner className="h-3.5 w-3.5" />}
            {shelf.isPublic ? t('shelves.makePrivate') : t('shelves.makePublic')}
          </button>
        )}
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : empty ? (
        <div className="card px-6 py-16 text-center text-sm text-slate-500">
          {t('shelves.emptyDetail')}
        </div>
      ) : (
        <BookGrid>
          {series.map((c) => (
            <SeriesShelfCard
              key={`s${c.id}`}
              card={c}
              canRemove={owned}
              onRemove={() => removeSeries.mutate(c.id)}
            />
          ))}
          {books.map((book) => (
            <BookCard
              key={`b${book.id}`}
              book={book}
              action={
                owned ? (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.preventDefault()
                      removeBook.mutate(book.id)
                    }}
                    className="rounded-lg bg-black/60 p-1.5 text-slate-200 opacity-0 backdrop-blur transition-opacity hover:bg-red-500/80 hover:text-white group-hover:opacity-100"
                    aria-label={t('shelves.removeFromShelf')}
                    title={t('shelves.removeFromShelf')}
                  >
                    <IconClose width={14} height={14} />
                  </button>
                ) : undefined
              }
            />
          ))}
        </BookGrid>
      )}
    </div>
  )
}

// ShelfSearchResults searches across every shelf's books by title (the header
// search on the Shelves page), loading each shelf's books and flattening them.
function ShelfSearchResults({ shelves, search }: { shelves: Shelf[]; search: string }) {
  const { t } = useI18n()
  const results = useQueries({
    queries: shelves.map((s) => ({
      queryKey: ['shelf-books', s.id],
      queryFn: () => api.shelfBooks(s.id),
    })),
  })
  if (results.some((r) => r.isLoading)) return <FullPageSpinner />

  const needle = search.toLowerCase()
  const seen = new Set<number>()
  const books = results
    .flatMap((r) => r.data?.books ?? [])
    .filter((b) => {
      if (seen.has(b.id) || !b.title.toLowerCase().includes(needle)) return false
      seen.add(b.id)
      return true
    })

  if (books.length === 0) return <p className="text-sm text-slate-500">{t('library.emptyNoMatch')}</p>
  return (
    <BookGrid>
      {books.map((b) => (
        <BookCard key={b.id} book={b} />
      ))}
    </BookGrid>
  )
}

export function ShelvesPage() {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const [createOpen, setCreateOpen] = useState(false)
  const [selected, setSelected] = useState<Shelf | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Shelf | null>(null)
  const [params] = useSearchParams()
  const search = (params.get('search') ?? '').trim()

  const { data: shelves, isLoading } = useQuery({ queryKey: ['shelves'], queryFn: api.shelves })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteShelf(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
      setConfirmDelete(null)
    },
  })

  // A search takes over the whole page (across all shelves); otherwise a
  // selected shelf shows its own detail view.
  if (selected && !search) {
    // Keep the selected reference fresh from the list when it updates.
    const fresh = shelves?.find((s) => s.id === selected.id) ?? selected
    return <ShelfDetail shelf={fresh} onBack={() => setSelected(null)} />
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-white">{t('shelves.title')}</h1>
          <p className="mt-0.5 text-sm text-slate-500">{t('shelves.subtitle')}</p>
        </div>
        <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">{t('shelves.newShelf')}</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : search ? (
        <ShelfSearchResults shelves={shelves ?? []} search={search} />
      ) : shelves && shelves.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {shelves.map((shelf) => (
            <div
              key={shelf.id}
              className="card group flex cursor-pointer items-center gap-4 p-4 transition-colors hover:border-accent-500/40"
              onClick={() => setSelected(shelf)}
            >
              <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-accent-500/15 text-accentSoft">
                {shelf.isDefault ? <IconHeart width={22} height={22} /> : <IconShelf width={22} height={22} />}
              </span>
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-medium text-white">
                  {shelf.isDefault ? t('shelves.favorites') : shelf.name}
                </h3>
                <p className="text-xs text-slate-500">
                  {(() => {
                    const parts: string[] = []
                    if (shelf.seriesCount > 0)
                      parts.push(t('shelves.seriesCount', { count: shelf.seriesCount }))
                    if (shelf.bookCount > 0)
                      parts.push(
                        t(shelf.bookCount === 1 ? 'common.books_one' : 'common.books_other', {
                          count: shelf.bookCount,
                        }),
                      )
                    if (parts.length === 0) parts.push(t('shelves.emptyShort'))
                    if (shelf.isPublic) parts.push(t('shelves.public'))
                    return parts.join(' · ')
                  })()}
                </p>
              </div>
              {/* The built-in Favorites shelf can't be deleted. */}
              {!shelf.isDefault && (
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    setConfirmDelete(shelf)
                  }}
                  className="rounded-lg p-2 text-slate-500 opacity-0 transition-all hover:bg-red-500/10 hover:text-red-300 group-hover:opacity-100"
                  aria-label={t('shelves.deleteTitle')}
                >
                  <IconTrash width={18} height={18} />
                </button>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="card flex flex-col items-center gap-4 px-6 py-20 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-ink-800 text-slate-500">
            <IconShelf width={28} height={28} />
          </span>
          <div>
            <h2 className="text-lg font-medium text-white">{t('shelves.noneTitle')}</h2>
            <p className="mt-1 text-sm text-slate-500">{t('shelves.noneHint')}</p>
          </div>
          <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
            <IconPlus width={16} height={16} />
            {t('shelves.newShelf')}
          </button>
        </div>
      )}

      <CreateShelfModal open={createOpen} onClose={() => setCreateOpen(false)} />

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title={t('shelves.deleteTitle')}>
        <p className="text-sm text-slate-300">
          {t('shelves.deleteConfirmPrefix')}
          <span className="font-medium text-white">{confirmDelete?.name}</span>
          {t('shelves.deleteConfirmSuffix')}
        </p>
        {deleteMutation.isError && (
          <p className="mt-3 text-sm text-red-300">
            {(deleteMutation.error as Error)?.message ?? t('shelves.failedToDelete')}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(null)}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-danger"
            onClick={() => confirmDelete && deleteMutation.mutate(confirmDelete.id)}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.delete')}
          </button>
        </div>
      </Modal>
    </div>
  )
}
