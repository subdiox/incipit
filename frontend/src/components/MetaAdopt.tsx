// Shared building blocks for adopting コミックシーモア metadata onto a book —
// used both by the single-book EnrichModal and by every row of the multi-book
// MetaCompareModal, so the tag-merge and cover-compare UX is identical.
import { sortTagNames } from '@/lib/format'
import { mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Cover } from './Cover'
import { IconBook } from './icons'

export type TagMode = 'merge' | 'replace' | 'custom'

// A tag adoption is modelled as the exact final set of tags, split across the
// two columns (current + source). "merge"/"replace" are presets that compute
// that split; toggling any chip flips into free-form "custom".
export interface TagSelection {
  enabled: boolean
  mode: TagMode
  cur: Set<string>
  src: Set<string>
}

// merge = keep everything on the book, plus the source tags that are new.
function mergeSel(cur: string[], src: string[]) {
  return { cur: new Set(cur), src: new Set(src.filter((t) => !cur.includes(t))) }
}
// replace = adopt every source tag; on the current side keep only those that
// also exist on the source (i.e. the ones that survive the replace).
function replaceSel(cur: string[], src: string[]) {
  return { cur: new Set(cur.filter((t) => src.includes(t))), src: new Set(src) }
}

export function initTagSelection(current: string[], source: string[]): TagSelection {
  const s = mergeSel(sortTagNames(current), sortTagNames(source))
  return { enabled: source.length > 0, mode: 'merge', cur: s.cur, src: s.src }
}

// finalTags is the deduped set to persist (a tag can be selected on both sides).
export function finalTags(sel: TagSelection): string[] {
  return Array.from(new Set([...sel.cur, ...sel.src]))
}

function chipCls(selected: boolean): string {
  return `rounded-full border px-2 py-0.5 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
    selected
      ? 'border-accent-500/60 bg-accent-500/20 text-accentSoft'
      : 'border-ink-600 bg-transparent text-slate-500 hover:border-slate-500 hover:text-slate-300'
  }`
}

// TagMergeSelector renders the tag adoption box: an enable checkbox, the
// merge/replace/select presets, and the current/source chip columns. Fully
// controlled — the parent owns the TagSelection.
export function TagMergeSelector({
  current,
  source,
  value,
  onChange,
}: {
  current: string[]
  source: string[]
  value: TagSelection
  onChange: (next: TagSelection) => void
}) {
  const { t } = useI18n()
  const curTags = sortTagNames(current)
  const srcTags = sortTagNames(source)
  const has = curTags.length > 0 || srcTags.length > 0

  const pickMode = (m: TagMode) => {
    if (m === 'merge') {
      const s = mergeSel(curTags, srcTags)
      onChange({ ...value, mode: m, cur: s.cur, src: s.src })
    } else if (m === 'replace') {
      const s = replaceSel(curTags, srcTags)
      onChange({ ...value, mode: m, cur: s.cur, src: s.src })
    } else {
      onChange({ ...value, mode: m, cur: new Set(), src: new Set() })
    }
  }
  const toggle = (side: 'cur' | 'src', name: string) => {
    const key = side === 'cur' ? 'cur' : 'src'
    const n = new Set(value[key])
    if (n.has(name)) n.delete(name)
    else n.add(name)
    onChange({ ...value, mode: 'custom', [key]: n })
  }

  const column = (side: 'cur' | 'src', tags: string[], label: React.ReactNode) => (
    <div>
      <p className={`mb-1 ${side === 'cur' ? 'text-slate-500' : 'text-emerald-300/80'}`}>{label}</p>
      <div className="flex flex-wrap gap-1">
        {tags.length ? (
          tags.map((tg) => (
            <button
              key={tg}
              type="button"
              disabled={!value.enabled}
              onClick={() => toggle(side, tg)}
              className={chipCls((side === 'cur' ? value.cur : value.src).has(tg))}
            >
              {tg}
            </button>
          ))
        ) : (
          <span className="text-slate-600">—</span>
        )}
      </div>
    </div>
  )

  return (
    <div className="rounded-xl border border-ink-700 bg-ink-900 p-3">
      <label className="flex flex-wrap items-center gap-2">
        <input
          type="checkbox"
          className="h-4 w-4 accent-accent-500"
          disabled={!has}
          checked={has && value.enabled}
          onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
        />
        <span className="text-xs font-medium text-slate-400">{t('book.fieldTags')}</span>
        <span className={`ml-auto flex gap-3 text-xs ${value.enabled ? '' : 'pointer-events-none opacity-40'}`}>
          {(['merge', 'replace', 'custom'] as const).map((m) => (
            <label key={m} className="flex items-center gap-1">
              <input
                type="radio"
                className="accent-accent-500"
                disabled={!value.enabled}
                checked={value.mode === m}
                onChange={() => pickMode(m)}
              />
              {t(m === 'merge' ? 'enrich.tagMerge' : m === 'replace' ? 'enrich.tagReplace' : 'enrich.tagSelect')}
            </label>
          ))}
        </span>
      </label>
      <div className="mt-2 grid grid-cols-2 gap-3 text-xs">
        {column('cur', curTags, t('enrich.current'))}
        {column('src', srcTags, t('enrich.source'))}
      </div>
    </div>
  )
}

// CoverCompare shows the book's current cover beside the source cover with an
// adopt checkbox. token is the metadata-preview cover token (undefined = the
// source has no cover, so adoption is disabled).
export function CoverCompare({
  bookId,
  title,
  hasCover,
  version,
  token,
  checked,
  onChange,
}: {
  bookId: number
  title: string
  hasCover: boolean
  version?: string
  token?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  const { t } = useI18n()
  const hasSource = !!token
  return (
    <div className="flex items-center gap-3 rounded-xl border border-ink-700 bg-ink-900 p-3">
      <input
        type="checkbox"
        className="h-4 w-4 accent-accent-500"
        disabled={!hasSource}
        checked={hasSource && checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="text-xs font-medium text-slate-400">{t('enrich.cover')}</span>
      <div className="ml-auto flex items-end gap-4">
        <div className="text-center">
          <p className="mb-1 text-[11px] text-slate-500">{t('enrich.current')}</p>
          <div className="w-16 overflow-hidden rounded">
            <Cover bookId={bookId} title={title} hasCover={hasCover} version={version} width={200} rounded="rounded" />
          </div>
        </div>
        <div className="text-center">
          <p className="mb-1 text-[11px] text-emerald-300/80">{t('enrich.source')}</p>
          <div className="flex aspect-[2/3] w-16 items-center justify-center overflow-hidden rounded bg-ink-800">
            {hasSource ? (
              <img src={mediaUrl.metaPreviewCover(token)} alt="" className="h-full w-full object-cover" />
            ) : (
              <IconBook width={20} height={20} className="text-ink-600" />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
