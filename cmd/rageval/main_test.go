package main

import "testing"

func TestParseGeneratedCase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"纯JSON", `{"question":"图书馆几点开门？","groundTruth":"8 点。"}`, true},
		{"带围栏", "```json\n{\"question\":\"q\",\"groundTruth\":\"a\"}\n```", true},
		{"带噪声", "好的，以下是结果：\n{\"question\":\"q2\",\"groundTruth\":\"a2\"}\n希望有帮助", true},
		{"缺字段", `{"question":"q"}`, false},
		{"非JSON", `抱歉无法生成`, false},
		{"空白答案", `{"question":"q","groundTruth":"   "}`, false},
	}
	for _, c := range cases {
		got, err := parseGeneratedCase(c.in)
		if c.ok && err != nil {
			t.Fatalf("%s: 应解析成功: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("%s: 应解析失败，得到 %+v", c.name, got)
		}
	}
}

func TestThresholdFailures(t *testing.T) {
	min := Metrics{Faithfulness: 0.3, AnswerRelevancy: 0.25, ContextPrecision: 0.25, ContextRecall: 0.3, CitationAccuracy: 0.3}
	// 全部达标
	if f := thresholdFailures(Metrics{0.9, 0.8, 0.7, 0.6, 0.5}, min); len(f) != 0 {
		t.Fatalf("应全部达标: %v", f)
	}
	// 两项不达标
	f := thresholdFailures(Metrics{0.1, 0.9, 0.9, 0.2, 0.9}, min)
	if len(f) != 2 {
		t.Fatalf("应报告两项不达标: %v", f)
	}
}
