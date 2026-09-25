// Package retrieval 混合检索：BM25 关键词 + 向量相似度融合重排。
package retrieval

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/store"
)

// Candidate 检索候选项。
type Candidate struct {
	ChunkID      int64    `json:"chunkId"`
	DocID        int64    `json:"docId"`
	DocName      string   `json:"docName,omitempty"`
	Index        int      `json:"index"`
	Text         string   `json:"text"`
	Headings     []string `json:"headings,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Page         int      `json:"page,omitempty"`
	Offset       int      `json:"offset,omitempty"`
	VectorScore  float64  `json:"vectorScore"`
	KeywordScore float64  `json:"keywordScore"`
	Score        float64  `json:"score"`
}

// Retriever 检索器。
type Retriever struct {
	store *store.Store
	embed embed.Embedder
	cfg   config.RAGConfig
	log   *slog.Logger
}

// New 构造 Retriever。
func New(st *store.Store, em embed.Embedder, cfg config.RAGConfig, log *slog.Logger) *Retriever {
	if log == nil {
		log = slog.Default()
	}
	return &Retriever{store: st, embed: em, cfg: cfg, log: log}
}

// Retrieve 混合检索：返回 topK 候选（含融合分）。
func (r *Retriever) Retrieve(ctx context.Context, kbId int64, query string, topK int) ([]Candidate, error) {
	if topK <= 0 {
		topK = r.cfg.Retrieval.TopK
	}
	if topK <= 0 {
		topK = 8
	}
	chunks, err := r.store.ListChunksByKB(ctx, kbId)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	// 文档名映射
	docs, _ := r.store.ListDocs(ctx, kbId)
	docNames := map[int64]string{}
	for _, d := range docs {
		if d.Id != nil {
			docNames[*d.Id] = strOr(d.Name, "")
		}
	}

	kwScores := bm25Scores(query, chunks)
	vecScores, err := r.vectorScores(ctx, query, chunks)
	if err != nil {
		return nil, err
	}

	maxKw := maxOf(kwScores)
	vw, kw := r.cfg.Retrieval.VectorWeight, r.cfg.Retrieval.KeywordWeight
	if vw == 0 && kw == 0 {
		vw, kw = 0.7, 0.3
	}

	qTokens := map[string]bool{}
	for _, t := range chunker.Tokenize(query) {
		qTokens[t] = true
	}

	candidates := make([]Candidate, 0, len(chunks))
	for i, c := range chunks {
		// 向量分：绝对余弦（钳制到 [0,1]）；BM25 分：按本批最大值归一
		vecN := vecScores[i]
		if vecN < 0 {
			vecN = 0
		}
		kwN := 0.0
		if maxKw > 0 {
			kwN = kwScores[i] / maxKw
		}
		score := vw*vecN + kw*kwN
		// 标签/标题命中加成
		boost := 0.0
		headingText := strings.Join(headingList(c.Headings), " ")
		if overlapWithQuery(qTokens, chunker.Tokenize(headingText)) {
			boost += 0.05
		}
		if overlapWithQuery(qTokens, splitTerms(c.Tags)) {
			boost += 0.05
		}
		score += boost
		if c.Embedding == nil {
			continue
		}
		candidates = append(candidates, Candidate{
			ChunkID:      derefI64(c.Id),
			DocID:        derefI64(c.DocId),
			DocName:      docNames[derefI64(c.DocId)],
			Index:        derefInt(c.Idx),
			Text:         strOr(c.Text, ""),
			Headings:     parseHeadings(strOr(c.Headings, "")),
			Tags:         strings.Split(strOr(c.Tags, ""), ","),
			Page:         derefInt(c.Page),
			Offset:       derefInt(c.Offset),
			VectorScore:  vecN,
			KeywordScore: kwN,
			Score:        score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].ChunkID < candidates[j].ChunkID
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	// 过滤过低分
	minSim := r.cfg.Retrieval.MinSimilarity
	if minSim > 0 {
		filtered := candidates[:0]
		for _, c := range candidates {
			if c.Score >= minSim {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}
	return candidates, nil
}

func (r *Retriever) vectorScores(ctx context.Context, query string, chunks []store.Chunk) ([]float64, error) {
	vecs, err := r.embed.Encode(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	q := vecs[0]
	out := make([]float64, len(chunks))
	for i, c := range chunks {
		if len(c.Embedding) == 0 {
			continue
		}
		out[i] = embed.Cosine(q, embed.DecodeVector(c.Embedding))
	}
	return out, nil
}

// ---- BM25 ----

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// bm25Scores 对每个 chunk 计算 BM25 分。
func bm25Scores(query string, chunks []store.Chunk) []float64 {
	qTokens := unique(chunker.Tokenize(query))
	if len(qTokens) == 0 {
		return make([]float64, len(chunks))
	}
	// 文档频率与长度
	df := map[string]int{}
	dls := make([]int, len(chunks))
	for i, c := range chunks {
		terms := strings.Fields(strOr(c.Terms, ""))
		if len(terms) == 0 {
			terms = chunker.Tokenize(strOr(c.Text, ""))
		}
		dls[i] = len(terms)
		seen := map[string]bool{}
		for _, t := range terms {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	avgdl := 1.0
	total := 0
	for _, l := range dls {
		total += l
	}
	if total > 0 {
		avgdl = float64(total) / float64(len(chunks))
	}
	scores := make([]float64, len(chunks))
	for i, c := range chunks {
		termList := strings.Fields(strOr(c.Terms, ""))
		if len(termList) == 0 {
			termList = chunker.Tokenize(strOr(c.Text, ""))
		}
		tf := map[string]int{}
		for _, t := range termList {
			tf[t]++
		}
		dl := float64(dls[i])
		var score float64
		for _, q := range qTokens {
			f := tf[q]
			if f == 0 {
				continue
			}
			n := float64(len(chunks))
			d := float64(df[q])
			idf := math.Log(1 + (n-d+0.5)/(d+0.5))
			score += idf * (float64(f) * (bm25K1 + 1) / (float64(f) + bm25K1*(1-bm25B+bm25B*dl/avgdl)))
		}
		scores[i] = score
	}
	return scores
}

// ---- 小工具 ----

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func maxOf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}

func overlapWithQuery(qTokens map[string]bool, tokens []string) bool {
	for _, t := range tokens {
		if qTokens[t] {
			return true
		}
	}
	return false
}

func headingList(headings *string) []string { return parseHeadings(strOr(headings, "")) }

func parseHeadings(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, " > ")
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

func splitTerms(tags *string) []string {
	if tags == nil || *tags == "" {
		return nil
	}
	return strings.Split(*tags, ",")
}

func strOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

func derefI64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
