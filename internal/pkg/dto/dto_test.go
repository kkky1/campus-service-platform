package dto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResultNullOmission(t *testing.T) {
	b, _ := json.Marshal(Ok())
	if string(b) != `{"success":true}` {
		t.Fatalf("Ok() = %s, want {\"success\":true}", b)
	}
	b, _ = json.Marshal(Fail("库存不足"))
	if string(b) != `{"success":false,"errorMsg":"库存不足"}` {
		t.Fatalf("Fail() = %s", b)
	}
	b, _ = json.Marshal(OkData(map[string]any{"a": 1}))
	if string(b) != `{"success":true,"data":{"a":1}}` {
		t.Fatalf("OkData() = %s", b)
	}
}

func TestResultZeroValuesKept(t *testing.T) {
	// data 为 0 值时不可被省略（Java 非 null 恒输出）
	b, _ := json.Marshal(OkData(0))
	if string(b) != `{"success":true,"data":0}` {
		t.Fatalf("data=0 被省略: %s", b)
	}
	b, _ = json.Marshal(OkData(false))
	if string(b) != `{"success":true,"data":false}` {
		t.Fatalf("data=false 被省略: %s", b)
	}
}

type sample struct {
	Liked  *int     `json:"liked,omitempty"`
	IsLike *bool    `json:"isLike,omitempty"`
	Icon   *string  `json:"icon,omitempty"`
	Dist   *float64 `json:"distance,omitempty"`
}

func TestPointerNullSemantics(t *testing.T) {
	zero, f, empty := 0, false, ""
	obj := sample{Liked: &zero, IsLike: &f, Icon: &empty}
	b, _ := json.Marshal(obj)
	if string(b) != `{"liked":0,"isLike":false,"icon":""}` {
		t.Fatalf("空串/零值序列化不对: %s", b)
	}
	var null sample
	b, _ = json.Marshal(null)
	if string(b) != `{}` {
		t.Fatalf("null 字段未省略: %s", b)
	}
}

func TestTimeTFormat(t *testing.T) {
	tm := time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local)
	b, _ := json.Marshal(TimeT{tm})
	if string(b) != `"2026-09-01T12:00:00"` {
		t.Fatalf("TimeT = %s", b)
	}
	b, _ = json.Marshal(TimeSpace{tm})
	if string(b) != `"2026-09-01 12:00:00"` {
		t.Fatalf("TimeSpace = %s", b)
	}
	b, _ = json.Marshal(DateOnly{tm})
	if string(b) != `"2026-09-01"` {
		t.Fatalf("DateOnly = %s", b)
	}
}

func TestTimeUnmarshalBothFormats(t *testing.T) {
	var a TimeT
	if err := json.Unmarshal([]byte(`"2026-09-01 12:00:00"`), &a); err != nil {
		t.Fatal(err)
	}
	if a.Format("2006-01-02T15:04:05") != "2026-09-01T12:00:00" {
		t.Fatalf("TimeT 解析 space 格式失败: %v", a.Time)
	}
	var b TimeSpace
	if err := json.Unmarshal([]byte(`"2026-09-01T12:00:00"`), &b); err != nil {
		t.Fatal(err)
	}
	if b.Format(layoutSpace) != "2026-09-01 12:00:00" {
		t.Fatalf("TimeSpace 解析 T 格式失败: %v", b.Time)
	}
}
