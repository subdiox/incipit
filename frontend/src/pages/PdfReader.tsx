import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { IconClose } from '@/components/icons'

// PdfReader hands the file to the browser's built-in PDF viewer via an inline
// iframe — no extra dependency, full pagination/zoom/search for free.
export function PdfReader({ bookId, title }: { bookId: number; title: string }) {
  const navigate = useNavigate()
  const { t } = useI18n()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') navigate(`/books/${bookId}`)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, bookId])

  return (
    <div className="flex h-screen w-screen flex-col bg-ink-950">
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
      </div>
      <iframe
        title={title}
        src={mediaUrl.content(bookId)}
        className="w-full flex-1 border-0 bg-ink-800"
      />
    </div>
  )
}
