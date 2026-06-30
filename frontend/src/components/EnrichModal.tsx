import { Fragment, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, mediaUrl } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { Book, BookUpdate, MetaPreview } from '@/types'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { Rating } from './Rating'
import { Cover } from './Cover'
import { IconBook, IconSearch } from './icons'

type FieldKey = 'title' | 'authors' | 'series' | 'seriesIndex' | 'publisher' | 'pubdate' | 'rating' | 'comments'

function stripHtml(s: string): string {
  return s.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
}

export function EnrichModal({ book, open, onClose }: { book: Book; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const { t } = useI18n()

  const [query, setQuery] = useState(book.title)
  const [genre, setGenre] = useState('comic')
  const [preview, setPreview] = useState<MetaPreview | null>(null)
  const [adopt, setAdopt] = useState<Record<FieldKey, boolean>>({} as Record<FieldKey, boolean>)
  const [tagAdopt, setTagAdopt] = useState(true)
  const [tagMode, setTagMode] = useState<'merge' | 'replace'>('merge')
  const [coverAdopt, setCoverAdopt] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const genres = useQuery({
    queryKey: ['metadata-genres'],
    queryFn: api.metadataGenres,
    enabled: open,
    staleTime: Infinity,
  }).data ?? []

  const fetchM = useMutation({
    mutationFn: () => api.metadataPreview({ query: query.trim(), genre }),
    onSuccess: (res) => {
      setPreview(res)
      if (res.matched) {
        setAdopt({
          title: !!res.title,
          authors: !!res.authors?.length,
          series: !!res.series,
          seriesIndex: !!res.seriesIndex,
          publisher: !!res.publisher,
          pubdate: !!res.pubdate,
          rating: !!res.rating,
          comments: !!res.comments,
        })
        setTagAdopt(!!res.tags?.length)
        setCoverAdopt(!!res.hasCover)
      }
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('enrich.failed')),
  })

  const applyM = useMutation({
    mutationFn: async () => {
      const p = preview!
      const body: BookUpdate = {}
      if (adopt.title && p.title) body.title = p.title
      if (adopt.authors && p.authors?.length) body.authors = p.authors
      if (adopt.series && p.series) body.series = p.series
      if (adopt.seriesIndex && p.seriesIndex) body.seriesIndex = p.seriesIndex
      if (adopt.publisher && p.publisher) body.publisher = p.publisher
      if (adopt.pubdate && p.pubdate) body.pubdate = p.pubdate
      if (adopt.rating && p.rating) body.rating = p.rating
      if (adopt.comments && p.comments) body.comments = p.comments
      if (tagAdopt && p.tags?.length) {
        if (tagMode === 'merge') body.addTags = p.tags
        else body.tags = p.tags
      }
      let updated = book
      if (Object.keys(body).length > 0) updated = await api.updateBook(book.id, body)
      if (coverAdopt && p.hasCover && p.token) {
        const fd = new FormData()
        fd.append('metaToken', p.token)
        updated = await api.setBookCover(book.id, fd)
      }
      return updated
    },
    onSuccess: (updated) => {
      qc.setQueryData(['book', book.id], updated)
      qc.invalidateQueries({ queryKey: ['books'] })
      qc.invalidateQueries({ queryKey: ['facets'] })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('enrich.failed')),
  })

  const set = (k: FieldKey, v: boolean) => setAdopt((a) => ({ ...a, [k]: v }))

  // One comparison row: [checkbox + label] [current] [cmoa].
  const row = (k: FieldKey, label: string, cur: React.ReactNode, next: React.ReactNode, available: boolean) => (
    <Fragment key={k}>
      <label className="flex items-center gap-2 py-1">
        <input
          type="checkbox"
          className="h-4 w-4 accent-accent-500"
          disabled={!available}
          checked={available && !!adopt[k]}
          onChange={(e) => set(k, e.target.checked)}
        />
        <span className="text-xs font-medium text-slate-400">{label}</span>
      </label>
      <div className="min-w-0 break-words py-1 text-slate-300">{cur || <span className="text-slate-600">—</span>}</div>
      <div className={`min-w-0 break-words py-1 ${available ? 'text-emerald-200' : 'text-slate-600'}`}>
        {available ? next : '—'}
      </div>
    </Fragment>
  )

  const p = preview
  const curTags = book.tags.map((x) => x.name)

  return (
    <Modal open={open} onClose={onClose} title={t('enrich.title')} maxWidth="max-w-2xl">
      <div className="space-y-4">
        {/* Search */}
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-0 flex-1">
            <label className="label">{t('enrich.searchTitle')}</label>
            <input
              className="input"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && query.trim()) {
                  e.preventDefault()
                  setError(null)
                  fetchM.mutate()
                }
              }}
            />
          </div>
          <div>
            <label className="label">{t('enrich.genre')}</label>
            <select className="input w-auto" value={genre} onChange={(e) => setGenre(e.target.value)}>
              {genres.map((g) => (
                <option key={g.key} value={g.key}>
                  {g.label}
                </option>
              ))}
            </select>
          </div>
          <button
            type="button"
            className="btn-secondary"
            onClick={() => {
              setError(null)
              fetchM.mutate()
            }}
            disabled={fetchM.isPending || !query.trim()}
          >
            {fetchM.isPending ? <Spinner className="h-4 w-4" /> : <IconSearch width={16} height={16} />}
            {t('enrich.fetch')}
          </button>
        </div>

        {p && !p.matched && (
          <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-3.5 py-2.5 text-sm text-amber-300">
            {t('enrich.noMatch')}
          </div>
        )}

        {p?.matched && (
          <>
            <div className="grid grid-cols-[8rem_1fr_1fr] gap-x-3 border-b border-ink-700 pb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              <div>{t('enrich.field')}</div>
              <div>{t('enrich.current')}</div>
              <div className="text-emerald-300/80">{t('enrich.source')}</div>
            </div>
            <div className="grid grid-cols-[8rem_1fr_1fr] gap-x-3 text-sm">
              {row('title', t('book.fieldTitle'), book.title, p.title, !!p.title)}
              {row(
                'authors',
                t('book.fieldAuthors'),
                book.authors.map((a) => a.name).join(', '),
                p.authors?.join(', '),
                !!p.authors?.length,
              )}
              {row('series', t('book.fieldSeries'), book.series?.name ?? '', p.series, !!p.series)}
              {row(
                'seriesIndex',
                t('book.fieldSeriesIndex'),
                book.seriesIndex || '',
                p.seriesIndex || '',
                !!p.seriesIndex,
              )}
              {row('publisher', t('book.fieldPublisher'), book.publisher?.name ?? '', p.publisher, !!p.publisher)}
              {row('pubdate', t('book.fieldPubdate'), book.pubdate?.slice(0, 10) ?? '', p.pubdate, !!p.pubdate)}
              {row(
                'rating',
                t('book.fieldRating'),
                book.rating > 0 ? <Rating value={book.rating} size={14} /> : '',
                p.rating ? <Rating value={p.rating} size={14} /> : '',
                !!p.rating,
              )}
              {row(
                'comments',
                t('book.fieldComments'),
                <span className="line-clamp-2 text-xs text-slate-400">{stripHtml(book.comments ?? '')}</span>,
                <span className="line-clamp-2 text-xs">{stripHtml(p.comments ?? '')}</span>,
                !!p.comments,
              )}
            </div>

            {/* Tags */}
            <div className="rounded-xl border border-ink-700 bg-ink-900 p-3">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-accent-500"
                  disabled={!p.tags?.length}
                  checked={!!p.tags?.length && tagAdopt}
                  onChange={(e) => setTagAdopt(e.target.checked)}
                />
                <span className="text-xs font-medium text-slate-400">{t('book.fieldTags')}</span>
                {p.tags?.length ? (
                  <span className="ml-auto flex gap-3 text-xs">
                    <label className="flex items-center gap-1">
                      <input
                        type="radio"
                        className="accent-accent-500"
                        checked={tagMode === 'merge'}
                        onChange={() => setTagMode('merge')}
                      />
                      {t('enrich.tagMerge')}
                    </label>
                    <label className="flex items-center gap-1">
                      <input
                        type="radio"
                        className="accent-accent-500"
                        checked={tagMode === 'replace'}
                        onChange={() => setTagMode('replace')}
                      />
                      {t('enrich.tagReplace')}
                    </label>
                  </span>
                ) : null}
              </label>
              <div className="mt-2 grid grid-cols-2 gap-3 text-xs">
                <div>
                  <p className="mb-1 text-slate-500">{t('enrich.current')}</p>
                  <div className="flex flex-wrap gap-1">
                    {curTags.length ? (
                      curTags.map((tg) => (
                        <span key={tg} className="chip py-0.5">
                          {tg}
                        </span>
                      ))
                    ) : (
                      <span className="text-slate-600">—</span>
                    )}
                  </div>
                </div>
                <div>
                  <p className="mb-1 text-emerald-300/80">{t('enrich.source')}</p>
                  <div className="flex flex-wrap gap-1">
                    {p.tags?.length ? (
                      p.tags.map((tg) => (
                        <span
                          key={tg}
                          className={`chip py-0.5 ${curTags.includes(tg) ? '' : 'border-emerald-500/40 text-emerald-200'}`}
                        >
                          {tg}
                        </span>
                      ))
                    ) : (
                      <span className="text-slate-600">—</span>
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* Cover */}
            <div className="flex items-center gap-3 rounded-xl border border-ink-700 bg-ink-900 p-3">
              <input
                type="checkbox"
                className="h-4 w-4 accent-accent-500"
                disabled={!p.hasCover}
                checked={!!p.hasCover && coverAdopt}
                onChange={(e) => setCoverAdopt(e.target.checked)}
              />
              <span className="text-xs font-medium text-slate-400">{t('enrich.cover')}</span>
              <div className="ml-auto flex items-end gap-4">
                <div className="text-center">
                  <p className="mb-1 text-[11px] text-slate-500">{t('enrich.current')}</p>
                  <div className="w-16 overflow-hidden rounded">
                    <Cover bookId={book.id} title={book.title} hasCover={book.hasCover} width={200} rounded="rounded" />
                  </div>
                </div>
                <div className="text-center">
                  <p className="mb-1 text-[11px] text-emerald-300/80">{t('enrich.source')}</p>
                  <div className="flex aspect-[2/3] w-16 items-center justify-center overflow-hidden rounded bg-ink-800">
                    {p.hasCover && p.token ? (
                      <img src={mediaUrl.metaPreviewCover(p.token)} alt="" className="h-full w-full object-cover" />
                    ) : (
                      <IconBook width={20} height={20} className="text-ink-600" />
                    )}
                  </div>
                </div>
              </div>
            </div>
          </>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-secondary" onClick={onClose} disabled={applyM.isPending}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-primary"
            onClick={() => {
              setError(null)
              applyM.mutate()
            }}
            disabled={!p?.matched || applyM.isPending}
          >
            {applyM.isPending && <Spinner className="h-4 w-4" />}
            {t('enrich.apply')}
          </button>
        </div>
      </div>
    </Modal>
  )
}
