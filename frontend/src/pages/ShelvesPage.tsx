import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import type { Shelf } from '@/types'
import { BookCard, BookGrid } from '@/components/BookCard'
import { Modal } from '@/components/Modal'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconChevronLeft, IconClose, IconPlus, IconShelf, IconTrash } from '@/components/icons'

function CreateShelfModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
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
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Failed to create shelf.'),
  })

  return (
    <Modal open={open} onClose={onClose} title="Create shelf">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (name.trim()) mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">Name</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
        </div>
        <label className="flex items-center gap-2.5 text-sm text-slate-300">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={(e) => setIsPublic(e.target.checked)}
            className="h-4 w-4 rounded border-ink-600 bg-ink-800 text-accent-500 focus:ring-accent-500/40"
          />
          Make this shelf public
        </label>
        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending || !name.trim()}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            Create
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ShelfDetail({ shelf, onBack }: { shelf: Shelf; onBack: () => void }) {
  const queryClient = useQueryClient()
  const { data, isLoading } = useQuery({
    queryKey: ['shelf-books', shelf.id],
    queryFn: () => api.shelfBooks(shelf.id),
  })

  const removeMutation = useMutation({
    mutationFn: (bookId: number) => api.removeFromShelf(shelf.id, bookId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shelf-books', shelf.id] })
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
    },
  })

  return (
    <div>
      <button type="button" className="btn-ghost mb-4 -ml-2 inline-flex" onClick={onBack}>
        <IconChevronLeft width={18} height={18} />
        Shelves
      </button>

      <div className="mb-5 flex items-center gap-2">
        <h1 className="text-2xl font-semibold tracking-tight text-white">{shelf.name}</h1>
        {shelf.isPublic && (
          <span className="rounded-full bg-accent-500/15 px-2.5 py-0.5 text-xs font-medium text-accent-200">
            Public
          </span>
        )}
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : data && data.books.length > 0 ? (
        <BookGrid>
          {data.books.map((book) => (
            <BookCard
              key={book.id}
              book={book}
              action={
                <button
                  type="button"
                  onClick={(e) => {
                    e.preventDefault()
                    removeMutation.mutate(book.id)
                  }}
                  className="rounded-lg bg-black/60 p-1.5 text-slate-200 opacity-0 backdrop-blur transition-opacity hover:bg-red-500/80 hover:text-white group-hover:opacity-100"
                  aria-label="Remove from shelf"
                  title="Remove from shelf"
                >
                  <IconClose width={14} height={14} />
                </button>
              }
            />
          ))}
        </BookGrid>
      ) : (
        <div className="card px-6 py-16 text-center text-sm text-slate-500">
          This shelf is empty. Add books from their detail page.
        </div>
      )}
    </div>
  )
}

export function ShelvesPage() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [selected, setSelected] = useState<Shelf | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Shelf | null>(null)

  const { data: shelves, isLoading } = useQuery({ queryKey: ['shelves'], queryFn: api.shelves })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteShelf(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
      setConfirmDelete(null)
    },
  })

  if (selected) {
    // Keep the selected reference fresh from the list when it updates.
    const fresh = shelves?.find((s) => s.id === selected.id) ?? selected
    return <ShelfDetail shelf={fresh} onBack={() => setSelected(null)} />
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-white">Shelves</h1>
          <p className="mt-0.5 text-sm text-slate-500">Organize books into collections.</p>
        </div>
        <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">New shelf</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : shelves && shelves.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {shelves.map((shelf) => (
            <div
              key={shelf.id}
              className="card group flex cursor-pointer items-center gap-4 p-4 transition-colors hover:border-accent-500/40"
              onClick={() => setSelected(shelf)}
            >
              <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-accent-500/15 text-accent-300">
                <IconShelf width={22} height={22} />
              </span>
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-medium text-white">{shelf.name}</h3>
                <p className="text-xs text-slate-500">
                  {shelf.bookCount} {shelf.bookCount === 1 ? 'book' : 'books'}
                  {shelf.isPublic ? ' · Public' : ''}
                </p>
              </div>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  setConfirmDelete(shelf)
                }}
                className="rounded-lg p-2 text-slate-500 opacity-0 transition-all hover:bg-red-500/10 hover:text-red-300 group-hover:opacity-100"
                aria-label="Delete shelf"
              >
                <IconTrash width={18} height={18} />
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="card flex flex-col items-center gap-4 px-6 py-20 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-ink-800 text-slate-500">
            <IconShelf width={28} height={28} />
          </span>
          <div>
            <h2 className="text-lg font-medium text-white">No shelves yet</h2>
            <p className="mt-1 text-sm text-slate-500">Create a shelf to start grouping your comics.</p>
          </div>
          <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
            <IconPlus width={16} height={16} />
            New shelf
          </button>
        </div>
      )}

      <CreateShelfModal open={createOpen} onClose={() => setCreateOpen(false)} />

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title="Delete shelf">
        <p className="text-sm text-slate-300">
          Delete <span className="font-medium text-white">{confirmDelete?.name}</span>? The books themselves
          won't be removed from your library.
        </p>
        {deleteMutation.isError && (
          <p className="mt-3 text-sm text-red-300">
            {(deleteMutation.error as Error)?.message ?? 'Failed to delete shelf.'}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(null)}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-danger"
            onClick={() => confirmDelete && deleteMutation.mutate(confirmDelete.id)}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending && <Spinner className="h-4 w-4" />}
            Delete
          </button>
        </div>
      </Modal>
    </div>
  )
}
