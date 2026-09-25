package parser

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type jsonParser struct{}

func (jsonParser) Parse(_ string, data []byte) ([]Block, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	var lines []string
	flattenJSON("", v, &lines)
	if len(lines) == 0 {
		return nil, nil
	}
	return []Block{{Type: BlockText, Text: strings.Join(lines, "\n")}}, nil
}

// flattenJSON 递归展平为 "path: value" 行；数组按下标展开。
func flattenJSON(path string, v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			flattenJSON(p, x[k], out)
		}
	case []any:
		for i, item := range x {
			flattenJSON(fmt.Sprintf("%s[%d]", path, i), item, out)
		}
	case string:
		if strings.TrimSpace(x) != "" {
			*out = append(*out, fmt.Sprintf("%s: %s", path, x))
		}
	case nil:
		// 跳过
	default:
		*out = append(*out, fmt.Sprintf("%s: %v", path, x))
	}
}
