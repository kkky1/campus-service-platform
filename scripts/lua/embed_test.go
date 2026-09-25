package lua

import (
	"strings"
	"testing"
)

// 断言嵌入的脚本已去除 XADD 行（Stream 写入为死代码，本变更清理）。
func TestSeckillScriptNoXADD(t *testing.T) {
	if SeckillScript == "" {
		t.Fatal("脚本未嵌入")
	}
	if strings.Contains(strings.ToLower(SeckillScript), "xadd") {
		t.Fatalf("脚本仍包含 XADD:\n%s", SeckillScript)
	}
	for _, want := range []string{"seckill:stock:", "seckill:order:", "sismember", "sadd", "incrby"} {
		if !strings.Contains(SeckillScript, want) {
			t.Errorf("脚本缺少关键片段 %q", want)
		}
	}
}
