import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'
import { IconCheck, IconHeart, IconPlus, IconShelf } from './icons'

// AddToShelfMenu adds a book and/or its whole series to a shelf. When both a
// book and a series are given (a volume's detail page) it offers a toggle to
// pick which to add; with only one, it adds that.
export function AddToShelfMenu({
  bookId,
  series,
}: {
  bookId?: number
  series?: { id: number; name: string }
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()
  const [target, setTarget] = useState<'book' | 'series'>(bookId ? 'book' : 'series')

  const { data: shelves, isLoading } = useQuery({
    queryKey: ['shelves'],
    queryFn: api.shelves,
    enabled: open,
  })

  const [justAdded, setJustAdded] = useState<number | null>(null)

  const addMutation = useMutation({
    mutationFn: (shelfId: number) =>
      target === 'series' && series
        ? api.addSeriesToShelf(shelfId, series.id)
        : api.addToShelf(shelfId, bookId!),
    onSuccess: (_data, shelfId) => {
      setJustAdded(shelfId)
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
      queryClient.invalidateQueries({ queryKey: ['shelf-contents', shelfId] })
      setTimeout(() => setJustAdded(null), 1500)
    },
  })

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  return (
    <div className="relative w-full" ref={ref}>
      <button type="button" className="btn-secondary w-full" onClick={() => setOpen((v) => !v)}>
        <IconShelf width={16} height={16} />
        {t('addToShelf.button')}
      </button>
      {open && (
        <div className="absolute inset-x-0 z-20 mt-2 animate-fade-in rounded-xl border border-ink-700 bg-ink-800 p-1.5 shadow-soft">
          {/* Choose whether to add just this volume or the whole series. */}
          {bookId && series && (
            <div className="mb-1.5 flex rounded-lg border border-ink-700 bg-ink-900 p-0.5 text-xs font-medium">
              {(['book', 'series'] as const).map((tgt) => (
                <button
                  key={tgt}
                  type="button"
                  onClick={() => setTarget(tgt)}
                  className={`flex-1 truncate rounded-md px-2 py-1 transition-colors ${
                    target === tgt ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
                  }`}
                >
                  {tgt === 'book' ? t('addToShelf.volume') : t('addToShelf.series')}
                </button>
              ))}
            </div>
          )}
          {isLoading ? (
            <div className="flex justify-center py-4">
              <Spinner className="h-4 w-4 text-accent-400" />
            </div>
          ) : shelves && shelves.length > 0 ? (
            <ul className="max-h-64 overflow-y-auto">
              {shelves.map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    onClick={() => addMutation.mutate(s.id)}
                    disabled={addMutation.isPending}
                    className="flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-ink-700"
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      {s.isDefault && <IconHeart width={14} height={14} className="shrink-0 text-accentSoft" />}
                      <span className="truncate">{s.isDefault ? t('shelves.favorites') : s.name}</span>
                    </span>
                    {justAdded === s.id ? (
                      <IconCheck width={16} height={16} className="text-emerald-400" />
                    ) : (
                      <IconPlus width={16} height={16} className="text-slate-500" />
                    )}
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="px-3 py-3 text-center text-xs text-slate-500">{t('addToShelf.empty')}</p>
          )}
        </div>
      )}
    </div>
  )
}
