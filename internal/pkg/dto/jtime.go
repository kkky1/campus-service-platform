package dto

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// 时间序列化约定：
//   - TimeT     等价 Jackson LocalDateTime：marshal "2006-01-02T15:04:05"
//   - TimeSpace 等价 hutool 缓存序列化：marshal "2006-01-02 15:04:05"（店铺缓存路径）
//   - DateOnly  等价 Jackson LocalDate：marshal "2006-01-02"
//
// Unmarshal 均兼容两种日期时间格式，保证能解析 Java 版写入 Redis 的缓存内容。

const (
	layoutT      = "2006-01-02T15:04:05"
	layoutSpace  = "2006-01-02 15:04:05"
	layoutDate   = "2006-01-02"
)

func parseFlex(s string) (time.Time, error) {
	for _, l := range []string{layoutT, layoutSpace, time.RFC3339} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
}

// TimeT Jackson 风格 LocalDateTime。
type TimeT struct{ time.Time }

func (t TimeT) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.Time.Format(layoutT) + `"`), nil
}

func (t *TimeT) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		return nil
	}
	s = s[1 : len(s)-1]
	parsed, err := parseFlex(s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t TimeT) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

func (t *TimeT) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		t.Time = x
	case []byte:
		parsed, err := parseFlex(string(x))
		if err != nil {
			return err
		}
		t.Time = parsed
	case string:
		parsed, err := parseFlex(x)
		if err != nil {
			return err
		}
		t.Time = parsed
	default:
		return fmt.Errorf("TimeT.Scan 不支持 %T", v)
	}
	return nil
}

// TimeSpace hutool 风格 LocalDateTime（店铺缓存路径）。
type TimeSpace struct{ time.Time }

func (t TimeSpace) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.Time.Format(layoutSpace) + `"`), nil
}

func (t *TimeSpace) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		return nil
	}
	s = s[1 : len(s)-1]
	parsed, err := parseFlex(s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t TimeSpace) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

func (t *TimeSpace) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		t.Time = x
	case []byte:
		parsed, err := parseFlex(string(x))
		if err != nil {
			return err
		}
		t.Time = parsed
	case string:
		parsed, err := parseFlex(x)
		if err != nil {
			return err
		}
		t.Time = parsed
	default:
		return fmt.Errorf("TimeSpace.Scan 不支持 %T", v)
	}
	return nil
}

// DateOnly LocalDate（生日）。
type DateOnly struct{ time.Time }

func (d DateOnly) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Time.Format(layoutDate) + `"`), nil
}

func (d *DateOnly) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		return nil
	}
	s = s[1 : len(s)-1]
	parsed, err := time.ParseInLocation(layoutDate, s, time.Local)
	if err != nil {
		return err
	}
	d.Time = parsed
	return nil
}

func (d DateOnly) Value() (driver.Value, error) {
	if d.Time.IsZero() {
		return nil, nil
	}
	return d.Time.Format(layoutDate), nil
}

func (d *DateOnly) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		d.Time = x
	case []byte:
		parsed, err := time.ParseInLocation(layoutDate, string(x), time.Local)
		if err != nil {
			return err
		}
		d.Time = parsed
	case string:
		parsed, err := time.ParseInLocation(layoutDate, x, time.Local)
		if err != nil {
			return err
		}
		d.Time = parsed
	default:
		return fmt.Errorf("DateOnly.Scan 不支持 %T", v)
	}
	return nil
}
