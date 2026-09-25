package retrieval

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/parser"
	"campus-service-platform/internal/rag/store"
)

// seedKB 建库并写入若干文档（解析→切分→向量化→入库）。
func seedKB(t *testing.T, st *store.Store, em embed.Embedder) int64 {
	t.Helper()
	ctx := context.Background()
	kb := &store.KnowledgeBase{Name: strP("测试库"), EmbeddingProvider: strP("local"), EmbeddingDim: intP(em.Dim())}
	if err := st.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"食堂.md":  "# 食堂服务\n\n食堂位于南区，开放时间为每天 7:00 到 21:00，支持校园卡与移动支付。",
		"体育馆.md": "# 体育馆\n\n体育馆可在线预约羽毛球场地，预约入口在校园服务平台体育健身栏目。",
		"图书馆.md": "# 图书馆\n\n图书馆借书需要学生卡，每人最多借 10 本，借期 30 天。",
	}
	for name, content := range docs {
		blocks, err := parser.Parse(name, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		chunks := chunker.Split("naive", blocks, chunker.Config{TokenNum: 64})
		k := chunker.CompileKnowledge(chunks, blocks)
		chunker.ApplyTags(chunks, k.Tags, 4)
		doc := &store.Document{KbId: kb.Id, Name: strP(name), Status: strP(store.DocStatusDone), Tags: strP(strings.Join(k.Tags, ","))}
		if err := st.CreateDoc(ctx, doc); err != nil {
			t.Fatal(err)
		}
		var rows []store.Chunk
		for _, c := range chunks {
			vecs, err := em.Encode(ctx, []string{strings.Join(c.Headings, " ") + "\n" + c.Text})
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, store.Chunk{
				KbId: kb.Id, DocId: doc.Id, Idx: intP(c.Index), Text: strP(c.Text), Tokens: intP(c.Tokens),
				Headings: strP(strings.Join(c.Headings, " > ")), Tags: strP(strings.Join(c.Tags, ",")),
				Terms: strP(strings.Join(chunker.Tokenize(c.Text), " ")), Embedding: embed.EncodeVector(vecs[0]),
			})
		}
		if err := st.InsertChunks(ctx, rows); err != nil {
			t.Fatal(err)
		}
	}
	return *kb.Id
}

func newRetriever(t *testing.T) (*Retriever, *store.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:rag_retr_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.RAG.Embedding.Provider = "local"
	cfg.RAG.Embedding.Dim = 256
	cfg.RAG.Retrieval.TopK = 3
	cfg.RAG.Retrieval.VectorWeight = 0.7
	cfg.RAG.Retrieval.KeywordWeight = 0.3
	cfg.RAG.Retrieval.MinSimilarity = 0
	em := embed.New(cfg.RAG)
	return New(st, em, cfg.RAG, nil), st
}

func TestHybridRetrieval(t *testing.T) {
	r, st := newRetriever(t)
	cfg := config.Config{}
	cfg.RAG.Embedding.Dim = 256
	em := embed.New(cfg.RAG)
	kbId := seedKB(t, st, em)
	ctx := context.Background()

	cases := []struct {
		query string
		want  string
	}{
		{"食堂几点开门", "食堂"},
		{"体育馆怎么预约", "体育馆"},
		{"图书馆能借几本书", "图书馆"},
	}
	for _, c := range cases {
		got, err := r.Retrieve(ctx, kbId, c.query, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatalf("查询 %q 无结果", c.query)
		}
		if !strings.Contains(got[0].Text, c.want) {
			t.Fatalf("查询 %q top1 = %q, want 含 %q", c.query, got[0].Text, c.want)
		}
		if got[0].DocName == "" {
			t.Fatalf("缺文档名: %+v", got[0])
		}
	}
}

func TestKeywordOnlyAndVectorOnly(t *testing.T) {
	r, st := newRetriever(t)
	cfg := config.Config{}
	cfg.RAG.Embedding.Dim = 256
	em := embed.New(cfg.RAG)
	kbId := seedKB(t, st, em)
	ctx := context.Background()

	// 纯 BM25
	r.cfg.Retrieval.VectorWeight = 0
	r.cfg.Retrieval.KeywordWeight = 1
	got, err := r.Retrieve(ctx, kbId, "图书馆 借书", 3)
	if err != nil || len(got) == 0 || !strings.Contains(got[0].Text, "图书馆") {
		t.Fatalf("纯 BM25 检索失败: %v %v", got, err)
	}
	// 纯向量
	r.cfg.Retrieval.VectorWeight = 1
	r.cfg.Retrieval.KeywordWeight = 0
	got, err = r.Retrieve(ctx, kbId, "羽毛球场地", 3)
	if err != nil || len(got) == 0 || !strings.Contains(got[0].Text, "体育馆") {
		t.Fatalf("纯向量检索失败: %v %v", got, err)
	}
}

func TestTopKLimitAndEmpty(t *testing.T) {
	r, st := newRetriever(t)
	cfg := config.Config{}
	cfg.RAG.Embedding.Dim = 256
	em := embed.New(cfg.RAG)
	kbId := seedKB(t, st, em)
	ctx := context.Background()
	got, err := r.Retrieve(ctx, kbId, "校园", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 2 {
		t.Fatalf("topK 未生效: %d", len(got))
	}
	// 空库
	empty := &store.KnowledgeBase{Name: strP("空库")}
	_ = st.CreateKB(ctx, empty)
	got, err = r.Retrieve(ctx, *empty.Id, "任意", 3)
	if err != nil || len(got) != 0 {
		t.Fatalf("空库应返回空: %v %v", got, err)
	}
}

func strP(s string) *string { return &s }
func intP(v int) *int       { return &v }
