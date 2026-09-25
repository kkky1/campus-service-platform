package chat

import (
	"strings"
	"testing"

	"campus-service-platform/internal/rag/retrieval"
)

// makeChunk 构造带向量的候选项。
func makeChunk(id int64, text string, vec []float32) retrieval.Candidate {
	return retrieval.Candidate{ChunkID: id, Text: text, Vector: vec, Score: 1}
}

func TestInsertCitationsBasic(t *testing.T) {
	answer := "食堂每天早上 7 点开门。图书馆每人最多借 10 本书。今天天气不错哦。"
	chunks := []retrieval.Candidate{
		makeChunk(1, "食堂开放时间 7:00", []float32{1, 0, 0}),
		makeChunk(2, "图书馆借书 10 本", []float32{0, 1, 0}),
	}
	// 句子向量：句1 → chunk0，句2 → chunk1，句3 无关
	sentences, idx := SplitAnswer(answer)
	if len(sentences) != 3 {
		t.Fatalf("切句 = %d: %v", len(sentences), sentences)
	}
	vecs := [][]float32{
		{0.95, 0.05, 0},
		{0.05, 0.95, 0},
		{0, 0, 1},
	}
	out, cited := InsertCitationsWithVectors(answer, chunks, vecs, sentences, idx)
	if !strings.Contains(out, "开门。 [ID:0]") {
		t.Fatalf("第 1 句未引用: %q", out)
	}
	if !strings.Contains(out, "10 本书。 [ID:1]") {
		t.Fatalf("第 2 句未引用: %q", out)
	}
	if strings.Contains(out, "今天天气不错哦。 [ID:") {
		t.Fatalf("无关句不应引用: %q", out)
	}
	if len(cited) != 2 || cited[0] != 0 || cited[1] != 1 {
		t.Fatalf("cited = %v", cited)
	}
}

func TestInsertCitationsCodeBlockUntouched(t *testing.T) {
	answer := "食堂七点开门。\n\n```go\nfmt.Println(\"hello\")\n```\n\n图书馆十本上限。"
	chunks := []retrieval.Candidate{
		makeChunk(1, "食堂", []float32{1, 0}),
		makeChunk(2, "图书馆", []float32{0, 1}),
	}
	sentences, idx := SplitAnswer(answer)
	vecs := make([][]float32, len(sentences))
	for i, sent := range sentences {
		if strings.Contains(sent, "食堂") {
			vecs[i] = []float32{1, 0}
		} else {
			vecs[i] = []float32{0, 1}
		}
	}
	out, _ := InsertCitationsWithVectors(answer, chunks, vecs, sentences, idx)
	if !strings.Contains(out, "```go\nfmt.Println(\"hello\")\n```") {
		t.Fatalf("代码块被破坏: %q", out)
	}
	if strings.Contains(out, "``` [ID:") {
		t.Fatalf("代码块被引用: %q", out)
	}
}

func TestInsertCitationsThreshold(t *testing.T) {
	answer := "完全无法匹配的句子内容。"
	chunks := []retrieval.Candidate{makeChunk(1, "食堂", []float32{1, 0})}
	sentences, idx := SplitAnswer(answer)
	// 相似度 0.2：低于阈值搜索的停止线（0.3），最终不产生引用
	vecs := [][]float32{{0.2, 0.9797959}} // 与 chunk 余弦 = 0.2
	out, cited := InsertCitationsWithVectors(answer, chunks, vecs, sentences, idx)
	if len(cited) != 0 || strings.Contains(out, "[ID:") {
		t.Fatalf("低相似度不应引用: %q %v", out, cited)
	}
}

func TestInsertCitationsDedup(t *testing.T) {
	answer := "食堂七点开门。食堂位于南区。"
	chunks := []retrieval.Candidate{makeChunk(1, "食堂", []float32{1, 0})}
	sentences, idx := SplitAnswer(answer)
	vecs := [][]float32{{1, 0}, {1, 0}}
	out, cited := InsertCitationsWithVectors(answer, chunks, vecs, sentences, idx)
	if strings.Count(out, "[ID:0]") != 1 {
		t.Fatalf("同一 chunk 应只标注一次: %q", out)
	}
	if len(cited) != 1 {
		t.Fatalf("cited = %v", cited)
	}
}

func TestSplitAnswerShortSentencesFiltered(t *testing.T) {
	// "hi. " 仅 4 字节，低于 minSentenceLen=5 应被过滤
	sentences, idx := SplitAnswer("hi. 这是一句足够长的内容用于引用对齐。")
	if len(sentences) != 1 {
		t.Fatalf("短句应被过滤: %v", sentences)
	}
	if !strings.Contains(sentences[0], "足够长") {
		t.Fatalf("保留的句子不对: %v", sentences)
	}
	if idx[0] != 1 {
		t.Fatalf("sentenceIdx 不对: %v", idx)
	}
}
