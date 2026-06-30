import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api, type BookQuery } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import type { TranslationKey } from '@/i18n/en'
import { useDebounced } from '@/lib/hooks'
import type { Facet, SortKey, SortOrder } from '@/types'
import { BookCard, BookCardSkeleton, BookGrid } from '@/components/BookCard'
import { UploadModal } from '@/components/UploadModal'
import {
  IconChevronLeft,
  IconChevronRight,
  IconClose,
  IconFilter,
  IconSearch,
  IconUpload,
  IconLibrary,
} from '@/components/icons'

const DEFAULT_PAGE_SIZE = 36
const PAGE_SIZE_OPTIONS = [12, 24, 36, 48, 60, 96]

const SORT_OPTIONS: { value: SortKey; labelKey: TranslationKey }[] = [
  { value: 'timestamp', labelKey: 'library.sort.recentlyAdded' },
  { value: 'title', labelKey: 'library.sort.title' },
  { value: 'author', labelKey: 'library.sort.author' },
  { value: 'series', labelKey: 'library.sort.series' },
  { value: 'pubdate', labelKey: 'library.sort.pubdate' },
  { value: 'rating', labelKey: 'library.sort.rating' },
]

type FacetKind = 'author' | 'series' | 'tag'

// Rows rendered at once. Large libraries can have thousands of authors, so the
// list is searchable and capped — never dumped into the DOM all at once.
const FACET_LIMIT = 50
// Below this size the search box is unnecessary; just show the list.
const FACET_SEARCH_THRESHOLD = 8

function FacetFilter({
  title,
  kind,
  facets,
  activeId,
  onToggle,
}: {
  title: string
  kind: FacetKind
  facets: Facet[] | undefined
  activeId: number | null
  onToggle: (kind: FacetKind, id: number) => void
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')

  const active = activeId != null ? (facets?.find((f) => f.id === activeId) ?? null) : null

  const { rows, total } = useMemo(() => {
    const all = facets ?? []
    const needle = query.trim().toLowerCase()
    const matched = needle ? all.filter((f) => f.name.toLowerCase().includes(needle)) : all
    // Most-used first for discoverability; the active one floats to the top.
    const sorted = [...matched].sort((a, b) => {
      if (a.id === activeId) return -1
      if (b.id === activeId) return 1
      return b.count - a.count || a.name.localeCompare(b.name)
    })
    return { rows: sorted.slice(0, FACET_LIMIT), total: sorted.length }
  }, [facets, query, activeId])

  if (!facets || facets.length === 0) return null
  const searchable = facets.length > FACET_SEARCH_THRESHOLD

  return (
    <div>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">{title}</h3>
        <span className="text-[10px] tabular-nums text-slate-600">{facets.length.toLocaleString()}</span>
      </div>

      {active && (
        <button
          type="button"
          onClick={() => onToggle(kind, active.id)}
          className="chip chip-active mb-2 max-w-full"
          title={t('library.clearFilters')}
        >
          <span className="truncate">{active.name}</span>
          <IconClose width={12} height={12} className="shrink-0" />
        </button>
      )}

      {searchable && (
        <div className="relative mb-2">
          <IconSearch
            width={14}
            height={14}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500"
          />
          <input
            className="input py-1.5 pl-8 text-sm"
            placeholder={t('library.facetSearch')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      )}

      {rows.length === 0 ? (
        <p className="px-1 py-1.5 text-xs text-slate-600">{t('library.facetNoMatch')}</p>
      ) : (
        <ul className="-mr-1 max-h-60 space-y-0.5 overflow-y-auto pr-1">
          {rows.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                onClick={() => onToggle(kind, f.id)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1 text-left text-sm transition-colors ${
                  f.id === activeId
                    ? 'bg-accent-500/15 text-accentSoft'
                    : 'text-slate-300 hover:bg-ink-800'
                }`}
              >
                <span className="truncate">{f.name}</span>
                <span className="shrink-0 text-[10px] tabular-nums text-slate-500">{f.count}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {total > rows.length && (
        <p className="mt-1.5 px-1 text-[11px] text-slate-600">
          {t('library.facetMore', { count: total - rows.length })}
        </p>
      )}
    </div>
  )
}

export function LibraryPage() {
  const { user, setUser } = useAuth()
  const { t } = useI18n()
  const [params, setParams] = useSearchParams()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [filtersOpen, setFiltersOpen] = useState(false)

  const search = params.get('search') ?? ''
  const debouncedSearch = useDebounced(search, 350)
  const sort = (params.get('sort') as SortKey) ?? 'timestamp'
  const order = (params.get('order') as SortOrder) ?? 'desc'
  const authorId = params.get('author') ? Number(params.get('author')) : null
  const seriesId = params.get('series') ? Number(params.get('series')) : null
  const tagId = params.get('tag') ? Number(params.get('tag')) : null
  const offset = params.get('offset') ? Number(params.get('offset')) : 0
  const pageSize = user?.pageSize ?? DEFAULT_PAGE_SIZE

  const update = (mut: (p: URLSearchParams) => void, resetOffset = true) => {
    const next = new URLSearchParams(params)
    mut(next)
    if (resetOffset) next.delete('offset')
    setParams(next)
  }

  const toggleFacet = (kind: FacetKind, id: number) => {
    update((p) => {
      if (p.get(kind) === String(id)) p.delete(kind)
      else p.set(kind, String(id))
    })
  }

  const query = useMemo<BookQuery>(
    () => ({
      search: debouncedSearch || undefined,
      sort,
      order,
      author: authorId ?? undefined,
      series: seriesId ?? undefined,
      tag: tagId ?? undefined,
      limit: pageSize,
      offset,
    }),
    [debouncedSearch, sort, order, authorId, seriesId, tagId, offset, pageSize],
  )

  const { data, isLoading, isFetching, isError, error } = useQuery({
    queryKey: ['books', query],
    queryFn: () => api.books(query),
    placeholderData: keepPreviousData,
  })

  const authors = useQuery({ queryKey: ['facets', 'authors'], queryFn: api.authors, staleTime: 300_000 })
  const series = useQuery({ queryKey: ['facets', 'series'], queryFn: api.series, staleTime: 300_000 })
  const tags = useQuery({ queryKey: ['facets', 'tags'], queryFn: api.tags, staleTime: 300_000 })

  const total = data?.total ?? 0
  const hasFilters = authorId != null || seriesId != null || tagId != null || !!debouncedSearch
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const currentPage = Math.floor(offset / pageSize) + 1

  const changePageSize = (size: number) => {
    if (!user || size === user.pageSize) return
    setUser({ ...user, pageSize: size }) // optimistic
    update((p) => p.delete('offset')) // back to page 1
    api.setPageSize(size).then(setUser).catch(() => setUser(user))
  }

  const goToPage = (page: number) => {
    update((p) => {
      const newOffset = (page - 1) * pageSize
      if (newOffset > 0) p.set('offset', String(newOffset))
      else p.delete('offset')
    }, false)
  }

  const activeFacetCount =
    (authorId != null ? 1 : 0) + (seriesId != null ? 1 : 0) + (tagId != null ? 1 : 0)

  // Close the filters dropdown on outside click / Escape.
  const filtersRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!filtersOpen) return
    const onPointer = (e: MouseEvent) => {
      if (filtersRef.current && !filtersRef.current.contains(e.target as Node)) setFiltersOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setFiltersOpen(false)
    document.addEventListener('mousedown', onPointer)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onPointer)
      document.removeEventListener('keydown', onKey)
    }
  }, [filtersOpen])

  const clearFacets = () =>
    update((p) => {
      p.delete('author')
      p.delete('series')
      p.delete('tag')
    })

  const activeChips = [
    { kind: 'author' as const, id: authorId, name: authors.data?.find((f) => f.id === authorId)?.name },
    { kind: 'series' as const, id: seriesId, name: series.data?.find((f) => f.id === seriesId)?.name },
    { kind: 'tag' as const, id: tagId, name: tags.data?.find((f) => f.id === tagId)?.name },
  ].filter((c): c is { kind: FacetKind; id: number; name: string | undefined } => c.id != null)

  const facetPanel = (
    <div className="grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-3">
      <FacetFilter title={t('library.authors')} kind="author" facets={authors.data} activeId={authorId} onToggle={toggleFacet} />
      <FacetFilter title={t('library.series')} kind="series" facets={series.data} activeId={seriesId} onToggle={toggleFacet} />
      <FacetFilter title={t('library.tags')} kind="tag" facets={tags.data} activeId={tagId} onToggle={toggleFacet} />
    </div>
  )

  return (
      <div className="min-w-0 flex-1">
        {/* Header */}
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-white">{t('library.title')}</h1>
            <p className="mt-0.5 text-sm text-slate-500">
              {isLoading
                ? t('common.loading')
                : t(total === 1 ? 'common.books_one' : 'common.books_other', {
                    count: total.toLocaleString(),
                  })}
            </p>
          </div>

          {user?.canUpload && (
            <button type="button" onClick={() => setUploadOpen(true)} className="btn-primary">
              <IconUpload width={16} height={16} />
              <span className="hidden sm:inline">{t('library.upload')}</span>
            </button>
          )}
        </div>

        {/* Controls: filters + sort, sitting just under the search bar */}
        <div className="mb-5 flex flex-wrap items-center gap-2">
          <div className="relative" ref={filtersRef}>
            <button
              type="button"
              onClick={() => setFiltersOpen((v) => !v)}
              className={`btn-secondary ${filtersOpen || activeFacetCount > 0 ? 'border-accent-500/60 text-accentSoft' : ''}`}
              aria-expanded={filtersOpen}
            >
              <IconFilter width={16} height={16} />
              {t('library.filters')}
              {activeFacetCount > 0 && (
                <span className="ml-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-accent-600 px-1 text-[10px] font-semibold text-onaccent">
                  {activeFacetCount}
                </span>
              )}
            </button>

            {filtersOpen && (
              <div className="absolute left-0 z-30 mt-2 w-[min(92vw,44rem)] origin-top-left animate-fade-in rounded-2xl border border-ink-700 bg-ink-850 p-4 shadow-soft">
                <div className="mb-3 flex items-center justify-between">
                  <h2 className="text-sm font-semibold text-white">{t('library.filters')}</h2>
                  {activeFacetCount > 0 && (
                    <button
                      type="button"
                      onClick={clearFacets}
                      className="text-xs font-medium text-accentSoft hover:text-white"
                    >
                      {t('library.clearFilters')}
                    </button>
                  )}
                </div>
                {facetPanel}
              </div>
            )}
          </div>

          {/* Active filter chips, visible while the dropdown is closed */}
          {activeChips.map((c) => (
            <button
              key={`${c.kind}-${c.id}`}
              type="button"
              onClick={() => toggleFacet(c.kind, c.id)}
              className="chip chip-active"
              title={t('library.clearFilters')}
            >
              <span className="max-w-[12rem] truncate">{c.name ?? '…'}</span>
              <IconClose width={12} height={12} className="shrink-0" />
            </button>
          ))}

          <div className="ml-auto flex items-center gap-2">
            <select
              value={pageSize}
              onChange={(e) => changePageSize(Number(e.target.value))}
              className="input w-auto cursor-pointer py-2 pr-8"
              title={t('library.perPage')}
              aria-label={t('library.perPage')}
            >
              {PAGE_SIZE_OPTIONS.map((n) => (
                <option key={n} value={n}>
                  {t('library.perPageOption', { count: n })}
                </option>
              ))}
            </select>

            <select
              value={sort}
              onChange={(e) => update((p) => p.set('sort', e.target.value))}
              className="input w-auto cursor-pointer py-2 pr-8"
            >
              {SORT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.labelKey)}
                </option>
              ))}
            </select>

            <button
              type="button"
              onClick={() => update((p) => p.set('order', order === 'desc' ? 'asc' : 'desc'))}
              className="btn-secondary"
              title={order === 'desc' ? t('library.descending') : t('library.ascending')}
            >
              {order === 'desc' ? '↓' : '↑'}
            </button>
          </div>
        </div>

        {/* Content */}
        {isError ? (
          <div className="card p-8 text-center text-sm text-red-300">
            {(error as Error)?.message ?? t('library.failedToLoad')}
          </div>
        ) : isLoading ? (
          <BookGrid>
            {Array.from({ length: 14 }).map((_, i) => (
              <BookCardSkeleton key={i} />
            ))}
          </BookGrid>
        ) : data && (data.books?.length ?? 0) > 0 ? (
          <>
            <div className={isFetching ? 'opacity-60 transition-opacity' : 'transition-opacity'}>
              <BookGrid>
                {(data.books ?? []).map((book) => (
                  <BookCard key={book.id} book={book} />
                ))}
              </BookGrid>
            </div>

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
              <IconLibrary width={28} height={28} />
            </span>
            <div>
              <h2 className="text-lg font-medium text-white">
                {hasFilters ? t('library.emptyNoMatch') : t('library.emptyTitle')}
              </h2>
              <p className="mt-1 text-sm text-slate-500">
                {hasFilters
                  ? t('library.emptyNoMatchHint')
                  : user?.canUpload
                    ? t('library.emptyUploadHint')
                    : t('library.emptyNoneHint')}
              </p>
            </div>
            {!hasFilters && user?.canUpload && (
              <button type="button" onClick={() => setUploadOpen(true)} className="btn-primary">
                <IconUpload width={16} height={16} />
                {t('library.uploadFirst')}
              </button>
            )}
          </div>
        )}

        <UploadModal open={uploadOpen} onClose={() => setUploadOpen(false)} />
      </div>
  )
}
