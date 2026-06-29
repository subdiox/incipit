import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from './Spinner'
import { IconCheck, IconPlus, IconShelf } from './icons'

export function AddToShelfMenu({ bookId }: { bookId: number }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()

  const { data: shelves, isLoading } = useQuery({
    queryKey: ['shelves'],
    queryFn: api.shelves,
    enabled: open,
  })

  const [justAdded, setJustAdded] = useState<number | null>(null)

  const addMutation = useMutation({
    mutationFn: (shelfId: number) => api.addToShelf(shelfId, bookId),
    onSuccess: (_data, shelfId) => {
      setJustAdded(shelfId)
      queryClient.invalidateQueries({ queryKey: ['shelves'] })
      queryClient.invalidateQueries({ queryKey: ['shelf-books', shelfId] })
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
    <div className="relative" ref={ref}>
      <button type="button" className="btn-secondary" onClick={() => setOpen((v) => !v)}>
        <IconShelf width={16} height={16} />
        {t('addToShelf.button')}
      </button>
      {open && (
        <div className="absolute right-0 z-20 mt-2 w-60 animate-fade-in rounded-xl border border-ink-700 bg-ink-800 p-1.5 shadow-soft">
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
                    <span className="truncate">{s.name}</span>
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
            <p className="px-3 py-3 text-center text-xs text-slate-500">
              {t('addToShelf.empty')}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
