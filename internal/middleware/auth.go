// Package middleware 认证与异常处理中间件。
package middleware

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
)

const ctxUserKey = "currentUser"

// CurrentUser 取请求上下文中的当前用户（未登录为 nil）。
func CurrentUser(c *gin.Context) *dto.UserDTO {
	if v, ok := c.Get(ctxUserKey); ok {
		if u, ok := v.(*dto.UserDTO); ok {
			return u
		}
	}
	return nil
}

// Recovery 等价 WebExceptionAdvice：panic → 200 + {"success":false,"errorMsg":"服务器异常"}。
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				if log != nil {
					log.Error("panic", "err", err, "path", c.Request.URL.Path)
				}
				c.AbortWithStatusJSON(200, dto.Fail("服务器异常"))
			}
		}()
		c.Next()
	}
}

// TokenRefresh 等价 RefreshTokenInterceptor：
// authorization 头 → login:token:{token} Hash → 上下文 + EXPIRE 30min。
func TokenRefresh(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("authorization")
		if strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		key := rds.LoginUserKey + token
		m, err := rdb.HGetAll(c.Request.Context(), key).Result()
		if err != nil || len(m) == 0 {
			c.Next()
			return
		}
		u := &dto.UserDTO{}
		if v, ok := m["id"]; ok {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				u.Id = &id
			}
		}
		if v, ok := m["nickName"]; ok {
			u.NickName = &v
		}
		if v, ok := m["icon"]; ok {
			u.Icon = &v
		}
		c.Set(ctxUserKey, u)
		rdb.Expire(c.Request.Context(), key, rds.LoginUserTTL)
		c.Next()
	}
}

// 免登录白名单（对齐 MvcConfig.excludePathPatterns）。
var whitelistExact = map[string]bool{
	"/user/login": true,
	"/user/code":  true,
	"/blog/hot":   true,
}

var whitelistPrefix = []string{
	"/upload/", "/voucher/", "/shop/", "/shop-type/",
}

func inWhitelist(path string) bool {
	if whitelistExact[path] {
		return true
	}
	for _, p := range whitelistPrefix {
		// Spring 的 /** 可匹配零段：/voucher/** 同时覆盖 /voucher 本身
		if strings.HasPrefix(path, p) || path == strings.TrimSuffix(p, "/") {
			return true
		}
	}
	return false
}

// LoginAuth 等价 LoginInterceptor：非白名单且无用户 → 401 空响应体。
func LoginAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if inWhitelist(c.Request.URL.Path) {
			c.Next()
			return
		}
		if CurrentUser(c) == nil {
			c.AbortWithStatus(401)
			return
		}
		c.Next()
	}
}
