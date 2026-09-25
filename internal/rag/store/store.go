package store

import (
	"context"

	"gorm.io/gorm"
)

// Store RAG 数据访问。
type Store struct {
	db *gorm.DB
}

// New 构造 Store。
func New(db *gorm.DB) *Store { return &Store{db: db} }

// DB 暴露底层连接。
func (s *Store) DB() *gorm.DB { return s.db }

// Migrate 创建 RAG 表。
func (s *Store) Migrate() error {
	return s.db.AutoMigrate(AllModels()...)
}

// ---- 知识库 ----

func (s *Store) CreateKB(ctx context.Context, kb *KnowledgeBase) error {
	return s.db.WithContext(ctx).Create(kb).Error
}

func (s *Store) GetKB(ctx context.Context, id int64) (*KnowledgeBase, error) {
	var kb KnowledgeBase
	if err := s.db.WithContext(ctx).First(&kb, id).Error; err != nil {
		return nil, err
	}
	return &kb, nil
}

func (s *Store) ListKB(ctx context.Context) ([]KnowledgeBase, error) {
	var list []KnowledgeBase
	err := s.db.WithContext(ctx).Order("id DESC").Find(&list).Error
	return list, err
}

func (s *Store) DeleteKB(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("kb_id = ?", id).Delete(&Chunk{}).Error; err != nil {
			return err
		}
		if err := tx.Where("kb_id = ?", id).Delete(&Document{}).Error; err != nil {
			return err
		}
		if err := tx.Where("kb_id = ?", id).Delete(&Task{}).Error; err != nil {
			return err
		}
		// 级联清理评估与问答记录（QA B5）
		var runIds []int64
		if err := tx.Model(&EvalRun{}).Where("kb_id = ?", id).Pluck("id", &runIds).Error; err != nil {
			return err
		}
		if len(runIds) > 0 {
			if err := tx.Where("run_id IN ?", runIds).Delete(&EvalCase{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("kb_id = ?", id).Delete(&EvalRun{}).Error; err != nil {
			return err
		}
		if err := tx.Where("kb_id = ?", id).Delete(&ChatLog{}).Error; err != nil {
			return err
		}
		return tx.Delete(&KnowledgeBase{}, id).Error
	})
}

func (s *Store) UpdateKBCounts(ctx context.Context, kbId int64) error {
	var docCount, chunkCount int64
	if err := s.db.WithContext(ctx).Model(&Document{}).Where("kb_id = ?", kbId).Count(&docCount).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&Chunk{}).Where("kb_id = ?", kbId).Count(&chunkCount).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&KnowledgeBase{}).Where("id = ?", kbId).
		Updates(map[string]any{"doc_count": docCount, "chunk_count": chunkCount}).Error
}

// ---- 文档 ----

func (s *Store) CreateDoc(ctx context.Context, doc *Document) error {
	return s.db.WithContext(ctx).Create(doc).Error
}

func (s *Store) GetDoc(ctx context.Context, id int64) (*Document, error) {
	var doc Document
	if err := s.db.WithContext(ctx).First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *Store) ListDocs(ctx context.Context, kbId int64) ([]Document, error) {
	var list []Document
	err := s.db.WithContext(ctx).Where("kb_id = ?", kbId).Order("id DESC").Find(&list).Error
	return list, err
}

func (s *Store) UpdateDocFields(ctx context.Context, docId int64, fields map[string]any) error {
	return s.db.WithContext(ctx).Model(&Document{}).Where("id = ?", docId).Updates(fields).Error
}

func (s *Store) DeleteDoc(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("doc_id = ?", id).Delete(&Chunk{}).Error; err != nil {
			return err
		}
		if err := tx.Where("doc_id = ?", id).Delete(&Task{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Document{}, id).Error
	})
}

// ---- 切片 ----

func (s *Store) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).CreateInBatches(chunks, 100).Error
}

func (s *Store) ListChunksByKB(ctx context.Context, kbId int64) ([]Chunk, error) {
	var list []Chunk
	err := s.db.WithContext(ctx).Where("kb_id = ?", kbId).Order("doc_id, idx").Find(&list).Error
	return list, err
}

func (s *Store) ListChunksByDoc(ctx context.Context, docId int64) ([]Chunk, error) {
	var list []Chunk
	err := s.db.WithContext(ctx).Where("doc_id = ?", docId).Order("idx").Find(&list).Error
	return list, err
}

func (s *Store) DeleteChunksByDoc(ctx context.Context, docId int64) error {
	return s.db.WithContext(ctx).Where("doc_id = ?", docId).Delete(&Chunk{}).Error
}

// ---- 任务 ----

func (s *Store) CreateTask(ctx context.Context, t *Task) error {
	return s.db.WithContext(ctx).Create(t).Error
}

func (s *Store) UpdateTask(ctx context.Context, id int64, fields map[string]any) error {
	return s.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(fields).Error
}

// ---- 问答记录 ----

func (s *Store) CreateChatLog(ctx context.Context, log *ChatLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

// ---- 评估 ----

func (s *Store) CreateEvalRun(ctx context.Context, run *EvalRun) error {
	return s.db.WithContext(ctx).Create(run).Error
}

func (s *Store) UpdateEvalRun(ctx context.Context, id int64, fields map[string]any) error {
	return s.db.WithContext(ctx).Model(&EvalRun{}).Where("id = ?", id).Updates(fields).Error
}

func (s *Store) GetEvalRun(ctx context.Context, id int64) (*EvalRun, error) {
	var run EvalRun
	if err := s.db.WithContext(ctx).First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Store) InsertEvalCases(ctx context.Context, cases []EvalCase) error {
	if len(cases) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).CreateInBatches(cases, 100).Error
}

func (s *Store) ListEvalCases(ctx context.Context, runId int64) ([]EvalCase, error) {
	var list []EvalCase
	err := s.db.WithContext(ctx).Where("run_id = ?", runId).Order("id").Find(&list).Error
	return list, err
}
