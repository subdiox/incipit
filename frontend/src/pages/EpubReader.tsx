import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import ePub, { type Book as EpubBook, type Rendition } from 'epubjs'
import { mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from '@/components/Spinner'
import { IconChevronLeft, IconChevronRight, IconClose } from '@/components/icons'

const FONT_KEY = 'incipit.epub.fontSize'

export function EpubReader({ bookId, title }: { bookId: number; title: string }) {
  const navigate = useNavigate()
  const { t } = useI18n()
  const hostRef = useRef<HTMLDivElement>(null)
  const renditionRef = useRef<Rendition | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [percent, setPercent] = useState(0)
  // Right-to-left reading (Japanese vertical / manga EPUBs): flips which side
  // advances the page so gestures/keys feel natural.
  const [rtl, setRtl] = useState(false)
  const [fontSize, setFontSize] = useState<number>(() => {
    const v = Number(localStorage.getItem(FONT_KEY))
    return v >= 80 && v <= 220 ? v : 100
  })

  const locKey = `incipit.epub.loc.${bookId}`

  // prev/next follow reading order; onLeft/onRight map the physical sides,
  // reversed for right-to-left books so the left side advances.
  const prev = useCallback(() => void renditionRef.current?.prev(), [])
  const next = useCallback(() => void renditionRef.current?.next(), [])
  const onLeft = useCallback(() => (rtl ? next() : prev()), [rtl, next, prev])
  const onRight = useCallback(() => (rtl ? prev() : next()), [rtl, next, prev])

  const onKey = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') onLeft()
      else if (e.key === 'ArrowRight') onRight()
      else if (e.key === 'Escape') navigate(`/books/${bookId}`)
    },
    [onLeft, onRight, navigate, bookId],
  )

  // Load the EPUB (fetched with credentials so the session cookie is sent).
  useEffect(() => {
    let cancelled = false
    let book: EpubBook | null = null
    ;(async () => {
      try {
        const res = await fetch(mediaUrl.content(bookId), { credentials: 'include' })
        if (!res.ok) throw new Error('fetch failed')
        const buf = await res.arrayBuffer()
        if (cancelled || !hostRef.current) return

        book = ePub(buf)
        await book.ready
        if (cancelled || !hostRef.current) return

        // Honor the package's declared reading direction (rtl for most vertical
        // Japanese / manga EPUBs) so pagination progresses the right way.
        const meta = (book.packaging?.metadata ?? {}) as { direction?: string }
        if (meta.direction === 'rtl') setRtl(true)

        const rendition = book.renderTo(hostRef.current, {
          width: '100%',
          height: '100%',
          flow: 'paginated',
          spread: 'auto',
          allowScriptedContent: true,
        })
        renditionRef.current = rendition

        // Minimal theme: set colours but don't impose writing-mode/line-height so
        // vertical (tategaki) content keeps its own layout.
        rendition.themes.register('incipit-dark', {
          html: { background: '#0b0b0f' },
          body: { background: '#0b0b0f', color: '#cbd5e1' },
          a: { color: '#9384f2 !important' },
          'a:visited': { color: '#9384f2 !important' },
          img: { 'max-width': '100%', 'max-height': '100%' },
          svg: { 'max-width': '100%', 'max-height': '100%' },
        })
        rendition.themes.select('incipit-dark')
        rendition.themes.fontSize(`${fontSize}%`)

        // Detect vertical writing from the actual content (some EPUBs use CSS
        // writing-mode without declaring page-progression-direction).
        rendition.on(
          'rendered',
          (_section: unknown, view: { contents?: { writingMode?: () => string } }) => {
            try {
              if (view?.contents?.writingMode?.().includes('vertical')) setRtl(true)
            } catch {
              /* ignore */
            }
          },
        )

        const saved = localStorage.getItem(locKey) || undefined
        await rendition.display(saved)
        if (cancelled) return
        setLoading(false)
        // A resize after first paint fixes column geometry for some books.
        requestAnimationFrame(() => {
          const el = hostRef.current
          if (el && renditionRef.current) renditionRef.current.resize(el.clientWidth, el.clientHeight)
        })

        rendition.on('relocated', (location: { start?: { cfi?: string; percentage?: number } }) => {
          if (location.start?.cfi) localStorage.setItem(locKey, location.start.cfi)
          if (typeof location.start?.percentage === 'number') {
            setPercent(Math.round(location.start.percentage * 100))
          }
        })
        // Keys pressed inside the rendered iframe are forwarded here.
        rendition.on('keyup', onKey)
      } catch {
        if (!cancelled) {
          setError(true)
          setLoading(false)
        }
      }
    })()

    return () => {
      cancelled = true
      book?.destroy()
      renditionRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookId])

  useEffect(() => {
    window.addEventListener('keyup', onKey)
    return () => window.removeEventListener('keyup', onKey)
  }, [onKey])

  useEffect(() => {
    const onResize = () => {
      const el = hostRef.current
      if (el && renditionRef.current) renditionRef.current.resize(el.clientWidth, el.clientHeight)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  const changeFont = (delta: number) => {
    setFontSize((cur) => {
      const nv = Math.min(220, Math.max(80, cur + delta))
      localStorage.setItem(FONT_KEY, String(nv))
      renditionRef.current?.themes.fontSize(`${nv}%`)
      return nv
    })
  }

  return (
    <div className="dark flex h-screen w-screen flex-col bg-ink-950">
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

        {/* Side click zones (middle stays free for selection / links). */}
        <button
          type="button"
          onClick={onLeft}
          aria-label={t('reader.prevPage')}
          className="absolute inset-y-0 left-0 z-0 w-[28%] cursor-w-resize"
        />
        <button
          type="button"
          onClick={onRight}
          aria-label={t('reader.nextPage')}
          className="absolute inset-y-0 right-0 z-0 w-[28%] cursor-e-resize"
        />
        {/* Hidden on touch-sized screens — the side tap zones handle turns. */}
        <button
          type="button"
          onClick={onLeft}
          aria-label={t('reader.prevPage')}
          className="absolute left-2 top-1/2 z-20 hidden -translate-y-1/2 rounded-full bg-black/40 p-2 text-white backdrop-blur transition-colors hover:bg-black/70 sm:block"
        >
          <IconChevronLeft width={22} height={22} />
        </button>
        <button
          type="button"
          onClick={onRight}
          aria-label={t('reader.nextPage')}
          className="absolute right-2 top-1/2 z-20 hidden -translate-y-1/2 rounded-full bg-black/40 p-2 text-white backdrop-blur transition-colors hover:bg-black/70 sm:block"
        >
          <IconChevronRight width={22} height={22} />
        </button>
      </div>
    </div>
  )
}
