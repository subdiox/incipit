import { Link } from 'react-router-dom'
import type { Book, SeriesCard } from '@/types'
import { authorNames } from '@/lib/format'
import { useI18n } from '@/i18n'
import { Cover } from './Cover'
import { IconCheck, IconShelf } from './icons'

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

// LibrarySeriesCard is one series collapsed to a tile: the latest volume's cover
// under a stacked-card frame, the series name, and a "全N巻" count badge. Links
// to the series listing.
export function LibrarySeriesCard({ card }: { card: SeriesCard }) {
  const { t } = useI18n()
  return (
    <div className="group relative">
      <Link to={`/?series=${card.id}`} className="block">
        {/* Offset layers behind the cover imply a stack of volumes. */}
        <div className="relative">
          <div className="absolute inset-0 translate-x-1.5 translate-y-1.5 rounded-xl bg-ink-800 ring-1 ring-ink-700" aria-hidden />
          <div className="absolute inset-0 translate-x-[3px] translate-y-[3px] rounded-xl bg-ink-850 ring-1 ring-ink-700" aria-hidden />
          <div className="relative overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all duration-200 group-hover:ring-accent-500/50 group-hover:shadow-glow">
            {card.cover ? (
              <Cover
                bookId={card.cover.id}
                title={card.name}
                hasCover={card.cover.hasCover}
                version={card.cover.lastModified}
                rounded="rounded-none"
                className="transition-transform duration-300 group-hover:scale-[1.03]"
              />
            ) : (
              <div className="flex aspect-[2/3] w-full items-center justify-center bg-ink-800 text-slate-600">
                <IconShelf width={28} height={28} />
              </div>
            )}
            {/* text-onaccent (not text-white, which is the theme fg = near-black
                in light mode) so the badge stays white on its dark pill. */}
            <span className="absolute bottom-1.5 left-1.5 rounded-full bg-black/70 px-2 py-0.5 text-[11px] font-medium text-onaccent backdrop-blur">
              {t('library.volumeCount', { count: card.bookCount })}
            </span>
          </div>
        </div>
        <div className="mt-2.5 px-0.5">
          <h3
            title={card.name}
            className="break-words text-sm font-medium text-accentSoft transition-colors group-hover:text-accent-300"
          >
            {card.name}
          </h3>
          <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
            {t('library.seriesLabel')}
          </p>
        </div>
      </Link>
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
