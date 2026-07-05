import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { mapPool } from '@/lib/pool'
import { useI18n } from '@/i18n'
import type { Book, BookUpdate } from '@/types'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { Rating } from './Rating'

type FieldName = 'authors' | 'series' | 'publisher' | 'languages' | 'rating' | 'comments' | 'pubdate'
type TagsMode = 'add' | 'replace' | 'remove'

const split = (v: string) =>
  v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

// BulkEditModal applies the SAME change to every selected book. Only the fields
// the user ticks are sent; per-volume fields (title, series index) are omitted
// because they can't be shared. Tags can be added (union), replaced wholesale,
// or removed — each computed against the individual book's current tags.
export function BulkEditModal({
  books,
  open,
  onClose,
}: {
  books: Book[]
  open: boolean
  onClose: (changed: boolean) => void
}) {
  const qc = useQueryClient()
  const { t } = useI18n()

  const [enabled, setEnabled] = useState<Record<FieldName, boolean>>({
    authors: false,
    series: false,
    publisher: false,
    languages: false,
    rating: false,
    comments: false,
    pubdate: false,
  })
  const [vals, setVals] = useState({
    authors: '',
    series: '',
    publisher: '',
    languages: '',
    comments: '',
    pubdate: '',
    rating: 0,
  })
  const [tagsOn, setTagsOn] = useState(false)
  const [tagsMode, setTagsMode] = useState<TagsMode>('add')
  const [tagsValue, setTagsValue] = useState('')
  // Title is a per-book field, so it can't be *set* in bulk — but a find/replace
  // keeps each title unique while fixing a shared substring.
  const [titleOn, setTitleOn] = useState(false)
  const [titleFind, setTitleFind] = useState('')
  const [titleReplace, setTitleReplace] = useState('')
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [error, setError] = useState<string | null>(null)

  const titleActive = titleOn && titleFind !== ''
  const anyEnabled = tagsOn || titleActive || (Object.values(enabled) as boolean[]).some(Boolean)

  const buildBody = (book: Book): BookUpdate => {
    const body: BookUpdate = {}
    if (titleActive) {
      const next = book.title.split(titleFind).join(titleReplace)
      if (next !== book.title && next.trim() !== '') body.title = next
    }
    if (enabled.authors) body.authors = split(vals.authors)
    if (enabled.series) body.series = vals.series
    if (enabled.publisher) body.publisher = vals.publisher
    if (enabled.languages) body.languages = split(vals.languages)
    if (enabled.rating) body.rating = vals.rating
    if (enabled.comments) body.comments = vals.comments
    if (enabled.pubdate && vals.pubdate) body.pubdate = vals.pubdate
    if (tagsOn) {
      const set = split(tagsValue)
      if (tagsMode === 'add') body.addTags = set
      else if (tagsMode === 'replace') body.tags = set
      else body.tags = book.tags.map((x) => x.name).filter((n) => !set.includes(n))
    }
    return body
  }

  const run = useMutation({
    mutationFn: async () => {
      setProgress({ done: 0, total: books.length })
      const res = await mapPool(
        books,
        1, // the server serializes writes; keep it sequential and gentle
        (book) => api.updateBook(book.id, buildBody(book)),
        (done, total) => setProgress({ done, total }),
      )
      return res.filter((r) => !r.ok).length
    },
    onSuccess: (failed) => {
      qc.invalidateQueries({ queryKey: ['books'] })
      qc.invalidateQueries({ queryKey: ['facets'] })
      if (failed > 0) {
        setError(t('bulk.failedSome', { count: failed }))
        setProgress(null)
      } else {
        onClose(true)
      }
    },
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : t('book.failedToSave'))
      setProgress(null)
    },
  })

  const field = (name: FieldName, label: string, control: React.ReactNode) => (
    <div className="flex items-start gap-3 rounded-xl border border-ink-700 bg-ink-900 px-3 py-2.5">
      <input
        type="checkbox"
        className="mt-1 h-4 w-4 shrink-0 accent-accent-500"
        checked={enabled[name]}
        onChange={(e) => setEnabled((s) => ({ ...s, [name]: e.target.checked }))}
      />
      <div className="min-w-0 flex-1">
        <label className="label mb-1">{label}</label>
        <div className={enabled[name] ? '' : 'pointer-events-none opacity-40'}>{control}</div>
      </div>
    </div>
  )

  return (
    <Modal open={open} onClose={() => onClose(false)} title={t('bulk.editTitle', { count: books.length })} maxWidth="max-w-xl">
      <div className="space-y-4">
        <p className="text-sm text-slate-400">{t('bulk.editHint')}</p>

        <div className="space-y-2.5">
          {/* Title find/replace: substring-replaces across every selected book,
              so each title stays distinct (unlike the other "set" fields). */}
          <div className="rounded-xl border border-ink-700 bg-ink-900 px-3 py-2.5">
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                className="h-4 w-4 shrink-0 accent-accent-500"
                checked={titleOn}
                onChange={(e) => setTitleOn(e.target.checked)}
              />
              <span className="label mb-0 flex-1">{t('book.fieldTitle')}</span>
              <span className="text-[11px] text-slate-500">{t('bulk.replace')}</span>
            </div>
            <div className={`mt-2 grid grid-cols-2 gap-2 ${titleOn ? '' : 'pointer-events-none opacity-40'}`}>
              <input
                className="input"
                placeholder={t('bulk.find')}
                value={titleFind}
                onChange={(e) => setTitleFind(e.target.value)}
              />
              <input
                className="input"
                placeholder={t('bulk.replaceWith')}
                value={titleReplace}
                onChange={(e) => setTitleReplace(e.target.value)}
              />
            </div>
            <p className="mt-1.5 text-[11px] text-slate-500">{t('bulk.replaceHint')}</p>
          </div>

          {/* Tags — the most common bulk operation. */}
          <div className="rounded-xl border border-ink-700 bg-ink-900 px-3 py-2.5">
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                className="h-4 w-4 shrink-0 accent-accent-500"
                checked={tagsOn}
                onChange={(e) => setTagsOn(e.target.checked)}
              />
              <span className="label mb-0 flex-1">{t('book.fieldTags')}</span>
              <span className={`flex gap-3 text-xs ${tagsOn ? '' : 'pointer-events-none opacity-40'}`}>
                {(['add', 'replace', 'remove'] as const).map((m) => (
                  <label key={m} className="flex items-center gap-1">
                    <input
                      type="radio"
                      className="accent-accent-500"
                      checked={tagsMode === m}
                      onChange={() => setTagsMode(m)}
                    />
                    {t(m === 'add' ? 'bulk.tags.add' : m === 'replace' ? 'bulk.tags.replace' : 'bulk.tags.remove')}
                  </label>
                ))}
              </span>
            </div>
            <input
              className={`input mt-2 ${tagsOn ? '' : 'pointer-events-none opacity-40'}`}
              placeholder={t('bulk.tagsPlaceholder')}
              value={tagsValue}
              onChange={(e) => setTagsValue(e.target.value)}
            />
          </div>

          {field(
            'authors',
            t('book.fieldAuthors'),
            <input className="input" value={vals.authors} onChange={(e) => setVals({ ...vals, authors: e.target.value })} />,
          )}
          {field(
            'series',
            t('book.fieldSeries'),
            <input className="input" value={vals.series} onChange={(e) => setVals({ ...vals, series: e.target.value })} />,
          )}
          {field(
            'publisher',
            t('book.fieldPublisher'),
            <input
              className="input"
              value={vals.publisher}
              onChange={(e) => setVals({ ...vals, publisher: e.target.value })}
            />,
          )}
          {field(
            'languages',
            t('book.fieldLanguages'),
            <input
              className="input"
              value={vals.languages}
              onChange={(e) => setVals({ ...vals, languages: e.target.value })}
            />,
          )}
          {field(
            'pubdate',
            t('book.fieldPubdate'),
            <input
              className="input"
              type="date"
              value={vals.pubdate}
              onChange={(e) => setVals({ ...vals, pubdate: e.target.value })}
            />,
          )}
          {field(
            'rating',
            t('book.fieldRating'),
            <Rating value={vals.rating} onChange={(v) => setVals({ ...vals, rating: v })} />,
          )}
          {field(
            'comments',
            t('book.fieldComments'),
            <textarea
              className="input min-h-[80px] resize-y"
              value={vals.comments}
              onChange={(e) => setVals({ ...vals, comments: e.target.value })}
            />,
          )}
        </div>

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">{error}</div>
        )}

        <div className="flex items-center justify-end gap-3 pt-1">
          {progress && (
            <span className="mr-auto text-xs text-slate-400">
              {t('bulk.progress', { done: progress.done, total: progress.total })}
            </span>
          )}
          <button type="button" className="btn-secondary" onClick={() => onClose(false)} disabled={run.isPending}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-primary"
            onClick={() => {
              setError(null)
              if (!anyEnabled) {
                setError(t('bulk.nothingEnabled'))
                return
              }
              run.mutate()
            }}
            disabled={run.isPending}
          >
            {run.isPending && <Spinner className="h-4 w-4" />}
            {t('bulk.apply', { count: books.length })}
          </button>
        </div>
      </div>
    </Modal>
  )
}
