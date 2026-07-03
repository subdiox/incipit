import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Facet } from '@/types'
import type { FacetParams } from '@/lib/api'

// useFacetNames resolves a set of facet IDs to their {id,name,count} via the
// server, so already-selected chips (filters, collection base tags, etc.) can be
// labelled without downloading the whole — possibly 100k-row — facet list.
export function useFacetNames(
  base: 'tags' | 'authors',
  fetcher: (p: FacetParams) => Promise<Facet[]>,
  ids: number[],
): Map<number, Facet> {
  const uniq = useMemo(() => Array.from(new Set(ids)).sort((a, b) => a - b), [ids])
  const key = uniq.join(',')
  const { data } = useQuery({
    queryKey: ['facets', base, 'names', key],
    queryFn: () => fetcher({ ids: uniq }),
    enabled: uniq.length > 0,
    staleTime: 300_000,
  })
  return useMemo(() => new Map((data ?? []).map((f) => [f.id, f])), [data])
}
