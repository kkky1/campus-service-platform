// Package parser 多文档类别解析：把各类文档统一解析为带结构的 Block 序列。
package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BlockType 块类型。
type BlockType string

const (
	BlockTitle BlockType = "title" // 标题（带层级）
	BlockText  BlockType = "text"  // 正文
	BlockTable BlockType = "table" // 表格（markdown 管道格式）
	BlockList  BlockType = "list"  // 列表
	BlockImage BlockType = "image" // 图片（占位提示，OCR 暂未实现）
)

// Block 解析结果的一个结构块。
type Block struct {
	Type   BlockType
	Text   string
	Level  int // 标题层级 1-6
	Page   int // 页码（PDF/PPTX）
	Offset int // 文档内字符偏移估算
}

// Parser 文档解析器。
type Parser interface {
	// Parse 解析文档为块序列。filename 用于格式提示。
	Parse(filename string, data []byte) ([]Block, error)
}

// Ext 返回小写扩展名（含点）。
func Ext(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}

// SupportedExts 支持的扩展名。
var SupportedExts = []string{
	".txt", ".text", ".md", ".markdown", ".csv", ".json",
	".html", ".htm", ".docx", ".xlsx", ".pptx", ".pdf",
}

// IsSupported 是否支持该扩展名。
func IsSupported(filename string) bool {
	ext := Ext(filename)
	for _, e := range SupportedExts {
		if e == ext {
			return true
		}
	}
	return false
}

// Parse 按扩展名分发解析。
func Parse(filename string, data []byte) ([]Block, error) {
	switch Ext(filename) {
	case ".txt", ".text":
		return textParser{}.Parse(filename, data)
	case ".md", ".markdown":
		return markdownParser{}.Parse(filename, data)
	case ".csv":
		return csvParser{}.Parse(filename, data)
	case ".json":
		return jsonParser{}.Parse(filename, data)
	case ".html", ".htm":
		return htmlParser{}.Parse(filename, data)
	case ".docx":
		return docxParser{}.Parse(filename, data)
	case ".xlsx":
		return xlsxParser{}.Parse(filename, data)
	case ".pptx":
		return pptxParser{}.Parse(filename, data)
	case ".pdf":
		return pdfParser{}.Parse(filename, data)
	default:
		return nil, fmt.Errorf("不支持的文档格式: %s", Ext(filename))
	}
}
