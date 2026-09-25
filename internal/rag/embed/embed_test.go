package embed

import (
	"context"
	"math"
	"testing"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
)

func TestLocalEmbedderDeterministic(t *testing.T) {
	e := &LocalEmbedder{Dimension: 128}
	ctx := context.Background()
	v1, err := e.Encode(ctx, []string{"校园食堂开放时间"})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := e.Encode(ctx, []string{"校园食堂开放时间"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v1[0]) != 128 {
		t.Fatalf("维度 = %d", len(v1[0]))
	}
	for i := range v1[0] {
		if v1[0][i] != v2[0][i] {
			t.Fatalf("不确定：%d", i)
		}
	}
	// 归一化检查
	var norm float64
	for _, x := range v1[0] {
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1.0) > 1e-6 {
		t.Fatalf("未归一化: %f", norm)
	}
}

func TestLocalEmbedderSimilarity(t *testing.T) {
	e := &LocalEmbedder{Dimension: 512}
	ctx := context.Background()
	vecs, err := e.Encode(ctx, []string{
		"食堂开放时间 7 点到 21 点",
		"食堂几点开门",
		"图书馆借书规则",
	})
	if err != nil {
		t.Fatal(err)
	}
	related := Cosine(vecs[0], vecs[1])
	unrelated := Cosine(vecs[0], vecs[2])
	if related <= unrelated {
		t.Fatalf("相关应当高于不相关: related=%f unrelated=%f", related, unrelated)
	}
	if related <= 0.2 {
		t.Fatalf("相关文本相似度过低: %f", related)
	}
}

func TestVectorSerialization(t *testing.T) {
	src := []float32{0, 1, -1, 3.14159, -2.71828}
	got := DecodeVector(EncodeVector(src))
	if len(got) != len(src) {
		t.Fatalf("长度 = %d", len(got))
	}
	for i := range src {
		if math.Abs(float64(src[i]-got[i])) > 1e-5 {
			t.Fatalf("第 %d 项 = %f, want %f", i, got[i], src[i])
		}
	}
	if DecodeVector([]byte{1, 2, 3}) != nil {
		t.Fatal("非法长度应返回 nil")
	}
}

func TestCosineEdgeCases(t *testing.T) {
	if Cosine([]float32{0, 0}, []float32{1, 1}) != 0 {
		t.Fatal("零向量应为 0")
	}
	if Cosine([]float32{1, 0}, []float32{1, 0}) != 1 {
		t.Fatal("自身应为 1")
	}
}

func TestNewProviderSelection(t *testing.T) {
	cfg := config.Config{}
	cfg.RAG.Embedding.Provider = "local"
	cfg.RAG.Embedding.Dim = 64
	if e := New(cfg.RAG); e.Dim() != 64 {
		t.Fatalf("local 维度 = %d", e.Dim())
	}
	cfg.RAG.Embedding.Provider = "openai"
	cfg.RAG.Embedding.BaseURL = "https://api.example.com"
	if e := New(cfg.RAG); e.Dim() != 64 {
		t.Fatalf("openai 维度 = %d", e.Dim())
	}
}

var _ = chunker.Tokenize
