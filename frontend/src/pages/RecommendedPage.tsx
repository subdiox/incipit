import { Navigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { usePagedOffset, useRecommendationsEnabled } from '@/lib/hooks'
import { BookCard, BookCardSkeleton, BookGrid } from '@/components/BookCard'
import { Pagination } from '@/components/Pagination'
import { IconStar } from '@/components/icons'

const DEFAULT_PAGE_SIZE = 36

// RecommendedPage is the dedicated "For You" section: the full ranked set of
// personalized picks, paged like the library (page size follows the user's
// setting). The whole ranked list is fetched once from the cache and sliced
// client-side, so page switches are instant. The server recomputes on activity
// changes, so it always reflects the latest reads (never a book already read).
export function RecommendedPage() {
  const { t } = useI18n()
  const { user } = useAuth()
  const show = useRecommendationsEnabled() && user?.showRecommended !== false

  const { data, isLoading } = useQuery({
    queryKey: ['recommendations', 'all'],
    queryFn: () => api.recommendations(500),
    enabled: show,
    staleTime: 0,
    refetchOnMount: 'always',
  })

  const pageSize = user?.pageSize ?? DEFAULT_PAGE_SIZE
  const { offset, currentPage, goToPage } = usePagedOffset(pageSize)

  if (!show) return <Navigate to="/" replace />

  const items = data ?? []
  const total = items.length
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const pageItems = items.slice(offset, offset + pageSize)

  return (
    <div className="min-w-0 flex-1">
      <div className="mb-6">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight text-white">
          <IconStar width={22} height={22} className="text-accent-400" />
          {t('home.recommended')}
        </h1>
        <p className="mt-0.5 text-sm text-slate-500">
          {isLoading
            ? t('common.loading')
            : t(total === 1 ? 'common.books_one' : 'common.books_other', { count: total.toLocaleString() })}
        </p>
      </div>

      {isLoading ? (
        <BookGrid>
          {Array.from({ length: 12 }).map((_, i) => (
            <BookCardSkeleton key={i} />
          ))}
        </BookGrid>
      ) : total > 0 ? (
        <>
          <BookGrid>
            {pageItems.map((it) => (
              <BookCard key={it.book.id} book={it.book} />
            ))}
          </BookGrid>
          <Pagination currentPage={currentPage} totalPages={totalPages} onGoTo={goToPage} />
        </>
      ) : (
        <div className="card flex flex-col items-center justify-center gap-4 px-6 py-20 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-ink-800 text-slate-500">
            <IconStar width={28} height={28} />
          </span>
          <p className="text-sm text-slate-500">{t('recommend.empty')}</p>
        </div>
      )}
    </div>
  )
}
