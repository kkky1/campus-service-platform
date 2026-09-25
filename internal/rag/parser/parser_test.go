package parser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestTextAndMarkdown(t *testing.T) {
	blocks, err := Parse("a.txt", []byte("第一段。\n\n第二段。"))
	if err != nil || len(blocks) != 2 {
		t.Fatalf("txt: %v %v", blocks, err)
	}
	if blocks[0].Text != "第一段。" || blocks[1].Text != "第二段。" {
		t.Fatalf("txt 内容不对: %+v", blocks)
	}
	md := "# 标题一\n\n正文段落。\n\n- 列表项\n\n## 小标题\n\n| 列A | 列B |\n| --- | --- |\n| 1 | 2 |\n"
	blocks, err = Parse("a.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}
	var types []BlockType
	for _, b := range blocks {
		types = append(types, b.Type)
	}
	want := []BlockType{BlockTitle, BlockText, BlockList, BlockTitle, BlockTable}
	if fmt.Sprint(types) != fmt.Sprint(want) {
		t.Fatalf("md 块类型 = %v, want %v", types, want)
	}
	if blocks[0].Level != 1 || blocks[3].Level != 2 {
		t.Fatalf("标题层级不对: %+v", blocks)
	}
}

func TestCSVAndJSON(t *testing.T) {
	csv := "name,score\n小明,95\n小红,88\n"
	blocks, err := Parse("a.csv", []byte(csv))
	if err != nil || len(blocks) != 1 || blocks[0].Type != BlockTable {
		t.Fatalf("csv: %v %v", blocks, err)
	}
	if !strings.Contains(blocks[0].Text, "| name | score |") || !strings.Contains(blocks[0].Text, "小明") {
		t.Fatalf("csv 内容不对: %s", blocks[0].Text)
	}
	js := `{"campus":{"name":"明德大学","services":["食堂","体育馆"]}}`
	blocks, err = Parse("a.json", []byte(js))
	if err != nil || len(blocks) != 1 {
		t.Fatalf("json: %v %v", blocks, err)
	}
	if !strings.Contains(blocks[0].Text, "campus.name: 明德大学") || !strings.Contains(blocks[0].Text, "campus.services[0]: 食堂") {
		t.Fatalf("json 展平不对: %s", blocks[0].Text)
	}
}

func TestHTML(t *testing.T) {
	doc := `<html><head><title>校园指南</title></head><body>
	<h1>服务介绍</h1><p>食堂开放时间为 7:00-21:00。</p>
	<table><tr><th>服务</th><th>位置</th></tr><tr><td>食堂</td><td>南区</td></tr></table>
	<ul><li>注意事项一</li></ul></body></html>`
	blocks, err := Parse("a.html", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var hasTitle, hasText, hasTable, hasList bool
	for _, b := range blocks {
		switch b.Type {
		case BlockTitle:
			hasTitle = true
		case BlockText:
			hasText = true
		case BlockTable:
			hasTable = true
		case BlockList:
			hasList = true
		}
	}
	if !hasTitle || !hasText || !hasTable || !hasList {
		t.Fatalf("html 解析缺块: %+v", blocks)
	}
}

// ---- office 测试文件构造 ----

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDOCX(t *testing.T) {
	doc := `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>校园服务手册</w:t></w:r></w:p>
<w:p><w:r><w:t>食堂位于南区，营业时间 7:00-21:00。</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>服务</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>价格</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>洗车</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>20元</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>注意事项</w:t></w:r></w:p>
</w:body></w:document>`
	data := buildZip(t, map[string]string{"word/document.xml": doc})
	blocks, err := Parse("a.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("docx 块数 = %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Type != BlockTitle || blocks[0].Level != 1 || blocks[0].Text != "校园服务手册" {
		t.Fatalf("docx 标题不对: %+v", blocks[0])
	}
	if blocks[1].Text != "食堂位于南区，营业时间 7:00-21:00。" {
		t.Fatalf("docx 正文不对: %+v", blocks[1])
	}
	if blocks[2].Type != BlockTable || !strings.Contains(blocks[2].Text, "| 服务 | 价格 |") {
		t.Fatalf("docx 表格不对: %+v", blocks[2])
	}
	if blocks[3].Level != 2 {
		t.Fatalf("docx 二级标题不对: %+v", blocks[3])
	}
}

func TestXLSX(t *testing.T) {
	shared := `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<si><t>服务</t></si><si><t>评分</t></si><si><t>食堂</t></si><si><t>体育馆</t></si></sst>`
	sheet := `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>4.8</v></c></row>
<row r="3"><c r="A3" t="s"><v>3</v></c><c r="B3"><v>4.5</v></c></row>
</sheetData></worksheet>`
	data := buildZip(t, map[string]string{
		"xl/sharedStrings.xml":     shared,
		"xl/worksheets/sheet1.xml": sheet,
	})
	blocks, err := Parse("a.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != BlockTitle {
		t.Fatalf("xlsx 块: %+v", blocks)
	}
	if !strings.Contains(blocks[1].Text, "| 服务 | 评分 |") || !strings.Contains(blocks[1].Text, "| 食堂 | 4.8 |") {
		t.Fatalf("xlsx 表格: %s", blocks[1].Text)
	}
}

func TestPPTX(t *testing.T) {
	slide := `<?xml version="1.0"?><p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:cSld><p:spTree><p:sp><p:txBody>
<a:p><a:r><a:t>校园活动安排</a:t></a:r></a:p>
<a:p><a:r><a:t>周末社团招新在体育馆</a:t></a:r></a:p>
</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
	data := buildZip(t, map[string]string{"ppt/slides/slide1.xml": slide})
	blocks, err := Parse("a.pptx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != BlockTitle || blocks[0].Text != "校园活动安排" {
		t.Fatalf("pptx: %+v", blocks)
	}
	if blocks[1].Page != 1 || !strings.Contains(blocks[1].Text, "体育馆") {
		t.Fatalf("pptx 正文: %+v", blocks[1])
	}
}

func TestPDF(t *testing.T) {
	data := minimalPDF("Hello RAG campus")
	blocks, err := Parse("a.pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 || !strings.Contains(blocks[0].Text, "Hello RAG campus") {
		t.Fatalf("pdf 提取失败: %+v", blocks)
	}
}

// minimalPDF 生成一个含单页文本的最小合法 PDF。
func minimalPDF(text string) []byte {
	stream := fmt.Sprintf("BT /F1 18 Tf 72 720 Td (%s) Tj ET", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buf.Bytes()
}

func TestUnsupported(t *testing.T) {
	if _, err := Parse("a.xyz", []byte("x")); err == nil {
		t.Fatal("未知扩展名应报错")
	}
	if IsSupported("a.xyz") || !IsSupported("a.pdf") {
		t.Fatal("IsSupported 判定错误")
	}
}
