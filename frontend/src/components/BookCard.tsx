import { Link } from 'react-router-dom'
import type { Book } from '@/types'
import { authorNames } from '@/lib/format'
import { useI18n } from '@/i18n'
import { Cover } from './Cover'
import { IconCheck } from './icons'

interface BookCardProps {
  book: Book
  action?: React.ReactNode
  // Selection mode: clicking the card toggles selection instead of navigating.
  selectable?: boolean
  selected?: boolean
  onToggleSelect?: (book: Book) => void
}

export function BookCard({ book, action, selectable, selected, onToggleSelect }: BookCardProps) {
  const { t } = useI18n()
  const toggle = () => onToggleSelect?.(book)
  return (
    <div className="group relative">
      <Link
        to={`/books/${book.id}`}
        className="block"
        onClick={(e) => {
          if (selectable) {
            e.preventDefault()
            toggle()
          }
        }}
      >
        <div
          className={`overflow-hidden rounded-xl shadow-soft ring-1 transition-all duration-200 ${
            selected
              ? 'ring-2 ring-accent-500 shadow-glow'
              : 'ring-ink-700 group-hover:ring-accent-500/50 group-hover:shadow-glow'
          }`}
        >
          <Cover
            bookId={book.id}
            title={book.title}
            hasCover={book.hasCover}
            version={book.lastModified}
            rounded="rounded-none"
            className={`transition-transform duration-300 ${
              selectable ? (selected ? 'opacity-95' : 'opacity-80') : 'group-hover:scale-[1.03]'
            }`}
          />
        </div>
        <div className="mt-2.5 px-0.5">
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
        </div>
      </Link>
      {/* Series name links to the series listing (not the volume). Kept outside
          the book link — the title already carries the volume number. Hidden in
          select mode so a stray tap can't navigate away. */}
      {book.series && !selectable && (
        <Link
          to={`/?series=${book.series.id}`}
          className="mt-0.5 line-clamp-1 block px-0.5 text-[11px] text-accentSoft/80 hover:text-accentSoft hover:underline"
        >
          {book.series.name}
        </Link>
      )}
      {selectable && (
        <button
          type="button"
          onClick={toggle}
          aria-pressed={selected}
          aria-label={t(selected ? 'select.deselectOne' : 'select.selectOne')}
          className={`absolute left-2 top-2 z-10 flex h-7 w-7 items-center justify-center rounded-full border-2 shadow-soft transition-colors ${
            selected
              ? 'border-accent-500 bg-accent-500 text-onaccent'
              : 'border-white/70 bg-ink-950/50 text-transparent hover:border-white'
          }`}
        >
          <IconCheck width={16} height={16} />
        </button>
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
