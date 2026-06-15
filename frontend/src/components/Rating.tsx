import { IconStar } from './icons'

interface RatingProps {
  /** value is 0-10 (2 per star) */
  value: number
  /** size in px */
  size?: number
  onChange?: (value: number) => void
}

export function Rating({ value, size = 18, onChange }: RatingProps) {
  const stars = value / 2 // 0-5
  const interactive = !!onChange

  return (
    <div className="inline-flex items-center gap-0.5" role={interactive ? 'radiogroup' : undefined}>
      {[1, 2, 3, 4, 5].map((star) => {
        const filled = stars >= star
        const half = !filled && stars >= star - 0.5
        const node = (
          <span className="relative inline-flex" style={{ width: size, height: size }}>
            <IconStar
              width={size}
              height={size}
              className="absolute inset-0 text-ink-600"
            />
            <span
              className="absolute inset-0 overflow-hidden text-amber-400"
              style={{ width: filled ? '100%' : half ? '50%' : '0%' }}
            >
              <IconStar width={size} height={size} fill="currentColor" />
            </span>
          </span>
        )
        if (!interactive) return <span key={star}>{node}</span>
        return (
          <button
            key={star}
            type="button"
            className="transition-transform hover:scale-110"
            aria-label={`${star} star${star > 1 ? 's' : ''}`}
            onClick={() => onChange?.(value === star * 2 ? 0 : star * 2)}
          >
            {node}
          </button>
        )
      })}
    </div>
  )
}
