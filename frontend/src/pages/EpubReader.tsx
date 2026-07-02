import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from '@/components/Spinner'
import { IconChevronLeft, IconChevronRight, IconClose, IconSettings } from '@/components/icons'

const FONT_KEY = 'incipit.epub.fontSize'
const FLOW_KEY = 'incipit.epub.flow'
type Flow = 'paginated' | 'scrolled'

// foliate-view element (from the vendored foliate-js). Typed loosely — it's a
// custom element with an imperative API.
type FoliateView = HTMLElement & {
  open: (file: File | Blob) => Promise<void>
  close: () => void
  goLeft: () => void
  goRight: () => void
  goTo: (target: string) => Promise<unknown>
  renderer: {
    setAttribute: (k: string, v: string) => void
    setStyles?: (css: string) => void
    next: () => Promise<void>
  }
  addEventListener: (t: string, cb: (e: CustomEvent) => void) => void
}

// Colour theme injected into each section document. Deliberately does not touch
// writing-mode/direction so vertical (tategaki) and rtl layout are preserved —
// foliate's paginator handles those correctly.
const themeCSS = (fontPercent: number) => `
  /* Disable mobile text auto-inflation ("font boosting"), which resizes text
     per-paragraph on phones and makes lines look different sizes — the book is
     uniform on desktop where inflation is off. */
  html { color-scheme: dark; -webkit-text-size-adjust: 100%; text-size-adjust: 100%; }
  html, body { background: #0b0b0f !important; color: #cbd5e1 !important; }
  body { font-size: ${fontPercent}% !important; }
  a, a:link, a:visited { color: #9384f2 !important; }
  img, svg { max-width: 100%; max-height: 100%; }
`

export function EpubReader({ bookId, title }: { bookId: number; title: string }) {
  const navigate = useNavigate()
  const { t } = useI18n()
  const hostRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<FoliateView | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [percent, setPercent] = useState(0)
  const [fontSize, setFontSize] = useState<number>(() => {
    const v = Number(localStorage.getItem(FONT_KEY))
    return v >= 80 && v <= 220 ? v : 100
  })
  const [flow, setFlowState] = useState<Flow>(() =>
    localStorage.getItem(FLOW_KEY) === 'scrolled' ? 'scrolled' : 'paginated',
  )
  const [settingsOpen, setSettingsOpen] = useState(false)
  // The paged/scroll setting (and its full-area gesture layer) is phone-only;
  // on larger screens we keep the original behavior: paginated with the middle
  // free for text selection and left/right zones for turning pages.
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(max-width: 639px)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 639px)')
    const on = () => setIsMobile(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  const effectiveFlow: Flow = isMobile ? flow : 'paginated'

  const locKey = `incipit.epub.loc.${bookId}`

  // Lock document scroll while the reader is open so the viewport can't drift or
  // rubber-band up/down on mobile (mirrors the CBZ reader).
  useEffect(() => {
    const html = document.documentElement
    const prevOverflow = document.body.style.overflow
    const prevOverscroll = html.style.overscrollBehavior
    document.body.style.overflow = 'hidden'
    html.style.overscrollBehavior = 'none'
    return () => {
      document.body.style.overflow = prevOverflow
      html.style.overscrollBehavior = prevOverscroll
    }
  }, [])

  // Physical sides; foliate maps them to prev/next per the book's direction.
  const goLeft = useCallback(() => viewRef.current?.goLeft(), [])
  const goRight = useCallback(() => viewRef.current?.goRight(), [])

  // Paged mode gestures over the whole area (covering foliate's own touch
  // handling, which would otherwise turn the page on a vertical swipe of a
  // vertical-writing book). A horizontal swipe turns the page, a tap turns by
  // the half tapped, and vertical swipes are ignored — no up/down scroll here.
  const swipeStart = useRef<{ x: number; y: number } | null>(null)
  const pageGestures = {
    onPointerDown: (e: React.PointerEvent) => {
      swipeStart.current = { x: e.clientX, y: e.clientY }
      e.currentTarget.setPointerCapture?.(e.pointerId)
    },
    onPointerUp: (e: React.PointerEvent) => {
      const s = swipeStart.current
      swipeStart.current = null
      if (!s) return
      const dx = e.clientX - s.x
      const dy = e.clientY - s.y
      if (Math.abs(dx) >= 40 && Math.abs(dx) > Math.abs(dy)) {
        if (dx < 0) goRight()
        else goLeft()
      } else if (Math.abs(dx) < 10 && Math.abs(dy) < 10) {
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
        if (e.clientX - rect.left < rect.width / 2) goLeft()
        else goRight()
      }
      // vertical swipe: intentionally ignored (paged mode has no scroll)
    },
  }

  const onKey = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') goLeft()
      else if (e.key === 'ArrowRight') goRight()
      else if (e.key === 'Escape') navigate(`/books/${bookId}`)
    },
    [goLeft, goRight, navigate, bookId],
  )

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch(mediaUrl.content(bookId), { credentials: 'include' })
        if (!res.ok) throw new Error('fetch failed')
        const blob = await res.blob()
        if (cancelled || !hostRef.current) return

        // Importing view.js registers the <foliate-view> custom element.
        await import('@/vendor/foliate-js/view.js')
        if (cancelled || !hostRef.current) return

        const view = document.createElement('foliate-view') as FoliateView
        view.style.width = '100%'
        view.style.height = '100%'
        hostRef.current.append(view)
        viewRef.current = view

        const file = new File([blob], 'book.epub', { type: 'application/epub+zip' })
        await view.open(file)
        if (cancelled) return

        view.renderer.setAttribute('flow', effectiveFlow)
        view.renderer.setStyles?.(themeCSS(fontSize))

        view.addEventListener('relocate', (e: CustomEvent) => {
          const d = e.detail as { fraction?: number; cfi?: string }
          if (typeof d.fraction === 'number') setPercent(Math.round(d.fraction * 100))
          if (d.cfi) localStorage.setItem(locKey, d.cfi)
        })
        // Forward key presses from inside each section iframe.
        view.addEventListener('load', (e: CustomEvent) => {
          const doc = (e.detail as { doc?: Document }).doc
          doc?.addEventListener('keyup', onKey as unknown as EventListener)
        })

        const saved = localStorage.getItem(locKey)
        if (saved) {
          try {
            await view.goTo(saved)
          } catch {
            await view.renderer.next()
          }
        } else {
          await view.renderer.next()
        }
        if (!cancelled) setLoading(false)
      } catch {
        if (!cancelled) {
          setError(true)
          setLoading(false)
        }
      }
    })()

    return () => {
      cancelled = true
      try {
        viewRef.current?.close()
      } catch {
        /* ignore */
      }
      viewRef.current = null
      if (hostRef.current) hostRef.current.replaceChildren()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookId])

  useEffect(() => {
    window.addEventListener('keyup', onKey)
    return () => window.removeEventListener('keyup', onKey)
  }, [onKey])

  const changeFont = (delta: number) => {
    setFontSize((cur) => {
      const nv = Math.min(220, Math.max(80, cur + delta))
      localStorage.setItem(FONT_KEY, String(nv))
      viewRef.current?.renderer.setStyles?.(themeCSS(nv))
      return nv
    })
  }

  // Phone-only paged/scroll toggle; the effect below applies it to the view.
  const setFlow = (next: Flow) => {
    setFlowState(next)
    localStorage.setItem(FLOW_KEY, next)
  }

  // Apply the effective flow to the open renderer and restore position. Runs on
  // toggle and when crossing the mobile breakpoint; the initial open sets it
  // first (this no-ops until the view exists).
  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.renderer.setAttribute('flow', effectiveFlow)
    view.renderer.setStyles?.(themeCSS(fontSize))
    const cfi = localStorage.getItem(locKey)
    if (cfi) view.goTo(cfi).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [effectiveFlow])

  return (
    <div className="dark fixed inset-0 flex flex-col overflow-hidden overscroll-none bg-ink-950">
      <div className="flex items-center gap-3 border-b border-ink-800 bg-ink-900 px-3 py-2">
        <button
          type="button"
          onClick={() => navigate(`/books/${bookId}`)}
          aria-label={t('reader.closeReader')}
          className="rounded-lg p-2 text-slate-200 transition-colors hover:bg-ink-700 hover:text-white"
        >
          <IconClose />
        </button>
        <p className="min-w-0 flex-1 truncate text-sm font-medium text-slate-200">{title}</p>
        <span className="text-xs tabular-nums text-slate-500">{percent}%</span>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => changeFont(-10)}
            aria-label={t('reader.fontSmaller')}
            className="rounded-lg px-2 py-1 text-xs font-medium text-slate-300 hover:bg-ink-700 hover:text-white"
          >
            A-
          </button>
          <button
            type="button"
            onClick={() => changeFont(10)}
            aria-label={t('reader.fontLarger')}
            className="rounded-lg px-2 py-1 text-base font-medium text-slate-300 hover:bg-ink-700 hover:text-white"
          >
            A+
          </button>
          {isMobile && (
            <button
              type="button"
              onClick={() => setSettingsOpen((o) => !o)}
              aria-label={t('reader.settings')}
              title={t('reader.settings')}
              className={`rounded-lg p-2 transition-colors hover:bg-ink-700 hover:text-white ${
                settingsOpen ? 'bg-ink-700 text-white' : 'text-slate-300'
              }`}
            >
              <IconSettings width={18} height={18} />
            </button>
          )}
        </div>
      </div>

      {/* Settings (phone only): paged vs scrolled reading. */}
      {settingsOpen && isMobile && (
        <>
          <button
            type="button"
            className="absolute inset-0 z-30 cursor-default"
            aria-label={t('reader.closeSettings')}
            onClick={() => setSettingsOpen(false)}
          />
          <div className="absolute right-2 top-14 z-40 w-56 rounded-xl border border-ink-700 bg-ink-850 p-3 shadow-soft">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
              {t('reader.flow')}
            </p>
            <div className="inline-flex w-full rounded-lg border border-ink-700 bg-ink-800 p-0.5">
              <button
                type="button"
                onClick={() => setFlow('paginated')}
                className={`flex-1 rounded-md px-2 py-1.5 text-sm font-medium transition-colors ${
                  flow === 'paginated' ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
                }`}
              >
                {t('reader.flowPaged')}
              </button>
              <button
                type="button"
                onClick={() => setFlow('scrolled')}
                className={`flex-1 rounded-md px-2 py-1.5 text-sm font-medium transition-colors ${
                  flow === 'scrolled' ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
                }`}
              >
                {t('reader.flowScroll')}
              </button>
            </div>
          </div>
        </>
      )}

      <div className="relative flex-1">
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center">
            <Spinner className="h-7 w-7 text-accent-400" />
          </div>
        )}
        {error && (
          <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 text-center">
            <p className="text-slate-400">{t('reader.unableToLoad')}</p>
            <button className="btn-secondary" onClick={() => navigate(`/books/${bookId}`)}>
              {t('reader.backToDetails')}
            </button>
          </div>
        )}

        <div ref={hostRef} className="h-full w-full" />

        {/* Paging controls (only in paged mode; scrolled mode scrolls freely).
            Phone: a full-area gesture layer turns pages by horizontal swipe/tap
            and ignores vertical motion. Desktop/tablet: side zones turn pages
            and the middle stays free for text selection (original behavior). */}
        {effectiveFlow === 'paginated' && !loading && !error && (
          <>
            {isMobile ? (
              <div {...pageGestures} className="absolute inset-0 z-10 touch-none select-none" aria-hidden />
            ) : (
              <>
                <button
                  type="button"
                  tabIndex={-1}
                  onClick={goLeft}
                  aria-label={t('reader.prevPage')}
                  className="absolute inset-y-0 left-0 z-10 w-[28%] cursor-w-resize outline-none focus:outline-none"
                />
                <button
                  type="button"
                  tabIndex={-1}
                  onClick={goRight}
                  aria-label={t('reader.nextPage')}
                  className="absolute inset-y-0 right-0 z-10 w-[28%] cursor-e-resize outline-none focus:outline-none"
                />
              </>
            )}
            <button
              type="button"
              tabIndex={-1}
              onClick={goLeft}
              aria-label={t('reader.prevPage')}
              className="absolute left-2 top-1/2 z-20 hidden -translate-y-1/2 rounded-full bg-black/40 p-2 text-white outline-none backdrop-blur transition-colors hover:bg-black/70 focus:outline-none sm:block"
            >
              <IconChevronLeft width={22} height={22} />
            </button>
            <button
              type="button"
              tabIndex={-1}
              onClick={goRight}
              aria-label={t('reader.nextPage')}
              className="absolute right-2 top-1/2 z-20 hidden -translate-y-1/2 rounded-full bg-black/40 p-2 text-white outline-none backdrop-blur transition-colors hover:bg-black/70 focus:outline-none sm:block"
            >
              <IconChevronRight width={22} height={22} />
            </button>
          </>
        )}
      </div>
    </div>
  )
}
