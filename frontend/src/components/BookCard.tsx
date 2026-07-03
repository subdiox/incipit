import { Link } from 'react-router-dom'
import type { Book } from '@/types'
import { authorNames } from '@/lib/format'
import { isReadable } from '@/lib/book'
import { useI18n } from '@/i18n'
import { Cover } from './Cover'

interface BookCardProps {
  book: Book
  action?: React.ReactNode
}

export function BookCard({ book, action }: BookCardProps) {
  const { t } = useI18n()
  const readable = isReadable(book)
  return (
    <div className="group relative">
      {/* The cover starts reading straight away (resuming saved progress) when
          the book is readable; the title below still opens the detail page. */}
      <Link
        to={readable ? `/books/${book.id}/read` : `/books/${book.id}`}
        className="block"
        aria-label={readable ? `${book.title} — ${t('book.read')}` : book.title}
      >
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
      </Link>
      <Link to={`/books/${book.id}`} className="mt-2.5 block px-0.5">
        {/* Wrap the full title on all viewports (titles get cut at one line
            otherwise). */}
        <h3
          title={book.title}
          className="break-words text-sm font-medium text-slate-100 transition-colors group-hover:text-white"
        >
          {book.title}
        </h3>
        <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
          {authorNames(book.authors) || t('common.unknownAuthor')}
        </p>
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
