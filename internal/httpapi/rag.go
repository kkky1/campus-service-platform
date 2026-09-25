package httpapi

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/middleware"
	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/rag/chat"
	"campus-service-platform/internal/rag/eval"
	"campus-service-platform/internal/rag/ingest"
	"campus-service-platform/internal/rag/parser"
	"campus-service-platform/internal/rag/retrieval"
	"campus-service-platform/internal/rag/store"
)

// RagDeps RAG 处理器依赖。
type RagDeps struct {
	Store     *store.Store
	Ingest    *ingest.Service
	Chat      *chat.Service
	Eval      *eval.Runner
	Retrieval *retrieval.Retriever
	Cfg       config.RAGConfig
}

// RagHandlers RAG 处理器集合。
type RagHandlers struct {
	deps RagDeps
}

// NewRagHandlers 构造 RAG 处理器。
func NewRagHandlers(d RagDeps) *RagHandlers { return &RagHandlers{deps: d} }

// CreateKB POST /rag/kb
func (h *RagHandlers) CreateKB(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(200, dto.Fail("知识库名称不能为空"))
		return
	}
	provider := h.deps.Cfg.Embedding.Provider
	if provider == "" {
		provider = "local"
	}
	dim := h.deps.Cfg.Embedding.Dim
	if dim <= 0 {
		dim = 384
	}
	kb := &store.KnowledgeBase{Name: &body.Name, Description: &body.Description, EmbeddingProvider: &provider, EmbeddingDim: &dim}
	if err := h.deps.Store.CreateKB(c.Request.Context(), kb); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, dto.OkData(kb))
}

// ListKB GET /rag/kb/list
func (h *RagHandlers) ListKB(c *gin.Context) {
	list, err := h.deps.Store.ListKB(c.Request.Context())
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if list == nil {
		list = []store.KnowledgeBase{}
	}
	c.JSON(200, dto.OkData(list))
}

// DeleteKB DELETE /rag/kb/:id
func (h *RagHandlers) DeleteKB(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if err := h.deps.Store.DeleteKB(c.Request.Context(), id); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, dto.Ok())
}

// UploadDoc POST /rag/doc/upload（multipart：kbId、chunkMethod、file）
func (h *RagHandlers) UploadDoc(c *gin.Context) {
	kbId, err := strconv.ParseInt(c.PostForm("kbId"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("kbId 不能为空"))
		return
	}
	if _, err := h.deps.Store.GetKB(c.Request.Context(), kbId); err != nil {
		c.JSON(200, dto.Fail("知识库不存在"))
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(200, dto.Fail("请选择要上传的文件"))
		return
	}
	filename := fileHeader.Filename
	if !parser.IsSupported(filename) {
		c.JSON(200, dto.Fail("不支持的文档格式："+parser.Ext(filename)))
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(200, dto.Fail("读取上传文件失败"))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || len(data) == 0 {
		c.JSON(200, dto.Fail("读取上传文件失败"))
		return
	}
	method := c.PostForm("chunkMethod")
	if method == "" {
		method = "naive"
	}
	size := fileHeader.Size
	doc := &store.Document{
		KbId: &kbId, Name: &filename, Ext: strPtr(parser.Ext(filename)), Size: &size,
		ChunkMethod: &method, Status: strPtr(store.DocStatusPending), Stage: strPtr("queued"),
	}
	if err := h.deps.Store.CreateDoc(c.Request.Context(), doc); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	h.deps.Ingest.Submit(doc, filename, data)
	c.JSON(200, dto.OkData(map[string]any{"docId": *doc.Id, "status": store.DocStatusPending}))
}

// ListDocs GET /rag/doc/list?kbId=
func (h *RagHandlers) ListDocs(c *gin.Context) {
	kbId, err := strconv.ParseInt(c.Query("kbId"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("kbId 不能为空"))
		return
	}
	list, err := h.deps.Store.ListDocs(c.Request.Context(), kbId)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if list == nil {
		list = []store.Document{}
	}
	c.JSON(200, dto.OkData(list))
}

// DocChunks GET /rag/doc/:id/chunks
func (h *RagHandlers) DocChunks(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	list, err := h.deps.Store.ListChunksByDoc(c.Request.Context(), id)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if list == nil {
		list = []store.Chunk{}
	}
	c.JSON(200, dto.OkData(list))
}

// DeleteDoc DELETE /rag/doc/:id
func (h *RagHandlers) DeleteDoc(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	doc, err := h.deps.Store.GetDoc(c.Request.Context(), id)
	if err != nil {
		c.JSON(200, dto.Fail("文档不存在"))
		return
	}
	if err := h.deps.Store.DeleteDoc(c.Request.Context(), id); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if doc.KbId != nil {
		_ = h.deps.Store.UpdateKBCounts(c.Request.Context(), *doc.KbId)
	}
	c.JSON(200, dto.Ok())
}

// Chat POST /rag/chat
func (h *RagHandlers) Chat(c *gin.Context) {
	var body struct {
		KbId     int64  `json:"kbId"`
		Question string `json:"question"`
		TopK     int    `json:"topK"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KbId == 0 {
		c.JSON(200, dto.Fail("参数不完整"))
		return
	}
	var userId int64
	if u := middleware.CurrentUser(c); u != nil && u.Id != nil {
		userId = *u.Id
	}
	c.JSON(200, h.deps.Chat.Ask(c.Request.Context(), body.KbId, userId, body.Question, body.TopK))
}

// Retrieve POST /rag/retrieve（检索调试/评估辅助）
func (h *RagHandlers) Retrieve(c *gin.Context) {
	var body struct {
		KbId     int64  `json:"kbId"`
		Question string `json:"question"`
		TopK     int    `json:"topK"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KbId == 0 {
		c.JSON(200, dto.Fail("参数不完整"))
		return
	}
	candidates, err := h.deps.Retrieval.Retrieve(c.Request.Context(), body.KbId, body.Question, body.TopK)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if candidates == nil {
		candidates = []retrieval.Candidate{}
	}
	c.JSON(200, dto.OkData(candidates))
}

// EvalRun POST /rag/eval/run
func (h *RagHandlers) EvalRun(c *gin.Context) {
	var body struct {
		KbId        int64            `json:"kbId"`
		Name        string           `json:"name"`
		UsePipeline bool             `json:"usePipeline"`
		Cases       []eval.CaseInput `json:"cases"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KbId == 0 || len(body.Cases) == 0 {
		c.JSON(200, dto.Fail("参数不完整：需要 kbId 与 cases"))
		return
	}
	if body.Name == "" {
		body.Name = "eval"
	}
	var userId int64
	if u := middleware.CurrentUser(c); u != nil && u.Id != nil {
		userId = *u.Id
	}
	run, _, err := h.deps.Eval.Run(c.Request.Context(), body.KbId, body.Name, body.Cases, body.UsePipeline, userId)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	var metrics any
	_ = json.Unmarshal([]byte(valOr(run.Metrics, "{}")), &metrics)
	c.JSON(200, dto.OkData(map[string]any{"runId": *run.Id, "caseCount": *run.CaseCount, "metrics": metrics}))
}

// EvalGet GET /rag/eval/:id
func (h *RagHandlers) EvalGet(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	run, err := h.deps.Store.GetEvalRun(c.Request.Context(), id)
	if err != nil {
		c.JSON(200, dto.Fail("评估记录不存在"))
		return
	}
	cases, err := h.deps.Store.ListEvalCases(c.Request.Context(), id)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	var metrics any
	_ = json.Unmarshal([]byte(valOr(run.Metrics, "{}")), &metrics)
	var caseViews []map[string]any
	for _, cs := range cases {
		var m, ctxs any
		_ = json.Unmarshal([]byte(valOr(cs.Metrics, "{}")), &m)
		_ = json.Unmarshal([]byte(valOr(cs.Contexts, "[]")), &ctxs)
		caseViews = append(caseViews, map[string]any{
			"question": valOr(cs.Question, ""), "answer": valOr(cs.Answer, ""),
			"groundTruth": valOr(cs.GroundTruth, ""), "contexts": ctxs, "metrics": m,
		})
	}
	c.JSON(200, dto.OkData(map[string]any{
		"runId": *run.Id, "name": valOr(run.Name, ""), "status": valOr(run.Status, ""),
		"caseCount": run.CaseCount, "metrics": metrics, "cases": caseViews,
	}))
}

func valOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func strPtr(s string) *string { return &s }
