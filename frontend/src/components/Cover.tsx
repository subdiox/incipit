import { useState } from 'react'
import { mediaUrl } from '@/lib/api'
import { IconBook } from './icons'

interface CoverProps {
  bookId: number
  title: string
  hasCover?: boolean
  /** thumbnail width to request */
  width?: number
  /** use full cover endpoint instead of thumbnail */
  full?: boolean
  /** cache-busting version (e.g. book.lastModified) so cover edits refresh */
  version?: string
  className?: string
  rounded?: string
}

export function Cover({
  bookId,
  title,
  hasCover = true,
  width = 400,
  full = false,
  version,
  className = '',
  rounded = 'rounded-xl',
}: CoverProps) {
  const [loaded, setLoaded] = useState(false)
  const [errored, setErrored] = useState(false)
  const src = full ? mediaUrl.cover(bookId, version) : mediaUrl.thumbnail(bookId, width, version)
  const showFallback = errored || !hasCover

  return (
    <div
      className={`relative aspect-[2/3] w-full overflow-hidden bg-ink-800 ${rounded} ${className}`}
    >
      {!loaded && !showFallback && (
        <div className={`skeleton absolute inset-0 ${rounded}`} />
      )}
      {showFallback ? (
        <div className="flex h-full w-full flex-col items-center justify-center gap-2 bg-gradient-to-br from-ink-800 to-ink-900 p-3 text-center">
          <IconBook className="text-ink-600" width={32} height={32} />
          <span className="line-clamp-3 text-xs font-medium text-slate-500">{title}</span>
        </div>
      ) : (
        <img
          src={src}
          alt={title}
          loading="lazy"
          decoding="async"
          onLoad={() => setLoaded(true)}
          onError={() => setErrored(true)}
          className={`h-full w-full object-cover transition-opacity duration-300 ${
            loaded ? 'opacity-100' : 'opacity-0'
          }`}
        />
      )}
    </div>
  )
}
