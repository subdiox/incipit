import { useCallback } from 'react'
import { useLocalStorage } from './hooks'

/** Page-turn direction / binding. `rtl` = right-bound (manga), `ltr` = left-bound (western). */
export type ReaderDirection = 'rtl' | 'ltr'
/** Single page, or a two-page spread. */
export type ReaderLayout = 'single' | 'spread'
/** How a page is scaled to the viewport. */
export type ReaderFit = 'height' | 'width'

export interface ReaderSettings {
  direction: ReaderDirection
  layout: ReaderLayout
  fit: ReaderFit
  /** In spread layout, show the first page (cover) alone so later pairs align. */
  coverAlone: boolean
}

export const defaultReaderSettings: ReaderSettings = {
  direction: 'rtl',
  layout: 'single',
  fit: 'height',
  coverAlone: true,
}

const STORAGE_KEY = 'incipit.reader.settings'

/**
 * Reader settings persisted to localStorage (shared across books). Stored
 * values are merged over the defaults so newly-added fields keep working for
 * users who already have settings saved.
 */
export function useReaderSettings(): [
  ReaderSettings,
  (patch: Partial<ReaderSettings>) => void,
] {
  const [stored, setStored] = useLocalStorage<ReaderSettings>(
    STORAGE_KEY,
    defaultReaderSettings,
  )
  const settings: ReaderSettings = { ...defaultReaderSettings, ...stored }

  const update = useCallback(
    (patch: Partial<ReaderSettings>) => {
      setStored((prev) => ({ ...defaultReaderSettings, ...prev, ...patch }))
    },
    [setStored],
  )

  return [settings, update]
}

/**
 * Pages visible for a given anchor index, in ascending order (1 page in single
 * layout, 1–2 in spread). The anchor is normalised to the start of its spread,
 * so any index inside a pair resolves to the same view.
 */
export function visiblePages(
  anchor: number,
  total: number,
  settings: ReaderSettings,
): number[] {
  if (total <= 0) return []
  const a = Math.min(Math.max(anchor, 0), total - 1)
  if (settings.layout === 'single' || total === 1) return [a]

  let start: number
  if (settings.coverAlone) {
    if (a <= 0) return [0] // lone cover
    start = a - ((a - 1) % 2) // 1, 3, 5, …
  } else {
    start = a - (a % 2) // 0, 2, 4, …
  }
  const pages = [start]
  if (start + 1 < total) pages.push(start + 1)
  return pages
}
