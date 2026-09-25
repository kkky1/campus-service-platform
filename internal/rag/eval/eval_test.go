package eval

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/store"
)

func newEvalEnv(t *testing.T) (*Runner, *store.Store, embed.Embedder) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:rag_eval_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.RAG.Embedding.Provider = "local"
	cfg.RAG.Embedding.Dim = 384
	em := embed.New(cfg.RAG)
	return New(st, em, nil, cfg.RAG), st, em
}

func TestComputeMetricsHighQuality(t *testing.T) {
	_, _, em := newEvalEnv(t)
	ctx := context.Background()
	contexts := []string{
		"食堂位于南区，开放时间为每天 7:00 到 21:00。",
		"图书馆借书需要学生卡，每人最多借 10 本。",
	}
	answer := "食堂的开放时间是每天 7:00 到 21:00。图书馆每人最多借 10 本书。"
	gt := answer
	m, err := ComputeMetrics(ctx, em, "食堂几点开门？图书馆能借几本书？", answer, contexts, gt)
	if err != nil {
		t.Fatal(err)
	}
	if m.Faithfulness < 0.5 {
		t.Fatalf("faithfulness 过低: %+v", m)
	}
	if m.ContextRecall < 0.5 {
		t.Fatalf("context_recall 过低: %+v", m)
	}
	if m.AnswerRelevancy <= 0 {
		t.Fatalf("answer_relevancy 应为正: %+v", m)
	}
	if m.ContextPrecision <= 0 {
		t.Fatalf("context_precision 应为正: %+v", m)
	}
}

func TestComputeMetricsHallucination(t *testing.T) {
	_, _, em := newEvalEnv(t)
	ctx := context.Background()
	contexts := []string{"食堂位于南区，开放时间为每天 7:00 到 21:00。"}
	// 答案第二句与上下文无关（幻觉）
	answer := "食堂开放时间为每天 7:00 到 21:00。体育馆今天举办星际旅行展览。"
	m, err := ComputeMetrics(ctx, em, "食堂几点开门？", answer, contexts, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Faithfulness >= 1.0 {
		t.Fatalf("含幻觉答案的 faithfulness 不应满分: %+v", m)
	}
}

func TestCitationAccuracy(t *testing.T) {
	_, _, em := newEvalEnv(t)
	ctx := context.Background()
	contexts := []string{
		"食堂位于南区，开放时间为每天 7:00 到 21:00。",
		"图书馆借书需要学生卡，每人最多借 10 本。",
	}
	// 正确引用
	answer := "食堂开放时间为每天 7:00 到 21:00。 [ID:0] 图书馆每人最多借 10 本。 [ID:1]"
	m, err := ComputeMetrics(ctx, em, "校园服务", answer, contexts, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.CitationAccuracy != 1.0 {
		t.Fatalf("正确引用应满分: %+v", m)
	}
	// 错误引用（[ID:1] 引到图书馆句子后但实际说的是食堂）
	wrong := "食堂开放时间为每天 7:00 到 21:00。 [ID:1]"
	m, err = ComputeMetrics(ctx, em, "校园服务", wrong, contexts, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.CitationAccuracy >= 1.0 {
		t.Fatalf("错误引用不应满分: %+v", m)
	}
	// 无引用标记 → 视为 1
	m, _ = ComputeMetrics(ctx, em, "校园服务", "食堂开放时间 7:00 到 21:00。", contexts, "")
	if m.CitationAccuracy != 1.0 {
		t.Fatalf("无引用应视为 1: %+v", m)
	}
}

func TestRunStoresResults(t *testing.T) {
	r, st, _ := newEvalEnv(t)
	ctx := context.Background()
	cases := []CaseInput{
		{Question: "食堂几点开门？", Answer: "食堂开放时间为每天 7:00 到 21:00。", Contexts: []string{"食堂位于南区，开放时间为每天 7:00 到 21:00。"}, GroundTruth: "食堂 7 点开门。"},
	}
	run, results, err := r.Run(ctx, 1, "baseline", cases, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if run.Id == nil || *run.Status != "done" || *run.CaseCount != 1 {
		t.Fatalf("run: %+v", run)
	}
	if len(results) != 1 {
		t.Fatalf("results: %d", len(results))
	}
	var agg Metrics
	if err := json.Unmarshal([]byte(*run.Metrics), &agg); err != nil {
		t.Fatal(err)
	}
	if agg.Faithfulness <= 0 || agg.CitationAccuracy <= 0 {
		t.Fatalf("聚合指标异常: %+v", agg)
	}
	// 落库检查
	saved, err := st.GetEvalRun(ctx, *run.Id)
	if err != nil || *saved.Status != "done" {
		t.Fatalf("GetEvalRun: %v %v", saved, err)
	}
	casesRows, err := st.ListEvalCases(ctx, *run.Id)
	if err != nil || len(casesRows) != 1 {
		t.Fatalf("ListEvalCases: %d %v", len(casesRows), err)
	}
}

func TestRunEmptyCases(t *testing.T) {
	r, _, _ := newEvalEnv(t)
	if _, _, err := r.Run(context.Background(), 1, "x", nil, false, 1); err == nil {
		t.Fatal("空样本应报错")
	}
}

func TestSplitSentences(t *testing.T) {
	got := splitSentences("第一句。第二句！Third sentence. 第四句")
	if len(got) != 4 {
		t.Fatalf("切句 = %d: %v", len(got), got)
	}
	if splitSentences("   ") != nil {
		t.Fatal("空白应返回 nil")
	}
}
