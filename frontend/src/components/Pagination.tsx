import { useI18n } from '@/i18n'
import { IconChevronLeft, IconChevronRight } from './icons'

// Pagination is the shared prev / "page X of Y" / next control used by the
// library-style paged pages (rankings, recommendations). Renders nothing for a
// single page, so callers can drop it in unconditionally.
export function Pagination({
  currentPage,
  totalPages,
  onGoTo,
}: {
  currentPage: number
  totalPages: number
  onGoTo: (page: number) => void
}) {
  const { t } = useI18n()
  if (totalPages <= 1) return null
  return (
    <div className="mt-8 flex items-center justify-center gap-2">
      <button type="button" className="btn-secondary" disabled={currentPage <= 1} onClick={() => onGoTo(currentPage - 1)}>
        <IconChevronLeft width={16} height={16} />
        {t('library.prev')}
      </button>
      <span className="px-3 text-sm text-slate-400">
        {t('library.pageOf', { current: currentPage, total: totalPages })}
      </span>
      <button
        type="button"
        className="btn-secondary"
        disabled={currentPage >= totalPages}
        onClick={() => onGoTo(currentPage + 1)}
      >
        {t('library.next')}
        <IconChevronRight width={16} height={16} />
      </button>
    </div>
  )
}
