import { useI18n } from '@/i18n'
import { TagPicker } from './TagPicker'

// TagFilterFields is the shared include/exclude tag editor used by both the
// collection editor and the server-settings home-library filter, so the two
// stay visually and behaviourally identical. Include tags combine by AND ("all")
// or OR ("any") via the inline toggle; exclude tags always hide any match.
export function TagFilterFields({
  tagIds,
  onTagIds,
  excludeTagIds,
  onExcludeTagIds,
  matchAny,
  onMatchAny,
}: {
  tagIds: number[]
  onTagIds: (ids: number[]) => void
  excludeTagIds: number[]
  onExcludeTagIds: (ids: number[]) => void
  matchAny: boolean
  onMatchAny: (v: boolean) => void
}) {
  const { t } = useI18n()
  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <div>
        <div className="mb-1 flex items-center justify-between gap-2">
          <label className="label mb-0">{t('collections.tags')}</label>
          <div className="inline-flex shrink-0 rounded-lg border border-ink-700 bg-ink-800 p-0.5">
            <button
              type="button"
              onClick={() => onMatchAny(false)}
              title={t('collections.matchAllHelp')}
              className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                !matchAny ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
              }`}
            >
              {t('collections.matchAll')}
            </button>
            <button
              type="button"
              onClick={() => onMatchAny(true)}
              title={t('collections.matchAnyHelp')}
              className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                matchAny ? 'bg-accent-600 text-onaccent' : 'text-slate-300 hover:text-white'
              }`}
            >
              {t('collections.matchAny')}
            </button>
          </div>
        </div>
        <p className="mb-2 text-xs text-slate-500">{t('collections.tagsHelp')}</p>
        <TagPicker value={tagIds} onChange={onTagIds} />
      </div>
      <div>
        <label className="label">{t('collections.excludeTags')}</label>
        <p className="mb-2 text-xs text-slate-500">{t('collections.excludeTagsHelp')}</p>
        <TagPicker value={excludeTagIds} onChange={onExcludeTagIds} />
      </div>
    </div>
  )
}
