// Package chunker 模板化切分与知识编译。
package chunker

import (
	"strings"
	"unicode"

	"campus-service-platform/internal/rag/parser"
)

// Tokenize 轻量分词：CJK 单字 + 二元组、ASCII 小写单词（用于 BM25 与标签）。
func Tokenize(text string) []string {
	runes := []rune(text)
	var tokens []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	for i, r := range runes {
		switch {
		case isCJK(r):
			flush()
			tokens = append(tokens, string(r))
			if i+1 < len(runes) && isCJK(runes[i+1]) {
				tokens = append(tokens, string(r)+string(runes[i+1]))
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// TokenCount 估算 token 数。
func TokenCount(text string) int { return len(Tokenize(text)) }

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF)
}

// Chunk 切片。
type Chunk struct {
	Index    int
	Text     string
	Tokens   int
	Headings []string
	Tags     []string
	Page     int
	Offset   int
}

// Config 切分配置。
type Config struct {
	TokenNum       int    // 每片目标 token 数
	OverlapPercent int    // 重叠百分比 0-30
	Delimiter      string // 段内追加分隔符（默认按 \n）
}

// DefaultConfig 默认配置。
func DefaultConfig() Config {
	return Config{TokenNum: 256, OverlapPercent: 10}
}

// Split 按 method 选择模板切分。支持：naive/paper/book/manual/laws/presentation/qa/table/resume/one。
func Split(method string, blocks []parser.Block, cfg Config) []Chunk {
	if cfg.TokenNum <= 0 {
		cfg.TokenNum = 256
	}
	if cfg.OverlapPercent < 0 {
		cfg.OverlapPercent = 0
	}
	if cfg.OverlapPercent > 30 {
		cfg.OverlapPercent = 30
	}
	switch method {
	case "", "naive":
		return chunkNaiveOpts(blocks, cfg, false)
	case "table":
		return chunkTableOnly(blocks, cfg)
	case "qa":
		return chunkQA(blocks, cfg)
	case "resume":
		// 简历：按列表项额外切分
		return chunkNaiveOpts(blocks, cfg, true)
	case "paper", "book", "manual", "laws", "presentation":
		// 论文/书籍/手册/法规/演示：naive 已按标题分节并维护完整标题路径
		return chunkNaiveOpts(blocks, cfg, false)
	case "one":
		return chunkOne(blocks)
	default:
		return chunkNaiveOpts(blocks, cfg, false)
	}
}

// sectionBuffer 维护标题层级栈。
type sectionBuffer struct {
	headings []string
}

// push 更新标题栈。
func (s *sectionBuffer) push(level int, title string) {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	if len(s.headings) >= level {
		s.headings = s.headings[:level-1]
	}
	for len(s.headings) < level-1 {
		s.headings = append(s.headings, "")
	}
	s.headings = append(s.headings, title)
}

func (s *sectionBuffer) path() []string {
	var out []string
	for _, h := range s.headings {
		if strings.TrimSpace(h) != "" {
			out = append(out, h)
		}
	}
	return out
}

// chunkNaive 段落级合并切分（保持块边界，超长块按分隔符/句子拆）。
func chunkNaive(blocks []parser.Block, cfg Config) []Chunk {
	return chunkNaiveOpts(blocks, cfg, false)
}

// chunkNaiveOpts 段落级合并切分；splitOnList=true 时列表项独立成片（简历模板）。
func chunkNaiveOpts(blocks []parser.Block, cfg Config, splitOnList bool) []Chunk {
	var chunks []Chunk
	sec := &sectionBuffer{}
	var cur strings.Builder
	curTokens := 0
	startPage, startOffset := 0, 0
	headingsAtStart := sec.path()

	flush := func() {
		text := strings.TrimSpace(cur.String())
		if text == "" {
			return
		}
		chunks = append(chunks, Chunk{
			Index:    len(chunks),
			Text:     text,
			Tokens:   TokenCount(text),
			Headings: headingsAtStart,
			Page:     startPage,
			Offset:   startOffset,
		})
		cur.Reset()
		curTokens = 0
	}

	for _, b := range blocks {
		if b.Type == parser.BlockTitle {
			// 标题变化：先冲刷当前片，再更新层级
			flush()
			sec.push(b.Level, b.Text)
			headingsAtStart = sec.path()
			startPage, startOffset = b.Page, b.Offset
			continue
		}
		if b.Type == parser.BlockImage {
			continue
		}
		// 简历：列表项独立成片
		if splitOnList && b.Type == parser.BlockList {
			flush()
			text := strings.TrimSpace(b.Text)
			if text != "" {
				chunks = append(chunks, Chunk{
					Index: len(chunks), Text: text, Tokens: TokenCount(text),
					Headings: sec.path(), Page: b.Page, Offset: b.Offset,
				})
			}
			headingsAtStart = sec.path()
			continue
		}
		if curTokens == 0 {
			headingsAtStart = sec.path()
			startPage, startOffset = b.Page, b.Offset
		}
		// 表格整块独立成片（超长再拆行）
		if b.Type == parser.BlockTable && TokenCount(b.Text) > cfg.TokenNum {
			flush()
			for _, piece := range splitTable(b.Text, cfg.TokenNum) {
				chunks = append(chunks, Chunk{
					Index: len(chunks), Text: piece, Tokens: TokenCount(piece),
					Headings: sec.path(), Page: b.Page, Offset: b.Offset,
				})
			}
			headingsAtStart = sec.path()
			continue
		}
		pieces := splitPiece(b.Text, cfg.TokenNum)
		for _, p := range pieces {
			pt := TokenCount(p)
			if curTokens > 0 && curTokens+pt > cfg.TokenNum {
				prev := strings.TrimSpace(cur.String())
				flush()
				// 重叠：保留上一片尾部
				if cfg.OverlapPercent > 0 {
					overlap := tailTokens(prev, cfg.TokenNum*cfg.OverlapPercent/100)
					if overlap != "" {
						cur.WriteString(overlap)
						curTokens = TokenCount(overlap)
					}
				}
				headingsAtStart = sec.path()
				startPage, startOffset = b.Page, b.Offset
			}
			if cur.Len() > 0 {
				cur.WriteString("\n")
				curTokens++
			}
			cur.WriteString(p)
			curTokens += pt
		}
	}
	flush()
	return reindex(chunks)
}

// chunkTableOnly 每张表独立成片，超长按行拆。
func chunkTableOnly(blocks []parser.Block, cfg Config) []Chunk {
	var chunks []Chunk
	for _, b := range blocks {
		if b.Type != parser.BlockTable {
			continue
		}
		pieces := splitTable(b.Text, cfg.TokenNum)
		for _, p := range pieces {
			chunks = append(chunks, Chunk{Index: len(chunks), Text: p, Tokens: TokenCount(p), Page: b.Page, Offset: b.Offset})
		}
	}
	return reindex(chunks)
}

// qaQuestionRE 判断是否问句开头。
func qaQuestion(text string) bool {
	t := strings.TrimSpace(text)
	for _, p := range []string{"问题", "问：", "问:", "Q:", "Q：", "Q.", "q:", "q：", "问 "} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func qaAnswer(text string) bool {
	t := strings.TrimSpace(text)
	for _, p := range []string{"回答", "答：", "答:", "A:", "A：", "答 "} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// chunkQA 按问答对切分；识别不到问答对时退化为 naive。
func chunkQA(blocks []parser.Block, cfg Config) []Chunk {
	var pairs []Chunk
	var q, a strings.Builder
	flushPair := func() {
		question := strings.TrimSpace(q.String())
		answer := strings.TrimSpace(a.String())
		if question == "" && answer == "" {
			return
		}
		text := question
		if answer != "" {
			text = question + "\n" + answer
		}
		pairs = append(pairs, Chunk{Index: len(pairs), Text: text, Tokens: TokenCount(text)})
		q.Reset()
		a.Reset()
	}
	matched := false
	for _, b := range blocks {
		if b.Type == parser.BlockTable {
			continue
		}
		if qaQuestion(b.Text) {
			flushPair()
			matched = true
			q.WriteString(b.Text)
		} else if qaAnswer(b.Text) {
			a.WriteString(b.Text)
		} else if matched {
			// 附加说明，归入当前答
			if a.Len() > 0 {
				a.WriteString("\n")
			}
			a.WriteString(b.Text)
		}
	}
	flushPair()
	if !matched || len(pairs) == 0 {
		return chunkNaive(blocks, cfg)
	}
	// 超长问答对再切
	var out []Chunk
	for _, p := range pairs {
		if p.Tokens <= cfg.TokenNum*2 {
			p.Index = len(out)
			out = append(out, p)
			continue
		}
		for i, piece := range splitPiece(p.Text, cfg.TokenNum) {
			out = append(out, Chunk{Index: len(out), Text: piece, Tokens: TokenCount(piece), Page: p.Page, Offset: p.Offset})
			_ = i
		}
	}
	return reindex(out)
}

// chunkOne 整篇一个切片（超长时按 token 上限切）。
func chunkOne(blocks []parser.Block) []Chunk {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == parser.BlockTitle {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
			continue
		}
		if b.Type == parser.BlockImage {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(b.Text)
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return nil
	}
	return []Chunk{{Index: 0, Text: text, Tokens: TokenCount(text)}}
}

// splitPiece 把超长文本按段落/句子拆到不超过 maxTokens。
func splitPiece(text string, maxTokens int) []string {
	if TokenCount(text) <= maxTokens {
		return []string{text}
	}
	var out []string
	var cur strings.Builder
	curTokens := 0
	for _, para := range strings.Split(text, "\n") {
		pt := TokenCount(para)
		if curTokens > 0 && curTokens+pt > maxTokens {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			curTokens = 0
		}
		if pt > maxTokens {
			// 单段超长：按句子拆
			if curTokens > 0 {
				out = append(out, strings.TrimSpace(cur.String()))
				cur.Reset()
				curTokens = 0
			}
			for _, sent := range splitSentences(para) {
				st := TokenCount(sent)
				if curTokens > 0 && curTokens+st > maxTokens {
					out = append(out, strings.TrimSpace(cur.String()))
					cur.Reset()
					curTokens = 0
				}
				if cur.Len() > 0 {
					cur.WriteString(" ")
				}
				cur.WriteString(sent)
				curTokens += st
			}
			continue
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(para)
		curTokens += pt
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// splitSentences 中英文句子切分。
func splitSentences(text string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range text {
		cur.WriteRune(r)
		if r == '。' || r == '！' || r == '？' || r == '；' || r == '.' || r == '!' || r == '?' || r == ';' {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// splitTable 表格超长时保留表头并按行拆。
func splitTable(markdown string, maxTokens int) []string {
	lines := strings.Split(markdown, "\n")
	if len(lines) < 3 {
		return splitPiece(markdown, maxTokens)
	}
	header := strings.Join(lines[:2], "\n")
	var out []string
	cur := header
	curTokens := TokenCount(header)
	for _, row := range lines[2:] {
		rt := TokenCount(row)
		if curTokens+rt > maxTokens && cur != header {
			out = append(out, cur)
			cur = header + "\n" + row
			curTokens = TokenCount(cur)
			continue
		}
		cur += "\n" + row
		curTokens += rt
	}
	out = append(out, cur)
	return out
}

// tailTokens 取尾部约 n 个 token 的文本（按 rune 近似）。
func tailTokens(text string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(text)
	// 近似：1 token ≈ 1 rune（CJK 场景）
	if len(runes) <= n {
		return ""
	}
	tail := string(runes[len(runes)-n:])
	if idx := strings.IndexByte(tail, '\n'); idx >= 0 && idx+1 < len(tail) {
		tail = tail[idx+1:]
	}
	return strings.TrimSpace(tail)
}

// reindex 重置 Index。
func reindex(chunks []Chunk) []Chunk {
	for i := range chunks {
		chunks[i].Index = i
	}
	return chunks
}
