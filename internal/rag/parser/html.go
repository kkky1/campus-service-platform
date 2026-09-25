package parser

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

type htmlParser struct{}

func (htmlParser) Parse(_ string, data []byte) ([]Block, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var blocks []Block
	offset := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript":
				return
			case "title", "h1", "h2", "h3", "h4", "h5", "h6":
				text := strings.TrimSpace(nodeText(n))
				if text != "" {
					level := 1
					if n.Data != "title" {
						level = int(n.Data[1] - '0')
					}
					blocks = append(blocks, Block{Type: BlockTitle, Text: text, Level: level, Offset: offset})
					offset += len(text)
				}
				return
			case "table":
				table := renderHTMLTable(n)
				if table != "" {
					blocks = append(blocks, Block{Type: BlockTable, Text: table, Offset: offset})
					offset += len(table)
				}
				return
			case "p", "li", "dd", "dt", "figcaption", "blockquote":
				text := strings.TrimSpace(nodeText(n))
				if text != "" {
					bt := BlockText
					if n.Data == "li" {
						bt = BlockList
					}
					blocks = append(blocks, Block{Type: bt, Text: text, Offset: offset})
					offset += len(text)
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return blocks, nil
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// renderHTMLTable 把 <table> 渲染为 markdown 管道表格。
func renderHTMLTable(table *html.Node) string {
	var rows [][]string
	var collectRows func(n *html.Node)
	collectRows = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, strings.ReplaceAll(nodeText(c), "|", "\\|"))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collectRows(c)
		}
	}
	collectRows(table)
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
