package appdb

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Pane is an admin-defined library view: a saved tag filter (AND-combined) that
// appears as its own nav entry under Library. Panes are server-wide.
type Pane struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	TagIDs    []int64   `json:"tagIds"`
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
	if s == "" {
		return nil
	}
	var ids []int64
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	return ids
}

// ListPanes returns all panes in display order (position, then id).
func (s *Store) ListPanes(ctx context.Context) ([]Pane, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, tag_ids, position, created_at FROM panes ORDER BY position, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	panes := []Pane{}
	for rows.Next() {
		var p Pane
		var tagIDs, created string
		if err := rows.Scan(&p.ID, &p.Name, &tagIDs, &p.Position, &created); err != nil {
			return nil, err
		}
		p.TagIDs = decodeTagIDs(tagIDs)
		p.CreatedAt, _ = time.Parse(timeLayout, created)
		panes = append(panes, p)
	}
	return panes, rows.Err()
}

// CreatePane adds a pane, appending it after the existing ones.
func (s *Store) CreatePane(ctx context.Context, name string, tagIDs []int64) (*Pane, error) {
	var nextPos int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(position)+1, 0) FROM panes").Scan(&nextPos); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(timeLayout)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO panes (name, tag_ids, position, created_at) VALUES (?, ?, ?, ?)",
		name, encodeTagIDs(tagIDs), nextPos, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Pane{ID: id, Name: name, TagIDs: tagIDs, Position: nextPos}, nil
}

// UpdatePane updates a pane's name, tags and position.
func (s *Store) UpdatePane(ctx context.Context, id int64, name string, tagIDs []int64, position int) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE panes SET name=?, tag_ids=?, position=? WHERE id=?",
		name, encodeTagIDs(tagIDs), position, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePane removes a pane.
func (s *Store) DeletePane(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM panes WHERE id=?", id)
	return err
}

// GetPane returns a single pane or ErrNotFound.
func (s *Store) GetPane(ctx context.Context, id int64) (*Pane, error) {
	var p Pane
	var tagIDs, created string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, tag_ids, position, created_at FROM panes WHERE id=?", id).
		Scan(&p.ID, &p.Name, &tagIDs, &p.Position, &created)
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
