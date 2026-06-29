import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

/** The admin-configurable site title, defaulting to "Incipit" while loading. */
export function useSiteTitle(): string {
  const { data } = useQuery({ queryKey: ['site'], queryFn: api.site, staleTime: 300_000 })
  return data?.title || 'Incipit'
}

/** Debounce a rapidly-changing value. */
export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}

/** Stable callback ref that always points at the latest function. */
export function useLatest<T>(value: T) {
  const ref = useRef(value)
  ref.current = value
  return ref
}

/**
 * Persist a JSON-serializable value in localStorage. The setter mirrors
 * useState (value or updater) and writes through on every change. Reads are
 * lazy so the stored value (if any) wins over the initial value on mount.
 */
export function useLocalStorage<T>(
  key: string,
  initial: T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key)
      return raw != null ? (JSON.parse(raw) as T) : initial
    } catch {
      return initial
    }
  })

  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const resolved =
          typeof next === 'function' ? (next as (p: T) => T)(prev) : next
        try {
          localStorage.setItem(key, JSON.stringify(resolved))
        } catch {
          // ignore quota / unavailable storage
        }
        return resolved
      })
    },
    [key],
  )

  return [value, set]
}
