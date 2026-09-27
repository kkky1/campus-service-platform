// rageval CD 自动 RAG 评估：
//
//  1. 从知识库随机抽取资料片段
//  2. 由 LLM 基于片段生成「问题 + 标准答案」（随机题型）
//  3. 调 /rag/eval/run（自动检索+生成）计算 RAGAS 指标
//  4. 打印报告并写文件；低于阈值时退出码非 0（可用 RAG_EVAL_ENFORCE=false 关闭）
//
// 环境变量：
//
//	RAG_BASE_URL           后端地址（默认 http://127.0.0.1:8081）
//	RAG_EVAL_PHONE         登录手机号（默认 13800000001）
//	RAG_EVAL_KB_ID         知识库 ID（默认 10；不存在则自动取第一个有文档的库）
//	RAG_EVAL_COUNT         生成样本数（默认 5）
//	RAG_EVAL_ENFORCE       true/false 是否按阈值让构建失败（默认 true）
//	RAG_EVAL_MIN_*         各指标最低阈值
//	RAG_LLM_BASE_URL / RAG_LLM_API_KEY / RAG_LLM_MODEL  大模型（生成数据用）
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/llm"
)

// ---------- API 结构 ----------

type resultEnvelope struct {
	Success  bool            `json:"success"`
	ErrorMsg string          `json:"errorMsg"`
	Data     json.RawMessage `json:"data"`
}

type sessionLoginResp struct {
	Success bool   `json:"success"`
	Data    string `json:"data"`
}

type docItem struct {
	Id     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type chunkItem struct {
	Text string `json:"text"`
}

type evalCaseIn struct {
	Question    string `json:"question"`
	GroundTruth string `json:"groundTruth"`
}

type generatedCase struct {
	Question    string `json:"question"`
	GroundTruth string `json:"groundTruth"`
}

type evalRunResp struct {
	RunId     int64   `json:"runId"`
	CaseCount int     `json:"caseCount"`
	Metrics   Metrics `json:"metrics"`
}

// Metrics RAGAS 指标（与后端 eval.Metrics 对应）。
type Metrics struct {
	Faithfulness     float64 `json:"faithfulness"`
	AnswerRelevancy  float64 `json:"answer_relevancy"`
	ContextPrecision float64 `json:"context_precision"`
	ContextRecall    float64 `json:"context_recall"`
	CitationAccuracy float64 `json:"citation_accuracy"`
}

type evalDetailResp struct {
	RunId   int64   `json:"runId"`
	Name    string  `json:"name"`
	Metrics Metrics `json:"metrics"`
	Cases   []struct {
		Question string  `json:"question"`
		Metrics  Metrics `json:"metrics"`
	} `json:"cases"`
}

// ---------- 纯函数（可测试） ----------

// parseGeneratedCase 解析 LLM 输出（容忍 ```json 围栏与前后噪声）。
func parseGeneratedCase(out string) (generatedCase, error) {
	var c generatedCase
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return c, err
	}
	c.Question = strings.TrimSpace(c.Question)
	c.GroundTruth = strings.TrimSpace(c.GroundTruth)
	if c.Question == "" || c.GroundTruth == "" {
		return c, fmt.Errorf("生成的样本不完整")
	}
	return c, nil
}

// thresholdFailures 返回低于阈值的指标列表。
func thresholdFailures(m Metrics, min Metrics) []string {
	var out []string
	add := func(name string, v, min float64) {
		if v < min {
			out = append(out, fmt.Sprintf("%s=%.3f < %.2f", name, v, min))
		}
	}
	add("faithfulness", m.Faithfulness, min.Faithfulness)
	add("answer_relevancy", m.AnswerRelevancy, min.AnswerRelevancy)
	add("context_precision", m.ContextPrecision, min.ContextPrecision)
	add("context_recall", m.ContextRecall, min.ContextRecall)
	add("citation_accuracy", m.CitationAccuracy, min.CitationAccuracy)
	return out
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ---------- HTTP 帮助 ----------

type client struct {
	base  string
	token string
	http  *http.Client
}

func (c *client) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *client) login(phone string) error {
	var r sessionLoginResp
	if err := c.do("POST", "/user/login", map[string]string{"phone": phone}, &r); err != nil {
		return err
	}
	if !r.Success || r.Data == "" {
		return fmt.Errorf("登录失败（手机号 %s）", phone)
	}
	c.token = r.Data
	return nil
}

func (c *client) listDocs(kbId int64) ([]docItem, error) {
	var r resultEnvelope
	if err := c.do("GET", fmt.Sprintf("/rag/doc/list?kbId=%d", kbId), nil, &r); err != nil {
		return nil, err
	}
	var docs []docItem
	if err := json.Unmarshal(r.Data, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (c *client) listKBs() ([]struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}, error) {
	var r resultEnvelope
	if err := c.do("GET", "/rag/kb/list", nil, &r); err != nil {
		return nil, err
	}
	var kbs []struct {
		Id   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(r.Data, &kbs); err != nil {
		return nil, err
	}
	return kbs, nil
}

func (c *client) docChunks(docId int64) ([]chunkItem, error) {
	var r resultEnvelope
	if err := c.do("GET", fmt.Sprintf("/rag/doc/%d/chunks", docId), nil, &r); err != nil {
		return nil, err
	}
	var chunks []chunkItem
	if err := json.Unmarshal(r.Data, &chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (c *client) runEval(kbId int64, name string, cases []evalCaseIn) (evalRunResp, error) {
	var r resultEnvelope
	err := c.do("POST", "/rag/eval/run", map[string]any{
		"kbId": kbId, "name": name, "usePipeline": true, "cases": cases,
	}, &r)
	if err != nil {
		return evalRunResp{}, err
	}
	if !r.Success {
		return evalRunResp{}, fmt.Errorf("评估失败: %s", r.ErrorMsg)
	}
	var out evalRunResp
	if err := json.Unmarshal(r.Data, &out); err != nil {
		return evalRunResp{}, err
	}
	return out, nil
}

func (c *client) evalDetail(runId int64) (evalDetailResp, error) {
	var r resultEnvelope
	if err := c.do("GET", fmt.Sprintf("/rag/eval/%d", runId), nil, &r); err != nil {
		return evalDetailResp{}, err
	}
	var out evalDetailResp
	if err := json.Unmarshal(r.Data, &out); err != nil {
		return evalDetailResp{}, err
	}
	return out, nil
}

// ---------- 主流程 ----------

func main() {
	rand.Seed(time.Now().UnixNano())
	logf := func(format string, args ...any) { fmt.Printf(format+"\n", args...) }

	base := env("RAG_BASE_URL", "http://127.0.0.1:8081")
	phone := env("RAG_EVAL_PHONE", "13800000001")
	kbId := int64(envInt("RAG_EVAL_KB_ID", 10))
	count := envInt("RAG_EVAL_COUNT", 5)
	enforce := env("RAG_EVAL_ENFORCE", "true") == "true"
	minMetrics := Metrics{
		Faithfulness:     envFloat("RAG_EVAL_MIN_FAITHFULNESS", 0.3),
		AnswerRelevancy:  envFloat("RAG_EVAL_MIN_RELEVANCY", 0.25),
		ContextPrecision: envFloat("RAG_EVAL_MIN_PRECISION", 0.25),
		ContextRecall:    envFloat("RAG_EVAL_MIN_RECALL", 0.3),
		CitationAccuracy: envFloat("RAG_EVAL_MIN_CITATION", 0.3),
	}

	c := &client{base: base, http: &http.Client{Timeout: 180 * time.Second}}
	if err := c.login(phone); err != nil {
		logf("❌ %v", err)
		os.Exit(1)
	}

	// 知识库：指定 ID 不存在时回退到第一个有文档的
	if kbId != 0 {
		docs, err := c.listDocs(kbId)
		if err != nil || len(docs) == 0 {
			logf("⚠️ 知识库 %d 无文档，尝试自动选择", kbId)
			kbId = 0
		}
	}
	if kbId == 0 {
		kbs, _ := c.listKBs()
		for _, kb := range kbs {
			if docs, err := c.listDocs(kb.Id); err == nil && len(docs) > 0 {
				kbId = kb.Id
				logf("ℹ️ 自动选择知识库 %d（%s）", kb.Id, kb.Name)
				break
			}
		}
	}
	if kbId == 0 {
		logf("⚠️ 没有可用的知识库，跳过 RAG 评估（首次部署属正常）")
		os.Exit(0)
	}

	// 1) 随机采样资料片段
	docs, _ := c.listDocs(kbId)
	rand.Shuffle(len(docs), func(i, j int) { docs[i], docs[j] = docs[j], docs[i] })
	var pool []string
	for _, d := range docs {
		if d.Status != "done" {
			continue
		}
		chunks, err := c.docChunks(d.Id)
		if err != nil {
			continue
		}
		for _, ch := range chunks {
			t := strings.TrimSpace(ch.Text)
			if len([]rune(t)) >= 60 {
				pool = append(pool, t)
			}
		}
	}
	if len(pool) < 3 {
		logf("⚠️ 可用资料片段不足（%d），跳过评估", len(pool))
		os.Exit(0)
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if count > len(pool) {
		count = len(pool)
	}
	samples := pool[:count]
	logf("📚 知识库 %d：随机抽取 %d 个资料片段，准备 AI 生成评测样本", kbId, len(samples))

	// 2) AI 生成「问题 + 标准答案」
	llmCfg := config.RAGConfig{}
	llmCfg.LLM.BaseURL = env("RAG_LLM_BASE_URL", "https://api.deepseek.com")
	llmCfg.LLM.APIKey = env("RAG_LLM_API_KEY", "")
	llmCfg.LLM.Model = env("RAG_LLM_MODEL", "deepseek-chat")
	llmCfg.LLM.Temperature = 0.8
	llmCfg.LLM.MaxTokens = 512
	genClient := llm.New(llmCfg)
	if llmCfg.LLM.APIKey == "" {
		logf("⚠️ 未配置 RAG_LLM_API_KEY，无法生成评测数据")
		os.Exit(0)
	}
	styles := []string{"事实型（问具体信息）", "总结型（概括要点）", "列举型（列出若干项）"}
	cases := make([]evalCaseIn, 0, len(samples))
	for i, chunk := range samples {
		style := styles[i%len(styles)]
		prompt := fmt.Sprintf("你是 RAG 评测数据集生成器。请基于下面这段资料，生成 1 个用户可能会问的问题和标准答案。\n"+
			"要求：\n- 题型：%s\n- 问题必须能由资料回答，不要问资料外内容\n"+
			"- 标准答案 1-3 句，只依据资料，不要编造\n- 只输出 JSON，格式：{\"question\":\"...\",\"groundTruth\":\"...\"}\n\n资料：\n%s",
			style, truncateRunes(chunk, 800))
		out, err := genClient.Chat(context.Background(), "", prompt)
		if err != nil {
			logf("  ⚠️ 第 %d 条生成失败：%v", i+1, err)
			continue
		}
		gc, err := parseGeneratedCase(out)
		if err != nil {
			logf("  ⚠️ 第 %d 条解析失败：%v", i+1, err)
			continue
		}
		cases = append(cases, evalCaseIn(gc))
		logf("  ✓ [%s] %s", style[:3], gc.Question)
	}
	if len(cases) == 0 {
		logf("⚠️ 未能生成任何评测样本，跳过")
		os.Exit(0)
	}

	// 3) 跑评估（自动检索+生成 → RAGAS 指标）
	runName := "cd-auto-" + time.Now().Format("20060102-150405")
	run, err := c.runEval(kbId, runName, cases)
	if err != nil {
		logf("❌ %v", err)
		os.Exit(1)
	}
	detail, _ := c.evalDetail(run.RunId)

	// 4) 报告
	report := buildReport(runName, kbId, detail)
	logf("\n%s", report)
	reportPath := env("RAG_EVAL_REPORT", "reports/rag-eval.md")
	if i := strings.LastIndex(reportPath, "/"); i > 0 {
		_ = os.MkdirAll(reportPath[:i], 0o755)
	}
	_ = os.WriteFile(reportPath, []byte(report), 0o644)
	logf("📄 报告已写入 %s", reportPath)

	fails := thresholdFailures(run.Metrics, minMetrics)
	if len(fails) > 0 {
		logf("\n⚠️ 指标低于阈值：%s", strings.Join(fails, ", "))
		if enforce {
			logf("❌ RAG 评估未通过（RAG_EVAL_ENFORCE=true）")
			os.Exit(1)
		}
		logf("ℹ️ 仅告警（RAG_EVAL_ENFORCE=false）")
	} else {
		logf("\n✅ RAG 评估通过")
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func buildReport(runName string, kbId int64, d evalDetailResp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## RAG 自动评估报告（AI 生成数据）\n\n")
	fmt.Fprintf(&b, "- 运行: `%s`（知识库 %d，evalRunId=%d）\n", runName, kbId, d.RunId)
	fmt.Fprintf(&b, "- 指标: faithfulness=%.3f relevance=%.3f precision=%.3f recall=%.3f citation=%.3f\n\n",
		d.Metrics.Faithfulness, d.Metrics.AnswerRelevancy, d.Metrics.ContextPrecision, d.Metrics.ContextRecall, d.Metrics.CitationAccuracy)
	b.WriteString("| # | 问题 | 忠实度 | 相关性 | 精度 | 召回 | 引用准确率 |\n|---|---|---|---|---|---|---|\n")
	for i, c := range d.Cases {
		fmt.Fprintf(&b, "| %d | %s | %.2f | %.2f | %.2f | %.2f | %.2f |\n", i+1,
			escapePipes(c.Question), c.Metrics.Faithfulness, c.Metrics.AnswerRelevancy,
			c.Metrics.ContextPrecision, c.Metrics.ContextRecall, c.Metrics.CitationAccuracy)
	}
	return b.String()
}

func escapePipes(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 40 {
		return string([]rune(s)[:40]) + "…"
	}
	return s
}
