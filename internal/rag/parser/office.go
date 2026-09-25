package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- 通用 zip/xml 帮助 ----

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("缺少 %s", name)
}

// ---- DOCX ----

type docxParser struct{}

func (docxParser) Parse(_ string, data []byte) ([]Block, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("docx 解压失败: %w", err)
	}
	xmlData, err := readZipEntry(zr, "word/document.xml")
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(xmlData))

	var blocks []Block
	offset := 0
	var para strings.Builder
	style := ""
	inPara := false
	// 表格状态
	inTable := false
	var rows [][]string
	var row []string
	var cell strings.Builder

	flushPara := func() {
		text := strings.TrimSpace(para.String())
		para.Reset()
		if text == "" {
			style = ""
			return
		}
		level := headingLevel(style)
		if level > 0 {
			blocks = append(blocks, Block{Type: BlockTitle, Text: text, Level: level, Offset: offset})
		} else {
			blocks = append(blocks, Block{Type: BlockText, Text: text, Offset: offset})
		}
		offset += len(text)
		style = ""
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("docx XML 解析失败: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inPara = true
			case "tbl":
				inTable = true
				rows = nil
			case "tr":
				if inTable {
					row = nil
				}
			case "tc":
				if inTable {
					cell.Reset()
				}
			case "pStyle":
				if v := attrVal(t, "val"); v != "" {
					style = v
				}
			case "tab":
				para.WriteString(" ")
			case "br":
				para.WriteString("\n")
			}
		case xml.CharData:
			if inTable {
				if inPara {
					cell.Write(t)
				}
			} else if inPara {
				para.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				inPara = false
				if !inTable {
					flushPara()
				}
			case "tc":
				if inTable {
					row = append(row, strings.TrimSpace(cell.String()))
					cell.Reset()
				}
			case "tr":
				if inTable && len(row) > 0 {
					rows = append(rows, row)
				}
			case "tbl":
				inTable = false
				if table := rowsToMarkdown(rows); table != "" {
					blocks = append(blocks, Block{Type: BlockTable, Text: table, Offset: offset})
					offset += len(table)
				}
			}
		}
	}
	return blocks, nil
}

// headingLevel 从样式名推断标题层级（支持 "Heading 1" / "1" / "标题 1"）。
func headingLevel(style string) int {
	if style == "" {
		return 0
	}
	lower := strings.ToLower(style)
	if strings.Contains(lower, "title") || strings.Contains(style, "标题") {
		// 提取数字
		if n := firstDigit(style); n > 0 {
			return n
		}
		return 1
	}
	if strings.Contains(lower, "heading") {
		if n := firstDigit(style); n > 0 {
			return n
		}
		return 1
	}
	return 0
}

func firstDigit(s string) int {
	for _, r := range s {
		if r >= '1' && r <= '9' {
			return int(r - '0')
		}
	}
	return 0
}

func attrVal(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func rowsToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, "| "+strings.Join(rows[0], " | ")+" |")
	sep := "|"
	for range rows[0] {
		sep += " --- |"
	}
	lines = append(lines, sep)
	for _, row := range rows[1:] {
		lines = append(lines, "| "+strings.Join(row, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

// ---- XLSX ----

var sheetNameRE = regexp.MustCompile(`xl/worksheets/sheet(\d+)\.xml$`)

type xlsxParser struct{}

func (xlsxParser) Parse(_ string, data []byte) ([]Block, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsx 解压失败: %w", err)
	}
	// 共享字符串
	shared, err := parseSharedStrings(zr)
	if err != nil {
		return nil, err
	}
	// 收集 sheet 文件并按编号排序
	type sheetFile struct {
		idx  int
		name string
	}
	var sheets []sheetFile
	for _, f := range zr.File {
		if m := sheetNameRE.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, sheetFile{idx: n, name: f.Name})
		}
	}
	sort.Slice(sheets, func(i, j int) bool { return sheets[i].idx < sheets[j].idx })

	var blocks []Block
	offset := 0
	for _, sf := range sheets {
		xmlData, err := readZipEntry(zr, sf.name)
		if err != nil {
			continue
		}
		rows, err := parseSheet(xmlData, shared)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		title := fmt.Sprintf("Sheet%d", sf.idx)
		blocks = append(blocks, Block{Type: BlockTitle, Text: title, Level: 1, Offset: offset})
		offset += len(title)
		table := rowsToMarkdown(rows)
		blocks = append(blocks, Block{Type: BlockTable, Text: table, Offset: offset})
		offset += len(table)
	}
	return blocks, nil
}

func parseSharedStrings(zr *zip.Reader) ([]string, error) {
	xmlData, err := readZipEntry(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil, nil // 无共享字符串也算正常
	}
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var out []string
	var cur strings.Builder
	inSI, inT := false, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "si" {
				inSI = true
				cur.Reset()
			}
			if t.Name.Local == "t" {
				inT = true
			}
		case xml.CharData:
			if inSI && inT {
				cur.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inT = false
			}
			if t.Name.Local == "si" {
				out = append(out, cur.String())
				inSI = false
			}
		}
	}
	return out, nil
}

func parseSheet(xmlData []byte, shared []string) ([][]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var rows [][]string
	var row []string
	var curRef string
	var curType string
	var val strings.Builder
	inV, inIsT := false, false

	flushCell := func() {
		text := strings.TrimSpace(val.String())
		if curType == "s" {
			if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(shared) {
				text = shared[n]
			}
		}
		col := colIndex(curRef)
		for len(row) < col {
			row = append(row, "")
		}
		if len(row) == col {
			row = append(row, text)
		} else {
			row[col] = text
		}
		val.Reset()
		curType = ""
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = nil
			case "c":
				curRef = attrVal(t, "r")
				curType = attrVal(t, "t")
			case "v":
				inV = true
			case "t":
				if curType == "inlineStr" {
					inIsT = true
				}
			}
		case xml.CharData:
			if inV || inIsT {
				val.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
				if curType != "inlineStr" {
					flushCell()
				}
			case "t":
				if inIsT {
					inIsT = false
					flushCell()
				}
			case "row":
				if len(row) > 0 {
					rows = append(rows, row)
				}
			}
		}
	}
	return rows, nil
}

// colIndex 从 "B12" 解析列下标（A=0）。
func colIndex(ref string) int {
	col := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			col = col*26 + int(r-'A'+1)
		} else {
			break
		}
	}
	if col == 0 {
		return 0
	}
	return col - 1
}

// ---- PPTX ----

var slideNameRE = regexp.MustCompile(`ppt/slides/slide(\d+)\.xml$`)

type pptxParser struct{}

func (pptxParser) Parse(_ string, data []byte) ([]Block, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pptx 解压失败: %w", err)
	}
	type slideFile struct {
		idx  int
		name string
	}
	var slides []slideFile
	for _, f := range zr.File {
		if m := slideNameRE.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, slideFile{idx: n, name: f.Name})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].idx < slides[j].idx })

	var blocks []Block
	offset := 0
	for _, sf := range slides {
		xmlData, err := readZipEntry(zr, sf.name)
		if err != nil {
			continue
		}
		paras, err := parseSlideParagraphs(xmlData)
		if err != nil {
			return nil, err
		}
		for i, p := range paras {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if i == 0 {
				blocks = append(blocks, Block{Type: BlockTitle, Text: p, Level: 1, Page: sf.idx, Offset: offset})
			} else {
				blocks = append(blocks, Block{Type: BlockText, Text: p, Page: sf.idx, Offset: offset})
			}
			offset += len(p)
		}
	}
	return blocks, nil
}

func parseSlideParagraphs(xmlData []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var paras []string
	var cur strings.Builder
	inPara := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				inPara = true
				cur.Reset()
			}
		case xml.CharData:
			if inPara {
				cur.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				inPara = false
				paras = append(paras, cur.String())
			}
		}
	}
	return paras, nil
}
