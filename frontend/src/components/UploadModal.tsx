import { useRef, useState, type DragEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, mediaUrl } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { useI18n } from '@/i18n'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { IconUpload, IconClose, IconCheck, IconBook } from './icons'

interface UploadModalProps {
  open: boolean
  onClose: () => void
}

const ALLOWED = ['cbz', 'cbr', 'epub', 'pdf', 'mobi', 'azw3', 'fb2', 'txt']

type Status = 'queued' | 'previewing' | 'previewed' | 'uploading' | 'done' | 'error'

interface Preview {
  matched: boolean
  token?: string
  title?: string
  authors?: string[]
  series?: string
  tags?: string[]
  publisher?: string
  pubdate?: string
  hasCover?: boolean
}

interface Item {
  id: string
  file: File
  status: Status
  progress: number
  error?: string
  query: string // search query (fetch mode), editable in review
  genre?: string // per-file genre override
  preview?: Preview
  previewError?: string
  metaMatched?: boolean
  // Manual mode (multi-file): per-file title + series index parsed from the
  // filename, editable in review.
  fileTitle: string
  fileIndex: string
}

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}
function stripExt(name: string): string {
  return name.replace(/\.[^.]+$/, '')
}
// Derive a search query from a (often noisy) filename: drop bracketed groups
// like [author] (site) 【tag】 and collapse whitespace. The user can still edit.
function cleanQuery(name: string): string {
  const s = stripExt(name)
    .replace(/[[(【［（].*?[\])】］）]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  return s || stripExt(name)
}
// Simple filename split for manual multi-upload: pull a trailing volume number
// off as the series index, leaving the rest as the per-file title. The user
// reviews/edits the result before uploading.
function parseManual(name: string): { title: string; index: string } {
  const base = cleanQuery(name)
  const m = base.match(/^(.+?)[\s_·\-—]*(?:第|vol\.?|v|#)?\s*0*(\d{1,4})\s*(?:巻|話)?$/i)
  if (m && m[1].trim()) return { title: m[1].trim(), index: m[2] }
  return { title: base, index: '' }
}
function csrfToken(): string {
  const m = document.cookie.match(/(?:^|; )incipit_csrf=([^;]*)/)
  return m ? decodeURIComponent(m[1]) : ''
}

export function UploadModal({ open, onClose }: UploadModalProps) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const idSeq = useRef(0)

  const [items, setItems] = useState<Item[]>([])
  const [phase, setPhase] = useState<'configure' | 'review'>('configure')
  // Manual metadata (applied when fetch is off).
  const [title, setTitle] = useState('')
  const [authors, setAuthors] = useState('')
  const [series, setSeries] = useState('')
  const [seriesIndex, setSeriesIndex] = useState('')
  const [tags, setTags] = useState('')
  const [publisher, setPublisher] = useState('')
  // Fetch options (global defaults).
  const [fetchMeta, setFetchMeta] = useState(false)
  const [genre, setGenre] = useState('comic')
  const [metaAdd, setMetaAdd] = useState('')
  const [metaExclude, setMetaExclude] = useState('')

  const [dragOver, setDragOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const genresQuery = useQuery({
    queryKey: ['metadata-genres'],
    queryFn: api.metadataGenres,
    enabled: open,
    staleTime: Infinity,
  })
  const genres = genresQuery.data ?? []

  const single = items.length === 1
  const busy = uploading || items.some((it) => it.status === 'previewing')

  const reset = () => {
    setItems([])
    setPhase('configure')
    setTitle('')
    setAuthors('')
    setSeries('')
    setSeriesIndex('')
    setTags('')
    setPublisher('')
    setFetchMeta(false)
    setGenre('comic')
    setMetaAdd('')
    setMetaExclude('')
    setError(null)
    setUploading(false)
  }

  const close = () => {
    if (busy) return
    reset()
    onClose()
  }

  const addFiles = (list: FileList | null) => {
    if (!list) return
    const accepted: Item[] = []
    const rejected: string[] = []
    for (const file of Array.from(list)) {
      if (ALLOWED.includes(extOf(file.name))) {
        idSeq.current += 1
        const parsed = parseManual(file.name)
        accepted.push({
          id: `f${idSeq.current}`,
          file,
          status: 'queued',
          progress: 0,
          query: cleanQuery(file.name),
          fileTitle: parsed.title,
          fileIndex: parsed.index,
        })
      } else {
        rejected.push(file.name)
      }
    }
    setError(rejected.length ? t('upload.unsupported', { names: rejected.join(', ') }) : null)
    if (accepted.length) {
      setItems((prev) => {
        const next = [...prev, ...accepted]
        if (next.length === 1 && !title) setTitle(stripExt(next[0].file.name))
        return next
      })
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    addFiles(e.dataTransfer.files)
  }

  const removeItem = (id: string) => setItems((prev) => prev.filter((it) => it.id !== id))

  const patch = (id: string, p: Partial<Item>) =>
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...p } : it)))

  // --- metadata preview (review phase) ---

  const previewOne = async (item: Item) => {
    patch(item.id, { status: 'previewing', previewError: undefined })
    try {
      const res = await api.metadataPreview({
        query: item.query,
        genre: item.genre || genre,
        metaAdd: metaAdd || undefined,
        metaExclude: metaExclude || undefined,
      })
      patch(item.id, { status: 'previewed', preview: res })
    } catch (e) {
      patch(item.id, {
        status: 'previewed',
        preview: undefined,
        previewError: e instanceof Error ? e.message : t('upload.previewFailed'),
      })
    }
  }

  const goReview = async () => {
    if (items.length === 0) {
      setError(t('upload.chooseFirst'))
      return
    }
    setError(null)
    setPhase('review')
    // Manual mode just shows the filename-parsed fields for review; only the
    // fetch mode hits the external source (sequentially, to be gentle).
    if (!fetchMeta) return
    for (const item of items) {
      if (item.preview || item.status === 'previewing') continue
      await previewOne(item)
    }
  }

  // --- upload (commit) ---

  const uploadOne = (item: Item) =>
    new Promise<boolean>((resolve) => {
      const form = new FormData()
      form.append('file', item.file)
      if (fetchMeta) {
        form.append('title', item.query || stripExt(item.file.name))
        if (item.preview?.matched && item.preview.token) {
          form.append('metaToken', item.preview.token)
        }
      } else {
        // Single: the configure-form title/index. Multi: per-file values parsed
        // from the filename (a shared series index makes no sense across volumes).
        const bookTitle = single ? title || stripExt(item.file.name) : item.fileTitle || stripExt(item.file.name)
        const idx = single ? seriesIndex : item.fileIndex
        if (bookTitle) form.append('title', bookTitle)
        if (authors) form.append('authors', authors)
        if (series) form.append('series', series)
        if (idx) form.append('seriesIndex', idx)
        if (tags) form.append('tags', tags)
        if (publisher) form.append('publisher', publisher)
      }

      const xhr = new XMLHttpRequest()
      xhr.open('POST', '/api/books')
      xhr.withCredentials = true
      const csrf = csrfToken()
      if (csrf) xhr.setRequestHeader('X-CSRF-Token', csrf)

      patch(item.id, { status: 'uploading', progress: 0, error: undefined, metaMatched: undefined })
      xhr.upload.onprogress = (ev) => {
        if (ev.lengthComputable) patch(item.id, { progress: Math.round((ev.loaded / ev.total) * 100) })
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          const matched = fetchMeta ? xhr.getResponseHeader('X-Metadata-Matched') !== 'false' : undefined
          patch(item.id, { status: 'done', progress: 100, metaMatched: matched })
          resolve(true)
        } else {
          let msg = t('upload.failed', { status: xhr.status })
          try {
            const data = JSON.parse(xhr.responseText)
            if (data?.error) msg = data.error
          } catch {
            /* ignore */
          }
          patch(item.id, { status: 'error', error: msg })
          resolve(false)
        }
      }
      xhr.onerror = () => {
        patch(item.id, { status: 'error', error: t('upload.networkError') })
        resolve(false)
      }
      xhr.send(form)
    })

  const submit = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (items.length === 0) {
      setError(t('upload.chooseFirst'))
      return
    }
    setError(null)
    setUploading(true)
    let failed = 0
    // Sequential: the library has a single serialized writer.
    for (const item of items) {
      if (item.status === 'done') continue
      const ok = await uploadOne(item)
      if (!ok) failed += 1
    }
    queryClient.invalidateQueries({ queryKey: ['books'] })
    queryClient.invalidateQueries({ queryKey: ['stats'] })
    queryClient.invalidateQueries({ queryKey: ['facets'] })
    setUploading(false)
    if (failed === 0) {
      reset()
      onClose()
    } else {
      setError(t('upload.someFailed', { failed, total: items.length }))
    }
  }

  // A review step is shown for fetch mode (any count) and for manual multi-file
  // (per-file title/index). Single manual uploads commit straight from configure.
  const needsReview = items.length > 0 && (fetchMeta || items.length > 1)
  const onPrimary = (e: React.FormEvent) => {
    e.preventDefault()
    if (phase === 'configure' && needsReview) {
      void goReview()
    } else {
      void submit()
    }
  }

  return (
    <Modal open={open} onClose={close} title={t('upload.title')} maxWidth="max-w-2xl">
      <form onSubmit={onPrimary} className="space-y-4">
        {phase === 'configure' ? (
          <>
            {/* Drop zone */}
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              onDragOver={(e) => {
                e.preventDefault()
                setDragOver(true)
              }}
              onDragLeave={() => setDragOver(false)}
              onDrop={onDrop}
              className={`flex w-full flex-col items-center justify-center gap-2 rounded-2xl border-2 border-dashed px-6 text-center transition-colors ${
                items.length ? 'py-5' : 'py-10'
              } ${
                dragOver
                  ? 'border-accent-500 bg-accent-500/10'
                  : 'border-ink-600 bg-ink-900 hover:border-accent-500/50'
              }`}
            >
              <IconUpload className="text-accent-400" width={items.length ? 22 : 28} height={items.length ? 22 : 28} />
              <div>
                <p className="text-sm font-medium text-slate-200">
                  {items.length ? t('upload.addMore') : t('upload.dropHere')}
                </p>
                {!items.length && <p className="mt-0.5 text-xs text-slate-500">{t('upload.orBrowse')}</p>}
                <p className="mt-1 text-[11px] text-slate-600">{t('upload.allowedTypes')}</p>
              </div>
            </button>

            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept={ALLOWED.map((e) => '.' + e).join(',')}
              className="hidden"
              onChange={(e) => {
                addFiles(e.target.files)
                e.target.value = ''
              }}
            />

            {/* File queue */}
            {items.length > 0 && (
              <ul className="max-h-40 space-y-1.5 overflow-y-auto">
                {items.map((it) => (
                  <li
                    key={it.id}
                    className="flex items-center gap-3 rounded-xl border border-ink-700 bg-ink-900 px-3 py-2"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-slate-200">{it.file.name}</p>
                      <p className="text-[11px] text-slate-500">{formatBytes(it.file.size)}</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => removeItem(it.id)}
                      className="rounded-lg p-1.5 text-slate-400 hover:bg-ink-700 hover:text-white"
                      aria-label={t('upload.removeFile')}
                    >
                      <IconClose width={14} height={14} />
                    </button>
                  </li>
                ))}
              </ul>
            )}

            {/* Metadata config */}
            {items.length > 0 && (
              <div className="space-y-4">
                <label className="flex cursor-pointer items-start gap-2.5 rounded-xl border border-ink-700 bg-ink-900 px-3.5 py-3">
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 accent-accent-500"
                    checked={fetchMeta}
                    onChange={(e) => setFetchMeta(e.target.checked)}
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-slate-200">{t('upload.fetchMeta')}</span>
                    <span className="mt-0.5 block text-xs text-slate-500">{t('upload.fetchMetaHint')}</span>
                  </span>
                </label>

                {fetchMeta ? (
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <div className="sm:col-span-2">
                      <label className="label">{t('upload.genre')}</label>
                      <select className="input" value={genre} onChange={(e) => setGenre(e.target.value)}>
                        {genres.map((g) => (
                          <option key={g.key} value={g.key}>
                            {g.label}
                          </option>
                        ))}
                      </select>
                      <p className="mt-1 text-[11px] text-slate-600">{t('upload.genreHint')}</p>
                    </div>
                    <div>
                      <label className="label">{t('upload.metaAdd')}</label>
                      <input className="input" value={metaAdd} onChange={(e) => setMetaAdd(e.target.value)} />
                    </div>
                    <div>
                      <label className="label">{t('upload.metaExclude')}</label>
                      <input
                        className="input"
                        placeholder={t('upload.metaExcludePlaceholder')}
                        value={metaExclude}
                        onChange={(e) => setMetaExclude(e.target.value)}
                      />
                    </div>
                    <p className="sm:col-span-2 text-[11px] text-slate-600">{t('upload.reviewHint')}</p>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    {single ? (
                      <div className="sm:col-span-2">
                        <label className="label">{t('book.fieldTitle')}</label>
                        <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
                      </div>
                    ) : (
                      <p className="sm:col-span-2 text-xs text-slate-500">{t('upload.manualReviewHint')}</p>
                    )}
                    <div className="sm:col-span-2">
                      <label className="label">{t('book.fieldAuthors')}</label>
                      <input className="input" value={authors} onChange={(e) => setAuthors(e.target.value)} />
                    </div>
                    <div>
                      <label className="label">{t('book.fieldSeries')}</label>
                      <input className="input" value={series} onChange={(e) => setSeries(e.target.value)} />
                    </div>
                    {/* Series index is per-file (reviewed next) for multi-upload. */}
                    {single && (
                      <div>
                        <label className="label">{t('book.fieldSeriesIndex')}</label>
                        <input
                          className="input"
                          type="number"
                          step="0.1"
                          value={seriesIndex}
                          onChange={(e) => setSeriesIndex(e.target.value)}
                        />
                      </div>
                    )}
                    <div>
                      <label className="label">{t('book.fieldTags')}</label>
                      <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} />
                    </div>
                    <div>
                      <label className="label">{t('book.fieldPublisher')}</label>
                      <input className="input" value={publisher} onChange={(e) => setPublisher(e.target.value)} />
                    </div>
                    {!single && (
                      <p className="sm:col-span-2 text-[11px] text-slate-600">{t('upload.appliedToAll')}</p>
                    )}
                  </div>
                )}
              </div>
            )}
          </>
        ) : (
          /* Review phase: per-file metadata preview cards */
          <ul className="max-h-[60vh] space-y-3 overflow-y-auto">
            {items.map((it) =>
              fetchMeta ? (
                <ReviewCard
                  key={it.id}
                  item={it}
                  genres={genres}
                  defaultGenre={genre}
                  onChange={(p) => patch(it.id, p)}
                  onRefetch={() => previewOne(it)}
                  onRemove={() => removeItem(it.id)}
                  disabled={uploading}
                />
              ) : (
                <ManualCard
                  key={it.id}
                  item={it}
                  onChange={(p) => patch(it.id, p)}
                  onRemove={() => removeItem(it.id)}
                  disabled={uploading}
                />
              ),
            )}
          </ul>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          {phase === 'review' && (
            <button
              type="button"
              className="btn-secondary mr-auto"
              onClick={() => setPhase('configure')}
              disabled={busy}
            >
              {t('common.back')}
            </button>
          )}
          <button type="button" className="btn-secondary" onClick={close} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={busy || items.length === 0}>
            {(uploading || items.some((it) => it.status === 'previewing')) && <Spinner className="h-4 w-4" />}
            {phase === 'configure' && needsReview
              ? fetchMeta
                ? t('upload.fetchAndReview')
                : t('upload.review')
              : items.length > 1
                ? t('upload.uploadCount', { count: items.length })
                : t('upload.upload')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ManualCard({
  item,
  onChange,
  onRemove,
  disabled,
}: {
  item: Item
  onChange: (p: Partial<Item>) => void
  onRemove: () => void
  disabled: boolean
}) {
  const { t } = useI18n()
  const done = item.status === 'done'
  const uploading = item.status === 'uploading'

  return (
    <li className="rounded-xl border border-ink-700 bg-ink-900 p-3">
      <div className="flex items-start gap-2">
        <p className="min-w-0 flex-1 truncate text-[11px] text-slate-500">{item.file.name}</p>
        {done ? (
          <IconCheck width={16} height={16} className="shrink-0 text-emerald-400" />
        ) : uploading ? (
          <Spinner className="h-4 w-4 shrink-0 text-accent-400" />
        ) : (
          !disabled && (
            <button
              type="button"
              onClick={onRemove}
              className="shrink-0 rounded-lg p-1 text-slate-400 hover:bg-ink-700 hover:text-white"
              aria-label={t('upload.removeFile')}
            >
              <IconClose width={14} height={14} />
            </button>
          )
        )}
      </div>

      {!done && !uploading && (
        <div className="mt-1.5 flex items-end gap-2">
          <div className="min-w-0 flex-1">
            <label className="label">{t('book.fieldTitle')}</label>
            <input
              className="input h-9 py-1.5 text-sm"
              value={item.fileTitle}
              onChange={(e) => onChange({ fileTitle: e.target.value })}
              disabled={disabled}
            />
          </div>
          <div className="w-24 shrink-0">
            <label className="label">{t('book.fieldSeriesIndex')}</label>
            <input
              className="input h-9 py-1.5 text-sm"
              type="number"
              step="0.1"
              value={item.fileIndex}
              placeholder="—"
              onChange={(e) => onChange({ fileIndex: e.target.value })}
              disabled={disabled}
            />
          </div>
        </div>
      )}

      {item.status === 'error' && <p className="mt-1.5 text-xs text-red-400">{item.error}</p>}
      {uploading && (
        <div className="mt-1.5 h-1 w-full overflow-hidden rounded-full bg-ink-700">
          <div className="h-full rounded-full bg-accent-500 transition-all" style={{ width: `${item.progress}%` }} />
        </div>
      )}
    </li>
  )
}

function ReviewCard({
  item,
  genres,
  defaultGenre,
  onChange,
  onRefetch,
  onRemove,
  disabled,
}: {
  item: Item
  genres: { key: string; label: string }[]
  defaultGenre: string
  onChange: (p: Partial<Item>) => void
  onRefetch: () => void
  onRemove: () => void
  disabled: boolean
}) {
  const { t } = useI18n()
  const p = item.preview
  const matched = p?.matched && p.token
  const done = item.status === 'done'

  return (
    <li className="rounded-xl border border-ink-700 bg-ink-900 p-3">
      <div className="flex gap-3">
        {/* Cover */}
        <div className="aspect-[2/3] w-16 shrink-0 overflow-hidden rounded-lg bg-ink-800">
          {matched && p?.hasCover ? (
            <img
              src={mediaUrl.metaPreviewCover(p.token!)}
              alt=""
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-ink-600">
              <IconBook width={20} height={20} />
            </div>
          )}
        </div>

        {/* Details */}
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-2">
            <p className="min-w-0 flex-1 truncate text-[11px] text-slate-500">{item.file.name}</p>
            {done ? (
              <IconCheck width={16} height={16} className="shrink-0 text-emerald-400" />
            ) : item.status === 'uploading' ? (
              <Spinner className="h-4 w-4 shrink-0 text-accent-400" />
            ) : (
              !disabled && (
                <button
                  type="button"
                  onClick={onRemove}
                  className="shrink-0 rounded-lg p-1 text-slate-400 hover:bg-ink-700 hover:text-white"
                  aria-label={t('upload.removeFile')}
                >
                  <IconClose width={14} height={14} />
                </button>
              )
            )}
          </div>

          {/* Query + genre + refetch */}
          {!done && item.status !== 'uploading' && (
            <div className="mt-1.5 flex flex-wrap items-center gap-2">
              <input
                className="input h-8 min-w-0 flex-1 py-1 text-sm"
                value={item.query}
                placeholder={t('upload.searchTitle')}
                onChange={(e) => onChange({ query: e.target.value })}
                disabled={disabled}
              />
              <select
                className="input h-8 w-auto py-1 text-sm"
                value={item.genre || defaultGenre}
                onChange={(e) => onChange({ genre: e.target.value })}
                disabled={disabled}
              >
                {genres.map((g) => (
                  <option key={g.key} value={g.key}>
                    {g.label}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="btn-secondary h-8 px-2.5 py-1 text-sm"
                onClick={onRefetch}
                disabled={disabled || item.status === 'previewing'}
              >
                {item.status === 'previewing' ? (
                  <Spinner className="h-3.5 w-3.5" />
                ) : (
                  t('upload.refetch')
                )}
              </button>
            </div>
          )}

          {/* Status line */}
          <div className="mt-1.5 text-xs">
            {item.status === 'previewing' ? (
              <span className="text-slate-500">{t('upload.searching')}</span>
            ) : item.previewError ? (
              <span className="text-red-400">{item.previewError}</span>
            ) : item.status === 'error' ? (
              <span className="text-red-400">{item.error}</span>
            ) : matched ? (
              <div className="space-y-0.5">
                <p className="font-medium text-emerald-300">{p?.title}</p>
                <p className="text-slate-400">
                  {[p?.authors?.join(', '), p?.series, p?.publisher].filter(Boolean).join(' · ')}
                </p>
                {p?.tags && p.tags.length > 0 && (
                  <p className="truncate text-[11px] text-slate-500">{p.tags.join(', ')}</p>
                )}
              </div>
            ) : item.preview ? (
              <span className="text-amber-400">{t('upload.noMatchFilename')}</span>
            ) : null}
          </div>

          {item.status === 'uploading' && (
            <div className="mt-1.5 h-1 w-full overflow-hidden rounded-full bg-ink-700">
              <div className="h-full rounded-full bg-accent-500 transition-all" style={{ width: `${item.progress}%` }} />
            </div>
          )}
        </div>
      </div>
    </li>
  )
}
