import { lazy, Suspense, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Spinner } from '@/components/Spinner'

// Lazy-loaded so each reader (and the heavy epubjs dependency) is only fetched
// when a book of that format is opened.
const CbzReader = lazy(() => import('./CbzReader').then((m) => ({ default: m.CbzReader })))
const PdfReader = lazy(() => import('./PdfReader').then((m) => ({ default: m.PdfReader })))
const EpubReader = lazy(() => import('./EpubReader').then((m) => ({ default: m.EpubReader })))

const ReaderFallback = () => (
  <div className="dark flex h-screen items-center justify-center bg-black">
    <Spinner className="h-8 w-8 text-accent-400" />
  </div>
)

// ReaderPage picks the right in-browser reader for the book's format.
export function ReaderPage() {
  const { id } = useParams()
  const bookId = Number(id)
  const navigate = useNavigate()
  const { t } = useI18n()
  const qc = useQueryClient()

  const { data: book, isLoading, isError } = useQuery({
    queryKey: ['book', bookId],
    queryFn: () => api.book(bookId),
    enabled: Number.isFinite(bookId),
  })

  // Count one view per reader open (anonymized, library-wide), then refresh the
  // detail page's cached count.
  useEffect(() => {
    if (!Number.isFinite(bookId)) return
    api.recordView(bookId).then(() => qc.invalidateQueries({ queryKey: ['views', bookId] })).catch(() => {})
  }, [bookId, qc])

  const back = () => navigate(`/books/${bookId}`)

  if (isLoading) return <ReaderFallback />
  if (isError || !book)
    return <ReaderError onBack={back} text={t('reader.unableToLoad')} back={t('reader.backToDetails')} />

  const formats = book.formats.map((f) => f.format.toUpperCase())
  let reader = null
  if (formats.includes('CBZ')) reader = <CbzReader bookId={bookId} />
  else if (formats.includes('PDF')) reader = <PdfReader bookId={bookId} title={book.title} />
  else if (formats.includes('EPUB')) reader = <EpubReader bookId={bookId} title={book.title} />

  if (!reader)
    return <ReaderError onBack={back} text={t('reader.unsupported')} back={t('reader.backToDetails')} />

  return <Suspense fallback={<ReaderFallback />}>{reader}</Suspense>
}

function ReaderError({ onBack, text, back }: { onBack: () => void; text: string; back: string }) {
  return (
    <div className="dark flex h-screen flex-col items-center justify-center gap-4 bg-black text-center">
      <p className="text-slate-400">{text}</p>
      <button className="btn-secondary" onClick={onBack}>
        {back}
      </button>
    </div>
  )
}
