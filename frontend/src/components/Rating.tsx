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

  // Toggling the exact current value clears the rating.
  const setVal = (v: number) => onChange?.(value === v ? 0 : v)

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
        // Two half-width hit zones: the left half sets a half star (odd value),
        // the right half a full star (even value) — so half stars are editable,
        // matching what the display can already show.
        return (
          <span
            key={star}
            className="relative inline-flex transition-transform hover:scale-110"
            style={{ width: size, height: size }}
          >
            {node}
            <button
              type="button"
              className="absolute inset-y-0 left-0 z-10 w-1/2 cursor-pointer"
              aria-label={`${star - 0.5} stars`}
              onClick={() => setVal(star * 2 - 1)}
            />
            <button
              type="button"
              className="absolute inset-y-0 right-0 z-10 w-1/2 cursor-pointer"
              aria-label={`${star} star${star > 1 ? 's' : ''}`}
              onClick={() => setVal(star * 2)}
            />
          </span>
        )
      })}
    </div>
  )
}
