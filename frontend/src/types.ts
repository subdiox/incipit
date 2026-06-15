export interface User {
  id: number
  username: string
  isAdmin: boolean
  source: string
  canDownload: boolean
  canUpload: boolean
  canEdit: boolean
  createdAt: string
}

export interface AuthorRef {
  id: number
  name: string
  sort: string
}

export interface SeriesRef {
  id: number
  name: string
  sort: string
}

export interface TagRef {
  id: number
  name: string
}

export interface PublisherRef {
  id: number
  name: string
  sort: string
}

export interface BookFormat {
  format: string
  size: number
  name: string
}

export interface Book {
  id: number
  title: string
  sort: string
  timestamp: string
  pubdate: string
  seriesIndex: number
  authorSort: string
  path: string
  uuid: string
  hasCover: boolean
  lastModified: string
  authors: AuthorRef[]
  series?: SeriesRef
  tags: TagRef[]
  publisher?: PublisherRef
  languages: string[]
  rating: number // 0-10, 2 per star
  identifiers: Record<string, string>
  comments: string
  formats: BookFormat[]
}

export interface BooksResponse {
  books: Book[]
  total: number
}

export interface Facet {
  id: number
  name: string
  count: number
}

export interface Stats {
  books: number
  authors: number
  series: number
  tags: number
  publishers: number
}

export interface Shelf {
  id: number
  userId: number
  name: string
  isPublic: boolean
  createdAt: string
  bookCount: number
}

export interface Progress {
  bookId: number
  format: string
  page: number
  totalPages: number
  updatedAt: string
}

export interface PagesResponse {
  count: number
  pages: string[]
}

export interface SetupStatus {
  needsSetup: boolean
}

export type SortKey = 'title' | 'timestamp' | 'pubdate' | 'author' | 'series' | 'rating'
export type SortOrder = 'asc' | 'desc'

export interface BookUpdate {
  title?: string
  authors?: string[]
  series?: string
  seriesIndex?: number
  tags?: string[]
  publisher?: string
  languages?: string[]
  rating?: number
  comments?: string
  identifiers?: Record<string, string>
  pubdate?: string
}
