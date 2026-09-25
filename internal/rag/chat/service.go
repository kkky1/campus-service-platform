// Package chat RAG 问答：检索 → 提示词 → LLM 生成 → 逐句引用对齐。
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/llm"
	"campus-service-platform/internal/rag/retrieval"
	"campus-service-platform/internal/rag/store"
)

const systemPrompt = `你是校园服务平台的知识库助手。请严格根据提供的资料回答用户问题：
1. 只使用资料中的信息，不要编造资料中不存在的内容；
2. 资料不足以回答时，明确说明"资料中未提及"；
3. 回答简洁、准确，使用与用户问题相同的语言；
4. 不要输出任何引用标记或 ID，引用由系统自动标注。`

// Reference 返回给前端的引用条目（下标即 [ID:i] 的 i）。
type Reference struct {
	retrieval.Candidate
	Cited bool `json:"cited"`
}

// AskResult 问答结果。
type AskResult struct {
	Answer     string      `json:"answer"`
	References []Reference `json:"references,omitempty"`
	Cited      []int       `json:"cited,omitempty"`
	LatencyMs  int64       `json:"latencyMs"`
	Model      string      `json:"model,omitempty"`
}

// Service RAG 问答服务。
type Service struct {
	store     *store.Store
	retriever *retrieval.Retriever
	embed     embed.Embedder
	llm       llm.Client
	cfg       config.RAGConfig
	log       *slog.Logger
}

// New 构造问答服务。
func New(st *store.Store, ret *retrieval.Retriever, em embed.Embedder, client llm.Client, cfg config.RAGConfig, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, retriever: ret, embed: em, llm: client, cfg: cfg, log: log}
}

// Ask 问答主流程。
func (s *Service) Ask(ctx context.Context, kbId, userId int64, question string, topK int) dto.Result {
	start := time.Now()
	question = strings.TrimSpace(question)
	if question == "" {
		return dto.Fail("问题不能为空")
	}
	candidates, err := s.retriever.Retrieve(ctx, kbId, question, topK)
	if err != nil {
		s.log.Error("检索失败", "err", err)
		return dto.Fail("服务器异常")
	}
	if len(candidates) == 0 {
		return dto.Fail("知识库中没有找到相关内容，请先上传资料或换个问法")
	}
	answer, err := s.llm.Chat(ctx, systemPrompt, buildUserPrompt(question, candidates))
	if err != nil {
		s.log.Error("大模型调用失败", "err", err)
		return dto.Fail("AI 服务暂时不可用，请稍后重试")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return dto.Fail("AI 未返回有效回答，请重试")
	}
	// 逐句引用对齐
	answer, cited := InsertCitations(ctx, answer, candidates, s.embed)
	refs := make([]Reference, len(candidates))
	citedSet := map[int]bool{}
	for _, i := range cited {
		citedSet[i] = true
	}
	for i, c := range candidates {
		refs[i] = Reference{Candidate: c, Cited: citedSet[i]}
	}
	latency := time.Since(start).Milliseconds()
	result := AskResult{Answer: answer, References: refs, Cited: cited, LatencyMs: latency, Model: s.llm.Model()}

	// 问答记录
	refsJSON, _ := json.Marshal(refs)
	logRow := &store.ChatLog{
		KbId: int64P(kbId), UserId: int64P(userId), Question: strP(question), Answer: strP(answer),
		Refs: strP(string(refsJSON)), LatencyMs: int64P(latency),
	}
	if err := s.store.CreateChatLog(ctx, logRow); err != nil {
		s.log.Warn("问答记录写入失败", "err", err)
	}
	return dto.OkData(result)
}

// buildUserPrompt 组装上下文与问题。
func buildUserPrompt(question string, chunks []retrieval.Candidate) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	for i, c := range chunks {
		b.WriteString(fmt.Sprintf("资料 %d", i))
		if len(c.Headings) > 0 {
			b.WriteString("（" + strings.Join(c.Headings, " > ") + "）")
		}
		b.WriteString(":\n")
		b.WriteString(c.Text)
		b.WriteString("\n\n")
	}
	b.WriteString("</context>\n\n问题: ")
	b.WriteString(question)
	return b.String()
}

func int64P(v int64) *int64 { return &v }
func strP(s string) *string { return &s }
