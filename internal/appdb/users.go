package appdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("appdb: not found")

// CreateUser inserts a new user and returns it with its assigned ID.
func (s *Store) CreateUser(ctx context.Context, u User) (*User, error) {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	if u.Source == "" {
		u.Source = SourceLocal
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO users
		(username, password_hash, is_admin, source, can_download, can_upload, can_edit, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, b2i(u.IsAdmin), string(u.Source),
		b2i(u.CanDownload), b2i(u.CanUpload), b2i(u.CanEdit), u.CreatedAt.Format(timeLayout))
	if err != nil {
		return nil, err
	}
	u.ID, _ = res.LastInsertId()
	return &u, nil
}

const userCols = `id, username, password_hash, is_admin, source, can_download, can_upload, can_edit, created_at`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	var u User
	var src, created string
	var admin, dl, up, ed int
	if err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &admin, &src, &dl, &up, &ed, &created); err != nil {
		return nil, err
	}
	u.IsAdmin = admin != 0
	u.CanDownload = dl != 0
	u.CanUpload = up != 0
	u.CanEdit = ed != 0
	u.Source = UserSource(src)
	u.CreatedAt, _ = time.Parse(timeLayout, created)
	return &u, nil
}

// GetUserByUsername looks up a user by (case-insensitive) username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE username=?", username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// GetUser looks up a user by ID.
func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE id=?", id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// ListUsers returns all users ordered by username.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userCols+" FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// CountUsers returns the number of users (used to decide first-run admin setup).
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// UpdateUser persists mutable fields (permissions, admin, password hash).
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET
		password_hash=?, is_admin=?, can_download=?, can_upload=?, can_edit=? WHERE id=?`,
		u.PasswordHash, b2i(u.IsAdmin), b2i(u.CanDownload), b2i(u.CanUpload), b2i(u.CanEdit), u.ID)
	return err
}

// DeleteUser removes a user and their sessions/shelves.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, q := range []string{
		"DELETE FROM sessions WHERE user_id=?",
		"DELETE FROM shelf_books WHERE shelf_id IN (SELECT id FROM shelves WHERE user_id=?)",
		"DELETE FROM shelves WHERE user_id=?",
		"DELETE FROM read_progress WHERE user_id=?",
		"DELETE FROM users WHERE id=?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
