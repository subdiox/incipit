import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { IconCheck, IconChevronDown, IconHeart, IconShelf } from './icons'

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
  const addedCount = otherShelves.filter((s) => memberOf(effTarget).includes(s.id)).length

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
    <div className="w-full space-y-2">
      {/* Target selector — applies to the heart and the shelf list alike, so the
          whole series can be favorited/shelved, not just this volume. */}
      {canPickTarget && (
        <div className="grid grid-cols-2 gap-1 rounded-xl bg-ink-900/70 p-1 ring-1 ring-inset ring-ink-700">
          {(['book', 'series'] as const).map((tgt) => (
            <button
              key={tgt}
              type="button"
              onClick={() => setTarget(tgt)}
              aria-pressed={target === tgt}
              className={`truncate rounded-lg px-3 py-1.5 text-sm font-medium transition-all duration-150 ${
                target === tgt
                  ? 'bg-ink-700 text-white shadow-soft'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              {tgt === 'book' ? t('addToShelf.volume') : t('addToShelf.series')}
            </button>
          ))}
        </div>
      )}

      <div className="flex w-full items-stretch gap-2">
        {/* Favorite toggle — fills with the accent heart when active so the state
            reads at a glance. Acts on the selected target. */}
        {favShelf && (
          <button
            type="button"
            onClick={() => toggle.mutate({ shelfId: favShelf.id, tgt: effTarget, active: favActive })}
            disabled={pending}
            aria-pressed={favActive}
            title={favActive ? t('addToShelf.unfavorite') : t('addToShelf.favorite')}
            className={`btn shrink-0 border transition-all ${
              favActive
                ? 'border-accent-500/60 bg-accent-500/15 text-accentSoft'
                : 'border-ink-600 bg-ink-800 text-slate-300 hover:border-ink-700 hover:bg-ink-700'
            }`}
          >
            <IconHeart
              width={18}
              height={18}
              fill={favActive ? 'currentColor' : 'none'}
              className={`transition-transform ${favActive ? 'scale-110 text-accent-400' : 'text-slate-400'}`}
            />
            <span className="hidden sm:inline">{t('shelves.favorites')}</span>
          </button>
        )}

        {/* Other shelves. */}
        <div className="relative min-w-0 flex-1" ref={ref}>
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            className={`btn w-full justify-between border transition-all ${
              open
                ? 'border-accent-500/50 bg-ink-700 text-white'
                : 'border-ink-600 bg-ink-800 text-slate-200 hover:border-ink-700 hover:bg-ink-700'
            }`}
          >
            <span className="flex min-w-0 items-center gap-2">
              <IconShelf width={18} height={18} className="shrink-0 text-slate-400" />
              <span className="truncate">{t('addToShelf.button')}</span>
            </span>
            <span className="flex shrink-0 items-center gap-1.5">
              {addedCount > 0 && (
                <span className="rounded-full bg-accent-500/20 px-1.5 py-0.5 text-[11px] font-semibold leading-none text-accentSoft">
                  {addedCount}
                </span>
              )}
              <IconChevronDown
                width={16}
                height={16}
                className={`text-slate-500 transition-transform duration-200 ${open ? 'rotate-180' : ''}`}
              />
            </span>
          </button>
          {open && (
            <div className="absolute inset-x-0 z-20 mt-2 origin-top animate-fade-in overflow-hidden rounded-2xl border border-ink-700 bg-ink-850 shadow-soft">
              {otherShelves.length > 0 ? (
                <ul className="max-h-72 overflow-y-auto p-1.5">
                  {otherShelves.map((s) => {
                    const active = memberOf(effTarget).includes(s.id)
                    return (
                      <li key={s.id}>
                        <button
                          type="button"
                          onClick={() => toggle.mutate({ shelfId: s.id, tgt: effTarget, active })}
                          disabled={pending}
                          className={`group flex w-full items-center gap-3 rounded-xl px-2.5 py-2 text-left text-sm transition-colors ${
                            active ? 'bg-accent-500/10 text-white' : 'text-slate-200 hover:bg-ink-800'
                          }`}
                        >
                          <span
                            className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors ${
                              active
                                ? 'bg-accent-500/20 text-accent-300'
                                : 'bg-ink-800 text-slate-400 group-hover:bg-ink-700 group-hover:text-slate-300'
                            }`}
                          >
                            <IconShelf width={16} height={16} />
                          </span>
                          <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
                          {active ? (
                            <IconCheck width={18} height={18} className="shrink-0 text-accent-400" />
                          ) : (
                            <span className="shrink-0 text-xs font-medium text-slate-500 group-hover:text-slate-400">
                              {t('addToShelf.add')}
                            </span>
                          )}
                        </button>
                      </li>
                    )
                  })}
                </ul>
              ) : (
                <p className="px-4 py-5 text-center text-xs text-slate-500">{t('addToShelf.empty')}</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
