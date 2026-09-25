package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

// 手机号正则（对齐 RegexPatterns.PHONE_REGEX）
var phoneRegex = regexp.MustCompile(`^1([38][0-9]|4[579]|5[0-3,5-9]|6[6]|7[0135678]|9[89])\d{8}$`)

const randAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randDigits(n int) string {
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		b[i] = byte('0' + v.Int64())
	}
	return string(b)
}

func randString(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(randAlphabet)))
	for i := range b {
		v, _ := rand.Int(rand.Reader, max)
		b[i] = randAlphabet[v.Int64()]
	}
	return string(b)
}

// newToken 无连字符 UUID（32 位 hex，等价 hutool UUID.randomUUID().toString(true)）。
func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// SendCode 发送验证码（仅落 Redis 与日志，不真实发送）。
func (a *App) SendCode(ctx context.Context, phone string) dto.Result {
	if !phoneRegex.MatchString(phone) {
		return dto.Fail("手机号格式错误")
	}
	code := randDigits(6)
	if err := a.RDB.Set(ctx, rds.LoginCodeKey+phone, code, rds.LoginCodeTTL).Err(); err != nil {
		return dto.Fail("服务器异常")
	}
	a.Log.Info("短信验证码发送成功", "phone", phone, "code", code)
	return dto.Ok()
}

// Login 手机号+验证码登录。
func (a *App) Login(ctx context.Context, phone, code string) dto.Result {
	if !phoneRegex.MatchString(phone) {
		return dto.Fail("手机号格式错误")
	}
	if !a.LoginSkipCode {
		cached, err := a.RDB.Get(ctx, rds.LoginCodeKey+phone).Result()
		if err != nil || cached != code {
			return dto.Fail("验证码不一致，请重新输入")
		}
	}
	// 查用户，不存在则注册
	var user repo.User
	err := a.DB.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = repo.User{
			Phone:    &phone,
			NickName: strPtr(rds.NickNamePrefix + randString(10)),
			Icon:     strPtr(""),
		}
		if e := a.DB.WithContext(ctx).Select("phone", "nick_name", "icon").Create(&user).Error; e != nil {
			a.Log.Error("注册用户失败", "err", e)
			return dto.Fail("服务器异常")
		}
	} else if err != nil {
		a.Log.Error("查询用户失败", "err", err)
		return dto.Fail("服务器异常")
	}
	// 生成 token，写入 Redis Hash
	token := newToken()
	key := rds.LoginUserKey + token
	fields := map[string]any{"id": fmt.Sprintf("%d", *user.Id), "nickName": orEmpty(user.NickName), "icon": orEmpty(user.Icon)}
	pipe := a.RDB.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, rds.LoginUserTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		a.Log.Error("写登录态失败", "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.OkData(token)
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ = redis.Nil
var _ = time.Second
