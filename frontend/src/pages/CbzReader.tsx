import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from '@/components/Spinner'
import {
  useReaderSettings,
  visiblePages,
  type ReaderSettings,
} from '@/lib/readerSettings'
import {
  IconChevronLeft,
  IconChevronRight,
  IconClose,
  IconFitHeight,
  IconFitWidth,
  IconSettings,
  IconSinglePage,
  IconSpread,
} from '@/components/icons'

export function CbzReader({ bookId }: { bookId: number }) {
  const navigate = useNavigate()
  const { t } = useI18n()

  const [settings, updateSettings] = useReaderSettings()
  const [page, setPage] = useState(0)
  const [chromeVisible, setChromeVisible] = useState(true)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [loadedPages, setLoadedPages] = useState<Record<number, boolean>>({})
  const restoredRef = useRef(false)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const hideChromeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const { data: pagesData, isLoading, isError } = useQuery({
    queryKey: ['pages', bookId],
    queryFn: () => api.pages(bookId),
    enabled: Number.isFinite(bookId),
  })

  const { data: progress } = useQuery({
    queryKey: ['progress', bookId],
    queryFn: () => api.progress(bookId).catch(() => null),
    enabled: Number.isFinite(bookId),
    staleTime: Infinity,
  })

  const total = pagesData?.count ?? 0

  // The pages currently on screen (1 in single layout, 1–2 in spread), in
  // ascending order regardless of binding direction.
  const view = useMemo(
    () => visiblePages(page, total, settings),
    [page, total, settings],
  )
  const first = view[0] ?? 0
  const last = view[view.length - 1] ?? 0
  const isSpread = view.length > 1

  // Keep the anchor normalised to the start of its spread so toggling layout /
  // saving progress always works off the leading page index.
  useEffect(() => {
    if (total && page !== first) setPage(first)
  }, [page, first, total])

  const forward = useCallback(() => {
    setPage((p) => {
      const v = visiblePages(p, total, settings)
      const end = v[v.length - 1] ?? p
      return end < total - 1 ? end + 1 : p
    })
  }, [total, settings])

  const backward = useCallback(() => {
    setPage((p) => {
      const v = visiblePages(p, total, settings)
      const start = v[0] ?? p
      return start > 0 ? start - 1 : p
    })
  }, [total, settings])

  // Physical left/right depend on binding direction: right-bound (manga) turns
  // pages leftward, so the left side advances.
  const rtl = settings.direction === 'rtl'
  const onLeft = rtl ? forward : backward
  const onRight = rtl ? backward : forward
  const canForward = last < total - 1
  const canBackward = first > 0
  const canLeft = rtl ? canForward : canBackward
  const canRight = rtl ? canBackward : canForward

  // Restore reading position once, after both pages + progress have loaded.
  useEffect(() => {
    if (restoredRef.current || !pagesData) return
    if (progress === undefined) return // progress query still pending
    restoredRef.current = true
    if (progress && progress.page > 0 && progress.page < total) {
      setPage(progress.page)
    }
  }, [pagesData, progress, total])

  // Debounced progress save when the leading page changes.
  useEffect(() => {
    if (!total || !restoredRef.current) return
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      api.saveProgress(bookId, first, total).catch(() => {})
    }, 800)
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current)
    }
  }, [first, total, bookId])

  // Reset per-page loaded state whenever the visible set changes.
  const viewKey = view.join('-')
  useEffect(() => {
    setLoadedPages({})
  }, [viewKey])
  const markLoaded = useCallback((n: number) => {
    setLoadedPages((prev) => (prev[n] ? prev : { ...prev, [n]: true }))
  }, [])
  const allLoaded = view.every((n) => loadedPages[n])

  // Preload the next few pages in reading order.
  useEffect(() => {
    if (!total) return
    for (let i = 1; i <= 3; i++) {
      const n = last + i
      if (n < total) {
        const img = new Image()
        img.src = mediaUrl.page(bookId, n)
      }
    }
  }, [last, total, bookId])

  // Keyboard navigation (reading order for space/page keys; physical for arrows).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (settingsOpen) setSettingsOpen(false)
        else navigate(`/books/${bookId}`)
        return
      }
      if (e.key === 'ArrowRight') {
        e.preventDefault()
        onRight()
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault()
        onLeft()
      } else if (e.key === ' ' || e.key === 'PageDown') {
        e.preventDefault()
        forward()
      } else if (e.key === 'PageUp') {
        e.preventDefault()
        backward()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onLeft, onRight, forward, backward, navigate, bookId, settingsOpen])

  // Auto-hide chrome after inactivity (kept open while the settings panel is).
  const revealChrome = useCallback(() => {
    setChromeVisible(true)
    if (hideChromeTimer.current) clearTimeout(hideChromeTimer.current)
    if (!settingsOpen) {
      hideChromeTimer.current = setTimeout(() => setChromeVisible(false), 2800)
    }
  }, [settingsOpen])

  useEffect(() => {
    revealChrome()
    return () => {
      if (hideChromeTimer.current) clearTimeout(hideChromeTimer.current)
    }
  }, [revealChrome])

  // Display order: right-bound shows the higher page number on the left.
  const displayPages = rtl ? [...view].reverse() : view

  const imgClass = useMemo(() => {
    if (!isSpread) {
      return settings.fit === 'height'
        ? 'max-h-full w-auto'
        : 'h-auto w-full max-w-[1100px]'
    }
    // Spread: fit each half so the pair always fits the viewport height; width
    // mode fills the width and may scroll vertically.
    return settings.fit === 'height'
      ? 'max-h-full w-auto max-w-[50%] object-contain'
      : 'h-auto w-1/2 object-contain'
  }, [isSpread, settings.fit])

  if (isLoading)
    return (
      <div className="flex h-screen items-center justify-center bg-black">
        <Spinner className="h-8 w-8 text-accent-400" />
      </div>
    )

  if (isError || total === 0)
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4 bg-black text-center">
        <p className="text-slate-400">{t('reader.unableToLoad')}</p>
        <button className="btn-secondary" onClick={() => navigate(`/books/${bookId}`)}>
          {t('reader.backToDetails')}
        </button>
      </div>
    )

  const pageLabel = isSpread
    ? `${first + 1}–${last + 1} / ${total}`
    : `${first + 1} / ${total}`

  return (
    <div
      className="relative h-screen w-screen select-none overflow-hidden bg-black"
      onMouseMove={revealChrome}
    >
      {/* Top bar */}
      <div
        className={`pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between bg-gradient-to-b from-black/80 to-transparent px-4 py-3 transition-opacity duration-300 ${
          chromeVisible ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <button
          type="button"
          onClick={() => navigate(`/books/${bookId}`)}
          className="pointer-events-auto rounded-lg bg-black/40 p-2 text-slate-200 backdrop-blur transition-colors hover:bg-black/70 hover:text-white"
          aria-label={t('reader.closeReader')}
        >
          <IconClose />
        </button>
        <div className="pointer-events-auto rounded-full bg-black/40 px-4 py-1.5 text-sm font-medium text-slate-200 backdrop-blur">
          {pageLabel}
        </div>
        <button
          type="button"
          onClick={() => {
            setSettingsOpen((o) => !o)
            setChromeVisible(true)
          }}
          className={`pointer-events-auto rounded-lg p-2 backdrop-blur transition-colors ${
            settingsOpen
              ? 'bg-accent-600 text-white'
              : 'bg-black/40 text-slate-200 hover:bg-black/70 hover:text-white'
          }`}
          aria-label={t('reader.settings')}
          title={t('reader.settings')}
        >
          <IconSettings />
        </button>
      </div>

      {/* Settings panel + click-away backdrop */}
      {settingsOpen && (
        <>
          <button
            type="button"
            className="absolute inset-0 z-30 cursor-default"
            aria-label={t('reader.closeSettings')}
            onClick={() => setSettingsOpen(false)}
          />
          <ReaderSettingsPanel settings={settings} onChange={updateSettings} />
        </>
      )}

      {/* Page image(s) */}
      <div className="flex h-full w-full items-center justify-center overflow-auto">
        {!allLoaded && (
          <div className="absolute inset-0 flex items-center justify-center">
            <Spinner className="h-7 w-7 text-accent-400" />
          </div>
        )}
        {displayPages.map((n) => (
          <img
            key={n}
            src={mediaUrl.page(bookId, n)}
            alt={`Page ${n + 1}`}
            onLoad={() => markLoaded(n)}
            className={`object-contain transition-opacity duration-200 ${imgClass} ${
              loadedPages[n] ? 'opacity-100' : 'opacity-0'
            }`}
            draggable={false}
          />
        ))}
      </div>

      {/* Click navigation halves */}
      <button
        type="button"
        onClick={onLeft}
        disabled={!canLeft}
        className="absolute inset-y-0 left-0 z-10 w-[35%] cursor-w-resize disabled:cursor-default"
        aria-label={rtl ? t('reader.nextPage') : t('reader.prevPage')}
      />
      <button
        type="button"
        onClick={onRight}
        disabled={!canRight}
        className="absolute inset-y-0 right-0 z-10 w-[35%] cursor-e-resize disabled:cursor-default"
        aria-label={rtl ? t('reader.prevPage') : t('reader.nextPage')}
      />

      {/* On-screen prev/next (physical sides) */}
      <div
        className={`pointer-events-none absolute inset-y-0 left-0 z-20 flex items-center pl-3 transition-opacity duration-300 ${
          chromeVisible && canLeft ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <button
          type="button"
          onClick={onLeft}
          className="pointer-events-auto rounded-full bg-black/50 p-3 text-white backdrop-blur transition-colors hover:bg-black/80"
          aria-label={rtl ? t('reader.nextPage') : t('reader.prevPage')}
        >
          <IconChevronLeft width={24} height={24} />
        </button>
      </div>
      <div
        className={`pointer-events-none absolute inset-y-0 right-0 z-20 flex items-center pr-3 transition-opacity duration-300 ${
          chromeVisible && canRight ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <button
          type="button"
          onClick={onRight}
          className="pointer-events-auto rounded-full bg-black/50 p-3 text-white backdrop-blur transition-colors hover:bg-black/80"
          aria-label={rtl ? t('reader.prevPage') : t('reader.nextPage')}
        >
          <IconChevronRight width={24} height={24} />
        </button>
      </div>

      {/* Bottom progress bar (fills in reading direction) */}
      <div
        className={`absolute inset-x-0 bottom-0 z-20 h-1 bg-ink-800 transition-opacity duration-300 ${
          chromeVisible ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <div
          className={`h-full bg-accent-500 transition-all ${rtl ? 'ml-auto' : ''}`}
          style={{ width: `${((last + 1) / total) * 100}%` }}
        />
      </div>
    </div>
  )
}

// ---- Settings panel ----

function ReaderSettingsPanel({
  settings,
  onChange,
}: {
  settings: ReaderSettings
  onChange: (patch: Partial<ReaderSettings>) => void
}) {
  const { t } = useI18n()
  return (
    <div className="absolute right-3 top-16 z-40 w-72 animate-fade-in rounded-2xl border border-ink-700 bg-ink-900/95 p-4 text-slate-200 shadow-soft backdrop-blur">
      <div className="space-y-4">
        <Field label={t('reader.binding')}>
          <Seg active={settings.direction === 'rtl'} onClick={() => onChange({ direction: 'rtl' })}>
            {t('reader.bindingRight')}
          </Seg>
          <Seg active={settings.direction === 'ltr'} onClick={() => onChange({ direction: 'ltr' })}>
            {t('reader.bindingLeft')}
          </Seg>
        </Field>

        <Field label={t('reader.layout')}>
          <Seg
            active={settings.layout === 'single'}
            onClick={() => onChange({ layout: 'single' })}
            icon={<IconSinglePage width={18} height={18} />}
          >
            {t('reader.layoutSingle')}
          </Seg>
          <Seg
            active={settings.layout === 'spread'}
            onClick={() => onChange({ layout: 'spread' })}
            icon={<IconSpread width={18} height={18} />}
          >
            {t('reader.layoutSpread')}
          </Seg>
        </Field>

        {settings.layout === 'spread' && (
          <button
            type="button"
            onClick={() => onChange({ coverAlone: !settings.coverAlone })}
            className="flex w-full items-center justify-between rounded-lg bg-ink-800 px-3 py-2 text-sm text-slate-300 transition-colors hover:bg-ink-700"
          >
            <span>{t('reader.coverAlone')}</span>
            <span
              className={`flex h-5 w-9 items-center rounded-full px-0.5 transition-colors ${
                settings.coverAlone ? 'bg-accent-600' : 'bg-ink-600'
              }`}
            >
              <span
                className={`h-4 w-4 rounded-full bg-white shadow transition-transform ${
                  settings.coverAlone ? 'translate-x-4' : ''
                }`}
              />
            </span>
          </button>
        )}

        <Field label={t('reader.fit')}>
          <Seg
            active={settings.fit === 'height'}
            onClick={() => onChange({ fit: 'height' })}
            icon={<IconFitHeight width={18} height={18} />}
          >
            {t('reader.fitHeight')}
          </Seg>
          <Seg
            active={settings.fit === 'width'}
            onClick={() => onChange({ fit: 'width' })}
            icon={<IconFitWidth width={18} height={18} />}
          >
            {t('reader.fitWidth')}
          </Seg>
        </Field>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-400">
        {label}
      </div>
      <div className="flex gap-1.5">{children}</div>
    </div>
  )
}

function Seg({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean
  onClick: () => void
  icon?: ReactNode
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
        active
          ? 'bg-accent-600 text-white'
          : 'bg-ink-800 text-slate-300 hover:bg-ink-700 hover:text-white'
      }`}
    >
      {icon}
      {children}
    </button>
  )
}
