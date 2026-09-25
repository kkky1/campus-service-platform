// Package store RAG 模块数据模型与数据访问。
package store

import (
	"campus-service-platform/internal/pkg/dto"
)

// KnowledgeBase 知识库。
type KnowledgeBase struct {
	Id                *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	Name              *string    `gorm:"column:name;size:128;index" json:"name,omitempty"`
	Description       *string    `gorm:"column:description;size:512" json:"description,omitempty"`
	EmbeddingProvider *string    `gorm:"column:embedding_provider;size:32" json:"embeddingProvider,omitempty"`
	EmbeddingDim      *int       `gorm:"column:embedding_dim" json:"embeddingDim,omitempty"`
	DocCount          *int       `gorm:"column:doc_count" json:"docCount,omitempty"`
	ChunkCount        *int       `gorm:"column:chunk_count" json:"chunkCount,omitempty"`
	CreatedAt         *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
	UpdatedAt         *dto.TimeT `gorm:"column:updated_at" json:"updatedAt,omitempty"`
}

func (KnowledgeBase) TableName() string { return "rag_knowledge_base" }

// 文档处理状态
const (
	DocStatusPending = "pending"
	DocStatusRunning = "running"
	DocStatusDone    = "done"
	DocStatusFailed  = "failed"
)

// Document 文档。
type Document struct {
	Id          *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	KbId        *int64     `gorm:"column:kb_id;index" json:"kbId,omitempty"`
	Name        *string    `gorm:"column:name;size:255" json:"name,omitempty"`
	Ext         *string    `gorm:"column:ext;size:16" json:"ext,omitempty"`
	Size        *int64     `gorm:"column:size" json:"size,omitempty"`
	ChunkMethod *string    `gorm:"column:chunk_method;size:32" json:"chunkMethod,omitempty"`
	Status      *string    `gorm:"column:status;size:16;index" json:"status,omitempty"`
	Stage       *string    `gorm:"column:stage;size:32" json:"stage,omitempty"`
	Error       *string    `gorm:"column:error;size:1024" json:"error,omitempty"`
	ChunkCount  *int       `gorm:"column:chunk_count" json:"chunkCount,omitempty"`
	TokenCount  *int       `gorm:"column:token_count" json:"tokenCount,omitempty"`
	Tags        *string    `gorm:"column:tags;size:512" json:"tags,omitempty"`
	Summary     *string    `gorm:"column:summary;size:1024" json:"summary,omitempty"`
	CreatedAt   *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
	UpdatedAt   *dto.TimeT `gorm:"column:updated_at" json:"updatedAt,omitempty"`
}

func (Document) TableName() string { return "rag_document" }

// Chunk 切片。
type Chunk struct {
	Id       *int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	KbId     *int64  `gorm:"column:kb_id;index" json:"kbId,omitempty"`
	DocId    *int64  `gorm:"column:doc_id;index" json:"docId,omitempty"`
	Idx      *int    `gorm:"column:idx" json:"idx,omitempty"`
	Text     *string `gorm:"column:text;type:text" json:"text,omitempty"`
	Tokens   *int    `gorm:"column:tokens" json:"tokens,omitempty"`
	Headings *string `gorm:"column:headings;size:512" json:"headings,omitempty"`
	Tags     *string `gorm:"column:tags;size:512" json:"tags,omitempty"`
	Page     *int    `gorm:"column:page" json:"page,omitempty"`
	Offset   *int    `gorm:"column:offset" json:"offset,omitempty"`
	// Terms 空格分隔的检索词（BM25 用）；Embedding float32 小端二进制。
	Terms     *string `gorm:"column:terms;type:text" json:"-"`
	Embedding []byte  `gorm:"column:embedding;type:blob" json:"-"`
}

func (Chunk) TableName() string { return "rag_chunk" }

// Task 入库任务。
type Task struct {
	Id        *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	KbId      *int64     `gorm:"column:kb_id;index" json:"kbId,omitempty"`
	DocId     *int64     `gorm:"column:doc_id;index" json:"docId,omitempty"`
	Type      *string    `gorm:"column:type;size:32" json:"type,omitempty"`
	Status    *string    `gorm:"column:status;size:16;index" json:"status,omitempty"`
	Stage     *string    `gorm:"column:stage;size:32" json:"stage,omitempty"`
	Error     *string    `gorm:"column:error;size:1024" json:"error,omitempty"`
	CreatedAt *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
	UpdatedAt *dto.TimeT `gorm:"column:updated_at" json:"updatedAt,omitempty"`
}

func (Task) TableName() string { return "rag_task" }

// ChatLog 问答记录。
type ChatLog struct {
	Id        *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	KbId      *int64     `gorm:"column:kb_id;index" json:"kbId,omitempty"`
	UserId    *int64     `gorm:"column:user_id;index" json:"userId,omitempty"`
	Question  *string    `gorm:"column:question;type:text" json:"question,omitempty"`
	Answer    *string    `gorm:"column:answer;type:text" json:"answer,omitempty"`
	Refs      *string    `gorm:"column:refs;type:text" json:"refs,omitempty"`
	LatencyMs *int64     `gorm:"column:latency_ms" json:"latencyMs,omitempty"`
	CreatedAt *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
}

func (ChatLog) TableName() string { return "rag_chat_log" }

// EvalRun 评估运行。
type EvalRun struct {
	Id        *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	KbId      *int64     `gorm:"column:kb_id;index" json:"kbId,omitempty"`
	Name      *string    `gorm:"column:name;size:128" json:"name,omitempty"`
	Status    *string    `gorm:"column:status;size:16" json:"status,omitempty"`
	CaseCount *int       `gorm:"column:case_count" json:"caseCount,omitempty"`
	Metrics   *string    `gorm:"column:metrics;type:text" json:"metrics,omitempty"` // 聚合指标 JSON
	CreatedAt *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
	UpdatedAt *dto.TimeT `gorm:"column:updated_at" json:"updatedAt,omitempty"`
}

func (EvalRun) TableName() string { return "rag_eval_run" }

// EvalCase 单条评估样本结果。
type EvalCase struct {
	Id          *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	RunId       *int64     `gorm:"column:run_id;index" json:"runId,omitempty"`
	Question    *string    `gorm:"column:question;type:text" json:"question,omitempty"`
	Answer      *string    `gorm:"column:answer;type:text" json:"answer,omitempty"`
	Contexts    *string    `gorm:"column:contexts;type:text" json:"contexts,omitempty"` // JSON 数组
	GroundTruth *string    `gorm:"column:ground_truth;type:text" json:"groundTruth,omitempty"`
	Metrics     *string    `gorm:"column:metrics;type:text" json:"metrics,omitempty"` // 单条指标 JSON
	CreatedAt   *dto.TimeT `gorm:"column:created_at" json:"createdAt,omitempty"`
}

func (EvalCase) TableName() string { return "rag_eval_case" }

// AllModels AutoMigrate 用。
func AllModels() []any {
	return []any{&KnowledgeBase{}, &Document{}, &Chunk{}, &Task{}, &ChatLog{}, &EvalRun{}, &EvalCase{}}
}
