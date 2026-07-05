import { Fragment, useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { mapPool } from '@/lib/pool'
import { useI18n } from '@/i18n'
import type { Book, BookUpdate, MetaPreview } from '@/types'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { Rating } from './Rating'
import { Cover } from './Cover'
import { IconSearch, IconCheck } from './icons'
import { CoverCompare, TagMergeSelector, initTagSelection, finalTags, type TagSelection } from './MetaAdopt'

type FieldKey = 'title' | 'authors' | 'series' | 'seriesIndex' | 'publisher' | 'pubdate' | 'rating' | 'comments'
const FIELD_KEYS: FieldKey[] = ['title', 'authors', 'series', 'seriesIndex', 'publisher', 'pubdate', 'rating', 'comments']

type Status = 'loading' | 'ok' | 'nomatch' | 'error'

interface Row {
  book: Book
  query: string
  genre: string
  status: Status
  preview?: MetaPreview
  include: boolean
  adopt: Record<FieldKey, boolean>
  tagSel: TagSelection
  coverOn: boolean
}

function stripHtml(s: string): string {
  return s.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
}

// Given a fresh preview, compute the row's default selections: adopt every
// field cmoa offers. Any field can still be unticked per book before applying.
function applyPreview(row: Row, p: MetaPreview): Row {
  if (!p.matched) return { ...row, status: 'nomatch', preview: p, include: false }
  return {
    ...row,
    status: 'ok',
    preview: p,
    include: true,
    adopt: {
      title: !!p.title,
      authors: !!p.authors?.length,
      series: !!p.series,
      seriesIndex: !!p.seriesIndex,
      publisher: !!p.publisher,
      pubdate: !!p.pubdate,
      rating: !!p.rating,
      comments: !!p.comments,
    },
    tagSel: initTagSelection(row.book.tags.map((x) => x.name), p.tags ?? []),
    coverOn: !!p.hasCover,
  }
}

function blankRow(book: Book): Row {
  return {
    book,
    query: book.title,
    genre: 'comic',
    status: 'loading',
    include: false,
    adopt: { title: false, authors: false, series: false, seriesIndex: false, publisher: false, pubdate: false, rating: false, comments: false },
    tagSel: initTagSelection([], []),
    coverOn: false,
  }
}

// MetaCompareModal shows every selected book stacked vertically: the library's
// current values on the left, コミックシーモア's on the right, with a per-field
// checkbox so only the ticked fields are copied. Each book has its own include
// toggle; Apply writes only the included books.
export function MetaCompareModal({
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
  const [rows, setRows] = useState<Row[]>([])
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [changed, setChanged] = useState(false)
  // Search refinement applied to every book's cmoa lookup (same as the single
  // enrich flow): extra AND word(s) and excluded word(s), space-separated.
  const [metaAdd, setMetaAdd] = useState('')
  const [metaExclude, setMetaExclude] = useState('')

  const genres = useQuery({ queryKey: ['metadata-genres'], queryFn: api.metadataGenres, enabled: open, staleTime: Infinity }).data ?? []

  const booksKey = books.map((b) => b.id).join(',')

  // On open, seed a row per book and fetch each preview (bounded concurrency).
  useEffect(() => {
    if (!open) return
    let cancelled = false
    setRows(books.map(blankRow))
    setError(null)
    setProgress(null)
    setChanged(false)
    mapPool(books, 4, async (b, i) => {
      const p = await api.metadataPreview({ query: b.title, genre: 'comic' })
      if (cancelled) return
      setRows((prev) => prev.map((r, j) => (j === i ? applyPreview(r, p) : r)))
    }).then((res) => {
      if (cancelled) return
      // Mark rows whose fetch threw as errors.
      setRows((prev) => prev.map((r, j) => (res[j] && !res[j].ok ? { ...r, status: 'error' } : r)))
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, booksKey])

  const patch = (i: number, up: Partial<Row>) => setRows((prev) => prev.map((r, j) => (j === i ? { ...r, ...up } : r)))
  const setAdopt = (i: number, k: FieldKey, v: boolean) =>
    setRows((prev) => prev.map((r, j) => (j === i ? { ...r, adopt: { ...r.adopt, [k]: v } } : r)))

  const previewArgs = (row: Row) => ({
    query: row.query.trim() || row.book.title,
    genre: row.genre,
    metaAdd: metaAdd.trim() || undefined,
    metaExclude: metaExclude.trim() || undefined,
  })

  const refetch = async (i: number) => {
    const row = rows[i]
    patch(i, { status: 'loading' })
    try {
      const p = await api.metadataPreview(previewArgs(row))
      setRows((prev) => prev.map((r, j) => (j === i ? applyPreview(r, p) : r)))
    } catch {
      patch(i, { status: 'error' })
    }
  }

  // Re-run every book's lookup with the current add/exclude (and each row's own
  // title/genre) — used after editing the shared refinement words.
  const refetchAll = () => {
    const current = rows
    setRows((prev) => prev.map((r) => ({ ...r, status: 'loading' as Status })))
    mapPool(current, 4, async (row, i) => {
      try {
        const p = await api.metadataPreview(previewArgs(row))
        setRows((prev) => prev.map((r, j) => (j === i ? applyPreview(r, p) : r)))
      } catch {
        setRows((prev) => prev.map((r, j) => (j === i ? { ...r, status: 'error' as Status } : r)))
      }
    })
  }

  const buildBody = (row: Row): BookUpdate => {
    const p = row.preview!
    const body: BookUpdate = {}
    if (row.adopt.title && p.title) body.title = p.title
    if (row.adopt.authors && p.authors?.length) body.authors = p.authors
    if (row.adopt.series && p.series) body.series = p.series
    if (row.adopt.seriesIndex && p.seriesIndex) body.seriesIndex = p.seriesIndex
    if (row.adopt.publisher && p.publisher) body.publisher = p.publisher
    if (row.adopt.pubdate && p.pubdate) body.pubdate = p.pubdate
    if (row.adopt.rating && p.rating) body.rating = p.rating
    if (row.adopt.comments && p.comments) body.comments = p.comments
    if (row.tagSel.enabled) body.tags = finalTags(row.tagSel)
    return body
  }

  const applicable = rows.filter((r) => r.include && r.status === 'ok')

  const apply = useMutation({
    mutationFn: async () => {
      const targets = applicable
      setProgress({ done: 0, total: targets.length })
      const res = await mapPool(
        targets,
        1,
        async (row) => {
          const body = buildBody(row)
          if (Object.keys(body).length > 0) await api.updateBook(row.book.id, body)
          if (row.coverOn && row.preview?.hasCover && row.preview.token) {
            const fd = new FormData()
            fd.append('metaToken', row.preview.token)
            await api.setBookCover(row.book.id, fd)
          }
        },
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
        setChanged(true)
      } else {
        onClose(true)
      }
    },
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : t('enrich.failed'))
      setProgress(null)
    },
  })

  const matchedCount = rows.filter((r) => r.status === 'ok').length
  const stillLoading = rows.some((r) => r.status === 'loading')

  // One field comparison row inside a book card.
  const fieldRow = (i: number, row: Row, k: FieldKey, label: string, cur: React.ReactNode, next: React.ReactNode, available: boolean) => (
    <Fragment key={k}>
      <label className="flex items-center gap-2 py-0.5">
        <input
          type="checkbox"
          className="h-3.5 w-3.5 accent-accent-500"
          disabled={!available}
          checked={available && row.adopt[k]}
          onChange={(e) => setAdopt(i, k, e.target.checked)}
        />
        <span className="text-[11px] font-medium text-slate-400">{label}</span>
      </label>
      <div className="min-w-0 break-words py-0.5 text-slate-300">{cur || <span className="text-slate-600">—</span>}</div>
      <div className={`min-w-0 break-words py-0.5 ${available ? 'text-emerald-200' : 'text-slate-600'}`}>
        {available ? next : '—'}
      </div>
    </Fragment>
  )

  const statusBadge = (row: Row) => {
    if (row.status === 'loading') return <Spinner className="h-4 w-4" />
    if (row.status === 'ok') return <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300">{t('compare.matched')}</span>
    if (row.status === 'nomatch') return <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] text-amber-300">{t('compare.noMatch')}</span>
    return <span className="rounded-full bg-red-500/15 px-2 py-0.5 text-[11px] text-red-300">{t('compare.error')}</span>
  }

  return (
    <Modal open={open} onClose={() => onClose(changed)} title={t('compare.title')} maxWidth="max-w-3xl">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-sm text-slate-400">
          <span>{t('compare.subtitle', { count: books.length })}</span>
          <span className="text-slate-600">·</span>
          <span className="text-emerald-300/90">{t('compare.matchedCount', { matched: matchedCount, total: books.length })}</span>
          <div className="ml-auto flex items-center gap-2">
            <button
              type="button"
              className="text-xs font-medium text-accentSoft hover:text-white disabled:opacity-40"
              disabled={stillLoading}
              onClick={() => setRows((prev) => prev.map((r) => (r.status === 'ok' ? { ...r, include: true } : r)))}
            >
              {t('compare.selectAll')}
            </button>
            <span className="text-slate-700">|</span>
            <button
              type="button"
              className="text-xs font-medium text-slate-400 hover:text-white"
              onClick={() => setRows((prev) => prev.map((r) => ({ ...r, include: false })))}
            >
              {t('compare.selectNone')}
            </button>
          </div>
        </div>

        {/* Shared cmoa search refinement, applied to every book. */}
        <div className="flex flex-wrap items-end gap-2 rounded-xl border border-ink-700 bg-ink-900 px-3 py-2.5">
          <div className="min-w-0 flex-1">
            <label className="label">{t('upload.metaAdd')}</label>
            <input
              className="input h-9 py-1.5 text-sm"
              value={metaAdd}
              onChange={(e) => setMetaAdd(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), refetchAll())}
            />
          </div>
          <div className="min-w-0 flex-1">
            <label className="label">{t('upload.metaExclude')}</label>
            <input
              className="input h-9 py-1.5 text-sm"
              placeholder={t('upload.metaExcludePlaceholder')}
              value={metaExclude}
              onChange={(e) => setMetaExclude(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), refetchAll())}
            />
          </div>
          <button type="button" className="btn-secondary h-9 py-1.5" onClick={refetchAll} disabled={stillLoading}>
            <IconSearch width={15} height={15} />
            {t('compare.refetchAll')}
          </button>
        </div>

        <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
          {rows.map((row, i) => {
            const p = row.preview
            const dimmed = row.status !== 'ok' || !row.include
            return (
              <div
                key={row.book.id}
                className={`rounded-2xl border p-3 transition-colors ${
                  row.include && row.status === 'ok' ? 'border-accent-500/50 bg-ink-900' : 'border-ink-700 bg-ink-900/60'
                }`}
              >
                {/* Header: include toggle + cover + title + status + re-search */}
                <div className="flex items-start gap-3">
                  <button
                    type="button"
                    disabled={row.status !== 'ok'}
                    onClick={() => patch(i, { include: !row.include })}
                    aria-pressed={row.include}
                    className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-md border-2 transition-colors disabled:opacity-30 ${
                      row.include && row.status === 'ok'
                        ? 'border-accent-500 bg-accent-500 text-onaccent'
                        : 'border-ink-600 text-transparent hover:border-slate-500'
                    }`}
                  >
                    <IconCheck width={14} height={14} />
                  </button>
                  <div className="w-10 shrink-0 overflow-hidden rounded ring-1 ring-ink-700">
                    <Cover bookId={row.book.id} title={row.book.title} hasCover={row.book.hasCover} version={row.book.lastModified} width={120} rounded="rounded-none" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="min-w-0 truncate text-sm font-medium text-slate-100" title={row.book.title}>
                        {row.book.title}
                      </h3>
                      {statusBadge(row)}
                    </div>
                    {/* Re-search this one book (edit title / genre). */}
                    <div className="mt-1.5 flex items-center gap-1.5">
                      <input
                        className="input h-8 min-w-0 flex-1 py-1 text-xs"
                        value={row.query}
                        onChange={(e) => patch(i, { query: e.target.value })}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault()
                            refetch(i)
                          }
                        }}
                      />
                      <select
                        className="input h-8 w-auto py-1 text-xs"
                        value={row.genre}
                        onChange={(e) => patch(i, { genre: e.target.value })}
                      >
                        {genres.map((g) => (
                          <option key={g.key} value={g.key}>
                            {g.label}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        className="btn-secondary h-8 px-2 py-1 text-xs"
                        onClick={() => refetch(i)}
                        disabled={row.status === 'loading'}
                      >
                        <IconSearch width={13} height={13} />
                      </button>
                    </div>
                  </div>
                </div>

                {/* Comparison table (only when matched) */}
                {row.status === 'ok' && p && (
                  <div className={`mt-3 ${dimmed ? 'opacity-60' : ''}`}>
                    <div className="grid grid-cols-[6.5rem_1fr_1fr] gap-x-2 border-b border-ink-700 pb-1 text-[10px] font-semibold uppercase tracking-wide text-slate-500">
                      <div>{t('enrich.field')}</div>
                      <div>{t('enrich.current')}</div>
                      <div className="text-emerald-300/80">{t('enrich.source')}</div>
                    </div>
                    <div className="grid grid-cols-[6.5rem_1fr_1fr] gap-x-2 text-xs">
                      {FIELD_KEYS.map((k) => {
                        switch (k) {
                          case 'title':
                            return fieldRow(i, row, k, t('book.fieldTitle'), row.book.title, p.title, !!p.title)
                          case 'authors':
                            return fieldRow(i, row, k, t('book.fieldAuthors'), row.book.authors.map((a) => a.name).join(', '), p.authors?.join(', '), !!p.authors?.length)
                          case 'series':
                            return fieldRow(i, row, k, t('book.fieldSeries'), row.book.series?.name ?? '', p.series, !!p.series)
                          case 'seriesIndex':
                            return fieldRow(i, row, k, t('book.fieldSeriesIndex'), row.book.seriesIndex || '', p.seriesIndex || '', !!p.seriesIndex)
                          case 'publisher':
                            return fieldRow(i, row, k, t('book.fieldPublisher'), row.book.publisher?.name ?? '', p.publisher, !!p.publisher)
                          case 'pubdate':
                            return fieldRow(i, row, k, t('book.fieldPubdate'), row.book.pubdate?.slice(0, 10) ?? '', p.pubdate, !!p.pubdate)
                          case 'rating':
                            return fieldRow(i, row, k, t('book.fieldRating'), row.book.rating > 0 ? <Rating value={row.book.rating} size={12} /> : '', p.rating ? <Rating value={p.rating} size={12} /> : '', !!p.rating)
                          case 'comments':
                            return fieldRow(i, row, k, t('book.fieldComments'), <span className="line-clamp-2 text-[11px] text-slate-400">{stripHtml(row.book.comments ?? '')}</span>, <span className="line-clamp-2 text-[11px]">{stripHtml(p.comments ?? '')}</span>, !!p.comments)
                        }
                      })}
                    </div>

                    {/* Tags + cover, same components as the single-book flow. */}
                    <div className="mt-3 grid gap-3 sm:grid-cols-2">
                      <TagMergeSelector
                        current={row.book.tags.map((x) => x.name)}
                        source={p.tags ?? []}
                        value={row.tagSel}
                        onChange={(next) => patch(i, { tagSel: next })}
                      />
                      <CoverCompare
                        bookId={row.book.id}
                        title={row.book.title}
                        hasCover={row.book.hasCover}
                        version={row.book.lastModified}
                        token={p.hasCover ? p.token : undefined}
                        checked={row.coverOn}
                        onChange={(v) => patch(i, { coverOn: v })}
                      />
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">{error}</div>
        )}

        <div className="flex items-center justify-end gap-3 border-t border-ink-700 pt-3">
          {progress ? (
            <span className="mr-auto text-xs text-slate-400">{t('compare.applying', { done: progress.done, total: progress.total })}</span>
          ) : (
            <span className="mr-auto text-xs text-slate-500">{t('compare.willApply', { count: applicable.length })}</span>
          )}
          <button type="button" className="btn-secondary" onClick={() => onClose(changed)} disabled={apply.isPending}>
            {t('common.close')}
          </button>
          <button
            type="button"
            className="btn-primary"
            onClick={() => {
              setError(null)
              apply.mutate()
            }}
            disabled={apply.isPending || applicable.length === 0}
          >
            {apply.isPending ? <Spinner className="h-4 w-4" /> : <IconCheck width={16} height={16} />}
            {t('compare.apply', { count: applicable.length })}
          </button>
        </div>
      </div>
    </Modal>
  )
}
