package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chat"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/eval"
	"campus-service-platform/internal/rag/ingest"
	"campus-service-platform/internal/rag/llm"
	"campus-service-platform/internal/rag/retrieval"
	ragstore "campus-service-platform/internal/rag/store"
	"campus-service-platform/internal/repo"
	"campus-service-platform/internal/service"
)

// fullRagEnv 完整路由 + RAG 模块（Mock LLM）。
func fullRagEnv(t *testing.T) (*gin.Engine, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db, err := gorm.Open(sqlite.Open("file:httpapi_rag_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&repo.User{}, &repo.UserInfo{}, &repo.Shop{}, &repo.ShopType{},
		&repo.Blog{}, &repo.BlogComments{}, &repo.Follow{},
		&repo.Voucher{}, &repo.SeckillVoucher{}, &repo.VoucherOrder{},
	); err != nil {
		t.Fatal(err)
	}
	rstore := ragstore.New(db)
	if err := rstore.Migrate(); err != nil {
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
	retriever := retrieval.New(rstore, em, cfg.RAG, nil)
	mockLLM := &llm.FuncClient{Name: "mock", Fn: func(_ context.Context, _, _ string) (string, error) {
		return "食堂每天早上 7 点开门。图书馆每人最多可以借 10 本书。", nil
	}}
	chatSvc := chat.New(rstore, retriever, em, mockLLM, cfg.RAG, nil)
	app := service.New(db, rdb, fakePub{}, nil, t.TempDir())
	gin.SetMode(gin.TestMode)
	h := New(app).WithRag(NewRagHandlers(RagDeps{
		Store:     rstore,
		Ingest:    ingest.New(rstore, em, cfg.RAG, nil),
		Chat:      chatSvc,
		Eval:      eval.New(rstore, em, chatSvc, cfg.RAG),
		Retrieval: retriever,
		Cfg:       cfg.RAG,
	}))
	return NewRouter(h), db, mr
}

func uploadDoc(t *testing.T, r *gin.Engine, token string, kbId int64, filename, content string) int64 {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("kbId", fmt.Sprint(kbId))
	_ = w.WriteField("chunkMethod", "naive")
	fw, _ := w.CreateFormFile("file", filename)
	_, _ = fw.Write([]byte(content))
	_ = w.Close()
	req := httptest.NewRequest("POST", "/rag/doc/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("authorization", token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	data, _ := m["data"].(map[string]any)
	if data == nil {
		t.Fatalf("上传失败: %s", rec.Body.String())
	}
	return int64(data["docId"].(float64))
}

func waitDocDone(t *testing.T, r *gin.Engine, token string, kbId, docId int64) {
	t.Helper()
	for i := 0; i < 60; i++ {
		_, m := doJSON(t, r, "GET", fmt.Sprintf("/rag/doc/list?kbId=%d", kbId), "", map[string]string{"authorization": token})
		arr, _ := m["data"].([]any)
		for _, it := range arr {
			d := it.(map[string]any)
			if int64(d["id"].(float64)) == docId {
				if d["status"] == "done" {
					return
				}
				if d["status"] == "failed" {
					t.Fatalf("入库失败: %v", d["error"])
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("入库超时")
}

func TestRagEndToEnd(t *testing.T) {
	r, _, mr := fullRagEnv(t)
	token, _ := loginToken(t, r, mr, "13800000021")
	auth := map[string]string{"authorization": token}

	// 1. 建库
	_, m := doJSON(t, r, "POST", "/rag/kb", `{"name":"校园知识库","description":"测试"}`, auth)
	if m["success"] != true {
		t.Fatalf("建库失败: %v", m)
	}
	kb := m["data"].(map[string]any)
	kbId := int64(kb["id"].(float64))

	// 2. 上传两份文档
	doc1 := uploadDoc(t, r, token, kbId, "食堂.md", "# 食堂\n\n食堂位于南区，开放时间为每天 7:00 到 21:00，支持校园卡。")
	doc2 := uploadDoc(t, r, token, kbId, "图书馆.md", "# 图书馆\n\n图书馆借书需要学生卡，每人最多借 10 本，借期 30 天。")
	waitDocDone(t, r, token, kbId, doc1)
	waitDocDone(t, r, token, kbId, doc2)

	// 3. 文档列表与切片
	_, m = doJSON(t, r, "GET", fmt.Sprintf("/rag/doc/list?kbId=%d", kbId), "", auth)
	docs := m["data"].([]any)
	if len(docs) != 2 {
		t.Fatalf("文档数 = %d", len(docs))
	}
	first := docs[0].(map[string]any)
	if first["status"] != "done" || first["chunkCount"].(float64) < 1 {
		t.Fatalf("文档状态: %v", first)
	}
	_, m = doJSON(t, r, "GET", fmt.Sprintf("/rag/doc/%d/chunks", doc1), "", auth)
	chunks := m["data"].([]any)
	if len(chunks) == 0 {
		t.Fatal("切片为空")
	}
	ck := chunks[0].(map[string]any)
	if _, hasEmbedding := ck["embedding"]; hasEmbedding {
		t.Fatal("响应不应暴露 embedding")
	}
	if ck["text"] == nil || ck["text"] == "" {
		t.Fatalf("切片文本为空: %v", ck)
	}

	// 4. 检索
	_, m = doJSON(t, r, "POST", "/rag/retrieve", fmt.Sprintf(`{"kbId":%d,"question":"食堂几点开门？","topK":3}`, kbId), auth)
	rets := m["data"].([]any)
	if len(rets) == 0 {
		t.Fatal("检索为空")
	}
	if !containsStr(rets[0].(map[string]any)["text"].(string), "食堂") {
		t.Fatalf("检索 top1 不对: %v", rets[0])
	}

	// 5. 问答（Mock LLM + 引用对齐）
	_, m = doJSON(t, r, "POST", "/rag/chat", fmt.Sprintf(`{"kbId":%d,"question":"食堂几点开门？图书馆能借几本？"}`, kbId), auth)
	if m["success"] != true {
		t.Fatalf("问答失败: %v", m)
	}
	ans := m["data"].(map[string]any)
	answer, _ := ans["answer"].(string)
	if !containsStr(answer, "[ID:") {
		t.Fatalf("答案缺少引用标记: %s", answer)
	}
	refs, _ := ans["references"].([]any)
	if len(refs) == 0 {
		t.Fatal("引用列表为空")
	}
	if ans["latencyMs"] == nil {
		t.Fatal("缺少 latencyMs")
	}

	// 6. 评估（含自定义引用准确率）
	evalBody := fmt.Sprintf(`{"kbId":%d,"name":"e2e","cases":[{"question":"食堂几点开门？","groundTruth":"食堂 7 点开门。","answer":"食堂开放时间为每天 7:00 到 21:00。 [ID:0]","contexts":["食堂位于南区，开放时间为每天 7:00 到 21:00。"]}]}`, kbId)
	_, m = doJSON(t, r, "POST", "/rag/eval/run", evalBody, auth)
	if m["success"] != true {
		t.Fatalf("评估失败: %v", m)
	}
	runData := m["data"].(map[string]any)
	runId := int64(runData["runId"].(float64))
	metrics := runData["metrics"].(map[string]any)
	for _, k := range []string{"faithfulness", "answer_relevancy", "context_precision", "context_recall", "citation_accuracy"} {
		if metrics[k] == nil {
			t.Fatalf("缺少指标 %s: %v", k, metrics)
		}
	}
	_, m = doJSON(t, r, "GET", fmt.Sprintf("/rag/eval/%d", runId), "", auth)
	detail := m["data"].(map[string]any)
	if detail["status"] != "done" || len(detail["cases"].([]any)) != 1 {
		t.Fatalf("评估详情: %v", detail)
	}

	// 7. 删除文档与知识库
	_, m = doJSON(t, r, "DELETE", fmt.Sprintf("/rag/doc/%d", doc2), "", auth)
	if m["success"] != true {
		t.Fatalf("删除文档失败: %v", m)
	}
	_, m = doJSON(t, r, "DELETE", fmt.Sprintf("/rag/kb/%d", kbId), "", auth)
	if m["success"] != true {
		t.Fatalf("删除知识库失败: %v", m)
	}
}

func TestRagRequiresLogin(t *testing.T) {
	r, _, _ := fullRagEnv(t)
	code, _ := doJSON(t, r, "POST", "/rag/kb", `{"name":"x"}`, nil)
	if code != 401 {
		t.Fatalf("RAG 接口未登录应 401, got %d", code)
	}
}

func TestRagBadInput(t *testing.T) {
	r, _, mr := fullRagEnv(t)
	token, _ := loginToken(t, r, mr, "13800000022")
	auth := map[string]string{"authorization": token}
	// 空名称
	_, m := doJSON(t, r, "POST", "/rag/kb", `{"name":""}`, auth)
	if m["success"] != false {
		t.Fatalf("空名称应失败: %v", m)
	}
	// 不存在的知识库上传
	code, m := doJSON(t, r, "POST", "/rag/chat", `{"kbId":999,"question":"x"}`, auth)
	if code != 200 || m["success"] != false {
		t.Fatalf("不存在知识库: %v", m)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
