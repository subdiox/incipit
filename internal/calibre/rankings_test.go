package calibre

import (
	"context"
	"testing"
)

// TestRankingsRoundTrip verifies the ranking side tables: an importer (simulated
// with raw INSERTs, exactly what the crawler does) populates ranking_lists and
// book_rankings, and the reader surfaces the lists in tab order with counts and
// the books in explicit rank order — skipping any book deleted since.
func TestRankingsRoundTrip(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()

	b1 := addSample(t, a, "Alpha", []string{"Asimov"})
	b2 := addSample(t, a, "Beta", []string{"Asimov"})
	b3 := addSample(t, a, "Gamma", []string{"Asimov"})

	// Two lists, deliberately out of position order to prove ORDER BY position.
	if _, err := a.db.Exec(`INSERT INTO ranking_lists (key,label,position) VALUES
		('popular-week','週間人気',1), ('popular-today','今日の人気',0)`); err != nil {
		t.Fatalf("seed ranking_lists: %v", err)
	}
	// popular-today ranks b3, b1 (in that rank order); popular-week ranks b1 only.
	if _, err := a.db.Exec(`INSERT INTO book_rankings (list,rank,book) VALUES
		('popular-today',1,?), ('popular-today',2,?), ('popular-week',1,?)`,
		b3.ID, b1.ID, b1.ID); err != nil {
		t.Fatalf("seed book_rankings: %v", err)
	}

	lists, err := a.RankingLists(ctx)
	if err != nil {
		t.Fatalf("RankingLists: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("want 2 lists, got %d: %+v", len(lists), lists)
	}
	if lists[0].Key != "popular-today" || lists[1].Key != "popular-week" {
		t.Fatalf("lists not in position order: %+v", lists)
	}
	if lists[0].Count != 2 || lists[1].Count != 1 {
		t.Fatalf("wrong counts: %+v", lists)
	}

	ids, err := a.RankedBookIDs(ctx, "popular-today")
	if err != nil {
		t.Fatalf("RankedBookIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != b3.ID || ids[1] != b1.ID {
		t.Fatalf("want [%d %d] in rank order, got %v", b3.ID, b1.ID, ids)
	}

	// Deleting a ranked book drops it from the list (join), order preserved.
	if err := a.DeleteBook(ctx, b3.ID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	ids, err = a.RankedBookIDs(ctx, "popular-today")
	if err != nil {
		t.Fatalf("RankedBookIDs after delete: %v", err)
	}
	if len(ids) != 1 || ids[0] != b1.ID {
		t.Fatalf("want [%d] after delete, got %v", b1.ID, ids)
	}

	// Unknown list → empty, not an error.
	if got, err := a.RankedBookIDs(ctx, "nope"); err != nil || len(got) != 0 {
		t.Fatalf("unknown list: got %v err %v", got, err)
	}
	_ = b2
}

// TestRankingListsSkipsEmpty verifies a list header with no books doesn't appear
// (the reader only surfaces lists that actually have rankings).
func TestRankingListsSkipsEmpty(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()
	if _, err := a.db.Exec(`INSERT INTO ranking_lists (key,label,position) VALUES ('empty','空',0)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	lists, err := a.RankingLists(ctx)
	if err != nil {
		t.Fatalf("RankingLists: %v", err)
	}
	if len(lists) != 0 {
		t.Fatalf("want no lists (header has no books), got %+v", lists)
	}
}
