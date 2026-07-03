import { useEffect, useMemo, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { Facet } from '@/types'
import type { FacetParams } from '@/lib/api'
import { useI18n } from '@/i18n'
import { useDebounced } from '@/lib/hooks'
import { useFacetNames, rememberFacets } from '@/lib/facets'
import { IconClose, IconSearch } from './icons'
import { Spinner } from './Spinner'

const LIMIT = 40

// AsyncFacet is the searchable filter for very large categories (tags, authors):
// it shows the most-used entries by default and queries the server as you type,
// instead of downloading the whole (possibly 100k-row) list. Selected IDs are
// resolved to names so their chips render even when off the current list.
export function AsyncFacet({
  title,
  base,
  fetcher,
  activeIds,
  onToggle,
  hiddenIds = [],
}: {
  title: string
  base: 'tags' | 'authors'
  fetcher: (p: FacetParams) => Promise<Facet[]>
  activeIds: number[]
  onToggle: (id: number) => void
  hiddenIds?: number[]
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')
  const debounced = useDebounced(query.trim(), 250)

  const hidden = useMemo(() => new Set(hiddenIds), [hiddenIds])
  const activeSet = useMemo(() => new Set(activeIds), [activeIds])

  const results = useQuery({
    queryKey: ['facets', base, 'search', debounced],
    queryFn: () => fetcher({ q: debounced || undefined, limit: LIMIT }),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })

  // Feed every result into the shared name cache so a just-picked chip resolves
  // instantly (no extra lookup round-trip).
  useEffect(() => {
    if (results.data) rememberFacets(base, results.data)
  }, [base, results.data])

  const names = useFacetNames(base, fetcher, activeIds)
  const actives = activeIds.map((id) => names.get(id) ?? { id, name: '…', count: 0 })
  const rows = (results.data ?? []).filter((f) => !hidden.has(f.id))

  return (
    <div>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{title}</h3>

      {actives.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {actives.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => onToggle(a.id)}
              className="chip chip-active max-w-full"
              title={t('library.clearFilters')}
            >
              <span className="truncate">{a.name}</span>
              <IconClose width={12} height={12} className="shrink-0" />
            </button>
          ))}
        </div>
      )}

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

      {results.isLoading ? (
        <div className="flex justify-center py-4">
          <Spinner className="h-4 w-4 text-accent-400" />
        </div>
      ) : rows.length === 0 ? (
        <p className="px-1 py-1.5 text-xs text-slate-600">{t('library.facetNoMatch')}</p>
      ) : (
        <ul className="-mr-1 max-h-60 space-y-0.5 overflow-y-auto pr-1">
          {rows.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                onClick={() => onToggle(f.id)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1 text-left text-sm transition-colors ${
                  activeSet.has(f.id) ? 'bg-accent-500/15 text-accentSoft' : 'text-slate-300 hover:bg-ink-800'
                }`}
              >
                <span className="truncate">{f.name}</span>
                <span className="shrink-0 text-[10px] tabular-nums text-slate-500">{f.count}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
