package appdb

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Collection is an admin-defined library view: a saved tag filter that appears as its
// own nav entry under Library. Collections are server-wide. MatchAny picks how the
// tags combine: false = match all (AND, default), true = match any (OR).
type Collection struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	TagIDs    []int64   `json:"tagIds"`
	MatchAny  bool      `json:"matchAny"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

func encodeTagIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	return strings.Join(parts, ",")
}

func decodeTagIDs(s string) []int64 {
	// Non-nil so it marshals as [] not null (a tagless "all books" collection must not
	// send null — the client types tagIds as number[] and reads .length).
	ids := []int64{}
	if s == "" {
		return ids
	}
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	return ids
}

// ListCollections returns all collections in display order (position, then id).
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, tag_ids, match_any, position, created_at FROM collections ORDER BY position, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collections := []Collection{}
	for rows.Next() {
		var p Collection
		var tagIDs, created string
		if err := rows.Scan(&p.ID, &p.Name, &tagIDs, &p.MatchAny, &p.Position, &created); err != nil {
			return nil, err
		}
		p.TagIDs = decodeTagIDs(tagIDs)
		p.CreatedAt, _ = time.Parse(timeLayout, created)
		collections = append(collections, p)
	}
	return collections, rows.Err()
}

// CreateCollection adds a collection, appending it after the existing ones.
func (s *Store) CreateCollection(ctx context.Context, name string, tagIDs []int64, matchAny bool) (*Collection, error) {
	var nextPos int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(position)+1, 0) FROM collections").Scan(&nextPos); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(timeLayout)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO collections (name, tag_ids, match_any, position, created_at) VALUES (?, ?, ?, ?, ?)",
		name, encodeTagIDs(tagIDs), matchAny, nextPos, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Collection{ID: id, Name: name, TagIDs: tagIDs, MatchAny: matchAny, Position: nextPos}, nil
}

// UpdateCollection updates a collection's name, tags, match mode and position.
func (s *Store) UpdateCollection(ctx context.Context, id int64, name string, tagIDs []int64, matchAny bool, position int) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE collections SET name=?, tag_ids=?, match_any=?, position=? WHERE id=?",
		name, encodeTagIDs(tagIDs), matchAny, position, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCollection removes a collection.
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM collections WHERE id=?", id)
	return err
}

// ReorderCollections sets each collection's position to its index in ids, in one
// transaction, so ListCollections returns them in the given order. Ids not listed
// keep their stored position (and sort after by id). Unknown ids are no-ops.
func (s *Store) ReorderCollections(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "UPDATE collections SET position=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.ExecContext(ctx, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetCollection returns a single collection or ErrNotFound.
func (s *Store) GetCollection(ctx context.Context, id int64) (*Collection, error) {
	var p Collection
	var tagIDs, created string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, tag_ids, match_any, position, created_at FROM collections WHERE id=?", id).
		Scan(&p.ID, &p.Name, &tagIDs, &p.MatchAny, &p.Position, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.TagIDs = decodeTagIDs(tagIDs)
	p.CreatedAt, _ = time.Parse(timeLayout, created)
	return &p, nil
}
