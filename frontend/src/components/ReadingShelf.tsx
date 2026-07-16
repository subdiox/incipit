import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { useAuth } from '@/auth/AuthContext'
import { useRecommendationsEnabled } from '@/lib/hooks'
import type { Book, RecommendItem } from '@/types'
import { authorNames } from '@/lib/format'
import { Cover } from './Cover'

// progressPct maps a 0-based page within totalPages to a 0-100 percentage.
export function progressPct(page: number, total: number): number {
  if (!total || total <= 1) return 0
  return Math.min(100, Math.max(0, Math.round((page / (total - 1)) * 100)))
}

// ShelfBook is a fixed-width book tile for horizontal shelves, with an optional
// reading-progress bar.
function ShelfBook({ book, progress }: { book: Book; progress?: number }) {
  const { t } = useI18n()
  return (
    <Link to={`/books/${book.id}`} className="group block w-[120px] shrink-0 sm:w-[136px]">
      <div className="overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all group-hover:ring-accent-500/50">
        <Cover
          bookId={book.id}
          title={book.title}
          hasCover={book.hasCover}
          version={book.lastModified}
          width={300}
          rounded="rounded-none"
        />
      </div>
      {progress != null && (
        <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-ink-700">
          <div className="h-full rounded-full bg-accent-500" style={{ width: `${progress}%` }} />
        </div>
      )}
      <h3 title={book.title} className="mt-1.5 break-words text-xs font-medium text-slate-200">
        {book.title}
      </h3>
      <p className="line-clamp-1 text-[11px] text-slate-500">
        {authorNames(book.authors) || t('common.unknownAuthor')}
      </p>
    </Link>
  )
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
        <ShelfBook key={it.book.id} book={it.book} progress={progressPct(it.page, it.totalPages)} />
      ))}
    </Shelf>
  )
}

// reasonCaption turns a recommendation's reason into a localized "because you
// like …" line, falling back to a generic label when the trait has no name.
function reasonCaption(t: (k: any, v?: any) => string, item: RecommendItem): string {
  if (item.reasonName) {
    if (item.reasonKind === 'author') return t('recommend.reason.author', { name: item.reasonName })
    if (item.reasonKind === 'series') return t('recommend.reason.series', { name: item.reasonName })
    if (item.reasonKind === 'tag') return t('recommend.reason.tag', { name: item.reasonName })
  }
  return t('recommend.reason.generic')
}

// RecommendBook is a shelf tile whose subtitle is the reason it was suggested
// instead of the author, so the row reads as a set of rationales.
function RecommendBook({ item }: { item: RecommendItem }) {
  const { t } = useI18n()
  const book = item.book
  return (
    <Link to={`/books/${book.id}`} className="group block w-[120px] shrink-0 sm:w-[136px]">
      <div className="overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all group-hover:ring-accent-500/50">
        <Cover
          bookId={book.id}
          title={book.title}
          hasCover={book.hasCover}
          version={book.lastModified}
          width={300}
          rounded="rounded-none"
        />
      </div>
      <h3 title={book.title} className="mt-1.5 break-words text-xs font-medium text-slate-200">
        {book.title}
      </h3>
      <p title={reasonCaption(t, item)} className="line-clamp-1 text-[11px] text-accentSoft">
        {reasonCaption(t, item)}
      </p>
    </Link>
  )
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
  })
  if (!show || !data || data.length === 0) return null
  return (
    <Shelf title={t('home.recommended')}>
      {data.map((it) => (
        <RecommendBook key={it.book.id} item={it} />
      ))}
    </Shelf>
  )
}

