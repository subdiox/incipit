import { useMemo } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { useRankingsEnabled } from '@/lib/hooks'
import { BookCard, BookCardSkeleton, BookGrid } from '@/components/BookCard'
import { FullPageSpinner } from '@/components/Spinner'
import { IconChevronLeft, IconChevronRight, IconFlame } from '@/components/icons'

const DEFAULT_PAGE_SIZE = 36

// RankingsPage renders one externally-curated ranking list at a time in explicit
// rank order, with a period tab per list. It's a dedicated section (separate from
// the library and collections): no filters, no grouping, no sort — the order IS
// the data. Rank badges (offset + index) carry the position.
export function RankingsPage() {
  const { key } = useParams()
  const { t } = useI18n()
  const { user } = useAuth()
  const [params, setParams] = useSearchParams()
  const rankingsOn = useRankingsEnabled()

  const { data: lists, isLoading } = useQuery({
    queryKey: ['rankings'],
    queryFn: api.rankings,
    enabled: rankingsOn,
  })

  const pageSize = user?.pageSize ?? DEFAULT_PAGE_SIZE
  const offset = params.get('offset') ? Math.max(0, Number(params.get('offset'))) : 0

  const active = useMemo(
    () => (lists ?? []).find((l) => l.key === key) ?? (lists ?? [])[0],
    [lists, key],
  )

  const booksQ = useQuery({
    queryKey: ['books', { ranking: active?.key, limit: pageSize, offset }],
    queryFn: () => api.books({ ranking: active!.key, limit: pageSize, offset }),
    placeholderData: keepPreviousData,
    enabled: !!active,
  })

  if (rankingsOn && isLoading) return <FullPageSpinner />
  // Feature off or no lists: nothing to show — fall back to the library.
  if (!rankingsOn || !lists || lists.length === 0) return <Navigate to="/" replace />
  // No/unknown key in the URL: land on the first list.
  if (!key || !lists.some((l) => l.key === key)) {
    return <Navigate to={`/rankings/${encodeURIComponent(lists[0].key)}`} replace />
  }
  if (!active) return <Navigate to="/" replace />

  const total = booksQ.data?.total ?? 0
  const books = booksQ.data?.books ?? []
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const currentPage = Math.floor(offset / pageSize) + 1

  const goToPage = (page: number) => {
    const next = new URLSearchParams(params)
    const off = (page - 1) * pageSize
    if (off > 0) next.set('offset', String(off))
    else next.delete('offset')
    setParams(next)
    window.scrollTo({ top: 0 })
  }

  return (
    <div className="min-w-0 flex-1">
      {/* Header */}
      <div className="mb-4">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight text-white">
          <IconFlame width={22} height={22} className="text-accent-400" />
          {t('nav.rankings')}
        </h1>
        <p className="mt-0.5 text-sm text-slate-500">
          {booksQ.isLoading
            ? t('common.loading')
            : t(total === 1 ? 'common.books_one' : 'common.books_other', {
                count: total.toLocaleString(),
              })}
        </p>
      </div>

      {/* Period tabs — one per ranking list, in the server's order. Switching a
          tab is a fresh navigation (drops the page offset). */}
      <div className="mb-6 flex flex-wrap gap-1.5">
        {lists.map((l) => {
          const isActive = l.key === active.key
          return (
            <Link
              key={l.key}
              to={`/rankings/${encodeURIComponent(l.key)}`}
              className={`rounded-full px-4 py-1.5 text-sm font-medium transition-colors ${
                isActive
                  ? 'bg-accent-500/20 text-accentSoft ring-1 ring-accent-500/40'
                  : 'text-slate-400 hover:bg-ink-800 hover:text-white'
              }`}
            >
              {l.label}
            </Link>
          )
        })}
      </div>

      {booksQ.isLoading ? (
        <BookGrid>
          {Array.from({ length: 12 }).map((_, i) => (
            <BookCardSkeleton key={i} />
          ))}
        </BookGrid>
      ) : books.length > 0 ? (
        <>
          <BookGrid>
            {books.map((b, i) => (
              <BookCard key={b.id} book={b} rank={offset + i + 1} />
            ))}
          </BookGrid>

          {totalPages > 1 && (
            <div className="mt-8 flex items-center justify-center gap-2">
              <button
                type="button"
                className="btn-secondary"
                disabled={currentPage <= 1}
                onClick={() => goToPage(currentPage - 1)}
              >
                <IconChevronLeft width={16} height={16} />
                {t('library.prev')}
              </button>
              <span className="px-3 text-sm text-slate-400">
                {t('library.pageOf', { current: currentPage, total: totalPages })}
              </span>
              <button
                type="button"
                className="btn-secondary"
                disabled={currentPage >= totalPages}
                onClick={() => goToPage(currentPage + 1)}
              >
                {t('library.next')}
                <IconChevronRight width={16} height={16} />
              </button>
            </div>
          )}
        </>
      ) : (
        <div className="card flex flex-col items-center justify-center gap-4 px-6 py-20 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-ink-800 text-slate-500">
            <IconFlame width={28} height={28} />
          </span>
          <p className="text-sm text-slate-500">{t('rankings.empty')}</p>
        </div>
      )}
    </div>
  )
}
