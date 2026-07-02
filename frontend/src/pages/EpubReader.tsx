import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from '@/components/Spinner'
import { IconChevronLeft, IconChevronRight, IconClose } from '@/components/icons'

const FONT_KEY = 'incipit.epub.fontSize'

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
  html { color-scheme: dark; }
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

  // Tap zones handle both a tap (turn by side) and a horizontal swipe (swipe
  // left → forward, swipe right → back), so pages turn by swiping the sides too
  // — foliate handles swipes over its own middle area. Uses the same goLeft/
  // goRight so direction (LTR/RTL) stays correct.
  const swipeStart = useRef<{ x: number; y: number } | null>(null)
  const zoneHandlers = useCallback(
    (tap: () => void) => ({
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
          tap()
        }
      },
    }),
    [goLeft, goRight],
  )

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

        view.renderer.setAttribute('flow', 'paginated')
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
        </div>
      </div>

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

        {/* Side tap zones (middle stays free for selection / links). Pointer-only
            and non-focusable so an arrow-key press can't leave a focus outline
            framing the left/right halves. */}
        <button
          type="button"
          tabIndex={-1}
          {...zoneHandlers(goLeft)}
          aria-label={t('reader.prevPage')}
          className="absolute inset-y-0 left-0 z-0 w-[28%] touch-none cursor-w-resize outline-none focus:outline-none"
        />
        <button
          type="button"
          tabIndex={-1}
          {...zoneHandlers(goRight)}
          aria-label={t('reader.nextPage')}
          className="absolute inset-y-0 right-0 z-0 w-[28%] touch-none cursor-e-resize outline-none focus:outline-none"
        />
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
      </div>
    </div>
  )
}
