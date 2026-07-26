# CLAUDE.md — Incipit

Lightweight, modern, single-binary server for Calibre libraries. Comics (CBZ) are
the deepest-supported format — reader, per-user progress, auto-cover, OPDS-PSE —
with in-browser readers for EPUB and PDF too; any format can be stored/served.
Clean-room reimplementation (MIT) — reads the Calibre *format*, not its source.

## Architecture
- **Single container**: Go binary with the React SPA embedded via `web/embed.go` (`//go:embed all:dist`). No Node at runtime.
- **Two separate SQLite databases — never mix them**:
  - `metadata.db` (Calibre's, under `INCIPIT_LIBRARY`): the portable library. Accessed by `internal/calibre`. **Never run schema migrations against it.**
  - `app.db` (under `INCIPIT_CONFIG`): Incipit's own state (users, sessions, shelves, reading progress, CBZ page-list cache). Owned by `internal/appdb`, has its own migrator.
- SQLite driver is `modernc.org/sqlite` (pure Go, CGO-free → static binary / scratch image).

## Critical gotcha: Calibre's custom SQL functions
Calibre's triggers call application-defined SQL functions. Writing to `metadata.db`
with a plain driver fails with `no such function: title_sort`. We register Go
implementations globally in `internal/calibre/sqlfuncs.go`:
- `title_sort(title)` — moves a leading article: "The Hobbit" → "Hobbit, The".
- `author_to_author_sort(name)` — "First Last" → "Last, First".
- `uuid4()` — random v4 UUID for `books.uuid`.
Registration is via `sqlite.MustRegister[Deterministic]ScalarFunction` (global to all connections opened afterward), guarded by a `sync.Once`. The embedded `schema.sql` includes the `books_insert_trg` / `books_update_trg` / `books_delete_trg` triggers that exercise these.

## Read path performance (`internal/calibre/adapter.go`)
A 300k-book library is where listing queries either work or fall off a cliff, and the cliff is steep — the two rules below were worth ~100x on a real library.
- **Category filters are `b.id IN (SELECT book FROM <link> WHERE <cat>=?)`, never correlated `EXISTS`.** The EXISTS form reads better but makes the planner SCAN all of `books` (via a covering index, in *sort* order) and probe the link table once per book: work proportional to the whole library, every probe a random read. The IN form drives from the category index instead and touches only matching books. Measured: 48s → 1s for one tag filter on 300k books. `buildFilters` and the search clause both follow this; negative filters (exclude-tag, not-in-series) stay `NOT EXISTS`, since a negative can't drive a scan anyway.
- **Both databases open with a 256 MiB page cache** (`INCIPIT_DB_CACHE_MB`, 0 = SQLite's stock 2 MiB), and the pool is capped at 8 connections because the cache is per-connection. SQLite's default is far too small once the file is hundreds of MB, and the miss cost depends on where the library lives — on ZFS with lz4 and a 128K recordsize, every 4K page miss decompresses a 128K record.
- Deployments accumulate b-tree fragmentation as books are added; `VACUUM` on metadata.db is worth ~2-3x on large filters and is the right first move when listings get slow again.

## Write path invariants (`internal/calibre/write.go`)
- **Single serialized writer** (`writeMu` mutex) + WAL + busy_timeout. Incipit assumes it's the primary accessor (like calibre-web); `INCIPIT_READONLY=true` to share with desktop Calibre.
- Folder layout `Author/Title (id)/`; files named `Title - Author.<ext>`; cover is `cover.jpg`; `metadata.opf` is written for round-trip safety.
- Title/author edits **move the folder and rename files** (`relocateBook`), updating `books.path` and `data.name`, with FS rollback if the tx fails.
- Path components are sanitized + truncated (`path.go`, `maxComponent`).

## CBZ reader (`internal/reader`)
- `Pages()` opens the ZIP and reads only the **central directory** (not the archive contents); pages are sorted with a **natural** comparator (`natsort.go`) so `page2 < page10`.
- Single pages are extracted on demand; `?w=` resizes via `disintegration/imaging` (pure Go) and caches JPEGs on disk keyed by `(path, entry, width, mtime)`.
- Page-list is cached in `app.db` (`page_cache`), invalidated by CBZ mtime/size.

## Metadata fetch (`internal/metadata`)
- Clean-room Go port of the original Python "ookamura" uploader: scrapes **コミックシーモア (cmoa.jp)** public HTML (via `goquery`, pure Go) to enrich an upload from its (file)name — authors, publisher, pubdate, comments, tags, rating, official cover.
- Genre filtering (`GenreChoices`, the single source of truth; frontend mirrors the keys) avoids matching a same-named work in the wrong category. `"comic"` fans out across manga genres and takes the first hit; `"all"` is unfiltered.
- `Fetch()` distinguishes **transport failure (`ErrFetch`)** from **no match (`nil, nil`)** — 404 is cmoa's "no result in this genre", so the search step tolerates it and tries the next genre.
- Rating is stored on Calibre's ×2 scale (4.5 stars → 9). Cover is downloaded from cmoa and re-encoded JPEG, overriding the CBZ first-page cover (falls back to it on failure).
- Wired into upload via `handleAddBook` form fields `fetchMeta`/`genre`/`metaAdd`/`metaExclude`; a no-match still uploads (filename metadata) and sets the `X-Metadata-Matched: false` response header. Genres are served at `GET /api/metadata/genres`. The calibre-web-specific cover color-correction LUT was intentionally **not** ported (incipit does no ImageMagick transform).

## Auth (`internal/auth`)
- Local: argon2id PHC-encoded hashes. Sessions: server-side tokens in `app.db`, httpOnly SameSite cookie.
- **CSRF**: double-submit. Server sets a readable `incipit_csrf` cookie = HMAC(session-seed, secret); mutations must echo it in `X-CSRF-Token`. Enforced in `csrfProtect` for unsafe methods only.
- LDAP and reverse-proxy are behind the `ExternalAuthenticator` interface so login logic is unit-testable without a live server.
- HTTP Basic is accepted as a fallback in `authenticate()` so OPDS clients reuse `/api` cover/download URLs.

## Testing
- No Calibre CLI in this environment, so round-trip is verified against the embedded clean-room schema fixture (`go test ./internal/calibre` adds/edits/deletes and checks folders, files, OPF, cascade).
- `internal/httpapi/httpapi_test.go` is a full-stack httptest run (setup → login → multipart CBZ upload w/ auto cover → list → page render → progress → edit → shelves → admin → OPDS → CSRF/auth enforcement).
- Headless gate: `make test` == `go test ./...`. Frontend gate: `npm run build` + `go build ./...` (embed must compile).

## Conventions
- Swift/Apple notes do NOT apply here (this is a Go + TS web project).
- Commit trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
