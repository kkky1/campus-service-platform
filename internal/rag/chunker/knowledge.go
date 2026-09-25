package chunker

import (
	"sort"
	"strings"

	"campus-service-platform/internal/rag/parser"
)

// 停用词（标签提取时排除）。
var stopwords = map[string]bool{
	"的": true, "了": true, "是": true, "在": true, "和": true, "与": true, "有": true, "为": true,
	"对": true, "等": true, "中": true, "上": true, "下": true, "可": true, "以": true, "及": true,
	"或": true, "一个": true, "一些": true, "这个": true, "这些": true, "我们": true, "他们": true,
	"the": true, "a": true, "an": true, "of": true, "to": true, "and": true, "or": true, "in": true,
	"on": true, "for": true, "is": true, "are": true, "was": true, "were": true, "be": true,
	"it": true, "this": true, "that": true, "with": true, "as": true, "at": true, "by": true,
}

// Knowledge 知识编译结果（文档级）。
type Knowledge struct {
	Tags    []string // 文档标签（高频非停用词，最多 8 个）
	Summary string   // 摘要（正文前 300 字）
}

// CompileKnowledge 从切片与块中编译文档级知识：标签 + 摘要。
func CompileKnowledge(chunks []Chunk, blocks []parser.Block) Knowledge {
	freq := make(map[string]int)
	for _, c := range chunks {
		for _, tok := range Tokenize(c.Text) {
			if skipForTag(tok) {
				continue
			}
			freq[tok]++
		}
	}
	type kv struct {
		term string
		n    int
	}
	var pairs []kv
	for t, n := range freq {
		pairs = append(pairs, kv{t, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].term < pairs[j].term
	})
	var tags []string
	for _, p := range pairs {
		if len(tags) >= 8 {
			break
		}
		tags = append(tags, p.term)
	}

	// 摘要：正文前 300 字
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == parser.BlockTitle || b.Type == parser.BlockImage {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(b.Text)
		if len([]rune(sb.String())) >= 300 {
			break
		}
	}
	summary := string([]rune(strings.TrimSpace(sb.String())))
	if r := []rune(summary); len(r) > 300 {
		summary = string(r[:300])
	}
	return Knowledge{Tags: tags, Summary: summary}
}

// skipForTag 标签候选过滤：停用词、单字符、纯数字。
func skipForTag(tok string) bool {
	if stopwords[tok] {
		return true
	}
	runes := []rune(tok)
	if len(runes) < 1 {
		return true
	}
	if len(runes) == 1 {
		// 单个 CJK 字信息量低
		return true
	}
	allDigit := true
	for _, r := range runes {
		if r < '0' || r > '9' {
			allDigit = false
			break
		}
	}
	return allDigit
}

// ApplyTags 给每个切片打上文档标签（切片自身高频词优先，再补文档标签）。
func ApplyTags(chunks []Chunk, docTags []string, maxPerChunk int) {
	if maxPerChunk <= 0 {
		maxPerChunk = 4
	}
	for i := range chunks {
		freq := make(map[string]int)
		for _, tok := range Tokenize(chunks[i].Text) {
			if !skipForTag(tok) {
				freq[tok]++
			}
		}
		type kv struct {
			term string
			n    int
		}
		var pairs []kv
		for t, n := range freq {
			pairs = append(pairs, kv{t, n})
		}
		sort.Slice(pairs, func(a, b int) bool {
			if pairs[a].n != pairs[b].n {
				return pairs[a].n > pairs[b].n
			}
			return pairs[a].term < pairs[b].term
		})
		seen := map[string]bool{}
		var tags []string
		for _, p := range pairs {
			if len(tags) >= maxPerChunk {
				break
			}
			tags = append(tags, p.term)
			seen[p.term] = true
		}
		for _, t := range docTags {
			if len(tags) >= maxPerChunk {
				break
			}
			if !seen[t] {
				tags = append(tags, t)
				seen[t] = true
			}
		}
		chunks[i].Tags = tags
	}
}
