import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useSearchParams } from 'react-router-dom'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type BookQuery } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import type { TranslationKey } from '@/i18n/en'
import { useDebounced } from '@/lib/hooks'
import type { Collection, Facet, SortKey, SortOrder } from '@/types'
import { BookCard, BookCardSkeleton, BookGrid, LibrarySeriesCard } from '@/components/BookCard'
import { AsyncFacet } from '@/components/AsyncFacet'
import { useFacetNames } from '@/lib/facets'
import { ContinueReadingShelf } from '@/components/ReadingShelf'
import { useFilterSlot } from '@/components/Layout'
import { UploadModal } from '@/components/UploadModal'
import { BulkEditModal } from '@/components/BulkEditModal'
import { MetaCompareModal } from '@/components/MetaCompareModal'
import { Modal } from '@/components/Modal'
import { Spinner } from '@/components/Spinner'
import { mapPool } from '@/lib/pool'
import type { Book } from '@/types'
import {
  IconChevronLeft,
  IconChevronRight,
  IconCheck,
  IconClose,
  IconEdit,
  IconFilter,
  IconSearch,
  IconTrash,
  IconUpload,
  IconLibrary,
} from '@/components/icons'

const DEFAULT_PAGE_SIZE = 36

// Label for every sort field (a collection may pin ones the user menu doesn't
// offer, e.g. title/author/series), used by the locked-sort indicator.
const SORT_LABELS: Record<SortKey, TranslationKey> = {
  timestamp: 'library.sort.recentlyAdded',
  title: 'library.sort.title',
  author: 'library.sort.author',
  series: 'library.sort.series',
  pubdate: 'library.sort.pubdate',
  rating: 'library.sort.rating',
  favorites: 'library.sort.favorites',
  views: 'library.sort.views',
  lastread: 'library.sort.lastread',
}

const SORT_OPTIONS: { value: SortKey; labelKey: TranslationKey }[] = [
  { value: 'timestamp', labelKey: 'library.sort.recentlyAdded' },
  { value: 'pubdate', labelKey: 'library.sort.pubdate' },
  { value: 'rating', labelKey: 'library.sort.rating' },
  { value: 'favorites', labelKey: 'library.sort.favorites' },
  { value: 'views', labelKey: 'library.sort.views' },
  { value: 'lastread', labelKey: 'library.sort.lastread' },
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
  activeIds,
  onToggle,
  searchFirst,
}: {
  title: string
  kind: FacetKind
  facets: Facet[] | undefined
  activeIds: number[]
  onToggle: (kind: FacetKind, id: number) => void
  // Search-first (mobile): hide the long list until the user types, so three
  // facets fit in a cramped screen. Selections stay visible as chips.
  searchFirst?: boolean
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')

  const activeSet = useMemo(() => new Set(activeIds), [activeIds])
  const actives = (facets ?? []).filter((f) => activeSet.has(f.id))

  const { rows, total } = useMemo(() => {
    const all = facets ?? []
    const needle = query.trim().toLowerCase()
    const matched = needle ? all.filter((f) => f.name.toLowerCase().includes(needle)) : all
    // Most-used first for discoverability; active ones float to the top.
    const sorted = [...matched].sort((a, b) => {
      const aa = activeSet.has(a.id)
      const ba = activeSet.has(b.id)
      if (aa !== ba) return aa ? -1 : 1
      return b.count - a.count || a.name.localeCompare(b.name)
    })
    return { rows: sorted.slice(0, FACET_LIMIT), total: sorted.length }
  }, [facets, query, activeSet])

  if (!facets || facets.length === 0) return null
  const searchable = searchFirst || facets.length > FACET_SEARCH_THRESHOLD
  // In search-first mode the list is revealed only once the user types.
  const showList = !searchFirst || query.trim().length > 0

  return (
    <div>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">{title}</h3>
        <span className="text-[10px] tabular-nums text-slate-600">{facets.length.toLocaleString()}</span>
      </div>

      {actives.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {actives.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => onToggle(kind, a.id)}
              className="chip chip-active max-w-full"
              title={t('library.clearFilters')}
            >
              <span className="truncate">{a.name}</span>
              <IconClose width={12} height={12} className="shrink-0" />
            </button>
          ))}
        </div>
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

      {!showList ? (
        <p className="px-1 py-0.5 text-[11px] text-slate-600">{t('library.facetTypeToSearch')}</p>
      ) : rows.length === 0 ? (
        <p className="px-1 py-1.5 text-xs text-slate-600">{t('library.facetNoMatch')}</p>
      ) : (
        <ul className="-mr-1 max-h-60 space-y-0.5 overflow-y-auto pr-1">
          {rows.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                onClick={() => onToggle(kind, f.id)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1 text-left text-sm transition-colors ${
                  activeSet.has(f.id)
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

      {showList && total > rows.length && (
        <p className="mt-1.5 px-1 text-[11px] text-slate-600">
          {t('library.facetMore', { count: total - rows.length })}
        </p>
      )}
    </div>
  )
}

export function LibraryPage({ collection }: { collection?: Collection } = {}) {
  const { user } = useAuth()
  const { t } = useI18n()
  const [params, setParams] = useSearchParams()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const canEdit = !!user?.canEdit
  // Series-grouped browse: one tile per series. A PER-ACCOUNT preference
  // (default on), set in Account settings. Only applies while browsing (no
  // search/facet filter) and not selecting.
  const groupMode = user?.groupSeries ?? true
  // Multi-select: a select mode toggle, the chosen books (kept as full Book
  // objects so the bulk/compare modals don't need to refetch), and the three
  // bulk actions. Selection is scoped to the current view and cleared when the
  // query changes.
  const [selectMode, setSelectMode] = useState(false)
  const [selected, setSelected] = useState<Map<number, Book>>(new Map())
  const [bulkEditOpen, setBulkEditOpen] = useState(false)
  const [compareOpen, setCompareOpen] = useState(false)
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)
  const [deleteProgress, setDeleteProgress] = useState<{ done: number; total: number } | null>(null)
  const selectedBooks = useMemo(() => Array.from(selected.values()), [selected])
  const toggleSelect = (book: Book) =>
    setSelected((prev) => {
      const next = new Map(prev)
      if (next.has(book.id)) next.delete(book.id)
      else next.set(book.id, book)
      return next
    })
  const clearSelection = () => setSelected(new Map())
  const exitSelectMode = () => {
    setSelectMode(false)
    clearSelection()
  }
  // The Filters control is rendered into the header (right of search) via a
  // portal. The header unmounts while hidden on scroll, so the slot element is
  // published through context and re-read here — the portal re-targets when the
  // header remounts (and simply isn't shown while the header is hidden).
  const filterSlot = useFilterSlot()
  // On phones the filter dropdown is cramped, so switch its facets to a
  // search-first UI (list hidden until you type).
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(max-width: 639px)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 639px)')
    const on = () => setIsMobile(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])

  const search = params.get('search') ?? ''
  const debouncedSearch = useDebounced(search, 350)
  // Sort is a PER-ACCOUNT preference shared across every library page, with two
  // overrides (resolved below, once site config is loaded): a collection can pin
  // a fixed sort, and a single series always shows in volume order.
  const seriesSelected = !!params.get('series')
  const authorId = params.get('author') ? Number(params.get('author')) : null
  const seriesId = params.get('series') ? Number(params.get('series')) : null
  const tagIds = params.getAll('tag').map(Number).filter((n) => Number.isFinite(n) && n > 0)
  const offset = params.get('offset') ? Number(params.get('offset')) : 0
  const pageSize = user?.pageSize ?? DEFAULT_PAGE_SIZE

  // Page-count filter (only offered when the admin enabled it). Inputs are local
  // + debounced into the URL so typing doesn't refetch on every keystroke.
  const site = useQuery({ queryKey: ['site'], queryFn: api.site, staleTime: 300_000 }).data
  const pageFilterOn = !!site?.pageFilter
  const popularityOn = !!site?.popularity
  // Reading-activity sorts ("most viewed" / "recently read") are admin-gated.
  const readingActivityOn = site?.readingActivity !== false

  // Which sort fields the viewer may pick right now (feature-gated ones drop out).
  const sortAvailable = (v: SortKey) => {
    if (v === 'favorites') return popularityOn
    if (v === 'views' || v === 'lastread') return readingActivityOn
    return true
  }
  // A collection can pin a fixed sort: opening it forces that order and hides the
  // sort control. Empty = inherit the viewer's own preference.
  const lockedSort: SortKey | null =
    collection?.sort && sortAvailable(collection.sort as SortKey) ? (collection.sort as SortKey) : null
  const lockedOrder: SortOrder = (collection?.sortOrder as SortOrder) || 'desc'
  // The DEFAULT sort/order comes from the account setting; a per-screen override
  // lives in the URL (?sort/?order). So opening a page fresh uses the account
  // default, and changing the sort here only affects the current view — it is
  // NOT persisted. Coerced to an available field so a since-disabled feature
  // can't leave the list stuck on a hidden sort.
  const defaultSort: SortKey = (user?.sort as SortKey) ?? 'timestamp'
  const defaultOrder: SortOrder = (user?.sortOrder as SortOrder) ?? 'desc'
  const urlSort = params.get('sort') as SortKey | null
  const urlOrder = params.get('order')
  const rawSort: SortKey = urlSort ?? defaultSort
  const userSort: SortKey = sortAvailable(rawSort) ? rawSort : 'timestamp'
  const userOrder: SortOrder = urlOrder === 'asc' || urlOrder === 'desc' ? urlOrder : defaultOrder
  // Effective sort for the query. Precedence: a single series (volume order) >
  // a collection's pinned sort > the viewer's own preference.
  const sortLocked = !seriesSelected && lockedSort != null
  const sort: SortKey = seriesSelected ? 'series' : (lockedSort ?? userSort)
  const order: SortOrder = seriesSelected ? 'asc' : lockedSort ? lockedOrder : userOrder
  const [minPages, setMinPages] = useState(params.get('minPages') ?? '')
  const [maxPages, setMaxPages] = useState(params.get('maxPages') ?? '')
  const debMinPages = useDebounced(minPages, 400)
  const debMaxPages = useDebounced(maxPages, 400)
  const minPagesQ = pageFilterOn && params.get('minPages') ? Number(params.get('minPages')) : undefined
  const maxPagesQ = pageFilterOn && params.get('maxPages') ? Number(params.get('maxPages')) : undefined

  // A locked base filter scopes this view: on a collection page it's the
  // collection's tags; on the home library it's the admin-configured home filter
  // (server settings). Include tags AND together (a "match any" collection ORs its
  // own group instead); exclude tags hide any book carrying them.
  //
  // The home base filter scopes only the *default* home landing. The moment the
  // user searches or narrows the library (text search, author/series/tag facets
  // or the page-count filter), it steps aside so every matching book shows —
  // otherwise works the filter hides would be unreachable via search/filter.
  // Collections keep their own scope.
  const homeQueryActive =
    !!debouncedSearch ||
    authorId != null ||
    seriesId != null ||
    tagIds.length > 0 ||
    minPagesQ != null ||
    maxPagesQ != null
  const applyHomeFilter = !homeQueryActive
  const baseTagIds = collection ? collection.tagIds : applyHomeFilter ? (site?.homeTags ?? []) : []
  const baseExcludeTagIds = collection
    ? collection.excludeTagIds
    : applyHomeFilter
      ? (site?.homeExcludeTags ?? [])
      : []
  const matchAny = collection ? collection.matchAny : !!site?.homeMatchAny
  const effectiveTagIds = Array.from(new Set([...baseTagIds, ...tagIds])) // for display/locked state
  // Query params: interactive tags (AND) vs the collection's OR group (any-mode only).
  const andTagIds = matchAny ? tagIds : effectiveTagIds
  const anyTagIds = matchAny ? baseTagIds : []
  const andTagKey = andTagIds.join(',')
  const anyTagKey = anyTagIds.join(',')
  const excludeTagKey = baseExcludeTagIds.join(',')

  const update = (mut: (p: URLSearchParams) => void, resetOffset = true) => {
    const next = new URLSearchParams(params)
    mut(next)
    if (resetOffset) next.delete('offset')
    setParams(next)
  }

  // Push debounced page-count inputs into the URL (skipping no-op writes).
  useEffect(() => {
    const m = debMinPages.trim()
    const x = debMaxPages.trim()
    if (m === (params.get('minPages') ?? '') && x === (params.get('maxPages') ?? '')) return
    update((p) => {
      if (m) p.set('minPages', m)
      else p.delete('minPages')
      if (x) p.set('maxPages', x)
      else p.delete('maxPages')
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debMinPages, debMaxPages])

  // author/series are single-select (replace); tags are multi-select and
  // AND-combined, so toggling adds/removes one tag from the set. A collection's base
  // tags are locked and cannot be toggled off here.
  const toggleFacet = (kind: FacetKind, id: number) => {
    if (kind === 'tag' && baseTagIds.includes(id)) return
    update((p) => {
      if (kind === 'tag') {
        const cur = p.getAll('tag')
        p.delete('tag')
        const next = cur.includes(String(id))
          ? cur.filter((v) => v !== String(id))
          : [...cur, String(id)]
        next.forEach((v) => p.append('tag', v))
      } else if (p.get(kind) === String(id)) {
        p.delete(kind)
      } else {
        p.set(kind, String(id))
      }
    })
  }

  const query = useMemo<BookQuery>(
    () => ({
      search: debouncedSearch || undefined,
      sort,
      order,
      author: authorId ?? undefined,
      series: seriesId ?? undefined,
      tags: andTagKey ? andTagKey.split(',').map(Number) : undefined,
      anyTags: anyTagKey ? anyTagKey.split(',').map(Number) : undefined,
      excludeTags: excludeTagKey ? excludeTagKey.split(',').map(Number) : undefined,
      minPages: minPagesQ,
      maxPages: maxPagesQ,
      limit: pageSize,
      offset,
    }),
    [debouncedSearch, sort, order, authorId, seriesId, andTagKey, anyTagKey, excludeTagKey, minPagesQ, maxPagesQ, offset, pageSize],
  )

  // Grouping only applies while plainly browsing (no interactive query) and not
  // multi-selecting — searching/filtering shows individual volumes so a specific
  // one is findable, and selection is per-book.
  const canGroup = !homeQueryActive
  const grouped = canGroup && groupMode && !selectMode

  const flatQ = useQuery({
    queryKey: ['books', query],
    queryFn: () => api.books(query),
    placeholderData: keepPreviousData,
    enabled: !grouped,
  })
  const groupedQ = useQuery({
    queryKey: ['books-grouped', query],
    queryFn: () => api.booksGrouped(query),
    placeholderData: keepPreviousData,
    enabled: grouped,
  })
  const active = grouped ? groupedQ : flatQ
  const { isLoading, isFetching, isError, error } = active
  const flatData = flatQ.data
  const groupedData = groupedQ.data

  // Selection is view-scoped: drop it whenever the query changes (page, filter,
  // search, sort) so a stale cross-view selection can't be acted on.
  const queryKeyStr = JSON.stringify(query)
  useEffect(() => {
    setSelected(new Map())
  }, [queryKeyStr])

  const qc = useQueryClient()
  const bulkDelete = useMutation({
    mutationFn: async () => {
      setDeleteProgress({ done: 0, total: selectedBooks.length })
      const res = await mapPool(
        selectedBooks,
        1,
        (b) => api.deleteBook(b.id),
        (done, total) => setDeleteProgress({ done, total }),
      )
      return res.filter((r) => !r.ok).length
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['books'] })
      qc.invalidateQueries({ queryKey: ['facets'] })
      setDeleteProgress(null)
      setConfirmBulkDelete(false)
      exitSelectMode()
    },
    onError: () => setDeleteProgress(null),
  })

  // Series is small enough to return whole; tags & authors are searched
  // server-side (the library can have 100k+ of each), so only the *selected* ids
  // are resolved to names here — for the active filter chips and, on a collection,
  // its locked base/exclude tag chips.
  const series = useQuery({ queryKey: ['facets', 'series'], queryFn: api.series, staleTime: 300_000 })
  const baseTagKey = baseTagIds.join(',')
  const tagIdKey = tagIds.join(',')
  const tagNameIds = useMemo(
    () => Array.from(new Set([...tagIds, ...baseTagIds, ...baseExcludeTagIds])),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tagIdKey, baseTagKey, excludeTagKey],
  )
  const tagNames = useFacetNames('tags', api.tags, tagNameIds)
  const authorNamesMap = useFacetNames('authors', api.authors, authorId != null ? [authorId] : [])
  // The view's own base tags (include + exclude) are hidden from the interactive
  // tag list: redundant, can't be toggled off, and a base-excluded tag would just
  // empty the view.
  const hiddenTagIds = useMemo(
    () => [...baseTagIds, ...baseExcludeTagIds],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [baseTagKey, excludeTagKey],
  )

  const total = active.data?.total ?? 0
  const hasPageFilter = minPagesQ != null || maxPagesQ != null
  const hasFilters =
    authorId != null || seriesId != null || tagIds.length > 0 || !!debouncedSearch || hasPageFilter
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const currentPage = Math.floor(offset / pageSize) + 1

  // Sort is a per-screen override written to the URL (the account setting is the
  // default). update() resets the page offset for us.
  const changeSort = (value: SortKey) => update((p) => p.set('sort', value))
  const changeOrder = () => {
    const next: SortOrder = userOrder === 'desc' ? 'asc' : 'desc'
    update((p) => p.set('order', next))
  }

  const goToPage = (page: number) => {
    update((p) => {
      const newOffset = (page - 1) * pageSize
      if (newOffset > 0) p.set('offset', String(newOffset))
      else p.delete('offset')
    }, false)
    // Jump back to the top so the next page starts from the first row. Done here
    // (not in an offset effect) so it doesn't fight history scroll-restoration
    // when returning to the list from a book.
    window.scrollTo({ top: 0 })
  }

  const activeFacetCount =
    (authorId != null ? 1 : 0) + (seriesId != null ? 1 : 0) + tagIds.length + (hasPageFilter ? 1 : 0)

  // Close the filters dropdown on outside click / Escape.
  const filtersRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!filtersOpen) return
    // The mobile sheet is a full-screen portal outside filtersRef and closes via
    // its own button, so only wire outside-click for the desktop dropdown.
    const onPointer = (e: MouseEvent) => {
      if (!isMobile && filtersRef.current && !filtersRef.current.contains(e.target as Node))
        setFiltersOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setFiltersOpen(false)
    document.addEventListener('mousedown', onPointer)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onPointer)
      document.removeEventListener('keydown', onKey)
    }
  }, [filtersOpen, isMobile])

  const clearFacets = () => {
    setMinPages('')
    setMaxPages('')
    update((p) => {
      p.delete('author')
      p.delete('series')
      p.delete('tag')
      p.delete('minPages')
      p.delete('maxPages')
    })
  }

  const activeChips = [
    ...(authorId != null
      ? [{ kind: 'author' as const, id: authorId, name: authorNamesMap.get(authorId)?.name }]
      : []),
    ...(seriesId != null
      ? [{ kind: 'series' as const, id: seriesId, name: series.data?.find((f) => f.id === seriesId)?.name }]
      : []),
    ...tagIds.map((id) => ({ kind: 'tag' as const, id, name: tagNames.get(id)?.name })),
  ]

  const facetPanel = (
    <div className="space-y-5">
      <div className="grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-3">
        <AsyncFacet title={t('library.authors')} base="authors" fetcher={api.authors} activeIds={authorId != null ? [authorId] : []} onToggle={(id) => toggleFacet('author', id)} />
        <FacetFilter title={t('library.series')} kind="series" facets={series.data} activeIds={seriesId != null ? [seriesId] : []} onToggle={toggleFacet} searchFirst={isMobile} />
        <AsyncFacet title={t('library.tags')} base="tags" fetcher={api.tags} activeIds={tagIds} onToggle={(id) => toggleFacet('tag', id)} hiddenIds={hiddenTagIds} />
      </div>
      {pageFilterOn && (
        <div className="border-t border-ink-700 pt-4">
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
            {t('library.pages')}
          </h3>
          <div className="flex items-center gap-2">
            <input
              type="number"
              min="0"
              inputMode="numeric"
              className="input w-24 py-1.5 text-sm"
              placeholder={t('library.pagesMin')}
              value={minPages}
              onChange={(e) => setMinPages(e.target.value)}
            />
            <span className="text-slate-500">–</span>
            <input
              type="number"
              min="0"
              inputMode="numeric"
              className="input w-24 py-1.5 text-sm"
              placeholder={t('library.pagesMax')}
              value={maxPages}
              onChange={(e) => setMaxPages(e.target.value)}
            />
          </div>
        </div>
      )}
    </div>
  )

  // Filters button + dropdown, portaled into the header slot (right of search).
  const filtersNode = (
    <div className="relative" ref={filtersRef}>
      <button
        type="button"
        onClick={() => setFiltersOpen((v) => !v)}
        className={`btn-secondary ${filtersOpen || activeFacetCount > 0 ? 'border-accent-500/60 text-accentSoft' : ''}`}
        aria-expanded={filtersOpen}
      >
        <IconFilter width={16} height={16} />
        <span className="hidden sm:inline">{t('library.filters')}</span>
        {activeFacetCount > 0 && (
          <span className="ml-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-accent-600 px-1 text-[10px] font-semibold text-onaccent">
            {activeFacetCount}
          </span>
        )}
      </button>

      {/* Desktop: dropdown anchored under the button. */}
      {filtersOpen && !isMobile && (
        <div className="absolute right-0 z-40 mt-2 w-[min(92vw,44rem)] origin-top-right animate-fade-in rounded-2xl border border-ink-700 bg-ink-850 p-4 shadow-soft">
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
  )

  // Mobile: a full-screen sheet, portaled to <body> so the header's
  // backdrop-filter doesn't trap its fixed positioning. Closed via its own X.
  const mobileSheet =
    filtersOpen && isMobile
      ? createPortal(
          <div className="fixed inset-0 z-50 flex flex-col bg-ink-850 animate-fade-in">
            <div className="flex items-center justify-between border-b border-ink-700 p-4">
              <h2 className="text-base font-semibold text-white">{t('library.filters')}</h2>
              <div className="flex items-center gap-4">
                {activeFacetCount > 0 && (
                  <button
                    type="button"
                    onClick={clearFacets}
                    className="text-sm font-medium text-accentSoft hover:text-white"
                  >
                    {t('library.clearFilters')}
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => setFiltersOpen(false)}
                  className="text-slate-400 hover:text-white"
                  aria-label={t('common.close')}
                >
                  <IconClose width={24} height={24} />
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto overscroll-contain p-4">{facetPanel}</div>
          </div>,
          document.body,
        )
      : null

  return (
      <div className="min-w-0 flex-1">
        {filterSlot && createPortal(filtersNode, filterSlot)}
        {mobileSheet}
        {/* Header */}
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-white">
              {collection ? collection.name : t('library.title')}
            </h1>
            <p className="mt-0.5 text-sm text-slate-500">
              {isLoading
                ? t('common.loading')
                : grouped
                  ? t('library.worksCount', { count: total.toLocaleString() })
                  : t(total === 1 ? 'common.books_one' : 'common.books_other', {
                      count: total.toLocaleString(),
                    })}
            </p>
            {/* Collection base tags: a locked filter the page is scoped to. In "match
                any" mode they OR together, so chips are joined by an "or" hint. */}
            {collection && collection.tagIds.length > 0 && (
              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                <IconFilter width={13} height={13} className="shrink-0 text-slate-500" />
                {collection.tagIds.map((id, i) => (
                  <span key={id} className="flex items-center gap-1.5">
                    {i > 0 && matchAny && (
                      <span className="text-[10px] uppercase tracking-wide text-accentSoft/70">
                        {t('collections.or')}
                      </span>
                    )}
                    <span className="chip py-0.5 text-xs text-slate-300">
                      {tagNames.get(id)?.name ?? `#${id}`}
                    </span>
                  </span>
                ))}
              </div>
            )}
            {/* A collection's exclude tags: books carrying them are hidden. */}
            {collection && collection.excludeTagIds.length > 0 && (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <IconClose width={13} height={13} className="shrink-0 text-slate-500" />
                <span className="text-[10px] uppercase tracking-wide text-slate-500">
                  {t('collections.excludeBadge')}
                </span>
                {collection.excludeTagIds.map((id) => (
                  <span
                    key={id}
                    className="chip py-0.5 text-xs text-slate-400 line-through decoration-slate-600"
                  >
                    {tagNames.get(id)?.name ?? `#${id}`}
                  </span>
                ))}
              </div>
            )}
          </div>

          <div className="flex items-center gap-2">
            {canEdit && (
              <button
                type="button"
                onClick={() => (selectMode ? exitSelectMode() : setSelectMode(true))}
                className={`btn-secondary ${selectMode ? 'border-accent-500/60 text-accentSoft' : ''}`}
              >
                <IconCheck width={16} height={16} />
                <span className="hidden sm:inline">{selectMode ? t('select.exit') : t('select.enter')}</span>
              </button>
            )}
            {user?.canUpload && (
              <button type="button" onClick={() => setUploadOpen(true)} className="btn-primary">
                <IconUpload width={16} height={16} />
                <span className="hidden sm:inline">{t('library.upload')}</span>
              </button>
            )}
          </div>
        </div>

        {/* Continue reading: only on the library home, fresh & unfiltered. */}
        {!collection && !hasFilters && offset === 0 && <ContinueReadingShelf />}

        {/* Controls: active filter chips + sort (Filters button is in the header). */}
        <div className="mb-5 flex flex-wrap items-center gap-2">
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
          {hasPageFilter && (
            <button
              type="button"
              onClick={() => {
                setMinPages('')
                setMaxPages('')
              }}
              className="chip chip-active"
              title={t('library.clearFilters')}
            >
              <span>
                {t('library.pages')} {minPagesQ ?? ''}–{maxPagesQ ?? ''}
              </span>
              <IconClose width={12} height={12} className="shrink-0" />
            </button>
          )}

          <div className="flex w-full items-center gap-2 sm:ml-auto sm:w-auto">
            {sortLocked ? (
              // Collection with a pinned sort: the order is fixed, so show it as a
              // read-only badge instead of the interactive control.
              <span
                className="flex shrink-0 items-center gap-1.5 rounded-lg border border-ink-700 px-3 py-2 text-sm text-slate-400"
                title={t('library.sortLocked')}
              >
                {t(SORT_LABELS[sort])} {order === 'desc' ? '↓' : '↑'}
              </span>
            ) : (
              <>
                <select
                  value={userSort}
                  onChange={(e) => changeSort(e.target.value as SortKey)}
                  className="input min-w-0 flex-1 cursor-pointer py-2 pr-8 sm:w-auto sm:flex-none"
                >
                  {SORT_OPTIONS.filter((o) => sortAvailable(o.value)).map((o) => (
                    <option key={o.value} value={o.value}>
                      {t(o.labelKey)}
                    </option>
                  ))}
                </select>

                <button
                  type="button"
                  onClick={changeOrder}
                  className="btn-secondary shrink-0"
                  title={userOrder === 'desc' ? t('library.descending') : t('library.ascending')}
                >
                  {userOrder === 'desc' ? '↓' : '↑'}
                </button>
              </>
            )}
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
        ) : (grouped ? (groupedData?.units?.length ?? 0) : (flatData?.books?.length ?? 0)) > 0 ? (
          <>
            <div className={isFetching ? 'opacity-60 transition-opacity' : 'transition-opacity'}>
              <BookGrid>
                {grouped
                  ? (groupedData?.units ?? []).map((u) =>
                      u.kind === 'series' ? (
                        <LibrarySeriesCard key={`s${u.series.id}`} card={u.series} />
                      ) : (
                        <BookCard key={`b${u.book.id}`} book={u.book} />
                      ),
                    )
                  : (flatData?.books ?? []).map((book) => (
                      <BookCard
                        key={book.id}
                        book={book}
                        selectable={selectMode}
                        selected={selected.has(book.id)}
                        onToggleSelect={toggleSelect}
                      />
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

        {/* Spacer so the floating action bar never hides the last row. */}
        {selectMode && <div className="h-24" />}

        <UploadModal open={uploadOpen} onClose={() => setUploadOpen(false)} />

        {/* Floating bulk-action bar (select mode). */}
        {selectMode &&
          createPortal(
            <div className="fixed inset-x-0 bottom-0 z-40 flex justify-center px-3 pb-3 sm:pb-4">
              <div className="flex w-full max-w-3xl flex-wrap items-center gap-x-4 gap-y-2 rounded-2xl border border-ink-700 bg-ink-850/95 px-4 py-3 shadow-soft backdrop-blur">
                <span className="text-sm font-semibold text-white">
                  {t('select.count', { count: selected.size })}
                </span>
                <div className="flex items-center gap-3 text-xs">
                  <button
                    type="button"
                    className="font-medium text-accentSoft hover:text-white"
                    onClick={() =>
                      setSelected((prev) => {
                        const next = new Map(prev)
                        ;(flatData?.books ?? []).forEach((b) => next.set(b.id, b))
                        return next
                      })
                    }
                  >
                    {t('select.all')}
                  </button>
                  <button
                    type="button"
                    className="font-medium text-slate-400 hover:text-white disabled:opacity-40"
                    disabled={selected.size === 0}
                    onClick={clearSelection}
                  >
                    {t('select.none')}
                  </button>
                </div>
                <div className="ml-auto flex items-center gap-2">
                  <button
                    type="button"
                    className="btn-secondary"
                    disabled={selected.size === 0}
                    onClick={() => setCompareOpen(true)}
                  >
                    <IconSearch width={16} height={16} />
                    <span className="hidden sm:inline">{t('select.compare')}</span>
                  </button>
                  <button
                    type="button"
                    className="btn-secondary"
                    disabled={selected.size === 0}
                    onClick={() => setBulkEditOpen(true)}
                  >
                    <IconEdit width={16} height={16} />
                    <span className="hidden sm:inline">{t('select.edit')}</span>
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1.5 rounded-lg border border-red-500/40 px-3 py-2 text-sm font-medium text-red-300 transition-colors hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-40"
                    disabled={selected.size === 0}
                    onClick={() => setConfirmBulkDelete(true)}
                  >
                    <IconTrash width={16} height={16} />
                    <span className="hidden sm:inline">{t('select.delete')}</span>
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )}

        {bulkEditOpen && (
          <BulkEditModal
            books={selectedBooks}
            open={bulkEditOpen}
            onClose={(changed) => {
              setBulkEditOpen(false)
              if (changed) exitSelectMode()
            }}
          />
        )}

        {compareOpen && (
          <MetaCompareModal
            books={selectedBooks}
            open={compareOpen}
            onClose={(changed) => {
              setCompareOpen(false)
              if (changed) exitSelectMode()
            }}
          />
        )}

        <Modal
          open={confirmBulkDelete}
          onClose={() => !bulkDelete.isPending && setConfirmBulkDelete(false)}
          title={t('bulk.deleteTitle', { count: selected.size })}
        >
          <div className="space-y-4">
            <p className="text-sm text-slate-300">{t('bulk.deleteConfirm', { count: selected.size })}</p>
            <div className="flex items-center justify-end gap-3">
              {deleteProgress && (
                <span className="mr-auto text-xs text-slate-400">
                  {t('bulk.deleting', { done: deleteProgress.done, total: deleteProgress.total })}
                </span>
              )}
              <button
                type="button"
                className="btn-secondary"
                onClick={() => setConfirmBulkDelete(false)}
                disabled={bulkDelete.isPending}
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                className="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-red-500 disabled:opacity-50"
                onClick={() => bulkDelete.mutate()}
                disabled={bulkDelete.isPending}
              >
                {bulkDelete.isPending && <Spinner className="h-4 w-4" />}
                {t('bulk.deleteConfirmBtn', { count: selected.size })}
              </button>
            </div>
          </div>
        </Modal>
      </div>
  )
}
