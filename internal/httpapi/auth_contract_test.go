package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/repo"
	"campus-service-platform/internal/service"
)

// newAuthTestEnv 内存 Redis + 内存 SQLite（仅 tb_user）。
func newAuthTestEnv(t *testing.T) (*gin.Engine, *miniredis.Miniredis, *service.App) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&repo.User{}); err != nil {
		t.Fatal(err)
	}
	app := service.New(db, rdb, nil, nil, t.TempDir())
	gin.SetMode(gin.TestMode)
	return NewRouter(New(app)), mr, app
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m
}

func assertKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	got := map[string]bool{}
	for k := range m {
		got[k] = true
	}
	for _, k := range keys {
		if !got[k] {
			t.Fatalf("响应缺键 %q: %v", k, m)
		}
		delete(got, k)
	}
	if len(got) > 0 {
		t.Fatalf("响应多出键 %v: %v", got, m)
	}
}

func TestSendCodeContract(t *testing.T) {
	r, _, _ := newAuthTestEnv(t)
	_, m := doJSON(t, r, "POST", "/user/code?phone=123", "", nil)
	if m["success"] != false || m["errorMsg"] != "手机号格式错误" {
		t.Fatalf("非法手机号响应不对: %v", m)
	}
	assertKeys(t, m, "success", "errorMsg")

	_, m = doJSON(t, r, "POST", "/user/code?phone=13812345678", "", nil)
	if m["success"] != true {
		t.Fatalf("合法手机号响应不对: %v", m)
	}
	assertKeys(t, m, "success")
}

func TestLoginContract(t *testing.T) {
	r, mr, _ := newAuthTestEnv(t)
	// 无验证码
	_, m := doJSON(t, r, "POST", "/user/login", `{"phone":"13812345678","code":"000000"}`, nil)
	if m["errorMsg"] != "验证码不一致，请重新输入" {
		t.Fatalf("验证码错误响应不对: %v", m)
	}
	// 设置验证码后登录
	mr.Set("login:code:13812345678", "123456")
	_, m = doJSON(t, r, "POST", "/user/login", `{"phone":"13812345678","code":"123456"}`, nil)
	if m["success"] != true {
		t.Fatalf("登录失败: %v", m)
	}
	token, _ := m["data"].(string)
	if len(token) != 32 {
		t.Fatalf("token 应为 32 位 hex: %q", token)
	}
	assertKeys(t, m, "success", "data")
	// Redis hash 存在
	fields := map[string]string{}
	for _, f := range []string{"id", "nickName", "icon"} {
		v := mr.HGet("login:token:"+token, f)
		fields[f] = v
	}
	if fields["id"] == "" {
		t.Fatalf("token hash 不对: %v", fields)
	}
	if !strings.HasPrefix(fields["nickName"], "user_") {
		t.Fatalf("昵称前缀不对: %v", fields)
	}
	// 二次登录同一手机号 → 同一用户
	mr.Set("login:code:13812345678", "654321")
	_, m = doJSON(t, r, "POST", "/user/login", `{"phone":"13812345678","code":"654321"}`, nil)
	token2, _ := m["data"].(string)
	f2 := mr.HGet("login:token:"+token2, "id")
	if f2 != fields["id"] {
		t.Fatalf("二次登录应复用用户: %v vs %v", f2, fields["id"])
	}
}

func TestMeAndLogoutContract(t *testing.T) {
	r, _, _ := newAuthTestEnv(t)
	// /user/me 未登录 → 401
	code, _ := doJSON(t, r, "GET", "/user/me", "", nil)
	if code != 401 {
		t.Fatalf("/user/me 未登录 = %d, want 401", code)
	}
	// /user/logout 未登录 → 401（不在白名单）
	code, _ = doJSON(t, r, "POST", "/user/logout", "", nil)
	if code != 401 {
		t.Fatalf("/user/logout 未登录 = %d, want 401", code)
	}
}

func TestLogoutKeepsOriginalBehavior(t *testing.T) {
	r, mr, _ := newAuthTestEnv(t)
	mr.HSet("login:token:t", "id", "1", "nickName", "user_x", "icon", "")
	_, m := doJSON(t, r, "POST", "/user/logout", "", map[string]string{"authorization": "t"})
	if m["success"] != false || m["errorMsg"] != "功能未完成" {
		t.Fatalf("登出应保留原占位行为: %v", m)
	}
	assertKeys(t, m, "success", "errorMsg")
}

var _ = dto.Ok
