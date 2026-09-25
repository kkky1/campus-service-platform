package parser

import (
	"bytes"
	"encoding/csv"
	"strings"
)

// ---- 纯文本 ----

type textParser struct{}

func (textParser) Parse(_ string, data []byte) ([]Block, error) {
	text := strings.TrimPrefix(string(data), "\ufeff") // 去 BOM
	return splitParagraphs(text, 0), nil
}

// splitParagraphs 按空行拆段落。
func splitParagraphs(text string, page int) []Block {
	var blocks []Block
	offset := 0
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		trimmed := strings.TrimSpace(para)
		start := strings.Index(text[offset:], trimmed)
		if start >= 0 {
			start += offset
		} else {
			start = offset
		}
		if trimmed != "" {
			blocks = append(blocks, Block{Type: BlockText, Text: trimmed, Page: page, Offset: start})
		}
		offset += len(para) + 2
		if offset > len(text) {
			offset = len(text)
		}
	}
	return blocks
}

// ---- Markdown ----

type markdownParser struct{}

func (markdownParser) Parse(_ string, data []byte) ([]Block, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var blocks []Block
	var para []string
	var table []string
	offset := 0
	inCode := false

	flushPara := func() {
		if len(para) > 0 {
			blocks = append(blocks, Block{Type: BlockText, Text: strings.TrimSpace(strings.Join(para, "\n")), Offset: offset})
			para = nil
		}
	}
	flushTable := func() {
		if len(table) > 0 {
			blocks = append(blocks, Block{Type: BlockTable, Text: strings.Join(table, "\n"), Offset: offset})
			table = nil
		}
	}

	for _, line := range lines {
		lineOffset := offset
		offset += len(line) + 1
		trimmed := strings.TrimSpace(line)

		// 代码块：整体保留为文本
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			para = append(para, line)
			continue
		}
		if inCode {
			para = append(para, line)
			continue
		}
		// 标题
		if level, title := markdownHeading(trimmed); level > 0 {
			flushPara()
			flushTable()
			blocks = append(blocks, Block{Type: BlockTitle, Text: title, Level: level, Offset: lineOffset})
			continue
		}
		// 表格
		if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "|") {
			flushPara()
			table = append(table, trimmed)
			continue
		}
		flushTable()
		// 列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			flushPara()
			blocks = append(blocks, Block{Type: BlockList, Text: strings.TrimSpace(trimmed[2:]), Offset: lineOffset})
			continue
		}
		if trimmed == "" {
			flushPara()
			continue
		}
		para = append(para, line)
	}
	flushPara()
	flushTable()
	return blocks, nil
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) {
		return 0, ""
	}
	if line[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level:])
}

// ---- CSV ----

type csvParser struct{}

func (csvParser) Parse(_ string, data []byte) ([]Block, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	header := records[0]
	var lines []string
	lines = append(lines, "| "+strings.Join(header, " | ")+" |")
	sep := "|"
	for range header {
		sep += " --- |"
	}
	lines = append(lines, sep)
	for _, row := range records[1:] {
		lines = append(lines, "| "+strings.Join(row, " | ")+" |")
	}
	return []Block{{Type: BlockTable, Text: strings.Join(lines, "\n")}}, nil
}
