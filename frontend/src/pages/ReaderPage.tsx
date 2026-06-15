import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, mediaUrl } from '@/lib/api'
import { Spinner } from '@/components/Spinner'
import {
  IconChevronLeft,
  IconChevronRight,
  IconClose,
  IconFitHeight,
  IconFitWidth,
} from '@/components/icons'

type Fit = 'width' | 'height'

export function ReaderPage() {
  const { id } = useParams()
  const bookId = Number(id)
  const navigate = useNavigate()

  const [page, setPage] = useState(0)
  const [fit, setFit] = useState<Fit>('height')
  const [chromeVisible, setChromeVisible] = useState(true)
  const [pageLoaded, setPageLoaded] = useState(false)
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

  // Restore reading position once, after both pages + progress have loaded.
  useEffect(() => {
    if (restoredRef.current || !pagesData) return
    if (progress === undefined) return // progress query still pending
    restoredRef.current = true
    if (progress && progress.page > 0 && progress.page < total) {
      setPage(progress.page)
    }
  }, [pagesData, progress, total])

  // Debounced progress save when the page changes.
  useEffect(() => {
    if (!total || !restoredRef.current) return
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      api.saveProgress(bookId, page, total).catch(() => {})
    }, 800)
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current)
    }
  }, [page, total, bookId])

  const go = useCallback(
    (delta: number) => {
      setPage((p) => {
        const next = p + delta
        if (next < 0 || next >= total) return p
        return next
      })
    },
    [total],
  )

  // Reset the per-page loaded state whenever the page changes.
  useEffect(() => {
    setPageLoaded(false)
  }, [page])

  // Preload the next 2 pages.
  useEffect(() => {
    if (!total) return
    for (let i = 1; i <= 2; i++) {
      const n = page + i
      if (n < total) {
        const img = new Image()
        img.src = mediaUrl.page(bookId, n)
      }
    }
  }, [page, total, bookId])

  // Keyboard navigation.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') {
        e.preventDefault()
        go(1)
      } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
        e.preventDefault()
        go(-1)
      } else if (e.key === 'Escape') {
        navigate(`/books/${bookId}`)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [go, navigate, bookId])

  // Auto-hide chrome after inactivity.
  const revealChrome = useCallback(() => {
    setChromeVisible(true)
    if (hideChromeTimer.current) clearTimeout(hideChromeTimer.current)
    hideChromeTimer.current = setTimeout(() => setChromeVisible(false), 2800)
  }, [])

  useEffect(() => {
    revealChrome()
    return () => {
      if (hideChromeTimer.current) clearTimeout(hideChromeTimer.current)
    }
  }, [revealChrome])

  const fitClass = useMemo(
    () => (fit === 'height' ? 'max-h-full w-auto' : 'w-full h-auto max-w-[1100px]'),
    [fit],
  )

  if (isLoading)
    return (
      <div className="flex h-screen items-center justify-center bg-black">
        <Spinner className="h-8 w-8 text-accent-400" />
      </div>
    )

  if (isError || total === 0)
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4 bg-black text-center">
        <p className="text-slate-400">Unable to load pages for this book.</p>
        <button className="btn-secondary" onClick={() => navigate(`/books/${bookId}`)}>
          Back to details
        </button>
      </div>
    )

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
          aria-label="Close reader"
        >
          <IconClose />
        </button>
        <div className="pointer-events-auto rounded-full bg-black/40 px-4 py-1.5 text-sm font-medium text-slate-200 backdrop-blur">
          {page + 1} / {total}
        </div>
        <button
          type="button"
          onClick={() => setFit((f) => (f === 'height' ? 'width' : 'height'))}
          className="pointer-events-auto rounded-lg bg-black/40 p-2 text-slate-200 backdrop-blur transition-colors hover:bg-black/70 hover:text-white"
          aria-label="Toggle fit"
          title={fit === 'height' ? 'Fit width' : 'Fit height'}
        >
          {fit === 'height' ? <IconFitWidth /> : <IconFitHeight />}
        </button>
      </div>

      {/* Page image + click zones */}
      <div className="flex h-full w-full items-center justify-center overflow-auto">
        {!pageLoaded && (
          <div className="absolute inset-0 flex items-center justify-center">
            <Spinner className="h-7 w-7 text-accent-400" />
          </div>
        )}
        <img
          key={page}
          src={mediaUrl.page(bookId, page)}
          alt={`Page ${page + 1}`}
          onLoad={() => setPageLoaded(true)}
          className={`mx-auto object-contain transition-opacity duration-200 ${fitClass} ${
            pageLoaded ? 'opacity-100' : 'opacity-0'
          }`}
          draggable={false}
        />
      </div>

      {/* Click navigation halves */}
      <button
        type="button"
        onClick={() => go(-1)}
        disabled={page <= 0}
        className="absolute inset-y-0 left-0 z-10 w-[35%] cursor-w-resize disabled:cursor-default"
        aria-label="Previous page"
      />
      <button
        type="button"
        onClick={() => go(1)}
        disabled={page >= total - 1}
        className="absolute inset-y-0 right-0 z-10 w-[35%] cursor-e-resize disabled:cursor-default"
        aria-label="Next page"
      />

      {/* On-screen prev/next */}
      <div
        className={`pointer-events-none absolute inset-y-0 left-0 z-20 flex items-center pl-3 transition-opacity duration-300 ${
          chromeVisible && page > 0 ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <button
          type="button"
          onClick={() => go(-1)}
          className="pointer-events-auto rounded-full bg-black/50 p-3 text-white backdrop-blur transition-colors hover:bg-black/80"
          aria-label="Previous page"
        >
          <IconChevronLeft width={24} height={24} />
        </button>
      </div>
      <div
        className={`pointer-events-none absolute inset-y-0 right-0 z-20 flex items-center pr-3 transition-opacity duration-300 ${
          chromeVisible && page < total - 1 ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <button
          type="button"
          onClick={() => go(1)}
          className="pointer-events-auto rounded-full bg-black/50 p-3 text-white backdrop-blur transition-colors hover:bg-black/80"
          aria-label="Next page"
        >
          <IconChevronRight width={24} height={24} />
        </button>
      </div>

      {/* Bottom progress bar */}
      <div
        className={`absolute inset-x-0 bottom-0 z-20 h-1 bg-ink-800 transition-opacity duration-300 ${
          chromeVisible ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <div
          className="h-full bg-accent-500 transition-all"
          style={{ width: `${((page + 1) / total) * 100}%` }}
        />
      </div>
    </div>
  )
}
