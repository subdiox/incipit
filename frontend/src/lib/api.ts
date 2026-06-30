import type {
  Book,
  BooksResponse,
  BookUpdate,
  Facet,
  FsListing,
  LdapImportResult,
  LdapSettings,
  LdapTestResult,
  LdapUpdate,
  LibrarySettings,
  MetadataGenre,
  MetaPreview,
  PagesResponse,
  Progress,
  ReadingItem,
  SetupStatus,
  Shelf,
  SiteConfig,
  SortKey,
  SortOrder,
  Stats,
  User,
} from '@/types'

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

function readCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/([.$?*|{}()[\]\\/+^])/g, '\\$1') + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : null
}

const MUTATION_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase()
  const headers = new Headers(init.headers)

  if (MUTATION_METHODS.has(method)) {
    const csrf = readCookie('incipit_csrf')
    if (csrf) headers.set('X-CSRF-Token', csrf)
  }

  // Only set JSON content-type when sending a non-FormData body.
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`/api${path}`, {
    ...init,
    method,
    headers,
    credentials: 'include',
  })

  if (!res.ok) {
    let message = `Request failed (${res.status})`
    try {
      const data = await res.json()
      if (data && typeof data.error === 'string') message = data.error
    } catch {
      // ignore non-JSON error bodies
    }
    throw new ApiError(message, res.status)
  }

  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

function jsonBody(body: unknown): RequestInit {
  return { body: JSON.stringify(body) }
}

export interface BookQuery {
  search?: string
  sort?: SortKey
  order?: SortOrder
  author?: number
  series?: number
  tags?: number[] // AND-combined
  publisher?: number
  language?: string
  limit?: number
  offset?: number
}

function bookQueryString(q: BookQuery): string {
  const params = new URLSearchParams()
  if (q.search) params.set('search', q.search)
  if (q.sort) params.set('sort', q.sort)
  if (q.order) params.set('order', q.order)
  if (q.author != null) params.set('author', String(q.author))
  if (q.series != null) params.set('series', String(q.series))
  if (q.tags) q.tags.forEach((t) => params.append('tag', String(t)))
  if (q.publisher != null) params.set('publisher', String(q.publisher))
  if (q.language) params.set('language', q.language)
  if (q.limit != null) params.set('limit', String(q.limit))
  if (q.offset != null) params.set('offset', String(q.offset))
  const s = params.toString()
  return s ? `?${s}` : ''
}

// ---- Media URL helpers (used directly as <img src>) ----
// v is a cache-busting version token (e.g. book.lastModified): the cover/
// thumbnail URLs are long-lived (max-age), so changing v forces a refetch after
// a cover edit.
export const mediaUrl = {
  thumbnail: (id: number, w = 400, v?: string) =>
    `/api/books/${id}/thumbnail?w=${w}${v ? `&v=${encodeURIComponent(v)}` : ''}`,
  cover: (id: number, v?: string) => `/api/books/${id}/cover${v ? `?v=${encodeURIComponent(v)}` : ''}`,
  page: (id: number, n: number, w?: number) =>
    `/api/books/${id}/pages/${n}${w ? `?w=${w}` : ''}`,
  file: (id: number) => `/api/books/${id}/file`,
  content: (id: number) => `/api/books/${id}/content`,
  metaPreviewCover: (token: string) => `/api/metadata/preview/${token}/cover`,
}

export const api = {
  // Setup & auth
  setupStatus: () => request<SetupStatus>('/setup/status'),
  site: () => request<SiteConfig>('/site'),
  browseFs: (path?: string) =>
    request<FsListing>(`/fs${path ? `?path=${encodeURIComponent(path)}` : ''}`),
  updateSite: (title: string) =>
    request<SiteConfig>('/admin/site', { method: 'PUT', ...jsonBody({ title }) }),
  setup: (username: string, password: string, libraryPath?: string) =>
    request<User>('/setup', {
      method: 'POST',
      ...jsonBody({ username, password, libraryPath }),
    }),
  login: (username: string, password: string) =>
    request<User>('/auth/login', { method: 'POST', ...jsonBody({ username, password }) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  me: () => request<User>('/auth/me'),
  setLanguage: (language: string) =>
    request<User>('/auth/me', { method: 'PUT', ...jsonBody({ language }) }),
  setPageSize: (pageSize: number) =>
    request<User>('/auth/me', { method: 'PUT', ...jsonBody({ pageSize }) }),

  // Metadata
  metadataGenres: () => request<MetadataGenre[]>('/metadata/genres'),
  metadataPreview: (body: { query: string; genre: string; metaAdd?: string; metaExclude?: string }) =>
    request<MetaPreview>('/metadata/preview', { method: 'POST', ...jsonBody(body) }),

  // Books
  books: (q: BookQuery = {}) => request<BooksResponse>(`/books${bookQueryString(q)}`),
  book: (id: number) => request<Book>(`/books/${id}`),
  createBook: (form: FormData) => request<Book>('/books', { method: 'POST', body: form }),
  updateBook: (id: number, body: BookUpdate) =>
    request<Book>(`/books/${id}`, { method: 'PUT', ...jsonBody(body) }),
  setBookCover: (id: number, form: FormData) =>
    request<Book>(`/books/${id}/cover`, { method: 'PUT', body: form }),
  deleteBook: (id: number) => request<void>(`/books/${id}`, { method: 'DELETE' }),

  // Reading
  pages: (id: number) => request<PagesResponse>(`/books/${id}/pages`),
  progress: (id: number) => request<Progress>(`/books/${id}/progress`),
  saveProgress: (id: number, page: number, totalPages: number) =>
    request<void>(`/books/${id}/progress`, { method: 'PUT', ...jsonBody({ page, totalPages }) }),
  resetProgress: (id: number) => request<void>(`/books/${id}/progress`, { method: 'DELETE' }),
  // status: 'continue' = unfinished only, 'finished' = read to the end,
  // 'all' = everything.
  myReading: (status: 'continue' | 'finished' | 'all', limit?: number) => {
    const s = status === 'continue' ? 'in-progress' : status
    return request<ReadingItem[]>(`/me/reading?status=${s}${limit ? `&limit=${limit}` : ''}`)
  },
  bookViews: (id: number) => request<{ views: number }>(`/books/${id}/views`),
  recordView: (id: number) => request<{ views: number }>(`/books/${id}/views`, { method: 'POST' }),

  // Facets & stats
  authors: () => request<Facet[]>('/authors'),
  series: () => request<Facet[]>('/series'),
  tags: () => request<Facet[]>('/tags'),
  publishers: () => request<Facet[]>('/publishers'),
  languages: () => request<Facet[]>('/languages'),
  stats: () => request<Stats>('/stats'),

  // Shelves
  shelves: () => request<Shelf[]>('/shelves'),
  createShelf: (name: string, isPublic: boolean) =>
    request<Shelf>('/shelves', { method: 'POST', ...jsonBody({ name, isPublic }) }),
  deleteShelf: (id: number) => request<void>(`/shelves/${id}`, { method: 'DELETE' }),
  shelfBooks: (id: number) => request<BooksResponse>(`/shelves/${id}/books`),
  addToShelf: (shelfId: number, bookId: number) =>
    request<void>(`/shelves/${shelfId}/books/${bookId}`, { method: 'POST' }),
  removeFromShelf: (shelfId: number, bookId: number) =>
    request<void>(`/shelves/${shelfId}/books/${bookId}`, { method: 'DELETE' }),

  // Admin
  adminUsers: () => request<User[]>('/admin/users'),
  createUser: (body: {
    username: string
    password: string
    isAdmin: boolean
    canDownload: boolean
    canUpload: boolean
    canEdit: boolean
  }) => request<User>('/admin/users', { method: 'POST', ...jsonBody(body) }),
  updateUser: (
    id: number,
    body: {
      password?: string
      isAdmin?: boolean
      canDownload?: boolean
      canUpload?: boolean
      canEdit?: boolean
    },
  ) => request<User>(`/admin/users/${id}`, { method: 'PUT', ...jsonBody(body) }),
  deleteUser: (id: number) => request<void>(`/admin/users/${id}`, { method: 'DELETE' }),

  // Admin · Library
  library: () => request<LibrarySettings>('/admin/library'),
  updateLibrary: (path: string) =>
    request<LibrarySettings>('/admin/library', { method: 'PUT', ...jsonBody({ path }) }),

  // Admin · LDAP
  ldap: () => request<LdapSettings>('/admin/ldap'),
  updateLdap: (body: LdapUpdate) =>
    request<LdapSettings>('/admin/ldap', { method: 'PUT', ...jsonBody(body) }),
  testLdap: () => request<LdapTestResult>('/admin/ldap/test', { method: 'POST' }),
  importLdap: () => request<LdapImportResult>('/admin/ldap/import', { method: 'POST' }),
}
