import { useEffect, useMemo, useSyncExternalStore } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Facet } from '@/types'
import type { FacetParams } from '@/lib/api'

type Base = 'tags' | 'authors'

// A process-wide id→name cache for large facets. Every search result / lookup
// feeds it (via rememberFacets), so a chip for a tag the user just picked from
// the list renders its name instantly — no extra round-trip — and only ids we've
// genuinely never seen (e.g. a deep-linked ?tag=5 on a cold load) hit the server.
const names = new Map<string, string>()
let version = 0
const listeners = new Set<() => void>()
const keyOf = (base: Base, id: number) => `${base}:${id}`

export function rememberFacets(base: Base, facets: { id: number; name: string }[]): void {
  let changed = false
  for (const f of facets) {
    const k = keyOf(base, f.id)
    if (names.get(k) !== f.name) {
      names.set(k, f.name)
      changed = true
    }
  }
  if (changed) {
    version++
    listeners.forEach((l) => l())
  }
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}
const snapshot = () => version

// useFacetNames resolves facet IDs to their {id,name} without downloading the
// whole (possibly 100k-row) list: names already in the shared cache resolve
// synchronously; only the remaining unknown IDs are fetched from the server.
export function useFacetNames(
  base: Base,
  fetcher: (p: FacetParams) => Promise<Facet[]>,
  ids: number[],
): Map<number, Facet> {
  const v = useSyncExternalStore(subscribe, snapshot, snapshot)
  const uniq = useMemo(() => Array.from(new Set(ids)).sort((a, b) => a - b), [ids])

  const missing = uniq.filter((id) => !names.has(keyOf(base, id)))
  const { data } = useQuery({
    queryKey: ['facets', base, 'names', missing.join(',')],
    queryFn: () => fetcher({ ids: missing }),
    enabled: missing.length > 0,
    staleTime: 300_000,
  })
  useEffect(() => {
    if (data) rememberFacets(base, data)
  }, [base, data])

  return useMemo(() => {
    const m = new Map<number, Facet>()
    for (const id of uniq) {
      const n = names.get(keyOf(base, id))
      if (n != null) m.set(id, { id, name: n, count: 0 })
    }
    return m
    // v (from the external store) rebuilds the map whenever the cache gains
    // entries; base/uniq cover the inputs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uniq, base, v])
}
