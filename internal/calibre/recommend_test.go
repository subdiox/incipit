package calibre

import (
	"bytes"
	"context"
	"testing"
)

// addRec adds a minimal book with the given author, tags and (optional) series,
// for exercising the recommender's feature scoring.
func addRec(t *testing.T, a *Adapter, title, author string, tags []string, series string, idx float64) *Book {
	t.Helper()
	in := AddBookInput{
		Title:       title,
		Authors:     []string{author},
		Tags:        tags,
		Format:      "CBZ",
		Data:        bytes.NewReader([]byte("PK\x03\x04fake")),
		SeriesIndex: idx,
	}
	if series != "" {
		in.Series = series
	}
	b, err := a.AddBook(context.Background(), in)
	if err != nil {
		t.Fatalf("AddBook(%q): %v", title, err)
	}
	return b
}

// TestRecommendContentBased verifies the core scoring: a book sharing the seed's
// author outranks one sharing only a tag; a book sharing nothing is not
// suggested; the seed itself is excluded; a series is collapsed to one volume;
// and the reason (author) is reported.
func TestRecommendContentBased(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()

	seed := addRec(t, a, "Seed", "Aoki", []string{"battle", "romance"}, "", 0)
	sameAuthor := addRec(t, a, "SameAuthor", "Aoki", []string{"cooking"}, "", 0)
	sameTag := addRec(t, a, "SameTag", "Zzz", []string{"battle"}, "", 0)
	unrelated := addRec(t, a, "Unrelated", "Other", []string{"gardening"}, "", 0)
	e1 := addRec(t, a, "SeriesVol1", "Xyz", []string{"battle"}, "SharedSeries", 1)
	e2 := addRec(t, a, "SeriesVol2", "Xyz", []string{"battle"}, "SharedSeries", 2)

	seeds := map[int64]float64{seed.ID: 1.0}
	exclude := map[int64]bool{seed.ID: true}

	recs, err := a.Recommend(ctx, seeds, exclude, 10)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("expected recommendations, got none")
	}

	byID := map[int64]Recommendation{}
	for _, r := range recs {
		byID[r.BookID] = r
	}

	// Seed and the unrelated book must not appear.
	if _, ok := byID[seed.ID]; ok {
		t.Error("seed book should be excluded")
	}
	if _, ok := byID[unrelated.ID]; ok {
		t.Error("book sharing no features should not be recommended")
	}

	// Same-author book is present, ranked first, with an author reason.
	if recs[0].BookID != sameAuthor.ID {
		t.Errorf("expected same-author book ranked first, got book %d", recs[0].BookID)
	}
	if recs[0].ReasonKind != "author" || recs[0].ReasonName != "Aoki" {
		t.Errorf("expected reason author=Aoki, got %s=%q", recs[0].ReasonKind, recs[0].ReasonName)
	}

	// Same-tag standalone book is present with a tag reason.
	if r, ok := byID[sameTag.ID]; !ok {
		t.Error("same-tag book should be recommended")
	} else if r.ReasonKind != "tag" || r.ReasonName != "battle" {
		t.Errorf("expected reason tag=battle, got %s=%q", r.ReasonKind, r.ReasonName)
	}

	// The two-volume series collapses to exactly one recommendation.
	seriesHits := 0
	if _, ok := byID[e1.ID]; ok {
		seriesHits++
	}
	if _, ok := byID[e2.ID]; ok {
		seriesHits++
	}
	if seriesHits != 1 {
		t.Errorf("series should collapse to one volume, got %d volumes", seriesHits)
	}

	// Author (boosted, rarer) outscores the tag-only matches.
	if byID[sameTag.ID].Score >= recs[0].Score {
		t.Errorf("author match (%.3f) should outscore tag match (%.3f)", recs[0].Score, byID[sameTag.ID].Score)
	}
}

// TestRecommendNoSeeds returns nothing (not an error) for a user with no
// activity, so the UI hides the section.
func TestRecommendNoSeeds(t *testing.T) {
	a := newTestAdapter(t)
	addRec(t, a, "Lonely", "Solo", []string{"x"}, "", 0)
	recs, err := a.Recommend(context.Background(), nil, nil, 10)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected no recommendations for empty seeds, got %d", len(recs))
	}
}
