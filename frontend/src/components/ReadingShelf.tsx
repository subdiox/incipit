import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { Book } from '@/types'
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
// first. Renders nothing when there is nothing in progress.
export function ContinueReadingShelf() {
  const { t } = useI18n()
  const { data } = useQuery({
    queryKey: ['reading', 'continue'],
    queryFn: () => api.myReading('continue', 20),
  })
  if (!data || data.length === 0) return null
  return (
    <Shelf title={t('history.continue')} to="/history">
      {data.map((it) => (
        <ShelfBook key={it.book.id} book={it.book} progress={progressPct(it.page, it.totalPages)} />
      ))}
    </Shelf>
  )
}

