package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func newTestRouter(t *testing.T) (*gin.Engine, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TokenRefresh(rdb))
	r.Use(LoginAuth())
	r.GET("/user/me", func(c *gin.Context) {
		u := CurrentUser(c)
		c.JSON(200, gin.H{"id": *u.Id, "nickName": *u.NickName})
	})
	r.GET("/blog/hot", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/shop/:id", func(c *gin.Context) { c.JSON(200, gin.H{"shop": c.Param("id")}) })
	return r, mr, rdb
}

func TestLoginAuth401(t *testing.T) {
	r, _, _ := newTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/user/me", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("未登录访问受保护接口 = %d, want 401", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("401 响应体应为空, got %q", w.Body.String())
	}
}

func TestWhitelistPass(t *testing.T) {
	r, _, _ := newTestRouter(t)
	for _, path := range []string{"/blog/hot", "/shop/1", "/user/login", "/user/code"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		if w.Code == 401 {
			t.Fatalf("白名单路径 %s 被拦截为 401", path)
		}
		// 未注册的路由为 404，已注册的为 200；两者都证明未被认证拦截
		if w.Code != 200 && w.Code != 404 {
			t.Fatalf("白名单路径 %s = %d", path, w.Code)
		}
	}
}

func TestTokenRefreshAndContext(t *testing.T) {
	r, mr, rdb := newTestRouter(t)
	// 写入 token hash
	key := "login:token:t123"
	mr.HSet(key, "id", "42", "nickName", "user_abc", "icon", "")
	mr.SetTTL(key, time.Minute)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/user/me", nil)
	req.Header.Set("authorization", "t123")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("携带 token 访问 = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != `{"id":42,"nickName":"user_abc"}` {
		t.Fatalf("上下文用户不对: %s", w.Body.String())
	}
	// TTL 被刷新为 30 分钟
	ttl := mr.TTL(key)
	if ttl > 30*time.Minute || ttl < 29*time.Minute {
		t.Fatalf("TTL 未刷新: %v", ttl)
	}
	_ = rdb
}

func TestInvalidTokenOnWhitelist(t *testing.T) {
	r, _, _ := newTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/blog/hot", nil)
	req.Header.Set("authorization", "ghost")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("白名单携带无效 token = %d, want 200", w.Code)
	}
}
