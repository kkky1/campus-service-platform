package store

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:rag_store_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s
}

func int64P(v int64) *int64 { return &v }
func intP(v int) *int       { return &v }
func strP(s string) *string { return &s }

func TestKBCRUDBasics(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	kb := &KnowledgeBase{Name: strP("校园服务知识库"), EmbeddingProvider: strP("local"), EmbeddingDim: intP(64)}
	if err := s.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetKB(ctx, *kb.Id)
	if err != nil || *got.Name != "校园服务知识库" {
		t.Fatalf("GetKB: %v %v", got, err)
	}
	list, err := s.ListKB(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListKB: %v %v", list, err)
	}
	// 文档 + 切片
	doc := &Document{KbId: kb.Id, Name: strP("a.md"), Status: strP(DocStatusDone)}
	if err := s.CreateDoc(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertChunks(ctx, []Chunk{
		{KbId: kb.Id, DocId: doc.Id, Idx: intP(0), Text: strP("第一段"), Tokens: intP(3), Embedding: []byte{1, 2, 3, 4}},
		{KbId: kb.Id, DocId: doc.Id, Idx: intP(1), Text: strP("第二段"), Tokens: intP(3), Embedding: []byte{5, 6, 7, 8}},
	}); err != nil {
		t.Fatal(err)
	}
	chunks, err := s.ListChunksByKB(ctx, *kb.Id)
	if err != nil || len(chunks) != 2 {
		t.Fatalf("ListChunksByKB: %v %v", chunks, err)
	}
	if err := s.UpdateKBCounts(ctx, *kb.Id); err != nil {
		t.Fatal(err)
	}
	kb2, _ := s.GetKB(ctx, *kb.Id)
	if *kb2.DocCount != 1 || *kb2.ChunkCount != 2 {
		t.Fatalf("counts: doc=%d chunk=%d", *kb2.DocCount, *kb2.ChunkCount)
	}
	// 删除知识库级联
	if err := s.DeleteKB(ctx, *kb.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetKB(ctx, *kb.Id); err == nil {
		t.Fatal("知识库未删除")
	}
	chunks, _ = s.ListChunksByKB(ctx, *kb.Id)
	if len(chunks) != 0 {
		t.Fatalf("切片未级联删除: %d", len(chunks))
	}
}

func TestTaskAndEval(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	task := &Task{KbId: int64P(1), DocId: int64P(1), Type: strP("ingest"), Status: strP(DocStatusPending)}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTask(ctx, *task.Id, map[string]any{"status": DocStatusDone, "stage": "done"}); err != nil {
		t.Fatal(err)
	}
	run := &EvalRun{KbId: int64P(1), Name: strP("baseline"), Status: strP("running")}
	if err := s.CreateEvalRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvalCases(ctx, []EvalCase{{RunId: run.Id, Question: strP("q1"), Answer: strP("a1")}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateEvalRun(ctx, *run.Id, map[string]any{"status": "done", "metrics": `{"faithfulness":0.9}`}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvalRun(ctx, *run.Id)
	if err != nil || *got.Metrics != `{"faithfulness":0.9}` {
		t.Fatalf("GetEvalRun: %v %v", got, err)
	}
	cases, _ := s.ListEvalCases(ctx, *run.Id)
	if len(cases) != 1 || *cases[0].Question != "q1" {
		t.Fatalf("ListEvalCases: %v", cases)
	}
}
