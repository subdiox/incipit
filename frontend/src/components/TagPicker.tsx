import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { IconClose, IconSearch } from './icons'

const LIMIT = 50

// TagPicker is a searchable multi-select over the library's tags, returning the
// selected Calibre tag IDs. Used to define a pane's filter.
export function TagPicker({ value, onChange }: { value: number[]; onChange: (ids: number[]) => void }) {
  const { t } = useI18n()
  const tags = useQuery({ queryKey: ['facets', 'tags'], queryFn: api.tags, staleTime: 300_000 }).data ?? []
  const [query, setQuery] = useState('')

  const selected = useMemo(() => new Set(value), [value])
  const byId = useMemo(() => new Map(tags.map((f) => [f.id, f])), [tags])

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const matched = needle ? tags.filter((f) => f.name.toLowerCase().includes(needle)) : tags
    return [...matched]
      .sort((a, b) => {
        const sa = selected.has(a.id)
        const sb = selected.has(b.id)
        if (sa !== sb) return sa ? -1 : 1
        return b.count - a.count || a.name.localeCompare(b.name)
      })
      .slice(0, LIMIT)
  }, [tags, query, selected])

  const toggle = (id: number) =>
    onChange(selected.has(id) ? value.filter((v) => v !== id) : [...value, id])

  return (
    <div>
      {value.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {value.map((id) => (
            <button key={id} type="button" onClick={() => toggle(id)} className="chip chip-active">
              <span className="truncate">{byId.get(id)?.name ?? `#${id}`}</span>
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

      {rows.length === 0 ? (
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
