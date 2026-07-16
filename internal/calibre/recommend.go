package calibre

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// Recommendation is one scored suggestion: the representative book to show, its
// relevance score, and the single strongest shared trait that earned it (used
// for the "because you like …" caption). ReasonKind is "author" | "series" |
// "tag".
type Recommendation struct {
	BookID     int64   `json:"bookId"`
	Score      float64 `json:"score"`
	ReasonKind string  `json:"reasonKind"`
	ReasonID   int64   `json:"reasonId"`
	ReasonName string  `json:"reasonName"`
}

// The recommender is content-based cosine similarity. Each book is a TF-IDF
// vector over its tags/authors/series: a feature's weight is its rarity (IDF)
// times a per-category boost (same-author/same-series are stronger comic signals
// than a shared tag). The user's taste profile is the sum of their seed books'
// *unit* vectors (each weighted by favorite/read strength), so a single very
// common tag can't dominate just by appearing in many reads. A candidate is
// scored by cosine(profile, candidate) — dividing by the candidate's own norm so
// tag-heavy anthologies don't win on breadth alone.
const (
	recSeriesBoost = 1.6
	recAuthorBoost = 1.4
	recTagBoost    = 1.0

	// Features carried by more than this fraction of the library behave like
	// stopwords (near-zero IDF): dropped so they neither skew scores nor bloat
	// candidate generation.
	recMaxDocFreqRatio = 0.30
	// Absolute cap on a feature's document frequency, to bound candidate-gen cost
	// on a large library (its IDF is tiny anyway).
	recMaxDocFreq = 20000
	// Floor for the ratio-derived cap so a small library (where 30% is only a
	// handful of books) doesn't drop every shared feature and recommend nothing.
	recMinDocFreqKeep = 50
	// Per-category cap on profile features (strongest by weight). Keeps the
	// meaningful, low-frequency tastes and keeps candidate-gen posting lists small.
	recMaxProfileFeatures = 150
	// How many top candidates (by dot product) get the exact cosine + diversity
	// pass. Bounds the per-candidate norm computation.
	recCandidatePool = 400
	// Max recommendations attributed to any single trait (an artist or a tag), so
	// one favorite doesn't fill the whole row.
	recMaxPerReason = 4
	// Backstop on seed books considered; callers pass the strongest already.
	recMaxSeeds = 500
)

// Feature kinds and their link tables.
const (
	kindTag    = "tag"
	kindAuthor = "author"
	kindSeries = "series"
)

type featKey struct {
	kind string
	id   int64
}

var recBoost = map[string]float64{kindTag: recTagBoost, kindAuthor: recAuthorBoost, kindSeries: recSeriesBoost}

// recLinkTable maps a feature kind to its {table, column}.
var recLinkTable = map[string][2]string{
	kindTag:    {"books_tags_link", "tag"},
	kindAuthor: {"books_authors_link", "author"},
	kindSeries: {"books_series_link", "series"},
}

// Recommend produces content-based suggestions for a user whose taste is given
// as seed books with weights (favorites weigh more than reads, recent reads more
// than old — the caller sets the weights). It builds a TF-IDF profile from the
// seeds, scores every book that shares a profile feature by cosine similarity,
// de-duplicates each series and caps per-trait for variety, and returns the top
// `limit`. exclude drops books the user already engaged with. Empty (not an
// error) when there are no seeds or nothing relevant is found.
func (a *Adapter) Recommend(ctx context.Context, seeds map[int64]float64, exclude map[int64]bool, limit int) ([]Recommendation, error) {
	if len(seeds) == 0 || limit <= 0 {
		return nil, nil
	}

	var total int
	if err := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&total); err != nil {
		return nil, fmt.Errorf("recommend: count books: %w", err)
	}
	if total == 0 {
		return nil, nil
	}

	seedIDs := make([]int64, 0, len(seeds))
	for id := range seeds {
		seedIDs = append(seedIDs, id)
		if len(seedIDs) >= recMaxSeeds {
			break
		}
	}

	// 1. Load the seeds' features and the IDF of each distinct feature.
	seedFeat, err := a.bookFeatures(ctx, seedIDs)
	if err != nil {
		return nil, err
	}
	distinct := map[featKey]bool{}
	for _, fs := range seedFeat {
		for _, f := range fs {
			distinct[f] = true
		}
	}
	idf, err := a.featureIDFs(ctx, distinct, total)
	if err != nil {
		return nil, err
	}

	// 2. Profile = Σ_b weight_b · unit(v_b). Normalizing each seed to unit length
	//    stops a common tag from dominating just by appearing in many seeds.
	profile := map[featKey]float64{}
	for b, fs := range seedFeat {
		w := seeds[b]
		if w <= 0 {
			continue
		}
		var norm2 float64
		for _, f := range fs {
			iw := idf[f]
			norm2 += iw * iw
		}
		if norm2 <= 0 {
			continue
		}
		scale := w / math.Sqrt(norm2)
		for _, f := range fs {
			if iw := idf[f]; iw > 0 {
				profile[f] += iw * scale
			}
		}
	}
	profile = capTopFeatures(profile, recMaxProfileFeatures)
	if len(profile) == 0 {
		return nil, nil
	}

	// 3. Candidate generation: dot(profile, v_c) = Σ_{shared f} profile[f]·idf[f].
	//    This is exactly the cosine numerator; the strongest single term is the
	//    book's reason.
	dot := map[int64]float64{}
	reason := map[int64]featKey{}
	reasonW := map[int64]float64{}
	postWeight := map[featKey]float64{}
	pByKind := map[string][]int64{}
	for f := range profile {
		postWeight[f] = profile[f] * idf[f]
		pByKind[f.kind] = append(pByKind[f.kind], f.id)
	}
	for kind, ids := range pByKind {
		lt := recLinkTable[kind]
		in := placeholders(len(ids))
		q := fmt.Sprintf("SELECT %s, book FROM %s WHERE %s IN (%s)", lt[1], lt[0], lt[1], in)
		err := a.eachRow(ctx, q, toAnySlice(ids), func(s scanner) error {
			var fid, book int64
			if err := s.Scan(&fid, &book); err != nil {
				return err
			}
			if exclude[book] {
				return nil
			}
			pw := postWeight[featKey{kind, fid}]
			dot[book] += pw
			if pw > reasonW[book] {
				reasonW[book] = pw
				reason[book] = featKey{kind, fid}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("recommend: candidates (%s): %w", kind, err)
		}
	}
	if len(dot) == 0 {
		return nil, nil
	}

	// 4. Keep the top pool by raw dot, then finish the cosine by dividing each by
	//    its own norm (so breadth doesn't beat specificity).
	cands := make([]int64, 0, len(dot))
	for b := range dot {
		cands = append(cands, b)
	}
	sortByScoreDesc(cands, dot)
	if len(cands) > recCandidatePool {
		cands = cands[:recCandidatePool]
	}

	candFeat, err := a.bookFeatures(ctx, cands)
	if err != nil {
		return nil, err
	}
	// IDF for candidate features not already known (skip ones already in `idf`).
	cdistinct := map[featKey]bool{}
	for _, fs := range candFeat {
		for _, f := range fs {
			if _, ok := idf[f]; !ok {
				cdistinct[f] = true
			}
		}
	}
	cidf, err := a.featureIDFs(ctx, cdistinct, total)
	if err != nil {
		return nil, err
	}
	idfOf := func(f featKey) float64 {
		if v, ok := idf[f]; ok {
			return v
		}
		return cidf[f]
	}

	seriesOf := map[int64]int64{}
	cosine := map[int64]float64{}
	for _, b := range cands {
		var norm2 float64
		for _, f := range candFeat[b] {
			if f.kind == kindSeries {
				seriesOf[b] = f.id
			}
			iw := idfOf(f)
			norm2 += iw * iw
		}
		norm := math.Sqrt(norm2)
		if norm <= 0 {
			norm = 1
		}
		cosine[b] = dot[b] / norm
	}
	sortByScoreDesc(cands, cosine)

	// 5. Select the page: one book per series, at most recMaxPerReason per trait.
	seenSeries := map[int64]bool{}
	perReason := map[featKey]int{}
	var out []Recommendation
	for _, b := range cands {
		sid, inSeries := seriesOf[b]
		if inSeries && seenSeries[sid] {
			continue
		}
		rk := reason[b]
		if perReason[rk] >= recMaxPerReason {
			continue
		}
		if inSeries {
			seenSeries[sid] = true
		}
		perReason[rk]++
		out = append(out, Recommendation{
			BookID: b, Score: cosine[b], ReasonKind: rk.kind, ReasonID: rk.id,
		})
		if len(out) >= limit {
			break
		}
	}
	if err := a.fillReasonNames(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// bookFeatures loads every tag/author/series feature of the given books, keyed
// by book id. One query per category.
func (a *Adapter) bookFeatures(ctx context.Context, ids []int64) (map[int64][]featKey, error) {
	out := make(map[int64][]featKey, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	in := placeholders(len(ids))
	args := toAnySlice(ids)
	for _, kind := range []string{kindTag, kindAuthor, kindSeries} {
		lt := recLinkTable[kind]
		q := fmt.Sprintf("SELECT book, %s FROM %s WHERE book IN (%s)", lt[1], lt[0], in)
		err := a.eachRow(ctx, q, args, func(s scanner) error {
			var book, fid int64
			if err := s.Scan(&book, &fid); err != nil {
				return err
			}
			out[book] = append(out[book], featKey{kind, fid})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("recommend: book features (%s): %w", kind, err)
		}
	}
	return out, nil
}

// featureIDFs computes the IDF weight (rarity × category boost) of each given
// feature, dropping stopword-like and over-large features (which get no entry,
// i.e. weight 0). Grouped by kind, document frequencies fetched in chunks.
func (a *Adapter) featureIDFs(ctx context.Context, feats map[featKey]bool, total int) (map[featKey]float64, error) {
	out := map[featKey]float64{}
	if len(feats) == 0 {
		return out, nil
	}
	maxDF := recMaxDocFreq
	if r := int(recMaxDocFreqRatio * float64(total)); r < maxDF {
		if r < recMinDocFreqKeep {
			r = recMinDocFreqKeep
		}
		maxDF = r
	}
	byKind := map[string][]int64{}
	for f := range feats {
		byKind[f.kind] = append(byKind[f.kind], f.id)
	}
	for kind, ids := range byKind {
		lt := recLinkTable[kind]
		df, err := a.featureDF(ctx, lt[0], lt[1], ids)
		if err != nil {
			return nil, err
		}
		boost := recBoost[kind]
		for _, id := range ids {
			d := df[id]
			if d <= 0 || d > maxDF {
				continue
			}
			out[featKey{kind, id}] = math.Log(1+float64(total)/float64(d)) * boost
		}
	}
	return out, nil
}

// featureDF returns the document frequency (number of books) for each feature id
// in a link table, querying in chunks so large id sets stay under SQLite's
// variable limit.
func (a *Adapter) featureDF(ctx context.Context, linkTable, col string, ids []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(ids))
	const chunk = 900
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		in := placeholders(len(part))
		q := fmt.Sprintf("SELECT %s, COUNT(book) FROM %s WHERE %s IN (%s) GROUP BY %s", col, linkTable, col, in, col)
		err := a.eachRow(ctx, q, toAnySlice(part), func(s scanner) error {
			var id, n int64
			if err := s.Scan(&id, &n); err != nil {
				return err
			}
			out[id] = int(n)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("recommend: feature df (%s): %w", col, err)
		}
	}
	return out, nil
}

// fillReasonNames resolves the human name of each recommendation's reason feature
// from its category table, in one query per category.
func (a *Adapter) fillReasonNames(ctx context.Context, recs []Recommendation) error {
	byKind := map[string][]int64{}
	for _, r := range recs {
		if r.ReasonID > 0 {
			byKind[r.ReasonKind] = append(byKind[r.ReasonKind], r.ReasonID)
		}
	}
	tables := map[string]string{kindAuthor: "authors", kindSeries: "series", kindTag: "tags"}
	names := map[string]map[int64]string{}
	for kind, ids := range byKind {
		table := tables[kind]
		if table == "" || len(ids) == 0 {
			continue
		}
		in := placeholders(len(ids))
		q := fmt.Sprintf("SELECT id, name FROM %s WHERE id IN (%s)", table, in)
		m := map[int64]string{}
		err := a.eachRow(ctx, q, toAnySlice(ids), func(s scanner) error {
			var id int64
			var name string
			if err := s.Scan(&id, &name); err != nil {
				return err
			}
			m[id] = name
			return nil
		})
		if err != nil {
			return fmt.Errorf("recommend: reason names (%s): %w", kind, err)
		}
		names[kind] = m
	}
	for i := range recs {
		if m := names[recs[i].ReasonKind]; m != nil {
			recs[i].ReasonName = m[recs[i].ReasonID]
		}
	}
	return nil
}

// capTopFeatures keeps only the highest-weight features per kind (up to n each),
// so the profile focuses on the user's clearest tastes and candidate-gen posting
// lists stay bounded.
func capTopFeatures(m map[featKey]float64, n int) map[featKey]float64 {
	byKind := map[string][]featKey{}
	for k := range m {
		byKind[k.kind] = append(byKind[k.kind], k)
	}
	out := make(map[featKey]float64, len(m))
	for _, keys := range byKind {
		if len(keys) > n {
			sort.Slice(keys, func(i, j int) bool {
				if m[keys[i]] != m[keys[j]] {
					return m[keys[i]] > m[keys[j]]
				}
				return keys[i].id < keys[j].id
			})
			keys = keys[:n]
		}
		for _, k := range keys {
			out[k] = m[k]
		}
	}
	return out
}

// sortByScoreDesc sorts ids by their score descending, id ascending for a
// deterministic order on ties (map iteration is random).
func sortByScoreDesc(ids []int64, score map[int64]float64) {
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})
}

// BookIDsInSeries returns the ids of every book belonging to any of the given
// series, in one scan — used to expand a favorited series into per-volume taste
// seeds. Empty when no series are given.
func (a *Adapter) BookIDsInSeries(ctx context.Context, seriesIDs []int64) ([]int64, error) {
	if len(seriesIDs) == 0 {
		return nil, nil
	}
	in := placeholders(len(seriesIDs))
	q := fmt.Sprintf("SELECT book FROM books_series_link WHERE series IN (%s)", in)
	var ids []int64
	err := a.eachRow(ctx, q, toAnySlice(seriesIDs), func(s scanner) error {
		var id int64
		if err := s.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("book ids in series: %w", err)
	}
	return ids, nil
}
