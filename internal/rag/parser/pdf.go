package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dslipak/pdf"
)

type pdfParser struct{}

func (pdfParser) Parse(_ string, data []byte) ([]Block, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("PDF 打开失败: %w", err)
	}
	numPages := r.NumPage()
	var blocks []Block
	offset := 0
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("PDF 第 %d 页文本提取失败: %w", i, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		for _, para := range splitParagraphs(text, i) {
			para.Offset = offset
			offset += len(para.Text)
			blocks = append(blocks, para)
		}
	}
	return blocks, nil
}
