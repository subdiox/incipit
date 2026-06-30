import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, mediaUrl } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import type { Book, BookUpdate } from '@/types'
import { formatBytes, formatDate, languageLabel } from '@/lib/format'
import { Cover } from '@/components/Cover'
import { Rating } from '@/components/Rating'
import { EnrichModal } from '@/components/EnrichModal'
import { Modal } from '@/components/Modal'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { AddToShelfMenu } from '@/components/AddToShelfMenu'
import {
  IconBook,
  IconChevronLeft,
  IconDownload,
  IconEdit,
  IconSearch,
  IconTrash,
} from '@/components/icons'

function Meta({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className="mt-1 text-sm text-slate-200">{children}</dd>
    </div>
  )
}

function EditModal({ book, open, onClose }: { book: Book; open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const [form, setForm] = useState({
    title: book.title,
    authors: book.authors.map((a) => a.name).join(', '),
    series: book.series?.name ?? '',
    seriesIndex: book.seriesIndex ? String(book.seriesIndex) : '',
    tags: book.tags.map((t) => t.name).join(', '),
    publisher: book.publisher?.name ?? '',
    languages: book.languages.join(', '),
    rating: book.rating,
    comments: book.comments ?? '',
    pubdate: book.pubdate?.slice(0, 10) ?? '',
  })
  const [error, setError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: (body: BookUpdate) => api.updateBook(book.id, body),
    onSuccess: (updated) => {
      queryClient.setQueryData(['book', book.id], updated)
      queryClient.invalidateQueries({ queryKey: ['books'] })
      queryClient.invalidateQueries({ queryKey: ['facets'] })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : t('book.failedToSave')),
  })

  const split = (v: string) =>
    v
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    const body: BookUpdate = {
      title: form.title,
      authors: split(form.authors),
      series: form.series,
      seriesIndex: form.seriesIndex ? Number(form.seriesIndex) : 0,
      tags: split(form.tags),
      publisher: form.publisher,
      languages: split(form.languages),
      rating: form.rating,
      comments: form.comments,
    }
    if (form.pubdate) body.pubdate = form.pubdate
    mutation.mutate(body)
  }

  return (
    <Modal open={open} onClose={onClose} title={t('book.editMetadata')} maxWidth="max-w-xl">
      <form onSubmit={submit} className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="label">{t('book.fieldTitle')}</label>
            <input className="input" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />
          </div>
          <div className="sm:col-span-2">
            <label className="label">{t('book.fieldAuthors')}</label>
            <input className="input" value={form.authors} onChange={(e) => setForm({ ...form, authors: e.target.value })} />
          </div>
          <div>
            <label className="label">{t('book.fieldSeries')}</label>
            <input className="input" value={form.series} onChange={(e) => setForm({ ...form, series: e.target.value })} />
          </div>
          <div>
            <label className="label">{t('book.fieldSeriesIndex')}</label>
            <input
              className="input"
              type="number"
              step="0.1"
              value={form.seriesIndex}
              onChange={(e) => setForm({ ...form, seriesIndex: e.target.value })}
            />
          </div>
          <div>
            <label className="label">{t('book.fieldTags')}</label>
            <input className="input" value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} />
          </div>
          <div>
            <label className="label">{t('book.fieldPublisher')}</label>
            <input className="input" value={form.publisher} onChange={(e) => setForm({ ...form, publisher: e.target.value })} />
          </div>
          <div>
            <label className="label">{t('book.fieldLanguages')}</label>
            <input
              className="input"
              value={form.languages}
              onChange={(e) => setForm({ ...form, languages: e.target.value })}
            />
          </div>
          <div>
            <label className="label">{t('book.fieldPubdate')}</label>
            <input
              className="input"
              type="date"
              value={form.pubdate}
              onChange={(e) => setForm({ ...form, pubdate: e.target.value })}
            />
          </div>
          <div className="sm:col-span-2">
            <label className="label">{t('book.fieldRating')}</label>
            <Rating value={form.rating} onChange={(v) => setForm({ ...form, rating: v })} />
          </div>
          <div className="sm:col-span-2">
            <label className="label">{t('book.fieldComments')}</label>
            <textarea
              className="input min-h-[100px] resize-y"
              value={form.comments}
              onChange={(e) => setForm({ ...form, comments: e.target.value })}
            />
          </div>
        </div>

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.saveChanges')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export function BookDetailPage() {
  const { id } = useParams()
  const bookId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { user } = useAuth()
  const { t } = useI18n()

  const [editing, setEditing] = useState(false)
  const [enriching, setEnriching] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const { data: book, isLoading, isError, error } = useQuery({
    queryKey: ['book', bookId],
    queryFn: () => api.book(bookId),
    enabled: Number.isFinite(bookId),
  })

  const { data: progress } = useQuery({
    queryKey: ['progress', bookId],
    queryFn: () => api.progress(bookId).catch(() => null),
    enabled: Number.isFinite(bookId),
  })

  const { data: views } = useQuery({
    queryKey: ['views', bookId],
    queryFn: () => api.bookViews(bookId),
    enabled: Number.isFinite(bookId),
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteBook(bookId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['books'] })
      queryClient.invalidateQueries({ queryKey: ['facets'] })
      navigate('/', { replace: true })
    },
  })

  const resetProgress = useMutation({
    mutationFn: () => api.resetProgress(bookId),
    onSuccess: () => {
      queryClient.setQueryData(['progress', bookId], null)
      queryClient.invalidateQueries({ queryKey: ['reading'] })
    },
  })

  if (isLoading) return <FullPageSpinner />
  if (isError || !book)
    return (
      <div className="card p-8 text-center text-sm text-red-300">
        {(error as Error)?.message ?? t('book.notFound')}
      </div>
    )

  const hasProgress = progress && progress.page > 0 && progress.totalPages > 0
  // Formats with an in-browser reader (CBZ image reader, PDF viewer, EPUB).
  const readable = book.formats.some((f) => ['cbz', 'pdf', 'epub'].includes(f.format.toLowerCase()))
  const downloadable = book.formats.length > 0

  return (
    <div>
      <Link to="/" className="btn-ghost mb-4 -ml-2 inline-flex">
        <IconChevronLeft width={18} height={18} />
        {t('nav.library')}
      </Link>

      <div className="grid grid-cols-1 gap-8 md:grid-cols-[300px_1fr]">
        {/* Cover + actions */}
        <div className="md:sticky md:top-20 md:self-start">
          <div className="mx-auto max-w-[300px] overflow-hidden rounded-2xl shadow-soft ring-1 ring-ink-700">
            <Cover bookId={book.id} title={book.title} hasCover={book.hasCover} version={book.lastModified} width={800} rounded="rounded-none" />
          </div>

          <div className="mt-4 space-y-2">
            {readable && (
              <Link to={`/books/${book.id}/read`} className="btn-primary w-full">
                <IconBook width={18} height={18} />
                {hasProgress
                  ? t('book.resume', { page: progress!.page + 1, total: progress!.totalPages })
                  : t('book.read')}
              </Link>
            )}

            {hasProgress && (
              <button
                type="button"
                className="btn-ghost w-full text-sm text-slate-400 hover:text-white"
                onClick={() => resetProgress.mutate()}
                disabled={resetProgress.isPending}
              >
                {resetProgress.isPending && <Spinner className="h-4 w-4" />}
                {t('history.reset')}
              </button>
            )}

            {user?.canDownload && downloadable && (
              <a
                href={mediaUrl.file(book.id)}
                className={`w-full ${readable ? 'btn-secondary' : 'btn-primary'}`}
                download
              >
                <IconDownload width={18} height={18} />
                {t('book.download')}
              </a>
            )}

            <AddToShelfMenu bookId={book.id} />

            {user?.canEdit && (
              <div className="space-y-2 border-t border-ink-700 pt-3">
                <button type="button" className="btn-secondary w-full" onClick={() => setEnriching(true)}>
                  <IconSearch width={16} height={16} />
                  {t('enrich.button')}
                </button>
                <div className="flex gap-2">
                  <button type="button" className="btn-secondary flex-1" onClick={() => setEditing(true)}>
                    <IconEdit width={16} height={16} />
                    {t('book.edit')}
                  </button>
                  <button type="button" className="btn-danger" onClick={() => setConfirmDelete(true)}>
                    <IconTrash width={16} height={16} />
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Details */}
        <div className="min-w-0">
          <h1 className="text-3xl font-semibold tracking-tight text-white">{book.title}</h1>
          <p className="mt-1.5 text-lg text-slate-400">
            {book.authors.length > 0
              ? book.authors.map((a, i) => (
                  <span key={a.id}>
                    {i > 0 && ', '}
                    <Link to={`/?author=${a.id}`} className="hover:text-accentSoft hover:underline">
                      {a.name}
                    </Link>
                  </span>
                ))
              : t('common.unknownAuthor')}
          </p>

          {book.series && (
            <p className="mt-1 text-sm text-accentSoft">
              <Link to={`/?series=${book.series.id}`} className="hover:underline">
                {book.series.name}
              </Link>
              {book.seriesIndex ? ` · ${t('book.volume', { index: book.seriesIndex })}` : ''}
            </p>
          )}

          {book.rating > 0 && (
            <div className="mt-3">
              <Rating value={book.rating} size={20} />
            </div>
          )}

          {book.tags.length > 0 && (
            <div className="mt-4 flex flex-wrap gap-1.5">
              {book.tags.map((t) => (
                <Link key={t.id} to={`/?tag=${t.id}`} className="chip">
                  {t.name}
                </Link>
              ))}
            </div>
          )}

          <dl className="mt-6 grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-3">
            {book.publisher && <Meta label={t('book.publisher')}>{book.publisher.name}</Meta>}
            {formatDate(book.pubdate) && <Meta label={t('book.published')}>{formatDate(book.pubdate)}</Meta>}
            {book.languages.length > 0 && (
              <Meta label={t('book.languages')}>{book.languages.map(languageLabel).join(', ')}</Meta>
            )}
            {formatDate(book.timestamp) && <Meta label={t('book.added')}>{formatDate(book.timestamp)}</Meta>}
            <Meta label={t('book.views')}>{(views?.views ?? 0).toLocaleString()}</Meta>
            {book.formats.length > 0 && (
              <Meta label={t('book.formats')}>
                {book.formats.map((f) => `${f.format} (${formatBytes(f.size)})`).join(', ')}
              </Meta>
            )}
            {Object.keys(book.identifiers).length > 0 && (
              <Meta label={t('book.identifiers')}>
                <div className="flex flex-col gap-0.5">
                  {Object.entries(book.identifiers).map(([k, v]) => (
                    <span key={k} className="text-xs text-slate-400">
                      <span className="uppercase text-slate-500">{k}</span>: {v}
                    </span>
                  ))}
                </div>
              </Meta>
            )}
          </dl>

          {book.comments && (
            <div className="mt-8">
              <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">
                {t('book.description')}
              </h2>
              <div
                className="prose-comments space-y-3 text-sm leading-relaxed text-slate-300 [&_a]:text-accentSoft [&_p]:mb-3"
                dangerouslySetInnerHTML={{ __html: book.comments }}
              />
            </div>
          )}
        </div>
      </div>

      {editing && <EditModal book={book} open={editing} onClose={() => setEditing(false)} />}
      {enriching && <EnrichModal book={book} open={enriching} onClose={() => setEnriching(false)} />}

      <Modal open={confirmDelete} onClose={() => setConfirmDelete(false)} title={t('book.deleteTitle')}>
        <p className="text-sm text-slate-300">
          {t('book.deleteConfirmPrefix')}
          <span className="font-medium text-white">{book.title}</span>
          {t('book.deleteConfirmSuffix')}
        </p>
        {deleteMutation.isError && (
          <p className="mt-3 text-sm text-red-300">
            {(deleteMutation.error as Error)?.message ?? t('book.failedToDelete')}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(false)}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-danger"
            onClick={() => deleteMutation.mutate()}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.delete')}
          </button>
        </div>
      </Modal>
    </div>
  )
}
