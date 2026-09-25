// Package eval RAGAS 精简评估：离线确定性指标 + 自定义引用准确率。
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chat"
	"campus-service-platform/internal/rag/embed"
	"campus-service-platform/internal/rag/retrieval"
	"campus-service-platform/internal/rag/store"
)

// 阈值：支持判定（faithfulness/recall）与相关性判定（precision/引用准确率）。
const (
	supportThreshold     = 0.40
	relevanceThreshold   = 0.25
	citationMarkerRegexp = `\[ID:(\d+)\]`
)

// Metrics 一条样本的指标。
type Metrics struct {
	Faithfulness     float64 `json:"faithfulness"`      // 答案句子被上下文支持的比例
	AnswerRelevancy  float64 `json:"answer_relevancy"`  // 问题与答案句子的平均语义相似度
	ContextPrecision float64 `json:"context_precision"` // 检索上下文的排序加权准确率
	ContextRecall    float64 `json:"context_recall"`    // 标准答案（或无 GT 时用答案）被上下文覆盖的比例
	CitationAccuracy float64 `json:"citation_accuracy"` // 自定义指标：引用标记与所引切片的语义一致性
}

// CaseInput 评估输入样本。
type CaseInput struct {
	Question    string   `json:"question"`
	GroundTruth string   `json:"groundTruth,omitempty"`
	Answer      string   `json:"answer,omitempty"`
	Contexts    []string `json:"contexts,omitempty"`
}

// Runner 评估运行器。
type Runner struct {
	store *store.Store
	embed embed.Embedder
	chat  *chat.Service // 可选：自动跑检索+生成
	cfg   config.RAGConfig
}

// New 构造评估运行器。
func New(st *store.Store, em embed.Embedder, chatSvc *chat.Service, cfg config.RAGConfig) *Runner {
	return &Runner{store: st, embed: em, chat: chatSvc, cfg: cfg}
}

// Run 执行评估：usePipeline=true 时自动检索+生成答案与上下文。
func (r *Runner) Run(ctx context.Context, kbId int64, name string, cases []CaseInput, usePipeline bool, userId int64) (*store.EvalRun, []store.EvalCase, error) {
	if len(cases) == 0 {
		return nil, nil, fmt.Errorf("评估样本为空")
	}
	run := &store.EvalRun{KbId: &kbId, Name: &name, Status: strPtr("running")}
	if err := r.store.CreateEvalRun(ctx, run); err != nil {
		return nil, nil, err
	}
	var results []store.EvalCase
	var sum Metrics
	for _, in := range cases {
		answer := in.Answer
		contexts := in.Contexts
		if usePipeline && r.chat != nil {
			res := r.chat.Ask(ctx, kbId, userId, in.Question, r.cfg.Retrieval.TopK)
			if !res.Success {
				answer, contexts = "", nil
			} else if ar, ok := res.Data.(chat.AskResult); ok {
				answer = ar.Answer
				contexts = make([]string, 0, len(ar.References))
				for _, ref := range ar.References {
					contexts = append(contexts, ref.Text)
				}
			}
		}
		m, err := ComputeMetrics(ctx, r.embed, in.Question, answer, contexts, in.GroundTruth)
		if err != nil {
			return nil, nil, err
		}
		sum.Faithfulness += m.Faithfulness
		sum.AnswerRelevancy += m.AnswerRelevancy
		sum.ContextPrecision += m.ContextPrecision
		sum.ContextRecall += m.ContextRecall
		sum.CitationAccuracy += m.CitationAccuracy
		mJSON, _ := json.Marshal(m)
		ctxJSON, _ := json.Marshal(contexts)
		results = append(results, store.EvalCase{
			RunId: run.Id, Question: strPtr(in.Question), Answer: strPtr(answer),
			Contexts: strPtr(string(ctxJSON)), GroundTruth: strPtr(in.GroundTruth), Metrics: strPtr(string(mJSON)),
		})
	}
	n := float64(len(cases))
	agg := Metrics{
		Faithfulness:     round4(sum.Faithfulness / n),
		AnswerRelevancy:  round4(sum.AnswerRelevancy / n),
		ContextPrecision: round4(sum.ContextPrecision / n),
		ContextRecall:    round4(sum.ContextRecall / n),
		CitationAccuracy: round4(sum.CitationAccuracy / n),
	}
	aggJSON, _ := json.Marshal(agg)
	if err := r.store.InsertEvalCases(ctx, results); err != nil {
		return nil, nil, err
	}
	count := len(cases)
	if err := r.store.UpdateEvalRun(ctx, *run.Id, map[string]any{
		"status": "done", "case_count": count, "metrics": string(aggJSON),
	}); err != nil {
		return nil, nil, err
	}
	run.Status = strPtr("done")
	run.CaseCount = &count
	run.Metrics = strPtr(string(aggJSON))
	return run, results, nil
}

// ComputeMetrics 计算一条样本的全部指标。
func ComputeMetrics(ctx context.Context, em embed.Embedder, question, answer string, contexts []string, groundTruth string) (Metrics, error) {
	var m Metrics
	// 向量化：问题、答案句子、上下文、GT 句子
	answerSentences := splitSentences(answer)
	if len(contexts) == 0 {
		return m, nil // 无上下文：全 0（Pipeline 模式下由调用方保证有上下文）
	}
	input := []string{question}
	offset := 1
	answerOffset := offset
	input = append(input, answerSentences...)
	offset += len(answerSentences)

	contextOffset := offset
	input = append(input, contexts...)
	offset += len(contexts)

	var gtSentences []string
	if strings.TrimSpace(groundTruth) != "" {
		gtSentences = splitSentences(groundTruth)
	} else {
		// 无标准答案时以答案句子作为覆盖代理（RAGAS 通常需要 GT，这里做降级）
		gtSentences = answerSentences
	}
	gtOffset := offset
	input = append(input, gtSentences...)

	if len(input) == 0 || (len(answerSentences) == 0 && len(gtSentences) == 0) {
		return m, nil
	}
	vecs, err := em.Encode(ctx, input)
	if err != nil {
		return m, err
	}
	qVec := vecs[0]
	answerVecs := vecs[answerOffset : answerOffset+len(answerSentences)]
	contextVecs := vecs[contextOffset : contextOffset+len(contexts)]
	gtVecs := vecs[gtOffset : gtOffset+len(gtSentences)]

	// Faithfulness：答案句子被上下文支持的比例
	if len(answerVecs) > 0 {
		supported := 0
		for _, av := range answerVecs {
			if maxCosine(av, contextVecs) >= supportThreshold {
				supported++
			}
		}
		m.Faithfulness = round4(float64(supported) / float64(len(answerVecs)))
	}

	// AnswerRelevancy：问题与答案句子的平均相似度
	if len(answerVecs) > 0 {
		var total float64
		for _, av := range answerVecs {
			total += embed.Cosine(qVec, av)
		}
		m.AnswerRelevancy = round4(total / float64(len(answerVecs)))
	}

	// ContextPrecision：排序加权的相关性精度
	var relSum, weighted float64
	for k, cv := range contextVecs {
		sim := embed.Cosine(qVec, cv)
		if sim >= relevanceThreshold {
			relSum++
			weighted += relSum / float64(k+1)
		}
	}
	if relSum > 0 {
		m.ContextPrecision = round4(weighted / relSum)
	}

	// ContextRecall：GT（或答案）句子被上下文覆盖的比例
	if len(gtVecs) > 0 {
		covered := 0
		for _, gv := range gtVecs {
			if maxCosine(gv, contextVecs) >= supportThreshold {
				covered++
			}
		}
		m.ContextRecall = round4(float64(covered) / float64(len(gtVecs)))
	}

	// CitationAccuracy（自定义）：每个 [ID:i] 标记所在句子与所引切片的语义一致性
	m.CitationAccuracy = citationAccuracy(ctx, em, answer, contexts)
	return m, nil
}

// citationAccuracy 引用准确率：命中比例；无引用标记时视为 1（无错误引用）。
func citationAccuracy(ctx context.Context, em embed.Embedder, answer string, contexts []string) float64 {
	re := regexp.MustCompile(citationMarkerRegexp)
	matches := re.FindAllStringSubmatchIndex(answer, -1)
	if len(matches) == 0 {
		return 1.0
	}
	type pair struct {
		idx int
		sen string
	}
	var pairs []pair
	for _, mt := range matches {
		idx, err := strconv.Atoi(answer[mt[2]:mt[3]])
		if err != nil {
			continue
		}
		start := 0
		for i := mt[0] - 1; i >= 0; i-- {
			if strings.ContainsRune("。！？.!?\n", rune(answer[i])) {
				start = i + 1
				break
			}
		}
		sentence := strings.TrimSpace(re.ReplaceAllString(answer[start:mt[0]], ""))
		if sentence != "" {
			pairs = append(pairs, pair{idx: idx, sen: sentence})
		}
	}
	if len(pairs) == 0 {
		return 1.0
	}
	texts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		texts = append(texts, p.sen)
	}
	vecs, err := em.Encode(ctx, texts)
	if err != nil {
		return 0
	}
	correct := 0
	for i, p := range pairs {
		if p.idx < 0 || p.idx >= len(contexts) {
			continue
		}
		cv, err := em.Encode(ctx, []string{contexts[p.idx]})
		if err != nil || len(cv) == 0 {
			continue
		}
		if embed.Cosine(vecs[i], cv[0]) >= relevanceThreshold {
			correct++
		}
	}
	return round4(float64(correct) / float64(len(pairs)))
}

// splitSentences 中英文句子切分（评估用轻量版）。
func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	for _, r := range text {
		cur.WriteRune(r)
		if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n' {
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

func maxCosine(v []float32, others [][]float32) float64 {
	max := 0.0
	for _, o := range others {
		if s := embed.Cosine(v, o); s > max {
			max = s
		}
	}
	return max
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func strPtr(s string) *string { return &s }

var _ = retrieval.Candidate{}
