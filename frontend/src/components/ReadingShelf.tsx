import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { useAuth } from '@/auth/AuthContext'
import { useRecommendationsEnabled } from '@/lib/hooks'
import type { RecommendItem } from '@/types'
import { BookCard } from './BookCard'

// progressPct maps a 0-based page within totalPages to a 0-100 percentage.
export function progressPct(page: number, total: number): number {
  if (!total || total <= 1) return 0
  return Math.min(100, Math.max(0, Math.round((page / (total - 1)) * 100)))
}

// ShelfCell fixes a horizontal-shelf tile's width; the tile itself is the shared
// BookCard, so shelves and grids render identical thumbnails (cover, reading-
// progress bar, hover) with no bespoke variants.
function ShelfCell({ children }: { children: React.ReactNode }) {
  return <div className="w-[120px] shrink-0 sm:w-[136px]">{children}</div>
}

function Shelf({ title, to, children }: { title: string; to?: string; children: React.ReactNode }) {
  const { t } = useI18n()
  return (
    <section className="mb-8">
      <div className="mb-3 flex items-baseline justify-between gap-3">
        <h2 className="text-base font-semibold text-white">{title}</h2>
        {to && (
          <Link to={to} className="shrink-0 text-xs font-medium text-accentSoft hover:underline">
            {t('common.seeAll')}
          </Link>
        )}
      </div>
      <div className="-mx-1 flex gap-3 overflow-x-auto px-1 pb-2 [scrollbar-width:thin]">{children}</div>
    </section>
  )
}

// ContinueReadingShelf shows the current user's unfinished books, most recent
// first. Renders nothing when there is nothing in progress or the user has
// turned the shelf off in their account settings.
export function ContinueReadingShelf() {
  const { t } = useI18n()
  const { user } = useAuth()
  const show = user?.showHistory !== false
  const { data } = useQuery({
    queryKey: ['reading', 'continue'],
    queryFn: () => api.myReading('continue', 20),
    enabled: show,
  })
  if (!show || !data || data.length === 0) return null
  return (
    <Shelf title={t('history.continue')} to="/history">
      {data.map((it) => (
        <ShelfCell key={it.book.id}>
          <BookCard book={it.book} />
        </ShelfCell>
      ))}
    </Shelf>
  )
}

// reasonCaption turns a recommendation's reason into a localized "because you
// like …" line, falling back to a generic label when the trait has no name.
export function reasonCaption(t: (k: any, v?: any) => string, item: RecommendItem): string {
  if (item.reasonName) {
    if (item.reasonKind === 'author') return t('recommend.reason.author', { name: item.reasonName })
    if (item.reasonKind === 'series') return t('recommend.reason.series', { name: item.reasonName })
    if (item.reasonKind === 'tag') return t('recommend.reason.tag', { name: item.reasonName })
  }
  return t('recommend.reason.generic')
}

// RecommendedShelf shows content-based suggestions built from the user's own
// favorites and reading history. Renders nothing when the feature is disabled,
// the user hid it, or there's not enough activity to suggest from.
export function RecommendedShelf() {
  const { t } = useI18n()
  const { user } = useAuth()
  const enabled = useRecommendationsEnabled()
  const show = enabled && user?.showRecommended !== false
  const { data } = useQuery({
    queryKey: ['recommendations'],
    queryFn: () => api.recommendations(24),
    enabled: show,
    // Always refetch when a recommendations view mounts: the server recomputes
    // when the user's favorites or reading history changed, so a book just read
    // drops off the row on the next visit instead of lingering for staleTime.
    staleTime: 0,
    refetchOnMount: 'always',
  })
  if (!show || !data || data.length === 0) return null
  return (
    <Shelf title={t('home.recommended')} to="/recommendations">
      {data.map((it) => (
        <ShelfCell key={it.book.id}>
          <BookCard book={it.book} subtitle={<span className="text-accentSoft">{reasonCaption(t, it)}</span>} />
        </ShelfCell>
      ))}
    </Shelf>
  )
}

