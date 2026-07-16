export interface User {
  id: number
  username: string
  isAdmin: boolean
  source: string
  canDownload: boolean
  canUpload: boolean
  canEdit: boolean
  language: string
  pageSize: number
  sort: string // library sort field (per-account, shared across pages)
  sortOrder: SortOrder // "asc" | "desc"
  groupSeries: boolean // group volumes into series tiles
  showRecommended: boolean // show the home "Recommended for you" shelf
  showHistory: boolean // show the home "Continue reading" shelf
  createdAt: string
  canLogin?: boolean // admin list only: false when an LDAP user is outside the login group
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
  favorites: number // optional favorites/popularity count from the book's source; 0 = none
  identifiers: Record<string, string>
  comments: string
  formats: BookFormat[]
}

export interface BooksResponse {
  books: Book[]
  total: number
}

// A series collapsed to one tile in the grouped library view.
export interface SeriesCard {
  id: number
  name: string
  bookCount: number
  cover?: Book // latest volume, for the thumbnail
}

export type GroupUnit =
  | { kind: 'book'; book: Book }
  | { kind: 'series'; series: SeriesCard }

export interface GroupedResponse {
  units: GroupUnit[]
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
  ownerName: string
  name: string
  isPublic: boolean
  isDefault: boolean
  createdAt: string
  bookCount: number
  seriesCount: number
}

// A whole series added to a shelf: shown as one card that expands to its volumes.
export interface ShelfSeriesCard {
  id: number
  name: string
  bookCount: number
  cover?: Book // first volume, for the thumbnail
}

export interface ShelfContents {
  series: ShelfSeriesCard[]
  books: Book[]
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

export interface ReadingItem {
  book: Book
  page: number
  totalPages: number
  updatedAt: string
}

// One personalized suggestion: a book plus the strongest shared trait that
// earned it (for the "because you like …" caption). reasonKind is
// 'author' | 'series' | 'tag'; reasonName is that feature's name (may be empty).
export interface RecommendItem {
  book: Book
  reasonKind: 'author' | 'series' | 'tag' | ''
  reasonName: string
}

export interface Collection {
  id: number
  name: string
  tagIds: number[]
  excludeTagIds: number[]
  matchAny: boolean
  // A pinned sort: when `sort` is non-empty the collection always shows in this
  // order and the sort control is hidden. Empty = inherit the viewer's own sort.
  sort: string // '' | SortKey
  sortOrder: SortOrder // ignored when sort is ''
  position: number
  createdAt: string
}

// One externally-curated ranking list (a self-describing entry the server reads
// from its ranking side tables). Incipit ships no knowledge of what a list means
// — the label comes from the data. Books show in explicit rank order.
export interface RankingList {
  key: string
  label: string
  count: number
}

export interface SetupStatus {
  needsSetup: boolean
  needsLibrary: boolean
}

export type SortKey =
  | 'title'
  | 'timestamp'
  | 'pubdate'
  | 'author'
  | 'series'
  | 'rating'
  | 'favorites'
  | 'views'
  | 'lastread'
export type SortOrder = 'asc' | 'desc'

export interface SiteConfig {
  title: string
  pageFilter: boolean
  // Popularity ("favorites") feature toggle for this library instance: the ♥
  // count badge, the popularity sort, and the detail-page count. Enabled by an
  // admin only for a library whose books carry a favorites count.
  popularity: boolean
  // Reading-activity features toggle: the "recently read" / "most viewed" sort
  // options and the detail-page view count. On by default.
  readingActivity: boolean
  // Ranking-lists feature toggle: a dedicated "Rankings" nav section surfacing
  // externally-curated ordered lists. Off by default; shown only when enabled and
  // the library actually has ranking lists.
  rankings: boolean
  // Personalized recommendations toggle: the home "Recommended for you" shelf,
  // backed by an hourly precomputed cache. Off by default; enabled by an admin.
  recommendations: boolean
  // Base tag filter always applied to the home ("/") library view (display scope,
  // set by an admin in server settings): books scoped to homeTags (AND) and with
  // homeExcludeTags hidden (NOT).
  homeTags: number[]
  homeExcludeTags: number[]
  // How homeTags combine: false = all (AND), true = any (OR). Mirrors a collection.
  homeMatchAny: boolean
}

export interface PageIndexStatus {
  enabled: boolean
  running: boolean
  done: number
  total: number
}

export interface MetadataGenre {
  key: string
  label: string
}

export interface MetaPreview {
  matched: boolean
  token?: string
  title?: string
  authors?: string[]
  series?: string
  seriesIndex?: number
  tags?: string[]
  publisher?: string
  pubdate?: string
  rating?: number
  comments?: string
  isbn?: string
  hasCover?: boolean
}

export interface FsEntry {
  name: string
  path: string
}

export interface FsListing {
  path: string
  parent: string
  entries: FsEntry[]
}

export interface LibrarySettings {
  path: string
  readOnly: boolean
  configured: boolean
}

export interface LdapSettings {
  enabled: boolean
  url: string
  startTLS: boolean
  bindDN: string
  bindPasswordSet: boolean
  baseDN: string
  userFilter: string
  usernameAttribute: string
  adminGroupDN: string
  loginGroupDN: string
}

export interface LdapUpdate {
  enabled: boolean
  url: string
  startTLS: boolean
  bindDN: string
  bindPassword?: string // omit/empty keeps the stored password
  baseDN: string
  userFilter: string
  usernameAttribute: string
  adminGroupDN: string
  loginGroupDN: string
}

export interface LdapTestResult {
  ok: boolean
  error?: string
}

export interface LdapImportResult {
  scanned: number
  created: number
  existing: number
  createdUsernames: string[]
}

export interface BookUpdate {
  title?: string
  authors?: string[]
  series?: string
  seriesIndex?: number
  tags?: string[]
  addTags?: string[] // append tags (union) without removing existing ones
  publisher?: string
  languages?: string[]
  rating?: number
  comments?: string
  identifiers?: Record<string, string>
  pubdate?: string
}
