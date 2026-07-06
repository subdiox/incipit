import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { IconCheck, IconHeart, IconShelf } from './icons'

type Target = 'book' | 'series'

// AddToShelfMenu offers a dedicated Favorite (heart) toggle plus a dropdown of
// the user's other shelves. When a book belongs to a series, a segmented control
// picks whether actions apply to this single volume or the whole series — and it
// governs BOTH the heart and the shelf list, so the series (not just the volume)
// can be favorited too. Every entry reflects current membership and toggles
// add/remove on click, so nothing gets "added again".
export function AddToShelfMenu({
  bookId,
  series,
}: {
  bookId?: number
  series?: { id: number; name: string }
}) {
  const { t } = useI18n()
  const { user } = useAuth()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const [target, setTarget] = useState<Target>(bookId ? 'book' : 'series')

  const { data: shelves } = useQuery({ queryKey: ['shelves'], queryFn: api.shelves })
  const membershipKey = ['shelf-membership', bookId ?? 0, series?.id ?? 0]
  const { data: membership } = useQuery({
    queryKey: membershipKey,
    queryFn: () => api.shelfMembership(bookId, series?.id),
    enabled: !!(bookId || series),
  })

  const ownShelves = useMemo(
    () => (shelves ?? []).filter((s) => s.userId === user?.id),
    [shelves, user?.id],
  )
  const favShelf = ownShelves.find((s) => s.isDefault)
  const otherShelves = ownShelves.filter((s) => !s.isDefault)

  // A volume and its series are both actionable only when both exist; otherwise
  // the one present target is forced.
  const canPickTarget = !!bookId && !!series
  const effTarget: Target = canPickTarget ? target : bookId ? 'book' : 'series'

  const memberOf = (tgt: Target) => (tgt === 'series' ? membership?.series : membership?.book) ?? []
  const favActive = !!favShelf && memberOf(effTarget).includes(favShelf.id)

  const toggle = useMutation({
    mutationFn: ({ shelfId, tgt, active }: { shelfId: number; tgt: Target; active: boolean }) => {
      if (tgt === 'series' && series) {
        return active ? api.removeSeriesFromShelf(shelfId, series.id) : api.addSeriesToShelf(shelfId, series.id)
      }
      return active ? api.removeFromShelf(shelfId, bookId!) : api.addToShelf(shelfId, bookId!)
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: membershipKey })
      qc.invalidateQueries({ queryKey: ['shelves'] })
      qc.invalidateQueries({ queryKey: ['shelf-contents', v.shelfId] })
    },
  })
  const pending = toggle.isPending

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  return (
    <div className="w-full space-y-2.5">
      {/* Target selector — applies to the heart and the shelf list alike, so the
          whole series can be favorited/shelved, not just this volume. */}
      {canPickTarget && (
        <div className="flex rounded-xl border border-ink-700 bg-ink-900 p-1 text-sm font-medium">
          {(['book', 'series'] as const).map((tgt) => (
            <button
              key={tgt}
              type="button"
              onClick={() => setTarget(tgt)}
              aria-pressed={target === tgt}
              className={`flex-1 truncate rounded-lg px-3 py-2 transition-colors ${
                target === tgt ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
              }`}
            >
              {tgt === 'book' ? t('addToShelf.volume') : t('addToShelf.series')}
            </button>
          ))}
        </div>
      )}

      <div className="flex w-full items-stretch gap-2">
        {/* Favorite toggle — its own button, acting on the selected target. */}
        {favShelf && (
          <button
            type="button"
            onClick={() => toggle.mutate({ shelfId: favShelf.id, tgt: effTarget, active: favActive })}
            disabled={pending}
            aria-pressed={favActive}
            title={favActive ? t('addToShelf.unfavorite') : t('addToShelf.favorite')}
            className={`btn-secondary shrink-0 px-4 py-2.5 ${favActive ? 'border-accent-500/60 text-accentSoft' : ''}`}
          >
            <IconHeart width={18} height={18} className={favActive ? 'text-accent-400' : 'text-slate-400'} />
            <span className="hidden sm:inline">{t('shelves.favorites')}</span>
          </button>
        )}

        {/* Other shelves. */}
        <div className="relative min-w-0 flex-1" ref={ref}>
          <button type="button" className="btn-secondary w-full py-2.5" onClick={() => setOpen((v) => !v)}>
            <IconShelf width={18} height={18} />
            {t('addToShelf.button')}
          </button>
          {open && (
            <div className="absolute inset-x-0 z-20 mt-2 animate-fade-in rounded-xl border border-ink-700 bg-ink-800 p-2 shadow-soft">
              {otherShelves.length > 0 ? (
                <ul className="max-h-72 space-y-0.5 overflow-y-auto">
                  {otherShelves.map((s) => {
                    const active = memberOf(effTarget).includes(s.id)
                    return (
                      <li key={s.id}>
                        <button
                          type="button"
                          onClick={() => toggle.mutate({ shelfId: s.id, tgt: effTarget, active })}
                          disabled={pending}
                          className={`flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2.5 text-left text-sm transition-colors hover:bg-ink-700 ${
                            active ? 'text-white' : 'text-slate-200'
                          }`}
                        >
                          <span className="truncate">{s.name}</span>
                          {active ? (
                            <span className="flex shrink-0 items-center gap-1 text-xs font-medium text-emerald-400">
                              <IconCheck width={14} height={14} />
                              {t('addToShelf.added')}
                            </span>
                          ) : (
                            <span className="shrink-0 text-xs text-slate-500">{t('addToShelf.add')}</span>
                          )}
                        </button>
                      </li>
                    )
                  })}
                </ul>
              ) : (
                <p className="px-3 py-3 text-center text-xs text-slate-500">{t('addToShelf.empty')}</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
