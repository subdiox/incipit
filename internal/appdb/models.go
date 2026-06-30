package appdb

import "time"

// UserSource identifies how a user authenticates.
type UserSource string

const (
	SourceLocal UserSource = "local"
	SourceLDAP  UserSource = "ldap"
	SourceProxy UserSource = "proxy"
)

// User is an Incipit account. PasswordHash is empty for externally-authenticated
// users (LDAP, reverse-proxy).
type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	IsAdmin      bool       `json:"isAdmin"`
	Source       UserSource `json:"source"`
	CanDownload  bool       `json:"canDownload"`
	CanUpload    bool       `json:"canUpload"`
	CanEdit      bool       `json:"canEdit"`
	Language     string     `json:"language"` // UI language preference: "en" | "ja"
	PageSize     int        `json:"pageSize"` // library page size preference
	CreatedAt    time.Time  `json:"createdAt"`
}

// Session is a server-side session record keyed by an opaque token.
type Session struct {
	ID        string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Shelf is a user-owned collection of books.
type Shelf struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Name      string    `json:"name"`
	IsPublic  bool      `json:"isPublic"`
	CreatedAt time.Time `json:"createdAt"`
	BookCount int       `json:"bookCount"`
}

// Progress is a user's reading position in a book.
type Progress struct {
	UserID     int64     `json:"-"`
	BookID     int64     `json:"bookId"`
	Format     string    `json:"format"`
	Page       int       `json:"page"`
	TotalPages int       `json:"totalPages"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// PageCacheEntry caches a CBZ's ordered page list so we avoid re-scanning the
// archive's central directory on every read.
type PageCacheEntry struct {
	BookID    int64
	Format    string
	FilePath  string
	Pages     []string
	PageCount int
	MTime     int64
	Size      int64
	ScannedAt time.Time
}

const timeLayout = time.RFC3339Nano
