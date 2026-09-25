package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/llm"
	"campus-service-platform/internal/rag/parser"
	"campus-service-platform/internal/rag/retrieval"
	"campus-service-platform/internal/rag/store"
)

type env struct {
	svc   *Service
	store *store.Store
	kbId  int64
}

func newChatEnv(t *testing.T, answer string, llmErr error) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:rag_chat_test?mode=memory&cache=shared"), &gorm.Config{})
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
	ret := retrieval.New(st, em, cfg.RAG, nil)
	mock := &llm.FuncClient{Fn: func(_ context.Context, _, _ string) (string, error) {
		return answer, llmErr
	}}
	return &env{svc: New(st, ret, em, mock, cfg.RAG, nil), store: st, kbId: seedChatKB(t, st, em)}
}

func seedChatKB(t *testing.T, st *store.Store, em embed.Embedder) int64 {
	t.Helper()
	ctx := context.Background()
	kb := &store.KnowledgeBase{Name: strP("问答测试库")}
	if err := st.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"食堂.md":  "# 食堂服务\n\n食堂位于南区，开放时间为每天 7:00 到 21:00。",
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
		doc := &store.Document{KbId: kb.Id, Name: strP(name), Status: strP(store.DocStatusDone)}
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
				KbId: kb.Id, DocId: doc.Id, Idx: intP(c.Index), Text: strP(c.Text),
				Terms: strP(strings.Join(chunker.Tokenize(c.Text), " ")), Embedding: embed.EncodeVector(vecs[0]),
			})
		}
		if err := st.InsertChunks(ctx, rows); err != nil {
			t.Fatal(err)
		}
	}
	return *kb.Id
}

func TestAskFullPipeline(t *testing.T) {
	mockAnswer := "食堂每天早上 7 点开门。图书馆每人最多可以借 10 本书。"
	e := newChatEnv(t, mockAnswer, nil)
	res := e.svc.Ask(context.Background(), e.kbId, 1, "食堂几点开门？图书馆能借几本？", 3)
	if !res.Success {
		t.Fatalf("问答失败: %v", res.ErrorMsg)
	}
	out := res.Data.(AskResult)
	if !strings.Contains(out.Answer, "[ID:") {
		t.Fatalf("答案未带引用标记: %s", out.Answer)
	}
	if len(out.References) == 0 {
		t.Fatal("引用列表为空")
	}
	if len(out.Cited) == 0 {
		t.Fatal("无被引用切片")
	}
	if out.Model != "mock" || out.LatencyMs < 0 {
		t.Fatalf("元数据不对: %+v", out)
	}
	// 引用下标有效
	for _, i := range out.Cited {
		if i < 0 || i >= len(out.References) {
			t.Fatalf("引用下标越界: %d", i)
		}
		if !out.References[i].Cited {
			t.Fatalf("references 未标记 cited: %d", i)
		}
	}
	// 问答记录落库
	var count int64
	e.store.DB().Model(&store.ChatLog{}).Count(&count)
	if count != 1 {
		t.Fatalf("问答记录数 = %d, want 1", count)
	}
}

func TestAskLLMError(t *testing.T) {
	e := newChatEnv(t, "", context.DeadlineExceeded)
	res := e.svc.Ask(context.Background(), e.kbId, 1, "食堂几点开门？", 3)
	if res.Success || res.ErrorMsg == nil || !strings.Contains(*res.ErrorMsg, "AI 服务暂时不可用") {
		t.Fatalf("LLM 失败应提示: %v", res)
	}
}

func TestAskEmptyKB(t *testing.T) {
	e := newChatEnv(t, "x", nil)
	empty := &store.KnowledgeBase{Name: strP("空库")}
	if err := e.store.CreateKB(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	res := e.svc.Ask(context.Background(), *empty.Id, 1, "任意问题", 3)
	if res.Success || res.ErrorMsg == nil || !strings.Contains(*res.ErrorMsg, "没有找到相关内容") {
		t.Fatalf("空库应提示: %v", res)
	}
}

func TestAskEmptyQuestion(t *testing.T) {
	e := newChatEnv(t, "x", nil)
	res := e.svc.Ask(context.Background(), e.kbId, 1, "   ", 3)
	if res.Success || res.ErrorMsg == nil || *res.ErrorMsg != "问题不能为空" {
		t.Fatalf("空问题: %v", res)
	}
}

func intP(v int) *int { return &v }
