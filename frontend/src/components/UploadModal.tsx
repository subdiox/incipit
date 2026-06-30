import { useRef, useState, type DragEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { useI18n } from '@/i18n'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { IconUpload, IconClose, IconCheck } from './icons'

interface UploadModalProps {
  open: boolean
  onClose: () => void
}

const ALLOWED = ['cbz', 'cbr', 'epub', 'pdf', 'mobi', 'azw3', 'fb2', 'txt']

type Status = 'queued' | 'uploading' | 'done' | 'error'
interface Item {
  id: string
  file: File
  status: Status
  progress: number
  error?: string
  metaMatched?: boolean
}

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}
function stripExt(name: string): string {
  return name.replace(/\.[^.]+$/, '')
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
  const [title, setTitle] = useState('')
  const [authors, setAuthors] = useState('')
  const [series, setSeries] = useState('')
  const [seriesIndex, setSeriesIndex] = useState('')
  const [tags, setTags] = useState('')
  const [publisher, setPublisher] = useState('')
  const [fetchMeta, setFetchMeta] = useState(false)
  const [genre, setGenre] = useState('comic')
  const [metaAdd, setMetaAdd] = useState('')
  const [metaExclude, setMetaExclude] = useState('')
  const [dragOver, setDragOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Search genres are fetched lazily; the backend is the source of truth.
  const genresQuery = useQuery({
    queryKey: ['metadata-genres'],
    queryFn: api.metadataGenres,
    enabled: open,
    staleTime: Infinity,
  })
  const genres = genresQuery.data ?? []

  const single = items.length === 1

  const reset = () => {
    setItems([])
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
    if (uploading) return
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
        accepted.push({ id: `f${idSeq.current}`, file, status: 'queued', progress: 0 })
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

  const removeItem = (id: string) => {
    setItems((prev) => prev.filter((it) => it.id !== id))
  }

  const patch = (id: string, p: Partial<Item>) =>
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...p } : it)))

  const uploadOne = (item: Item, isSingle: boolean) =>
    new Promise<boolean>((resolve) => {
      const form = new FormData()
      form.append('file', item.file)
      const bookTitle = isSingle ? title || stripExt(item.file.name) : stripExt(item.file.name)
      if (bookTitle) form.append('title', bookTitle)
      if (fetchMeta) {
        // Server fetches metadata + cover from cmoa using the title/filename.
        form.append('fetchMeta', 'true')
        form.append('genre', genre)
        if (metaAdd) form.append('metaAdd', metaAdd)
        if (metaExclude) form.append('metaExclude', metaExclude)
      } else {
        if (authors) form.append('authors', authors)
        if (series) form.append('series', series)
        if (seriesIndex) form.append('seriesIndex', seriesIndex)
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
          // The server flags an enrichment miss so the user can fix it later.
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

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (items.length === 0) {
      setError(t('upload.chooseFirst'))
      return
    }
    setError(null)
    setUploading(true)
    const isSingle = items.length === 1
    let failed = 0
    // Sequential: the library has a single serialized writer.
    for (const item of items) {
      if (item.status === 'done') continue
      const ok = await uploadOne(item, isSingle)
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

  return (
    <Modal open={open} onClose={close} title={t('upload.title')} maxWidth="max-w-xl">
      <form onSubmit={submit} className="space-y-4">
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
          <ul className="max-h-48 space-y-1.5 overflow-y-auto">
            {items.map((it) => (
              <li
                key={it.id}
                className="flex items-center gap-3 rounded-xl border border-ink-700 bg-ink-900 px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm text-slate-200">{it.file.name}</p>
                  <p className="text-[11px] text-slate-500">
                    {formatBytes(it.file.size)}
                    {it.status === 'uploading' && ` · ${it.progress}%`}
                    {it.status === 'error' && it.error && (
                      <span className="text-red-400"> · {it.error}</span>
                    )}
                    {it.status === 'done' && it.metaMatched === false && (
                      <span className="text-amber-400"> · {t('upload.metaUnmatched')}</span>
                    )}
                  </p>
                  {it.status === 'uploading' && (
                    <div className="mt-1 h-1 w-full overflow-hidden rounded-full bg-ink-700">
                      <div className="h-full rounded-full bg-accent-500 transition-all" style={{ width: `${it.progress}%` }} />
                    </div>
                  )}
                </div>
                {it.status === 'done' ? (
                  <IconCheck width={16} height={16} className="text-emerald-400" />
                ) : it.status === 'uploading' ? (
                  <Spinner className="h-4 w-4 text-accent-400" />
                ) : (
                  !uploading && (
                    <button
                      type="button"
                      onClick={() => removeItem(it.id)}
                      className="rounded-lg p-1.5 text-slate-400 hover:bg-ink-700 hover:text-white"
                      aria-label={t('upload.removeFile')}
                    >
                      <IconClose width={14} height={14} />
                    </button>
                  )
                )}
              </li>
            ))}
          </ul>
        )}

        {/* Metadata */}
        {items.length > 0 && (
          <div className="space-y-4">
            {/* Auto-fetch from filename */}
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

            {fetchMeta && (
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
              </div>
            )}

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {single ? (
                <div className="sm:col-span-2">
                  <label className="label">{fetchMeta ? t('upload.searchTitle') : t('book.fieldTitle')}</label>
                  <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
                  {fetchMeta && (
                    <p className="mt-1 text-[11px] text-slate-600">{t('upload.searchTitleHint')}</p>
                  )}
                </div>
              ) : (
                <p className="sm:col-span-2 text-xs text-slate-500">
                  {fetchMeta ? t('upload.searchFromName') : t('upload.titleFromName')}
                </p>
              )}

              {!fetchMeta && (
                <>
                  <div className="sm:col-span-2">
                    <label className="label">{t('book.fieldAuthors')}</label>
                    <input className="input" value={authors} onChange={(e) => setAuthors(e.target.value)} />
                  </div>
                  <div>
                    <label className="label">{t('book.fieldSeries')}</label>
                    <input className="input" value={series} onChange={(e) => setSeries(e.target.value)} />
                  </div>
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
                  <div>
                    <label className="label">{t('book.fieldTags')}</label>
                    <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} />
                  </div>
                  <div>
                    <label className="label">{t('book.fieldPublisher')}</label>
                    <input className="input" value={publisher} onChange={(e) => setPublisher(e.target.value)} />
                  </div>
                </>
              )}
            </div>

            {fetchMeta ? (
              <p className="text-[11px] text-slate-600">{t('upload.metaFromSource')}</p>
            ) : (
              !single && <p className="text-[11px] text-slate-600">{t('upload.appliedToAll')}</p>
            )}
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-secondary" onClick={close} disabled={uploading}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={uploading || items.length === 0}>
            {uploading && <Spinner className="h-4 w-4" />}
            {items.length > 1 ? t('upload.uploadCount', { count: items.length }) : t('upload.upload')}
          </button>
        </div>
      </form>
    </Modal>
  )
}
