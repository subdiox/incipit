# Incipit

A lightweight, modern, single-binary server for [Calibre](https://calibre-ebook.com/)
libraries — browse your collection, read **CBZ** comics in the browser, manage
metadata, and serve OPDS, with multi-user access (local / LDAP / reverse-proxy).

Incipit is a clean-room reimplementation that **reuses the Calibre library format**
(`metadata.db` + the `Author/Title (id)/` folder layout) so your library stays
100% compatible with desktop Calibre, while being far lighter and nicer to use
than calibre-web. It ships as one container, ~30 MB, no Calibre binaries required.

> *incipit* (Latin, "it begins") — the opening words of a manuscript.

## Highlights

- **Single container.** Go binary with the SPA embedded; distroless image, tiny footprint, no Node at runtime.
- **Calibre-compatible.** Reads and writes `metadata.db` directly, replicating Calibre's invariants — including the `title_sort` / `uuid4` SQL functions its triggers depend on — and writes `metadata.opf` for round-trip safety.
- **In-browser CBZ reader.** Pages are extracted one at a time from the ZIP central directory (the whole archive is never unpacked), resized on demand, and cached.
- **OPDS 1.2** catalog for external reader apps (HTTP Basic auth).
- **Auth:** local accounts (argon2id), LDAP, and reverse-proxy header auth — pluggable.
- **Multi-user:** per-user shelves, reading progress, and download/upload/edit permissions.

## Architecture

```
incipit (single Go binary)
├─ HTTP (chi)
│  ├─ /api/*                REST/JSON consumed by the embedded React SPA
│  ├─ /opds/*               OPDS 1.2 feeds (HTTP Basic auth)
│  ├─ /books/{id}/pages/{n} CBZ page images (server-side extraction + resize)
│  └─ /*                    SPA (history-API fallback)
├─ internal/calibre   read+write adapter for metadata.db (single serialized writer, WAL)
├─ internal/appdb     Incipit's own state: users, sessions, shelves, progress, page cache
├─ internal/auth      argon2id, login service, LDAP, reverse-proxy resolution
├─ internal/reader    CBZ central-directory extraction, natural page ordering, resize cache
└─ internal/httpapi   handlers, middleware (session/CSRF/rate-limit/logging), OPDS
```

Two databases are kept **separate on purpose**: the Calibre `metadata.db` (the
library, portable and desktop-Calibre-compatible) and Incipit's `app.db` (users,
sessions, shelves, reading progress, the CBZ page-list cache). Incipit never runs
schema migrations against `metadata.db`.

### Writing to the Calibre library safely

- All writes go through a **single serialized writer** with `WAL` and a high `busy_timeout`.
- Incipit assumes it is the primary accessor of the library (as calibre-web does). Set `INCIPIT_READONLY=true` to share a library with a running desktop Calibre.
- Adding/editing a book creates the `Author/Title (id)/` folder, writes the CBZ, `cover.jpg` (generated from the first page) and `metadata.opf`, and moves/renames files when the title or author changes.

## Quick start (Docker)

```bash
# Put your Calibre library under ./library (or let Incipit create an empty one).
docker compose up --build
# Open http://localhost:8080 and create the first admin account.
```

## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `INCIPIT_ADDR` | `:8080` | Listen address |
| `INCIPIT_LIBRARY` | `/library` | Calibre library directory (`metadata.db` lives here) |
| `INCIPIT_CONFIG` | `/config` | Incipit state: `app.db`, image cache, session key |
| `INCIPIT_READONLY` | `false` | Disable all writes to the Calibre library |
| `INCIPIT_SECURE_COOKIES` | `false` | Mark cookies `Secure` (enable behind HTTPS) |
| `INCIPIT_SESSION_SECRET` | *(generated)* | Cookie-signing secret (persisted under config if unset) |
| `INCIPIT_LDAP_ENABLED` | `false` | Enable LDAP auth |
| `INCIPIT_LDAP_URL` | | `ldap://host:389` or `ldaps://host:636` |
| `INCIPIT_LDAP_BIND_DN` | | Bind DN template, `%s` = username (e.g. `uid=%s,ou=people,dc=example,dc=com`) |
| `INCIPIT_LDAP_ADMIN_GROUP_DN` | | Members of this group become admins |
| `INCIPIT_LDAP_STARTTLS` | `false` | Use StartTLS |
| `INCIPIT_PROXY_AUTH_ENABLED` | `false` | Trust a reverse proxy for auth |
| `INCIPIT_PROXY_AUTH_HEADER` | `X-Authenticated-User` | Header carrying the username |
| `INCIPIT_PROXY_AUTH_ADMIN_HEADER` | | Header whose presence grants admin |

## Development

```bash
make frontend     # build the SPA into web/dist
make build        # build the Go binary (embeds web/dist)
make run          # run against ./library and ./config
make test         # full headless test suite (go test ./...)
make docker       # build the single-container image
```

Frontend dev server (proxies `/api` to a locally running `incipit`):

```bash
cd frontend && npm install && npm run dev
```

## Status

v1 focuses on **CBZ**. EPUB/PDF and format conversion are intentionally out of
scope for now (no Calibre binaries needed). The metadata model, OPDS, auth, and
multi-user features are format-agnostic and ready to extend.

## License

MIT — this is a clean-room implementation that reads the Calibre *format* (a fact),
not Calibre or calibre-web source code.
