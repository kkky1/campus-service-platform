// Package ingest 入库流水线：解析 → 切分 → 知识编译 → 向量化 → 落库（异步任务）。
package ingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/parser"
	"campus-service-platform/internal/rag/store"
)

// Service 入库服务（带并发上限的异步处理）。
type Service struct {
	store *store.Store
	embed embed.Embedder
	cfg   config.RAGConfig
	log   *slog.Logger
	sem   chan struct{}
}

// New 构造入库服务（最多 2 个文档并发处理）。
func New(st *store.Store, em embed.Embedder, cfg config.RAGConfig, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, embed: em, cfg: cfg, log: log, sem: make(chan struct{}, 2)}
}

// Submit 异步处理文档（daemon 立即返回，状态可查询）。
func (s *Service) Submit(doc *store.Document, filename string, data []byte) {
	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		s.Process(context.Background(), doc, filename, data)
	}()
}

// Process 同步处理文档（测试与内部调用用）。
func (s *Service) Process(ctx context.Context, doc *store.Document, filename string, data []byte) {
	started := time.Now()
	docId := *doc.Id
	kbId := *doc.KbId
	method := "naive"
	if doc.ChunkMethod != nil && *doc.ChunkMethod != "" {
		method = *doc.ChunkMethod
	}
	task := &store.Task{KbId: doc.KbId, DocId: doc.Id, Type: strPtr("ingest"), Status: strPtr(store.DocStatusRunning), Stage: strPtr("parsing")}
	_ = s.store.CreateTask(ctx, task)

	update := func(stage, status, errMsg string) {
		fields := map[string]any{"stage": stage, "status": status}
		if errMsg != "" {
			fields["error"] = errMsg
		}
		_ = s.store.UpdateDocFields(ctx, docId, fields)
		if task.Id != nil {
			tfields := map[string]any{"stage": stage, "status": status}
			if errMsg != "" {
				tfields["error"] = errMsg
			}
			_ = s.store.UpdateTask(ctx, *task.Id, tfields)
		}
	}

	// 1. 解析
	update("parsing", store.DocStatusRunning, "")
	if !parser.IsSupported(filename) {
		update("parsing", store.DocStatusFailed, "不支持的文档格式："+parser.Ext(filename))
		return
	}
	blocks, err := parser.Parse(filename, data)
	if err != nil {
		s.log.Error("解析失败", "doc", filename, "err", err)
		update("parsing", store.DocStatusFailed, "文档解析失败："+err.Error())
		return
	}
	if len(blocks) == 0 {
		update("parsing", store.DocStatusFailed, "文档内容为空")
		return
	}

	// 2. 切分 + 知识编译
	update("chunking", store.DocStatusRunning, "")
	chunks := chunker.Split(method, blocks, chunker.Config{
		TokenNum:       s.cfg.Chunk.TokenNum,
		OverlapPercent: s.cfg.Chunk.OverlapPercent,
	})
	if len(chunks) == 0 {
		update("chunking", store.DocStatusFailed, "未提取到有效内容")
		return
	}
	knowledge := chunker.CompileKnowledge(chunks, blocks)
	chunker.ApplyTags(chunks, knowledge.Tags, 4)

	// 3. 向量化 + 落库
	update("embedding", store.DocStatusRunning, "")
	_ = s.store.DeleteChunksByDoc(ctx, docId) // 幂等重入
	var rows []store.Chunk
	totalTokens := 0
	const batchSize = 32
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		texts := make([]string, 0, end-start)
		for _, c := range chunks[start:end] {
			texts = append(texts, strings.Join(c.Headings, " ")+"\n"+c.Text)
		}
		vecs, err := s.embed.Encode(ctx, texts)
		if err != nil {
			s.log.Error("向量化失败", "doc", filename, "err", err)
			update("embedding", store.DocStatusFailed, "向量化失败："+err.Error())
			return
		}
		for i, c := range chunks[start:end] {
			rows = append(rows, store.Chunk{
				KbId: &kbId, DocId: &docId, Idx: intPtr(c.Index),
				Text: strPtr(c.Text), Tokens: intPtr(c.Tokens),
				Headings: strPtr(strings.Join(c.Headings, " > ")),
				Tags:     strPtr(strings.Join(c.Tags, ",")),
				Page:     intPtr(c.Page), Offset: intPtr(c.Offset),
				Terms:     strPtr(strings.Join(chunker.Tokenize(c.Text), " ")),
				Embedding: embed.EncodeVector(vecs[i]),
			})
			totalTokens += c.Tokens
		}
	}
	if err := s.store.InsertChunks(ctx, rows); err != nil {
		s.log.Error("切片写入失败", "doc", filename, "err", err)
		update("embedding", store.DocStatusFailed, "切片写入失败")
		return
	}

	// 4. 完成
	_ = s.store.UpdateDocFields(ctx, docId, map[string]any{
		"status":      store.DocStatusDone,
		"stage":       "done",
		"chunk_count": len(chunks),
		"token_count": totalTokens,
		"tags":        strings.Join(knowledge.Tags, ","),
		"summary":     knowledge.Summary,
		"error":       nil,
	})
	if task.Id != nil {
		_ = s.store.UpdateTask(ctx, *task.Id, map[string]any{"status": store.DocStatusDone, "stage": "done"})
	}
	_ = s.store.UpdateKBCounts(ctx, kbId)
	s.log.Info("文档入库完成", "doc", filename, "chunks", len(chunks), "tokens", totalTokens, "cost", time.Since(started).String())
}

func strPtr(s string) *string { return &s }
func intPtr(v int) *int       { return &v }
