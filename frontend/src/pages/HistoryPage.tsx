import { Link, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { ReadingItem } from '@/types'
import { authorNames } from '@/lib/format'
import { Cover } from '@/components/Cover'
import { BookGrid, BookCardSkeleton } from '@/components/BookCard'
import { progressPct } from '@/components/ReadingShelf'
import { IconTrash } from '@/components/icons'

function HistoryCard({
  item,
  onReset,
  resetting,
}: {
  item: ReadingItem
  onReset: () => void
  resetting: boolean
}) {
  const { t } = useI18n()
  const { book } = item
  const pct = progressPct(item.page, item.totalPages)
  return (
    <div className="group relative">
      <Link to={`/books/${book.id}`} className="block">
        <div className="overflow-hidden rounded-xl shadow-soft ring-1 ring-ink-700 transition-all group-hover:ring-accent-500/50">
          <Cover
            bookId={book.id}
            title={book.title}
            hasCover={book.hasCover}
            version={book.lastModified}
            rounded="rounded-none"
          />
        </div>
        {item.totalPages > 0 && (
          <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-ink-700">
            <div className="h-full rounded-full bg-accent-500" style={{ width: `${pct}%` }} />
          </div>
        )}
        <h3
          title={book.title}
          className="mt-1.5 break-words text-sm font-medium text-slate-100 group-hover:text-white"
        >
          {book.title}
        </h3>
        <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
          {authorNames(book.authors) || t('common.unknownAuthor')}
        </p>
        {item.totalPages > 0 && (
          <p className="mt-0.5 text-[11px] text-slate-600">
            {t('history.pageOf', { page: item.page + 1, total: item.totalPages })}
          </p>
        )}
      </Link>
      <button
        type="button"
        onClick={onReset}
        disabled={resetting}
        title={t('history.reset')}
        aria-label={t('history.reset')}
        className="absolute right-2 top-2 z-10 rounded-lg bg-ink-950/70 p-1.5 text-slate-300 opacity-0 backdrop-blur transition-opacity hover:bg-red-600 hover:text-white focus:opacity-100 group-hover:opacity-100 disabled:opacity-50"
      >
        <IconTrash width={15} height={15} />
      </button>
    </div>
  )
}

function Section({
  title,
  status,
}: {
  title: string
  status: 'continue' | 'finished'
}) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const [params] = useSearchParams()
  const search = (params.get('search') ?? '').trim().toLowerCase()
  const { data, isLoading } = useQuery({
    queryKey: ['reading', status === 'finished' ? 'history' : 'continue'],
    queryFn: () => api.myReading(status, 200),
  })
  // Filter the shown books by title (header search on the History page).
  const items = search ? (data ?? []).filter((it) => it.book.title.toLowerCase().includes(search)) : data

  const reset = useMutation({
    mutationFn: (bookId: number) => api.resetProgress(bookId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['reading'] })
    },
  })

  if (!isLoading && (!items || items.length === 0)) {
    if (status === 'continue') return null // hide empty "continue" section
  }

  return (
    <section className="mb-10">
      <h2 className="mb-4 text-lg font-semibold text-white">{title}</h2>
      {isLoading ? (
        <BookGrid>
          {Array.from({ length: 7 }).map((_, i) => (
            <BookCardSkeleton key={i} />
          ))}
        </BookGrid>
      ) : !items || items.length === 0 ? (
        <p className="text-sm text-slate-500">
          {search ? t('library.emptyNoMatch') : t('history.empty')}
        </p>
      ) : (
        <BookGrid>
          {items.map((it) => (
            <HistoryCard
              key={it.book.id}
              item={it}
              resetting={reset.isPending && reset.variables === it.book.id}
              onReset={() => reset.mutate(it.book.id)}
            />
          ))}
        </BookGrid>
      )}
    </section>
  )
}

export function HistoryPage() {
  const { t } = useI18n()
  return (
    <div className="min-w-0 flex-1">
      <h1 className="mb-6 text-2xl font-semibold tracking-tight text-white">{t('history.title')}</h1>
      <Section title={t('history.continue')} status="continue" />
      <Section title={t('history.all')} status="finished" />
    </div>
  )
}
