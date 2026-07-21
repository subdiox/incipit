import type { Book } from '@/types'
import { useI18n } from '@/i18n'
import { useReadingProgressMap } from '@/lib/hooks'
import { Cover } from './Cover'
import { IconCheck } from './icons'

// readPercent maps a 0-based page within totalPages to 0-100, or null when the
// length is unknown (so no bar is drawn rather than a misleading empty one).
function readPercent(page: number, total: number): number | null {
  if (total <= 1) return null
  return Math.min(100, Math.max(0, Math.round((page / (total - 1)) * 100)))
}

// medalClass styles the rank badge: gold/silver/bronze for the podium, a dark
// pill otherwise.
function medalClass(rank: number): string {
  if (rank === 1) return 'bg-gradient-to-br from-amber-300 to-yellow-500 text-black ring-1 ring-amber-200/60'
  if (rank === 2) return 'bg-gradient-to-br from-slate-200 to-slate-400 text-black ring-1 ring-white/50'
  if (rank === 3) return 'bg-gradient-to-br from-amber-600 to-orange-800 text-white ring-1 ring-amber-400/40'
  return 'bg-black/75 text-onaccent backdrop-blur-sm'
}

interface BookCoverProps {
  book: Book
  // 'card' = grid/shelf thumbnail (rounded-xl); 'hero' = the large detail cover
  // (rounded-2xl, higher-res, stronger hover scale).
  size?: 'card' | 'hero'
  // Hover scale + glow ring. Off for static or select-mode covers.
  interactive?: boolean
  // Reading-state overlay: a partial bar while reading, a check badge when done.
  showProgress?: boolean
  // Favorites (heart count) pill, bottom-left.
  showFavorites?: boolean
  // Rank badge (1-based), top-left, for the Rankings section.
  rank?: number
  // Select-mode styling: accent ring when picked, dimmed when in select mode.
  selected?: boolean
  dimmed?: boolean
  // Pointer-events-none content revealed over a bottom gradient on hover (e.g. a
  // "resume" pill on the detail cover). The enclosing tile handles the click.
  hoverContent?: React.ReactNode
}

// BookCover is the one cover tile shared by every thumbnail — grid cards, shelf
// tiles, and the book-detail hero — so the artwork, hover treatment and reading
// state are identical everywhere. Callers wrap it in their own Link/button and
// toggle features via props. Interactive overlays (buttons) must be siblings of
// the wrapping link, not passed here.
export function BookCover({
  book,
  size = 'card',
  interactive = true,
  showProgress = true,
  showFavorites = false,
  rank,
  selected = false,
  dimmed = false,
  hoverContent,
}: BookCoverProps) {
  const { t } = useI18n()
  const pos = useReadingProgressMap().get(book.id)
  // Finished reads get an unambiguous check badge (a full bar reads the same as a
  // 99%-read one); books still in progress get the partial bar.
  const finished = showProgress && !!pos && pos.totalPages > 0 && pos.page >= pos.totalPages - 1
  const readPct = showProgress && pos && !finished ? readPercent(pos.page, pos.totalPages) : null
  const hero = size === 'hero'

  const ring = selected
    ? 'ring-2 ring-accent-500 shadow-glow'
    : `ring-1 ring-ink-700 ${interactive ? 'group-hover:ring-accent-500/60 group-hover:shadow-glow' : ''}`
  const coverClass = dimmed
    ? 'opacity-80'
    : selected
      ? 'opacity-95'
      : interactive
        ? `transition-transform duration-500 ${hero ? 'md:group-hover:scale-105' : 'group-hover:scale-[1.03]'}`
        : ''

  return (
    <div className={`relative overflow-hidden shadow-soft transition-all ${hero ? 'rounded-2xl' : 'rounded-xl'} ${ring}`}>
      <Cover
        bookId={book.id}
        title={book.title}
        hasCover={book.hasCover}
        version={book.lastModified}
        width={hero ? 800 : 300}
        rounded="rounded-none"
        className={coverClass}
      />

      {rank != null && (
        <div
          className={`absolute left-1.5 top-1.5 z-10 flex min-w-[1.5rem] items-center justify-center rounded-md px-1.5 py-0.5 text-[13px] font-bold tabular-nums shadow-soft ${medalClass(rank)}`}
        >
          {rank}
        </div>
      )}

      {showFavorites && book.favorites > 0 && (
        <div className="absolute bottom-1.5 left-1.5 z-10 flex items-center gap-0.5 rounded-md bg-black/70 px-1.5 py-0.5 text-[11px] font-semibold text-onaccent backdrop-blur-sm">
          <span className="text-rose-400">♥</span>
          {book.favorites.toLocaleString()}
        </div>
      )}

      {/* Soft bottom gradient — only as the backdrop for hoverContent (e.g. the
          detail cover's resume pill), so plain thumbnails don't darken on hover. */}
      {hoverContent && (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-1/3 bg-gradient-to-t from-black/65 to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
      )}
      {hoverContent && (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex justify-center px-3 pb-5 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          {hoverContent}
        </div>
      )}

      {finished ? (
        <div
          className="absolute bottom-1.5 right-1.5 z-30 flex h-5 w-5 items-center justify-center rounded-full bg-accent-500 text-onaccent shadow-soft ring-2 ring-ink-950/50"
          title={t('reading.finished')}
        >
          <IconCheck width={12} height={12} />
        </div>
      ) : readPct != null && readPct > 0 ? (
        <div className="absolute inset-x-0 bottom-0 z-30 h-1 bg-black/30 transition-all duration-200 group-hover:h-1.5">
          <div className="h-full rounded-r-full bg-accent-500" style={{ width: `${readPct}%` }} />
        </div>
      ) : null}
    </div>
  )
}
