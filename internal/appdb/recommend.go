package appdb

import "context"

// CachedRec is one precomputed recommendation: the book to show, its score, and
// the resolved reason trait ("because you like …").
type CachedRec struct {
	BookID     int64   `json:"bookId"`
	Score      float64 `json:"score"`
	ReasonKind string  `json:"reasonKind"`
	ReasonName string  `json:"reasonName"`
}

// UsersWithActivity returns the ids of users who have any reading history or any
// book/series on a shelf — i.e. the users worth precomputing recommendations
// for. Users with no activity would only yield an empty list, so they're skipped.
func (s *Store) UsersWithActivity(ctx context.Context) ([]int64, error) {
	return s.queryIDs(ctx, `
		SELECT user_id FROM read_progress
		UNION
		SELECT sh.user_id FROM shelves sh JOIN shelf_books  sb ON sb.shelf_id = sh.id
		UNION
		SELECT sh.user_id FROM shelves sh JOIN shelf_series ss ON ss.shelf_id = sh.id`)
}

// ReadBookIDs returns every book id the user has any reading progress for
// (uncapped, unlike ListReading which is limited for display). Used to exclude
// all already-read books from recommendations, not just the most recent ones.
func (s *Store) ReadBookIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.queryIDs(ctx, `SELECT book_id FROM read_progress WHERE user_id=?`, userID)
}

// ReplaceRecommendations atomically swaps a user's cached recommendations for a
// fresh ranked list (rank = slice index). An empty slice clears the cache.
func (s *Store) ReplaceRecommendations(ctx context.Context, userID int64, recs []CachedRec) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rec_cache WHERE user_id=?", userID); err != nil {
		tx.Rollback()
		return err
	}
	for i, r := range recs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rec_cache
			(user_id, rank, book_id, score, reason_kind, reason_name)
			VALUES (?, ?, ?, ?, ?, ?)`,
			userID, i, r.BookID, r.Score, r.ReasonKind, r.ReasonName); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// CachedRecommendations returns a user's precomputed recommendations in rank
// order, up to limit (<=0 for all). Empty when nothing has been computed yet.
func (s *Store) CachedRecommendations(ctx context.Context, userID int64, limit int) ([]CachedRec, error) {
	q := "SELECT book_id, score, reason_kind, reason_name FROM rec_cache WHERE user_id=? ORDER BY rank"
	args := []any{userID}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CachedRec
	for rows.Next() {
		var c CachedRec
		if err := rows.Scan(&c.BookID, &c.Score, &c.ReasonKind, &c.ReasonName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
