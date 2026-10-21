package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"unicode"

	"github.com/qbj1122/campusclaw-zww/internal/chunk"
	"github.com/qbj1122/campusclaw-zww/internal/config"
	"github.com/qbj1122/campusclaw-zww/internal/llm"
	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/store"
	"github.com/qbj1122/campusclaw-zww/internal/vector"
)

var (
	ErrBadRequest  = errors.New("bad request")
	ErrUnavailable = errors.New("service unavailable")
	ErrNotFound    = errors.New("not found")
)

type Retrieval struct {
	Store  *store.Store
	LLM    *llm.Client
	Vector *vector.Client
	Cfg    *config.Config
	Log    *slog.Logger
}

type SearchOptions struct {
	Mode        string
	Limit       int
	Offset      int
	MaterialIDs []int64
	Subject     string
}

type SearchResult struct {
	Status string      `json:"status"`
	Mode   string      `json:"mode"`
	Hits   []model.Hit `json:"hits"`
}

// IndexMaterial chunks and embeds one knowledge entry. The MySQL rows are
// authoritative; Qdrant only stores vectors and identifiers. Any embedding or
// vector failure leaves chunks marked failed and returns ErrUnavailable.
func (r *Retrieval) IndexMaterial(ctx context.Context, materialID int64, strategy string, opts chunk.Options) (int, error) {
	entryID, classID, body, err := r.Store.EntryForMaterial(ctx, materialID)
	if err != nil {
		return 0, ErrNotFound
	}
	if strategy == "" {
		strategy = "auto"
	}
	opts.Strategy = strategy
	opts = chunk.Normalize(opts)
	sum := sha256.Sum256([]byte(body + "|" + strategy + fmt.Sprintf("|%d|%d|%v|%v", opts.MaxChars, opts.OverlapChars, opts.StripURLs, opts.CollapseWS)))
	hash := hex.EncodeToString(sum[:])
	if oldHash, oldStrategy, err := r.Store.IndexState(ctx, entryID); err == nil && oldHash == hash && oldStrategy == strategy {
		return 0, nil
	}
	if err := r.Vector.EnsureCollection(ctx, r.LLM.EmbeddingDim); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err := r.Vector.DeleteEntry(ctx, entryID); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	parts := chunk.Split(body, opts)
	ids, err := r.Store.ReplaceChunks(ctx, classID, materialID, entryID, strategy, parts)
	if err != nil {
		return 0, err
	}
	points := make([]vector.Point, 0, len(ids))
	for i, id := range ids {
		vec, err := r.LLM.Embed(ctx, parts[i].Text)
		if err != nil {
			_ = r.Store.MarkChunks(ctx, ids, "failed")
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		points = append(points, vector.Point{ID: id, Vector: vec, Payload: map[string]any{
			"class_id": classID, "material_id": materialID, "knowledge_entry_id": entryID,
			"chunk_id": id, "chunk_index": parts[i].Index,
		}})
	}
	if err := r.Vector.Upsert(ctx, points); err != nil {
		_ = r.Store.MarkChunks(ctx, ids, "failed")
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err := r.Store.MarkChunks(ctx, ids, "ready"); err != nil {
		return 0, err
	}
	if err := r.Store.SetIndexState(ctx, entryID, hash, strategy); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (r *Retrieval) EnsureAllIndexed(ctx context.Context) {
	if err := r.Vector.EnsureCollection(ctx, r.LLM.EmbeddingDim); err != nil {
		r.Log.Warn("vector store unavailable at startup", "error", err.Error())
	}
	entries, err := r.Store.EntriesForReindex(ctx)
	if err != nil {
		r.Log.Warn("list entries for reindex failed", "error", err.Error())
		return
	}
	for _, e := range entries {
		if _, err := r.IndexMaterial(ctx, e.MaterialID, e.Strategy, chunk.Options{Strategy: e.Strategy, MaxChars: r.Cfg.ChunkMaxChars, OverlapChars: r.Cfg.ChunkOverlapChars}); err != nil {
			r.Log.Warn("seed index skipped", "material_id", e.MaterialID, "error", err.Error())
		}
	}
	r.Log.Info("startup index check finished", "entries", len(entries))
}

func (r *Retrieval) Search(ctx context.Context, classID int64, q string, opts SearchOptions) (*SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("%w: 查询不能为空", ErrBadRequest)
	}
	if opts.Limit < 0 || opts.Offset < 0 {
		return nil, fmt.Errorf("%w: limit 与 offset 不能为负数", ErrBadRequest)
	}
	mode := opts.Mode
	if mode == "" {
		mode = "hybrid"
	}
	if mode != "keyword" && mode != "vector" && mode != "hybrid" {
		return nil, fmt.Errorf("%w: 不支持的检索模式", ErrBadRequest)
	}
	if opts.Limit == 0 {
		opts.Limit = 10
	}
	if opts.Limit > r.Cfg.SearchMaxResults {
		opts.Limit = r.Cfg.SearchMaxResults
	}
	broad := r.Cfg.SearchMaxResults
	if opts.Limit > broad {
		broad = opts.Limit
	}

	var kw, vec []model.Hit
	var err error
	if mode == "keyword" || mode == "hybrid" {
		kw, err = r.Store.KeywordSearch(ctx, classID, q, broad)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if containsCJK(q) {
			// MySQL ngram matches any two-character token, including function
			// words. Keep only chunks with a topical (non-stopword) overlap so
			// an unrelated question does not become "evidence".
			filtered := kw[:0]
			for _, h := range kw {
				if hasTopicalOverlap(q, h.Excerpt) {
					filtered = append(filtered, h)
				}
			}
			kw = filtered
		}
	}
	if mode == "vector" || mode == "hybrid" {
		vec, err = r.vectorSearch(ctx, classID, q, broad)
		if err != nil {
			return nil, err
		}
	}
	hits := fuse(mode, kw, vec, r.Cfg.RRFK)
	hits = r.scope(hits, opts.MaterialIDs, opts.Subject)

	status := "ok"
	if len(hits) == 0 {
		status = "no_evidence"
	}
	// Pagination is applied after fusion. An offset beyond the result set is
	// still a successful empty page, not no_evidence.
	if opts.Offset >= len(hits) {
		if status == "ok" {
			hits = []model.Hit{}
		} else {
			hits = []model.Hit{}
		}
	} else {
		end := opts.Offset + opts.Limit
		if end > len(hits) {
			end = len(hits)
		}
		hits = hits[opts.Offset:end]
	}
	for i := range hits {
		hits[i].Rank = i + opts.Offset + 1
	}
	return &SearchResult{Status: status, Mode: mode, Hits: hits}, nil
}

func (r *Retrieval) vectorSearch(ctx context.Context, classID int64, q string, broad int) ([]model.Hit, error) {
	qv, err := r.LLM.Embed(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	pts, err := r.Vector.Search(ctx, qv, classID, broad)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	ids := make([]int64, 0, len(pts))
	scores := map[int64]float64{}
	minScore := r.Cfg.VectorMinScore
	if r.LLM.Offline() {
		// The deterministic offline embedding is a demo fallback, not a
		// semantic model; use a lower cutoff so the vector path stays usable.
		minScore = 0.12
	}
	for _, p := range pts {
		if p.Score < minScore {
			continue
		}
		ids = append(ids, p.ID)
		scores[p.ID] = p.Score
	}
	full, err := r.Store.ChunkFullTexts(ctx, classID, ids)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	out := make([]model.Hit, 0, len(ids))
	for _, id := range ids {
		h, ok := full[id]
		if !ok {
			continue
		}
		// Offline hash vectors have no real semantics. Require a shared
		// character bigram so unrelated questions do not produce false hits.
		if r.LLM.Offline() && !hasTopicalOverlap(q, h.Excerpt) {
			continue
		}
		h.Score = scores[id]
		out = append(out, h)
	}
	return out, nil
}

func fuse(mode string, kw, vec []model.Hit, k int) []model.Hit {
	if k <= 0 {
		k = 60
	}
	if mode == "keyword" {
		for i := range kw {
			kw[i].Rank = i + 1
		}
		return kw
	}
	if mode == "vector" {
		for i := range vec {
			vec[i].Rank = i + 1
		}
		return vec
	}
	type scored struct {
		hit   model.Hit
		score float64
		kwr   int
		vr    int
	}
	m := map[int64]*scored{}
	for i, h := range kw {
		s := 1.0 / float64(k+i+1)
		cp := h
		cp.Score = s
		m[h.ChunkID] = &scored{hit: cp, score: s, kwr: i + 1}
	}
	for i, h := range vec {
		s := 1.0 / float64(k+i+1)
		if x, ok := m[h.ChunkID]; ok {
			x.score += s
			x.hit.Score = x.score
			x.vr = i + 1
		} else {
			cp := h
			cp.Score = s
			m[h.ChunkID] = &scored{hit: cp, score: s, vr: i + 1}
		}
	}
	out := make([]scored, 0, len(m))
	for _, v := range m {
		v.hit.Score = v.score
		v.hit.KeywordRank = v.kwr
		v.hit.VectorRank = v.vr
		out = append(out, *v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score == out[j].score {
			return out[i].hit.ChunkID < out[j].hit.ChunkID
		}
		return out[i].score > out[j].score
	})
	hits := make([]model.Hit, len(out))
	for i := range out {
		out[i].hit.Rank = i + 1
		hits[i] = out[i].hit
	}
	return hits
}

// commonBigrams are function-word pairs that carry no topical signal. They are
// ignored by the offline vector fallback's lexical gate.
var commonBigrams = map[string]bool{
	"一个": true, "一下": true, "怎么": true, "什么": true, "可以": true,
	"我们": true, "这个": true, "那个": true, "如何": true, "请讲": true,
	"讲一": true, "为什": true, "么样": true, "今天": true, "天气": true,
	"和课": true, "课程": true, "无关": true, "的笑": true, "这道": true,
}

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func hasTopicalOverlap(query, text string) bool {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	t := strings.ToLower(text)
	seen := map[string]bool{}
	for i := 0; i+1 < len(q); i++ {
		if unicode.IsSpace(q[i]) || unicode.IsSpace(q[i+1]) {
			continue
		}
		bg := string(q[i : i+2])
		if seen[bg] || commonBigrams[bg] {
			continue
		}
		seen[bg] = true
		if strings.Contains(t, bg) {
			return true
		}
	}
	return false
}

func (r *Retrieval) scope(hits []model.Hit, materialIDs []int64, subject string) []model.Hit {
	materialSet := map[int64]bool{}
	for _, id := range materialIDs {
		materialSet[id] = true
	}
	subject = strings.TrimSpace(subject)
	out := make([]model.Hit, 0, len(hits))
	for _, h := range hits {
		if len(materialSet) > 0 && !materialSet[h.MaterialID] {
			continue
		}
		if subject != "" && !strings.Contains(h.MaterialTitle, subject) {
			continue
		}
		out = append(out, h)
	}
	return out
}

func (r *Retrieval) ChunkContent(ctx context.Context, classID int64, hits []model.Hit) (map[int64]string, error) {
	ids := make([]int64, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ChunkID)
	}
	full, err := r.Store.ChunkFullTexts(ctx, classID, ids)
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for id, h := range full {
		out[id] = h.Excerpt
	}
	return out, nil
}
