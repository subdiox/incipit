import { useEffect, useMemo, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { useDebounced } from '@/lib/hooks'
import { useFacetNames, rememberFacets } from '@/lib/facets'
import { IconClose, IconSearch } from './icons'
import { Spinner } from './Spinner'

const LIMIT = 40

// TagPicker is a searchable multi-select over the library's tags, returning the
// selected Calibre tag IDs. Tags are searched server-side (a library can have
// 100k+), showing the most-used by default; selected IDs are resolved to names.
export function TagPicker({ value, onChange }: { value: number[]; onChange: (ids: number[]) => void }) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')
  const debounced = useDebounced(query.trim(), 250)

  const results = useQuery({
    queryKey: ['facets', 'tags', 'search', debounced],
    queryFn: () => api.tags({ q: debounced || undefined, limit: LIMIT }),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })
  useEffect(() => {
    if (results.data) rememberFacets('tags', results.data)
  }, [results.data])

  const names = useFacetNames('tags', api.tags, value)

  const selected = useMemo(() => new Set(value), [value])
  const toggle = (id: number) =>
    onChange(selected.has(id) ? value.filter((v) => v !== id) : [...value, id])
  const rows = results.data ?? []

  return (
    <div>
      {value.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {value.map((id) => (
            <button key={id} type="button" onClick={() => toggle(id)} className="chip chip-active">
              <span className="truncate">{names.get(id)?.name ?? `#${id}`}</span>
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
        <ul className="-mr-1 max-h-56 space-y-0.5 overflow-y-auto pr-1">
          {rows.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                onClick={() => toggle(f.id)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1 text-left text-sm transition-colors ${
                  selected.has(f.id) ? 'bg-accent-500/15 text-accentSoft' : 'text-slate-300 hover:bg-ink-800'
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
