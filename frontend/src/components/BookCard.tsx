import { Link } from 'react-router-dom'
import type { Book } from '@/types'
import { authorNames } from '@/lib/format'
import { useI18n } from '@/i18n'
import { Cover } from './Cover'

interface BookCardProps {
  book: Book
  action?: React.ReactNode
}

export function BookCard({ book, action }: BookCardProps) {
  const { t } = useI18n()
  return (
    <div className="group relative">
      <Link to={`/books/${book.id}`} className="block">
        <div className="overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all duration-200 group-hover:ring-accent-500/50 group-hover:shadow-glow">
          <Cover
            bookId={book.id}
            title={book.title}
            hasCover={book.hasCover}
            version={book.lastModified}
            rounded="rounded-none"
            className="transition-transform duration-300 group-hover:scale-[1.03]"
          />
        </div>
        <div className="mt-2.5 px-0.5">
          <h3 className="line-clamp-1 text-sm font-medium text-slate-100 transition-colors group-hover:text-white">
            {book.title}
          </h3>
          <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
            {authorNames(book.authors) || t('common.unknownAuthor')}
          </p>
        </div>
      </Link>
      {/* Series name links to the series listing (not the volume). Kept outside
          the book link — the title already carries the volume number. */}
      {book.series && (
        <Link
          to={`/?series=${book.series.id}`}
          className="mt-0.5 line-clamp-1 block px-0.5 text-[11px] text-accentSoft/80 hover:text-accentSoft hover:underline"
        >
          {book.series.name}
        </Link>
      )}
      {action && <div className="absolute right-2 top-2 z-10">{action}</div>}
    </div>
  )
}

export function BookGrid({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
      {children}
    </div>
  )
}

export function BookCardSkeleton() {
  return (
    <div>
      <div className="skeleton aspect-[2/3] w-full rounded-xl" />
      <div className="mt-2.5 space-y-1.5 px-0.5">
        <div className="skeleton h-3.5 w-4/5 rounded" />
        <div className="skeleton h-3 w-3/5 rounded" />
      </div>
    </div>
  )
}
