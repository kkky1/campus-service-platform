package service

import (
	"context"
	"strings"
	"testing"
)

// 临时开关打开时：任意验证码（含空）都可登录成功
func TestLoginSkipCodeAllowsAnyCode(t *testing.T) {
	app, _ := newAppEnv(t)
	app.LoginSkipCode = true
	ctx := context.Background()
	for _, code := range []string{"000000", "", "wrong"} {
		res := app.Login(ctx, "13800009999", code)
		if !res.Success {
			t.Fatalf("跳过校验时验证码 %q 应可登录: %v", code, res)
		}
		if tok, ok := res.Data.(string); !ok || len(tok) != 32 {
			t.Fatalf("应返回 token: %v", res.Data)
		}
	}
}

// 开关关闭时（默认）：仍严格校验验证码
func TestLoginRequiresCodeByDefault(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	res := app.Login(ctx, "13800008888", "123456")
	if res.Success || res.ErrorMsg == nil || !strings.Contains(*res.ErrorMsg, "验证码不一致") {
		t.Fatalf("默认应校验验证码: %v", res)
	}
}

// 跳过校验时手机号格式仍必须合法
func TestLoginSkipCodeStillValidatesPhone(t *testing.T) {
	app, _ := newAppEnv(t)
	app.LoginSkipCode = true
	res := app.Login(context.Background(), "123", "000000")
	if res.ErrorMsg == nil || *res.ErrorMsg != "手机号格式错误" {
		t.Fatalf("手机号校验不应跳过: %v", res)
	}
}
