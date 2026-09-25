package chat

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"

	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/retrieval"
)

// sentenceSplitRE 中英文句子边界（移植 RAGFlow：匹配"前字+标点"防止误切）。
var sentenceSplitRE = regexp.MustCompile(`([^\|][；。？!！,.\n]|[a-z][.?;!][ \n])`)

// minSentenceLen 过短句子不参与引用（对齐 RAGFlow 的 5）。
const minSentenceLen = 5

// InsertCitations 对答案逐句对齐引用：返回带 [ID:i] 标注的答案与被引用的 chunk 序号。
func InsertCitations(ctx context.Context, answer string, chunks []retrieval.Candidate, em embed.Embedder) (string, []int) {
	sentences, sentenceIdx := SplitAnswer(answer)
	if len(sentences) == 0 || len(chunks) == 0 {
		return answer, nil
	}
	vecs, err := em.Encode(ctx, sentences)
	if err != nil || len(vecs) == 0 {
		return answer, nil
	}
	return InsertCitationsWithVectors(answer, chunks, vecs, sentences, sentenceIdx)
}

// InsertCitationsWithVectors 纯函数核心（便于测试）。
// chunks 的序号即引用标记的 i，与返回给前端的 references 数组下标一致。
func InsertCitationsWithVectors(answer string, chunks []retrieval.Candidate, sentenceVecs [][]float32, sentences []string, sentenceIdx []int) (string, []int) {
	if len(sentences) != len(sentenceVecs) {
		n := len(sentenceVecs)
		if n < len(sentences) {
			sentences = sentences[:n]
			sentenceIdx = sentenceIdx[:n]
		} else {
			sentenceVecs = sentenceVecs[:len(sentences)]
		}
	}
	chunkVecs := make([][]float32, len(chunks))
	for i := range chunks {
		chunkVecs[i] = chunks[i].Vector
	}
	sim := cosineSimMatrix(sentenceVecs, chunkVecs)
	cites := findCitations(sim)
	return applyCitations(answer, sentences, sentenceIdx, cites)
}

// SplitAnswer 切句，保留 ``` 代码块（代码块不参与引用）。
func SplitAnswer(answer string) ([]string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	var sentences []string
	var sentenceIdx []int
	for i, t := range rawPieces {
		if len(strings.TrimSpace(t)) >= minSentenceLen {
			sentences = append(sentences, t)
			sentenceIdx = append(sentenceIdx, i)
		}
	}
	return sentences, sentenceIdx
}

func sentenceSplit(text string) []string {
	indices := sentenceSplitRE.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}
	var result []string
	prev := 0
	for _, idx := range indices {
		result = append(result, text[prev:idx[1]])
		prev = idx[1]
	}
	if prev < len(text) {
		result = append(result, text[prev:])
	}
	return result
}

// applyCitations 在命中句子末尾插入 " [ID:i]"，每个 chunk 只标注首次出现。
func applyCitations(answer string, sentences []string, sentenceIdx []int, cites map[int][]int) (string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	citedMarkers := make(map[int]string)
	seen := map[int]bool{}
	var citedIndices []int
	for i, rawIdx := range sentenceIdx {
		chunkIdxs, ok := cites[i]
		if !ok {
			continue
		}
		var markers []string
		for _, ci := range chunkIdxs {
			if seen[ci] {
				continue
			}
			seen[ci] = true
			markers = append(markers, " [ID:"+strconv.Itoa(ci)+"]")
			citedIndices = append(citedIndices, ci)
		}
		if len(markers) > 0 {
			citedMarkers[rawIdx] = strings.Join(markers, "")
		}
	}
	var b strings.Builder
	for i, p := range rawPieces {
		b.WriteString(p)
		if m, ok := citedMarkers[i]; ok {
			b.WriteString(m)
		}
	}
	return b.String(), citedIndices
}

// cosineSimMatrix 句子 × 切片 相似度矩阵。
func cosineSimMatrix(a, b [][]float32) [][]float64 {
	m := make([][]float64, len(a))
	for i := range a {
		m[i] = make([]float64, len(b))
		na := vecNorm(a[i])
		if na == 0 {
			continue
		}
		for j := range b {
			nb := vecNorm(b[j])
			if nb == 0 {
				continue
			}
			m[i][j] = dot(a[i], b[j]) / (na * nb)
		}
	}
	return m
}

func vecNorm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func dot(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float64
	for i := 0; i < n; i++ {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// findCitations 阈值搜索（移植 RAGFlow：0.63 起步，未命中乘以 0.8，直到 0.3）。
func findCitations(sim [][]float64) map[int][]int {
	cites := make(map[int][]int)
	thr := 0.63
	for thr > 0.3 && len(cites) == 0 {
		for i := range sim {
			mx := maxRow(sim[i]) * 0.99
			if mx < thr {
				continue
			}
			var matches []int
			for j, s := range sim[i] {
				if s > mx {
					matches = append(matches, j)
				}
			}
			if len(matches) > 4 {
				matches = matches[:4]
			}
			if len(matches) > 0 {
				cites[i] = matches
			}
		}
		thr *= 0.8
	}
	return cites
}

func maxRow(row []float64) float64 {
	m := 0.0
	for _, x := range row {
		if x > m {
			m = x
		}
	}
	return m
}
