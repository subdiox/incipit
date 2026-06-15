import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api, type BookQuery } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useDebounced } from '@/lib/hooks'
import type { Facet, SortKey, SortOrder } from '@/types'
import { BookCard, BookCardSkeleton, BookGrid } from '@/components/BookCard'
import { UploadModal } from '@/components/UploadModal'
import { IconChevronLeft, IconChevronRight, IconFilter, IconUpload, IconLibrary } from '@/components/icons'

const PAGE_SIZE = 36

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: 'timestamp', label: 'Recently added' },
  { value: 'title', label: 'Title' },
  { value: 'author', label: 'Author' },
  { value: 'series', label: 'Series' },
  { value: 'pubdate', label: 'Publication date' },
  { value: 'rating', label: 'Rating' },
]

type FacetKind = 'author' | 'series' | 'tag'

function FacetSection({
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
  const [expanded, setExpanded] = useState(false)
  if (!facets || facets.length === 0) return null
  const visible = expanded ? facets : facets.slice(0, 8)

  return (
    <div>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{title}</h3>
      <div className="flex flex-wrap gap-1.5">
        {visible.map((f) => (
          <button
            key={f.id}
            type="button"
            onClick={() => onToggle(kind, f.id)}
            className={`chip ${activeId === f.id ? 'chip-active' : ''}`}
          >
            <span className="max-w-[10rem] truncate">{f.name}</span>
            <span className="text-[10px] text-slate-500">{f.count}</span>
          </button>
        ))}
        {facets.length > 8 && (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="chip border-transparent bg-transparent text-accent-300 hover:bg-transparent"
          >
            {expanded ? 'Show less' : `+${facets.length - 8} more`}
          </button>
        )}
      </div>
    </div>
  )
}

export function LibraryPage() {
  const { user } = useAuth()
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
      limit: PAGE_SIZE,
      offset,
    }),
    [debouncedSearch, sort, order, authorId, seriesId, tagId, offset],
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
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  const goToPage = (page: number) => {
    update((p) => {
      const newOffset = (page - 1) * PAGE_SIZE
      if (newOffset > 0) p.set('offset', String(newOffset))
      else p.delete('offset')
    }, false)
  }

  const facetSidebar = (
    <div className="space-y-5">
      {hasFilters && (
        <button
          type="button"
          onClick={() =>
            update((p) => {
              p.delete('author')
              p.delete('series')
              p.delete('tag')
            })
          }
          className="text-xs font-medium text-accent-300 hover:text-accent-200"
        >
          Clear filters
        </button>
      )}
      <FacetSection title="Authors" kind="author" facets={authors.data} activeId={authorId} onToggle={toggleFacet} />
      <FacetSection title="Series" kind="series" facets={series.data} activeId={seriesId} onToggle={toggleFacet} />
      <FacetSection title="Tags" kind="tag" facets={tags.data} activeId={tagId} onToggle={toggleFacet} />
    </div>
  )

  return (
    <div className="flex gap-8">
      {/* Filter sidebar (desktop) */}
      <aside className="hidden w-56 shrink-0 xl:block">
        <div className="sticky top-20">{facetSidebar}</div>
      </aside>

      <div className="min-w-0 flex-1">
        {/* Header / controls */}
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-white">Library</h1>
            <p className="mt-0.5 text-sm text-slate-500">
              {isLoading ? 'Loading…' : `${total.toLocaleString()} ${total === 1 ? 'book' : 'books'}`}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setFiltersOpen((v) => !v)}
              className="btn-secondary xl:hidden"
            >
              <IconFilter width={16} height={16} />
              Filters
            </button>

            <select
              value={sort}
              onChange={(e) => update((p) => p.set('sort', e.target.value))}
              className="input w-auto cursor-pointer py-2 pr-8"
            >
              {SORT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>

            <button
              type="button"
              onClick={() => update((p) => p.set('order', order === 'desc' ? 'asc' : 'desc'))}
              className="btn-secondary"
              title={order === 'desc' ? 'Descending' : 'Ascending'}
            >
              {order === 'desc' ? '↓' : '↑'}
            </button>

            {user?.canUpload && (
              <button type="button" onClick={() => setUploadOpen(true)} className="btn-primary">
                <IconUpload width={16} height={16} />
                <span className="hidden sm:inline">Upload</span>
              </button>
            )}
          </div>
        </div>

        {/* Mobile filters */}
        {filtersOpen && (
          <div className="mb-5 rounded-2xl border border-ink-700 bg-ink-850 p-4 xl:hidden">
            {facetSidebar}
          </div>
        )}

        {/* Content */}
        {isError ? (
          <div className="card p-8 text-center text-sm text-red-300">
            {(error as Error)?.message ?? 'Failed to load library.'}
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
                  Prev
                </button>
                <span className="px-3 text-sm text-slate-400">
                  Page {currentPage} of {totalPages}
                </span>
                <button
                  type="button"
                  className="btn-secondary"
                  disabled={currentPage >= totalPages}
                  onClick={() => goToPage(currentPage + 1)}
                >
                  Next
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
                {hasFilters ? 'No matching books' : 'Your library is empty'}
              </h2>
              <p className="mt-1 text-sm text-slate-500">
                {hasFilters
                  ? 'Try adjusting your search or filters.'
                  : user?.canUpload
                    ? 'Upload your first comic to get started.'
                    : 'No comics have been added yet.'}
              </p>
            </div>
            {!hasFilters && user?.canUpload && (
              <button type="button" onClick={() => setUploadOpen(true)} className="btn-primary">
                <IconUpload width={16} height={16} />
                Upload a comic
              </button>
            )}
          </div>
        )}
      </div>

      <UploadModal open={uploadOpen} onClose={() => setUploadOpen(false)} />
    </div>
  )
}
