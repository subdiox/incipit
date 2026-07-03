# Incipit

A lightweight, modern, single-binary server for [Calibre](https://calibre-ebook.com/)
comic libraries — browse your collection, **read CBZ comics (and EPUB/PDF) in the
browser**, enrich metadata, and serve OPDS, with multi-user access
(local / LDAP / reverse-proxy).

Incipit is a clean-room reimplementation that **reuses the Calibre library format**
(`metadata.db` + the `Author/Title (id)/` folder layout), so your library stays
100% compatible with desktop Calibre while being far lighter and nicer to use
than calibre-web. It ships as **one small container** — a static Go binary with
the React SPA embedded, no Calibre binaries and no Node at runtime.

> *incipit* (Latin, "it begins") — the opening words of a manuscript.

## Highlights

- **Single static binary.** Pure-Go (CGO-free) build with the SPA embedded via `go:embed`; distroless image, tiny footprint.
- **Calibre-compatible.** Reads and writes `metadata.db` directly, replicating Calibre's invariants — including the `title_sort` / `author_to_author_sort` / `uuid4` SQL functions its triggers depend on — and writes `metadata.opf` for round-trip safety. Your library stays portable.
- **In-browser readers.** A purpose-built **CBZ** reader (single/spread pages, left/right binding, fit-width/height, fullscreen, keyboard/tap/swipe, a draggable seek bar) plus **EPUB** and **PDF** viewers. Reading progress is saved per user and resumes where you left off.
- **Fast at scale.** Server-side full-text search across every field, gzipped API responses, and server-side tag/author search that stays instant on libraries with 100k+ tags.
- **Metadata enrichment.** Fills a book's authors, publisher, pubdate, description, tags, rating and official cover by scraping コミックシーモア (cmoa.jp) from the (file)name — on upload or on demand.
- **Organize.** Admin-defined **collections** (saved tag filters shown under Library), per-user **shelves** (with a built-in Favorites shelf), and an optional home-library base filter.
- **OPDS 1.2** catalog (+ OPDS-PSE page streaming) for external reader apps.
- **Auth:** local accounts (argon2id), LDAP, and reverse-proxy header auth — pluggable — with per-user download/upload/edit/admin permissions.
- **Bilingual UI:** English and Japanese.

## Architecture

```
incipit (single Go binary)
├─ HTTP (chi)
│  ├─ /api/*                REST/JSON consumed by the embedded React SPA (gzipped)
│  ├─ /opds/*               OPDS 1.2 feeds + PSE page streaming (HTTP Basic auth)
│  └─ /*                    SPA (history-API fallback)
├─ internal/calibre   read+write adapter for metadata.db (single serialized writer, WAL)
├─ internal/appdb     Incipit's own state: users, sessions, shelves, progress, collections, caches
├─ internal/auth      argon2id, login service, LDAP, reverse-proxy resolution
├─ internal/reader    CBZ central-directory extraction, natural page ordering, resize cache
├─ internal/metadata  コミックシーモア (cmoa.jp) scraper for metadata + covers
└─ internal/httpapi   handlers, middleware (session/CSRF/rate-limit/gzip/logging), OPDS, facets
```

Two databases are kept **separate on purpose**:

- **`metadata.db`** — Calibre's, under `INCIPIT_LIBRARY`. The portable library.
  Incipit reads/writes it but **never runs schema migrations against it**.
- **`app.db`** — Incipit's own, under `INCIPIT_CONFIG`. Users, sessions, shelves,
  reading progress, collections, site settings, and the CBZ page-list cache. It
  has its own migrator.

The SQLite driver is `modernc.org/sqlite` (pure Go), which is what keeps the
binary CGO-free and lets it run on a distroless `static` image.

### Writing to the Calibre library safely

- All writes go through a **single serialized writer** with `WAL` and a high `busy_timeout`.
- Incipit assumes it is the primary accessor of the library (as calibre-web does). Set `INCIPIT_READONLY=true` to share a library with a running desktop Calibre.
- Adding/editing a book creates the `Author/Title (id)/` folder, writes the file, `cover.jpg`, and `metadata.opf`, and **moves/renames** files when the title or author changes (with filesystem rollback if the transaction fails).

## CBZ reader

- Opens the ZIP and reads only the **central directory** — the whole archive is
  never unpacked — and orders pages with a **natural** comparator (`page2` before
  `page10`).
- Pages are extracted on demand, resized with a pure-Go image library, and cached
  on disk keyed by `(path, entry, width, mtime)`. The page list is cached in
  `app.db` and invalidated by the CBZ's mtime/size.
- Reading UI: single or two-page **spread**, left/right **binding direction**
  (manga vs. western), fit-width / fit-height, fullscreen, keyboard / tap-zone /
  swipe navigation, and a **draggable progress bar** to jump to any page.

## Search & browsing

- **Full-text search** spans title, author, series (name + volume), tags,
  publisher and the description — optimized to stay fast on large libraries.
- **Sorts:** recently added, title, author, series, publish date, rating, view
  count, and last read.
- **Filters:** author, series and tag facets (tags/authors are searched
  server-side so huge categories load instantly), plus an optional page-count
  filter (indexed in the background when enabled by an admin).
- **Collections:** admins define saved tag filters — include tags combined by
  AND or OR, plus exclude tags — that appear as their own entries under Library.
- **Home library filter:** an optional admin base filter (include/exclude tags)
  applied to the default home view; it steps aside automatically when the user
  searches or filters so nothing is unreachable.

## Metadata enrichment

A clean-room Go port of the original Python "ookamura" uploader scrapes public
コミックシーモア (cmoa.jp) HTML to enrich a book from its (file)name: authors,
publisher, pubdate, description, tags, rating and the official cover. Genre
filtering avoids matching a same-named work in the wrong category, and
extra/excluded search terms let you refine matches (e.g. exclude `単話版`).
It runs on upload (a no-match still uploads with filename metadata) or on demand
from a book's detail page.

## Multi-user

- **Accounts & permissions:** per-user download / upload / edit / admin flags.
- **Shelves:** per-user, public or private, with a built-in undeletable
  **Favorites** shelf everyone gets by default.
- **Reading:** per-user progress with a "Continue reading" shelf and history.

## Quick start (Docker)

```bash
# Put your Calibre library under ./library (or let Incipit create an empty one),
# and create a ./config directory owned by the same user as the library.
docker compose up --build
# Open http://localhost:8080 and create the first admin account.
```

`docker-compose.yml` in this repo is a documented template. Note the `user:`
setting: run as the host user that **owns** the Calibre library so Incipit can
move/rename book folders and write covers (`./config` must be owned by the same
uid).

## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `INCIPIT_ADDR` | `:8080` | Listen address |
| `INCIPIT_LIBRARY` | *(setup)* | Calibre library directory (`metadata.db` lives here). If unset, chosen during first-run setup and stored in `app.db` |
| `INCIPIT_CONFIG` | `/config` | Incipit state: `app.db`, image cache, session key |
| `INCIPIT_READONLY` | `false` | Disable all writes to the Calibre library |
| `INCIPIT_SECURE_COOKIES` | `false` | Force `Secure` on session cookies (they are also marked Secure per-request when `X-Forwarded-Proto: https`) |
| `INCIPIT_SESSION_SECRET` | *(generated)* | Cookie-signing secret (generated and persisted under config if unset) |
| `INCIPIT_LDAP_ENABLED` | `false` | Enable LDAP auth |
| `INCIPIT_LDAP_URL` | | `ldap://host:389` or `ldaps://host:636` |
| `INCIPIT_LDAP_BIND_DN` | | Bind DN template, `%s` = username (e.g. `uid=%s,ou=people,dc=example,dc=com`) |
| `INCIPIT_LDAP_BASE_DN` | | Search base for user/group lookups |
| `INCIPIT_LDAP_USER_FILTER` | `(uid=%s)` | User search filter, `%s` = username |
| `INCIPIT_LDAP_ADMIN_GROUP_DN` | | Members of this group become admins |
| `INCIPIT_LDAP_LOGIN_GROUP_DN` | | When set, only members of this group may log in / be imported |
| `INCIPIT_LDAP_STARTTLS` | `false` | Use StartTLS |
| `INCIPIT_PROXY_AUTH_ENABLED` | `false` | Trust a reverse proxy for auth (only behind a trusted proxy) |
| `INCIPIT_PROXY_AUTH_HEADER` | `X-Authenticated-User` | Header carrying the username |
| `INCIPIT_PROXY_AUTH_ADMIN_HEADER` | | Header whose presence/value grants admin |
| `INCIPIT_PROXY_AUTH_AUTOCREATE` | `true` | Create a local user record on first sight |

The LDAP connection (including the bind password, which is **only** stored in
`app.db`, never in env) is managed in the admin UI under **Admin → LDAP**; the
env vars just seed the initial values on first run.

### Authentication notes

- **Local:** argon2id password hashes; server-side session tokens in `app.db`
  behind an httpOnly, SameSite cookie.
- **CSRF:** double-submit token — the server sets a readable `incipit_csrf`
  cookie and unsafe requests must echo it in `X-CSRF-Token`.
- **HTTP Basic** is accepted as a fallback so OPDS clients can reuse the
  `/api` cover/download URLs.

## Development

```bash
make frontend     # build the SPA into web/dist
make build        # build the static Go binary (embeds web/dist)
make run          # run against ./config (library path via first-run setup or INCIPIT_LIBRARY)
make test         # full headless test suite (go test ./...)
make vet          # go vet ./...
make seed         # populate ./library with a few sample CBZ comics (ARGS="-reset" to replace)
make docker       # build the single-container image
```

Frontend dev server (proxies `/api` to a locally running `incipit`):

```bash
cd frontend && npm install && npm run dev
```

### Testing

- `internal/calibre` verifies the add/edit/delete round-trip (folders, files,
  OPF, cascade) against an embedded clean-room copy of Calibre's schema — no
  Calibre CLI required.
- `internal/httpapi` is a full-stack `httptest` run: setup → login → multipart
  CBZ upload with auto-cover → list → page render → progress → edit → shelves →
  collections → admin → OPDS → CSRF/auth enforcement.
- Gate: `make test` == `go test ./...`; the frontend gate is `npm run build`
  followed by `go build ./...` (the embed must compile).

## Supported formats

Comic **CBZ** is the primary target (upload, auto-cover, reader, metadata
enrichment). **EPUB** and **PDF** also open in the browser. Any format can be
stored and downloaded; format conversion is out of scope (no Calibre binaries).

## License

MIT — a clean-room implementation that reads the Calibre *format* (a fact), not
Calibre or calibre-web source code.
