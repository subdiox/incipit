import { useRef, useState, type DragEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { formatBytes } from '@/lib/format'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { IconUpload, IconClose } from './icons'

interface UploadModalProps {
  open: boolean
  onClose: () => void
}

export function UploadModal({ open, onClose }: UploadModalProps) {
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [authors, setAuthors] = useState('')
  const [series, setSeries] = useState('')
  const [seriesIndex, setSeriesIndex] = useState('')
  const [tags, setTags] = useState('')
  const [publisher, setPublisher] = useState('')
  const [dragOver, setDragOver] = useState(false)
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  const reset = () => {
    setFile(null)
    setTitle('')
    setAuthors('')
    setSeries('')
    setSeriesIndex('')
    setTags('')
    setPublisher('')
    setProgress(null)
    setError(null)
  }

  const close = () => {
    if (progress != null) return // don't close mid-upload
    reset()
    onClose()
  }

  const pickFile = (f: File | null) => {
    if (!f) return
    if (!f.name.toLowerCase().endsWith('.cbz')) {
      setError('Please choose a .cbz file.')
      return
    }
    setError(null)
    setFile(f)
    if (!title) setTitle(f.name.replace(/\.cbz$/i, ''))
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    pickFile(e.dataTransfer.files?.[0] ?? null)
  }

  // Use XHR for upload progress; mirrors api.createBook (multipart + CSRF).
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!file) {
      setError('Please choose a .cbz file first.')
      return
    }
    setError(null)
    setProgress(0)

    const form = new FormData()
    form.append('file', file)
    if (title) form.append('title', title)
    if (authors) form.append('authors', authors)
    if (series) form.append('series', series)
    if (seriesIndex) form.append('seriesIndex', seriesIndex)
    if (tags) form.append('tags', tags)
    if (publisher) form.append('publisher', publisher)

    const csrf = (document.cookie.match(/(?:^|; )incipit_csrf=([^;]*)/) ?? [])[1]
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/books')
    xhr.withCredentials = true
    if (csrf) xhr.setRequestHeader('X-CSRF-Token', decodeURIComponent(csrf))

    xhr.upload.onprogress = (ev) => {
      if (ev.lengthComputable) setProgress(Math.round((ev.loaded / ev.total) * 100))
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        queryClient.invalidateQueries({ queryKey: ['books'] })
        queryClient.invalidateQueries({ queryKey: ['stats'] })
        queryClient.invalidateQueries({ queryKey: ['facets'] })
        reset()
        onClose()
      } else {
        let msg = `Upload failed (${xhr.status})`
        try {
          const data = JSON.parse(xhr.responseText)
          if (data?.error) msg = data.error
        } catch {
          /* ignore */
        }
        setError(msg)
        setProgress(null)
      }
    }
    xhr.onerror = () => {
      setError('Network error during upload.')
      setProgress(null)
    }
    xhr.send(form)
  }

  const uploading = progress != null

  return (
    <Modal open={open} onClose={close} title="Upload comic" maxWidth="max-w-xl">
      <form onSubmit={submit} className="space-y-4">
        {!file ? (
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault()
              setDragOver(true)
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            className={`flex w-full flex-col items-center justify-center gap-3 rounded-2xl border-2 border-dashed px-6 py-12 text-center transition-colors ${
              dragOver
                ? 'border-accent-500 bg-accent-500/10'
                : 'border-ink-600 bg-ink-900 hover:border-accent-500/50'
            }`}
          >
            <IconUpload className="text-accent-400" width={28} height={28} />
            <div>
              <p className="text-sm font-medium text-slate-200">Drag &amp; drop a .cbz file</p>
              <p className="mt-0.5 text-xs text-slate-500">or click to browse</p>
            </div>
          </button>
        ) : (
          <div className="flex items-center justify-between gap-3 rounded-xl border border-ink-600 bg-ink-900 px-4 py-3">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-slate-200">{file.name}</p>
              <p className="text-xs text-slate-500">{formatBytes(file.size)}</p>
            </div>
            {!uploading && (
              <button
                type="button"
                onClick={() => setFile(null)}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-ink-700 hover:text-white"
                aria-label="Remove file"
              >
                <IconClose width={16} height={16} />
              </button>
            )}
          </div>
        )}

        <input
          ref={fileInputRef}
          type="file"
          accept=".cbz,application/vnd.comicbook+zip,application/zip"
          className="hidden"
          onChange={(e) => pickFile(e.target.files?.[0] ?? null)}
        />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="label">Title</label>
            <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="sm:col-span-2">
            <label className="label">Authors (comma separated)</label>
            <input className="input" value={authors} onChange={(e) => setAuthors(e.target.value)} />
          </div>
          <div>
            <label className="label">Series</label>
            <input className="input" value={series} onChange={(e) => setSeries(e.target.value)} />
          </div>
          <div>
            <label className="label">Series index</label>
            <input
              className="input"
              type="number"
              step="0.1"
              value={seriesIndex}
              onChange={(e) => setSeriesIndex(e.target.value)}
            />
          </div>
          <div>
            <label className="label">Tags (comma separated)</label>
            <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} />
          </div>
          <div>
            <label className="label">Publisher</label>
            <input className="input" value={publisher} onChange={(e) => setPublisher(e.target.value)} />
          </div>
        </div>

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}

        {uploading && (
          <div>
            <div className="mb-1.5 flex justify-between text-xs text-slate-400">
              <span>Uploading…</span>
              <span>{progress}%</span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-ink-700">
              <div
                className="h-full rounded-full bg-accent-500 transition-all"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-secondary" onClick={close} disabled={uploading}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={uploading || !file}>
            {uploading && <Spinner className="h-4 w-4" />}
            Upload
          </button>
        </div>
      </form>
    </Modal>
  )
}
