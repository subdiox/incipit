package calibre

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// NormalizeReport counts, per table, how many rows were scanned, how many had
// their text rewritten to NFC, and (for unique-name tables) how many duplicate
// rows were merged away when an NFD value collided with its NFC twin.
type NormalizeReport struct {
	Scanned map[string]int
	Changed map[string]int
	Merged  map[string]int
}

func newReport() *NormalizeReport {
	return &NormalizeReport{Scanned: map[string]int{}, Changed: map[string]int{}, Merged: map[string]int{}}
}

// NormalizeText rewrites metadata.db's user-visible text columns to Unicode NFC
// (see NFC). It does NOT move or rename any folder — books.path is left as-is —
// so it is safe to run on a large library without a filesystem storm; paths can
// be reconciled separately. It is idempotent (a second run is a no-op).
//
// books.title/sort/author_sort and comments.text are updated in place. The
// unique-name entities (series/authors/publishers/tags) are merged on collision:
// if both an NFD and an NFC spelling of the same name exist as separate rows, the
// NFC one is kept and the other's book links are repointed to it before it is
// deleted. Updating books.title fires Calibre's trigger, which recomputes sort —
// harmless, since we normalize to the same NFC value.
func (a *Adapter) NormalizeText(ctx context.Context, includeComments, dryRun bool) (*NormalizeReport, error) {
	if a.readOnly && !dryRun {
		return nil, ErrReadOnly
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	rep := newReport()
	if err := a.normalizeRows(ctx, "books", []string{"title", "sort", "author_sort"}, dryRun, rep); err != nil {
		return nil, fmt.Errorf("books: %w", err)
	}
	named := []struct {
		table, sortCol, linkTable, linkCol string
	}{
		{"series", "sort", "books_series_link", "series"},
		{"authors", "sort", "books_authors_link", "author"},
		{"publishers", "sort", "books_publishers_link", "publisher"},
		{"tags", "", "books_tags_link", "tag"},
	}
	for _, n := range named {
		if err := a.normalizeNamed(ctx, n.table, n.sortCol, n.linkTable, n.linkCol, dryRun, rep); err != nil {
			return nil, fmt.Errorf("%s: %w", n.table, err)
		}
	}
	if includeComments {
		if err := a.normalizeRows(ctx, "comments", []string{"text"}, dryRun, rep); err != nil {
			return nil, fmt.Errorf("comments: %w", err)
		}
	}
	return rep, nil
}

type normChange struct {
	id   int64
	vals []any
}

// normalizeRows NFC-normalizes plain columns of a table keyed by id (no UNIQUE
// concern), updating only rows whose text actually changed.
func (a *Adapter) normalizeRows(ctx context.Context, table string, cols []string, dryRun bool, rep *NormalizeReport) error {
	rows, err := a.db.QueryContext(ctx, "SELECT id, "+strings.Join(cols, ", ")+" FROM "+table)
	if err != nil {
		return err
	}
	var changes []normChange
	scanned := 0
	dest := make([]any, len(cols)+1)
	for rows.Next() {
		var id int64
		ns := make([]sql.NullString, len(cols))
		dest[0] = &id
		for i := range ns {
			dest[i+1] = &ns[i]
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return err
		}
		scanned++
		vals := make([]any, len(cols))
		diff := false
		for i := range ns {
			if ns[i].Valid {
				n := NFC(ns[i].String)
				vals[i] = n
				if n != ns[i].String {
					diff = true
				}
			} else {
				vals[i] = nil
			}
		}
		if diff {
			changes = append(changes, normChange{id: id, vals: vals})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rep.Scanned[table] += scanned
	rep.Changed[table] += len(changes)
	if dryRun || len(changes) == 0 {
		return nil
	}
	set := make([]string, len(cols))
	for i, c := range cols {
		set[i] = c + "=?"
	}
	return a.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, "UPDATE "+table+" SET "+strings.Join(set, ", ")+" WHERE id=?")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, ch := range changes {
			args := append(append([]any{}, ch.vals...), ch.id)
			if _, err := stmt.ExecContext(ctx, args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// normalizeNamed NFC-normalizes a UNIQUE(name) entity table, merging rows that
// collide after normalization (repoint their book links to the surviving row).
func (a *Adapter) normalizeNamed(ctx context.Context, table, sortCol, linkTable, linkCol string, dryRun bool, rep *NormalizeReport) error {
	hasSort := sortCol != ""
	sel := "SELECT id, name" + map[bool]string{true: ", " + sortCol, false: ""}[hasSort] + " FROM " + table
	rows, err := a.db.QueryContext(ctx, sel)
	if err != nil {
		return err
	}
	type ent struct {
		id   int64
		name string
		sort sql.NullString
	}
	var all []ent
	for rows.Next() {
		var e ent
		dest := []any{&e.id, &e.name}
		if hasSort {
			dest = append(dest, &e.sort)
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return err
		}
		all = append(all, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rep.Scanned[table] += len(all)

	// Choose one surviving id per NFC(name), preferring a row already stored NFC.
	canonical := map[string]int64{}
	canonName := map[string]string{}
	for _, e := range all {
		key := NFC(e.name)
		if _, ok := canonical[key]; !ok {
			canonical[key] = e.id
			canonName[key] = e.name
			continue
		}
		if e.name == key && canonName[key] != key {
			canonical[key] = e.id
			canonName[key] = e.name
		}
	}

	var updates []normChange
	type mergeOp struct{ dup, canon int64 }
	var merges []mergeOp
	for _, e := range all {
		key := NFC(e.name)
		canon := canonical[key]
		if e.id != canon {
			merges = append(merges, mergeOp{dup: e.id, canon: canon})
			continue
		}
		nameChanged := e.name != key
		vals := []any{key}
		sortChanged := false
		if hasSort {
			out := any(nil)
			if e.sort.Valid {
				ns := NFC(e.sort.String)
				out = ns
				sortChanged = ns != e.sort.String
			}
			vals = append(vals, out)
		}
		if nameChanged || sortChanged {
			updates = append(updates, normChange{id: e.id, vals: vals})
		}
	}
	rep.Changed[table] += len(updates)
	rep.Merged[table] += len(merges)
	if dryRun || (len(updates) == 0 && len(merges) == 0) {
		return nil
	}

	setName := "name=?"
	if hasSort {
		setName += ", " + sortCol + "=?"
	}
	return a.inTx(ctx, func(tx *sql.Tx) error {
		// Merges first: repoint links off each duplicate, drop leftover links that
		// would violate UNIQUE(book,…), then delete the duplicate entity row.
		repoint, err := tx.PrepareContext(ctx, "UPDATE OR IGNORE "+linkTable+" SET "+linkCol+"=? WHERE "+linkCol+"=?")
		if err != nil {
			return err
		}
		defer repoint.Close()
		delLink, err := tx.PrepareContext(ctx, "DELETE FROM "+linkTable+" WHERE "+linkCol+"=?")
		if err != nil {
			return err
		}
		defer delLink.Close()
		delEnt, err := tx.PrepareContext(ctx, "DELETE FROM "+table+" WHERE id=?")
		if err != nil {
			return err
		}
		defer delEnt.Close()
		for _, m := range merges {
			if _, err := repoint.ExecContext(ctx, m.canon, m.dup); err != nil {
				return err
			}
			if _, err := delLink.ExecContext(ctx, m.dup); err != nil {
				return err
			}
			if _, err := delEnt.ExecContext(ctx, m.dup); err != nil {
				return err
			}
		}
		upd, err := tx.PrepareContext(ctx, "UPDATE "+table+" SET "+setName+" WHERE id=?")
		if err != nil {
			return err
		}
		defer upd.Close()
		for _, u := range updates {
			args := append(append([]any{}, u.vals...), u.id)
			if _, err := upd.ExecContext(ctx, args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// inTx runs fn inside a transaction, committing on success and rolling back on
// error.
func (a *Adapter) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
