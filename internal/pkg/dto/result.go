// Package dto 响应契约：Result/ScrollResult/UserDTO 与时间类型。
package dto

// Result 统一响应包，复刻 Java Result + Jackson non_null 语义：
// success 恒输出；errorMsg/data/total 为 null 时省略。
type Result struct {
	Success  bool    `json:"success"`
	ErrorMsg *string `json:"errorMsg,omitempty"`
	Data     any     `json:"data,omitempty"`
	Total    *int64  `json:"total,omitempty"`
}

// Ok 等价 Result.ok()：无数据时 data 省略。
func Ok() Result { return Result{Success: true} }

// OkData 等价 Result.ok(data)：data 为 nil 时省略。
func OkData(data any) Result {
	r := Result{Success: true}
	if data != nil {
		r.Data = data
	}
	return r
}

// Fail 等价 Result.fail(msg)。
func Fail(msg string) Result {
	return Result{Success: false, ErrorMsg: &msg}
}

// ScrollResult 关注 feed 滚动分页结果。
type ScrollResult struct {
	List    any    `json:"list,omitempty"`
	MinTime *int64 `json:"minTime,omitempty"`
	Offset  *int   `json:"offset,omitempty"`
}

// UserDTO 用户摘要（token Hash 字段与 JSON 同构）。
type UserDTO struct {
	Id       *int64  `json:"id,omitempty"`
	NickName *string `json:"nickName,omitempty"`
	Icon     *string `json:"icon,omitempty"`
}
