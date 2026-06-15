package appdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CreateSession persists a session token for a user.
func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.CreatedAt.UTC().Format(timeLayout), sess.ExpiresAt.UTC().Format(timeLayout))
	return err
}

// GetSession returns a non-expired session, or ErrNotFound.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	var created, expires string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, created_at, expires_at FROM sessions WHERE id=?", id).
		Scan(&sess.ID, &sess.UserID, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(timeLayout, created)
	sess.ExpiresAt, _ = time.Parse(timeLayout, expires)
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, id)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id=?", id)
	return err
}

// DeleteExpiredSessions purges sessions past their expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now().UTC().Format(timeLayout))
	return err
}
