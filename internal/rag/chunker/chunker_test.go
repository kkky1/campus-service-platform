package chunker

import (
	"fmt"
	"strings"
	"testing"

	"campus-service-platform/internal/rag/parser"
)

func blocksOf(items ...parser.Block) []parser.Block { return items }

func TestTokenize(t *testing.T) {
	toks := Tokenize("校园食堂Campus Food 2024")
	want := []string{"校", "校园", "园", "园食", "食", "食堂", "堂", "campus", "food", "2024"}
	if fmt.Sprint(toks) != fmt.Sprint(want) {
		t.Fatalf("分词 = %v, want %v", toks, want)
	}
	if TokenCount("校园食堂") != 7 {
		t.Fatalf("token 数 = %d, want 7", TokenCount("校园食堂"))
	}
}

func TestNaiveChunkingWithHeadingsAndOverlap(t *testing.T) {
	blocks := []parser.Block{
		{Type: parser.BlockTitle, Text: "校园服务手册", Level: 1},
		{Type: parser.BlockTitle, Text: "食堂", Level: 2},
		{Type: parser.BlockText, Text: "食堂位于南区，开放时间 7:00-21:00。"},
		{Type: parser.BlockText, Text: "支持校园卡与移动支付。"},
		{Type: parser.BlockTable, Text: "| 窗口 | 菜系 |\n| --- | --- |\n| 1 | 川菜 |\n| 2 | 粤菜 |"},
	}
	chunks := Split("naive", blocks, Config{TokenNum: 12, OverlapPercent: 20})
	if len(chunks) < 2 {
		t.Fatalf("切片数 = %d, want >=2: %+v", len(chunks), chunks)
	}
	// 标题路径进入元数据
	found := false
	for _, c := range chunks {
		if len(c.Headings) >= 2 && c.Headings[0] == "校园服务手册" && c.Headings[1] == "食堂" {
			found = true
		}
	}
	if !found {
		t.Fatalf("标题路径未记录: %+v", chunks[0].Headings)
	}
	// 表格完整保留
	tableFound := false
	for _, c := range chunks {
		if strings.Contains(c.Text, "| 窗口 | 菜系 |") && strings.Contains(c.Text, "| 1 | 川菜 |") {
			tableFound = true
		}
	}
	if !tableFound {
		t.Fatalf("表格未完整保留: %+v", chunks)
	}
}

func TestNaiveOverlap(t *testing.T) {
	long := strings.Repeat("校园食堂服务说明。", 40)
	blocks := []parser.Block{{Type: parser.BlockText, Text: long}}
	chunks := Split("naive", blocks, Config{TokenNum: 30, OverlapPercent: 30})
	if len(chunks) < 2 {
		t.Fatalf("应为多片: %d", len(chunks))
	}
	// 相邻片之间有重叠（前片尾与后片头有公共子串）
	prevTail := tailTokens(chunks[0].Text, 6)
	if prevTail == "" || !strings.Contains(chunks[1].Text, prevTail) {
		t.Fatalf("重叠缺失: prev tail=%q, next=%q", prevTail, chunks[1].Text[:min(40, len(chunks[1].Text))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestQAChunking(t *testing.T) {
	blocks := []parser.Block{
		{Type: parser.BlockText, Text: "问题：食堂几点开门？"},
		{Type: parser.BlockText, Text: "回答：早上 7 点开门。"},
		{Type: parser.BlockText, Text: "问题：体育馆怎么预约？"},
		{Type: parser.BlockText, Text: "回答：在校园服务平台预约。"},
	}
	chunks := Split("qa", blocks, DefaultConfig())
	if len(chunks) != 2 {
		t.Fatalf("问答对切片 = %d, want 2: %+v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0].Text, "食堂几点开门") || !strings.Contains(chunks[0].Text, "早上 7 点") {
		t.Fatalf("问答对内容不对: %s", chunks[0].Text)
	}
}

func TestQAFallbackToNaive(t *testing.T) {
	blocks := []parser.Block{{Type: parser.BlockText, Text: "普通说明文字。"}, {Type: parser.BlockText, Text: "没有问答结构。"}}
	chunks := Split("qa", blocks, Config{TokenNum: 8})
	if len(chunks) == 0 {
		t.Fatal("退化失败")
	}
}

func TestTableChunking(t *testing.T) {
	var rows []string
	rows = append(rows, "| 服务 | 价格 |", "| --- | --- |")
	for i := 1; i <= 20; i++ {
		rows = append(rows, fmt.Sprintf("| 服务%d | %d元 |", i, i*10))
	}
	blocks := []parser.Block{{Type: parser.BlockTable, Text: strings.Join(rows, "\n")}}
	chunks := Split("table", blocks, Config{TokenNum: 30})
	if len(chunks) < 2 {
		t.Fatalf("表格应拆分: %d", len(chunks))
	}
	for _, c := range chunks {
		if !strings.Contains(c.Text, "| 服务 | 价格 |") {
			t.Fatalf("拆片丢失表头: %s", c.Text)
		}
	}
}

func TestResumeSectionsAndBullets(t *testing.T) {
	blocks := []parser.Block{
		{Type: parser.BlockTitle, Text: "张三", Level: 1},
		{Type: parser.BlockTitle, Text: "教育经历", Level: 2},
		{Type: parser.BlockText, Text: "明德大学 计算机科学与技术 本科 2022-2026"},
		{Type: parser.BlockTitle, Text: "项目经历", Level: 2},
		{Type: parser.BlockList, Text: "校园服务平台：Go/MySQL/Redis 高并发抢券"},
		{Type: parser.BlockList, Text: "RAG 知识库：多文档解析与引用问答"},
	}
	chunks := Split("resume", blocks, Config{TokenNum: 64})
	// 教育经历正文 1 片 + 项目经历两条列表各 1 片
	if len(chunks) != 3 {
		t.Fatalf("简历切片 = %d, want 3: %+v", len(chunks), chunks)
	}
	if len(chunks[0].Headings) != 2 || chunks[0].Headings[0] != "张三" || chunks[0].Headings[1] != "教育经历" {
		t.Fatalf("教育经历标题路径: %+v", chunks[0].Headings)
	}
	if len(chunks[2].Headings) != 2 || chunks[2].Headings[1] != "项目经历" {
		t.Fatalf("项目经历标题路径: %+v", chunks[2].Headings)
	}
}

func TestOneChunking(t *testing.T) {
	blocks := []parser.Block{{Type: parser.BlockTitle, Text: "标题"}, {Type: parser.BlockText, Text: "正文一二三。"}}
	chunks := Split("one", blocks, DefaultConfig())
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "标题") {
		t.Fatalf("one 切分: %+v", chunks)
	}
}

func TestCompileKnowledge(t *testing.T) {
	blocks := []parser.Block{
		{Type: parser.BlockTitle, Text: "校园福利券使用说明", Level: 1},
		{Type: parser.BlockText, Text: "校园福利券可在食堂使用，福利券每天限领一次。学生认证后领取福利券。"},
	}
	chunks := Split("naive", blocks, DefaultConfig())
	k := CompileKnowledge(chunks, blocks)
	if len(k.Tags) == 0 {
		t.Fatal("标签为空")
	}
	if !strings.Contains(k.Summary, "校园福利券可在食堂使用") {
		t.Fatalf("摘要不对: %s", k.Summary)
	}
	// 停用词不得进入标签
	for _, tag := range k.Tags {
		if stopwords[tag] {
			t.Fatalf("标签含停用词: %v", k.Tags)
		}
	}
	ApplyTags(chunks, k.Tags, 4)
	if len(chunks[0].Tags) == 0 || len(chunks[0].Tags) > 4 {
		t.Fatalf("切片标签: %v", chunks[0].Tags)
	}
}
