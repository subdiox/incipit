-- Clean-room reconstruction of the Calibre library schema (metadata.db).
-- Faithful to Calibre's structure and, crucially, to the triggers that call
-- application-defined SQL functions (title_sort, uuid4). Any client that writes
-- to this database MUST register those functions on its connection or inserts
-- abort with "no such function: title_sort".
--
-- This file is embedded and used both to bootstrap a brand-new library and to
-- build test fixtures.

CREATE TABLE IF NOT EXISTS books (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    title         TEXT NOT NULL DEFAULT 'Unknown' COLLATE NOCASE,
    sort          TEXT COLLATE NOCASE,
    timestamp     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    pubdate       TIMESTAMP DEFAULT '2000-01-01 00:00:00+00:00',
    series_index  REAL NOT NULL DEFAULT 1.0,
    author_sort   TEXT COLLATE NOCASE,
    isbn          TEXT DEFAULT '' COLLATE NOCASE,
    lccn          TEXT DEFAULT '' COLLATE NOCASE,
    path          TEXT NOT NULL DEFAULT '',
    flags         INTEGER NOT NULL DEFAULT 1,
    uuid          TEXT,
    has_cover     BOOL DEFAULT 0,
    last_modified TIMESTAMP NOT NULL DEFAULT '2000-01-01 00:00:00+00:00'
);

CREATE TABLE IF NOT EXISTS authors (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE,
    sort TEXT COLLATE NOCASE,
    link TEXT NOT NULL DEFAULT '',
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS books_authors_link (
    id     INTEGER PRIMARY KEY,
    book   INTEGER NOT NULL,
    author INTEGER NOT NULL,
    UNIQUE(book, author)
);

CREATE TABLE IF NOT EXISTS comments (
    id   INTEGER PRIMARY KEY,
    book INTEGER NOT NULL,
    text TEXT NOT NULL COLLATE NOCASE,
    UNIQUE(book)
);

CREATE TABLE IF NOT EXISTS data (
    id                INTEGER PRIMARY KEY,
    book              INTEGER NOT NULL,
    format            TEXT NOT NULL COLLATE NOCASE,
    uncompressed_size INTEGER NOT NULL,
    name              TEXT NOT NULL,
    UNIQUE(book, format)
);

CREATE TABLE IF NOT EXISTS identifiers (
    id   INTEGER PRIMARY KEY,
    book INTEGER NOT NULL,
    type TEXT NOT NULL DEFAULT 'isbn' COLLATE NOCASE,
    val  TEXT NOT NULL COLLATE NOCASE,
    UNIQUE(book, type)
);

CREATE TABLE IF NOT EXISTS languages (
    id        INTEGER PRIMARY KEY,
    lang_code TEXT NOT NULL COLLATE NOCASE,
    UNIQUE(lang_code)
);

CREATE TABLE IF NOT EXISTS books_languages_link (
    id         INTEGER PRIMARY KEY,
    book       INTEGER NOT NULL,
    lang_code  INTEGER NOT NULL,
    item_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE(book, lang_code)
);

CREATE TABLE IF NOT EXISTS publishers (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE,
    sort TEXT COLLATE NOCASE,
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS books_publishers_link (
    id        INTEGER PRIMARY KEY,
    book      INTEGER NOT NULL,
    publisher INTEGER NOT NULL,
    UNIQUE(book)
);

CREATE TABLE IF NOT EXISTS ratings (
    id     INTEGER PRIMARY KEY,
    rating INTEGER CHECK(rating > -1 AND rating < 11),
    UNIQUE(rating)
);

CREATE TABLE IF NOT EXISTS books_ratings_link (
    id     INTEGER PRIMARY KEY,
    book   INTEGER NOT NULL,
    rating INTEGER NOT NULL,
    UNIQUE(book, rating)
);

CREATE TABLE IF NOT EXISTS series (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE,
    sort TEXT COLLATE NOCASE,
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS books_series_link (
    id     INTEGER PRIMARY KEY,
    book   INTEGER NOT NULL,
    series INTEGER NOT NULL,
    UNIQUE(book)
);

CREATE TABLE IF NOT EXISTS tags (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE,
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS books_tags_link (
    id   INTEGER PRIMARY KEY,
    book INTEGER NOT NULL,
    tag  INTEGER NOT NULL,
    UNIQUE(book, tag)
);

-- Calibre stores the library's stable identity here (single row).
CREATE TABLE IF NOT EXISTS library_id (
    id   INTEGER PRIMARY KEY,
    uuid TEXT NOT NULL,
    UNIQUE(uuid)
);

-- Optional interop table modern Calibre uses for per-user reading positions.
CREATE TABLE IF NOT EXISTS last_read_positions (
    id      INTEGER PRIMARY KEY,
    book    INTEGER NOT NULL,
    format  TEXT NOT NULL COLLATE NOCASE,
    user    TEXT NOT NULL,
    device  TEXT NOT NULL,
    cfi     TEXT NOT NULL,
    epoch   REAL NOT NULL,
    pos_frac REAL NOT NULL DEFAULT 0,
    UNIQUE(user, device, book, format)
);

-- Triggers that depend on application-defined functions. These are the reason a
-- clean-room writer must register title_sort and uuid4.
CREATE TRIGGER IF NOT EXISTS books_insert_trg AFTER INSERT ON books
BEGIN
    UPDATE books SET sort=title_sort(NEW.title), uuid=uuid4() WHERE id=NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS books_update_trg AFTER UPDATE ON books
BEGIN
    UPDATE books SET sort=title_sort(NEW.title)
        WHERE id=NEW.id AND OLD.title <> NEW.title;
END;

-- Cascade deletes of all per-book rows, matching Calibre behaviour.
CREATE TRIGGER IF NOT EXISTS books_delete_trg AFTER DELETE ON books
BEGIN
    DELETE FROM books_authors_link   WHERE book=OLD.id;
    DELETE FROM books_publishers_link WHERE book=OLD.id;
    DELETE FROM books_ratings_link   WHERE book=OLD.id;
    DELETE FROM books_series_link    WHERE book=OLD.id;
    DELETE FROM books_tags_link      WHERE book=OLD.id;
    DELETE FROM books_languages_link WHERE book=OLD.id;
    DELETE FROM data                 WHERE book=OLD.id;
    DELETE FROM comments             WHERE book=OLD.id;
    DELETE FROM identifiers          WHERE book=OLD.id;
END;
