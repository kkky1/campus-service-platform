package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

// QueryUserInfo 用户资料（createTime/updateTime 置空省略；不存在返回空 data）。
func (a *App) QueryUserInfo(ctx context.Context, userId int64) dto.Result {
	var info repo.UserInfo
	err := a.DB.WithContext(ctx).Where("user_id = ?", userId).First(&info).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.Ok()
	}
	if err != nil {
		return dto.Fail("服务器异常")
	}
	info.CreateTime = nil
	info.UpdateTime = nil
	return dto.OkData(info)
}

// QueryUserById 用户摘要 {id, nickName, icon}。
func (a *App) QueryUserById(ctx context.Context, userId int64) dto.Result {
	var user repo.User
	err := a.DB.WithContext(ctx).Where("id = ?", userId).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.Ok()
	}
	if err != nil {
		return dto.Fail("服务器异常")
	}
	return dto.OkData(userToDTO(&user))
}

func userToDTO(u *repo.User) dto.UserDTO {
	d := dto.UserDTO{Id: u.Id}
	if u.NickName != nil {
		d.NickName = u.NickName
	}
	if u.Icon != nil {
		d.Icon = u.Icon
	}
	return d
}

// Sign 每日签到：SETBIT sign:{userId}:{yyyyMM} (dayOfMonth-1) 1。
func (a *App) Sign(ctx context.Context, userId int64) dto.Result {
	now := time.Now()
	key := fmt.Sprintf("%s%d:%s", rds.UserSignKey, userId, now.Format("200601"))
	if err := a.RDB.SetBit(ctx, key, int64(now.Day()-1), 1).Err(); err != nil {
		return dto.Fail("服务器异常")
	}
	return dto.Ok()
}

// SignCount 连续签到天数：BITFIELD GET u{dayOfMonth} 0，统计低位连续 1。
func (a *App) SignCount(ctx context.Context, userId int64) dto.Result {
	now := time.Now()
	key := fmt.Sprintf("%s%d:%s", rds.UserSignKey, userId, now.Format("200601"))
	vals, err := a.RDB.BitField(ctx, key, "GET", fmt.Sprintf("u%d", now.Day()), 0).Result()
	if err != nil {
		return dto.Fail("服务器异常")
	}
	if len(vals) == 0 {
		return dto.OkData(0)
	}
	num := vals[0]
	count := 0
	for num&1 == 1 {
		count++
		num >>= 1
	}
	return dto.OkData(count)
}

// javaHashCode 复刻 Java String.hashCode（int32 溢出语义）。
func javaHashCode(s string) int32 {
	var h int32
	for i := 0; i < len(s); i++ {
		h = 31*h + int32(s[i])
	}
	return h
}

// UploadImage 保存上传图片，返回 /blogs/{d1}/{d2}/{uuid}.{ext}。
func (a *App) UploadImage(ctx context.Context, originalFilename string, data []byte) dto.Result {
	suffix := ""
	if i := strings.LastIndexByte(originalFilename, '.'); i >= 0 {
		suffix = originalFilename[i+1:]
	}
	uuid := newUUID()
	h := javaHashCode(uuid)
	d1 := int(h) & 0xF
	d2 := (int(h) >> 4) & 0xF
	relDir := filepath.Join("blogs", fmt.Sprint(d1), fmt.Sprint(d2))
	dir := filepath.Join(a.UploadDir, relDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dto.Fail("服务器异常")
	}
	name := uuid
	if suffix != "" {
		name = uuid + "." + suffix
	} else {
		name = uuid + "."
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		a.Log.Error("保存文件失败", "err", err)
		return dto.Fail("服务器异常")
	}
	rel := "/blogs/" + fmt.Sprint(d1) + "/" + fmt.Sprint(d2) + "/" + name
	return dto.OkData(rel)
}

// DeleteImage 删除上传图片；目录路径拒绝；越界路径（路径穿越）拒绝。
func (a *App) DeleteImage(ctx context.Context, name string) dto.Result {
	// 规范化并锚定在上传目录内：/../x、绝对路径等一律收敛到目录内
	clean := filepath.Clean("/" + filepath.ToSlash(name))
	full := filepath.Join(a.UploadDir, filepath.FromSlash(clean))
	base := filepath.Clean(a.UploadDir)
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return dto.Fail("错误的文件名称")
	}
	st, err := os.Stat(full)
	if err == nil && st.IsDir() {
		return dto.Fail("错误的文件名称")
	}
	_ = os.Remove(full)
	return dto.Ok()
}

// newUUID 标准 UUID v4 字符串（8-4-4-4-12，带连字符）。
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// UpdateProfile 更新用户基础资料（昵称/头像）。
func (a *App) UpdateProfile(ctx context.Context, userId int64, nickName, icon *string) dto.Result {
	fields := map[string]any{}
	if nickName != nil && strings.TrimSpace(*nickName) != "" {
		fields["nick_name"] = strings.TrimSpace(*nickName)
	}
	if icon != nil {
		fields["icon"] = *icon
	}
	if len(fields) == 0 {
		return dto.Fail("没有需要更新的内容")
	}
	if err := a.DB.WithContext(ctx).Model(&repo.User{}).Where("id = ?", userId).Updates(fields).Error; err != nil {
		a.Log.Error("更新用户资料失败", "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.Ok()
}

// UpsertUserInfo 更新/创建用户详情（介绍/性别/校区/生日）。
func (a *App) UpsertUserInfo(ctx context.Context, userId int64, introduce *string, gender *bool, city *string, birthday *string) dto.Result {
	fields := map[string]any{}
	if introduce != nil {
		fields["introduce"] = *introduce
	}
	if gender != nil {
		fields["gender"] = *gender
	}
	if city != nil {
		fields["city"] = *city
	}
	if birthday != nil && strings.TrimSpace(*birthday) != "" {
		fields["birthday"] = strings.TrimSpace(*birthday)
	}
	if len(fields) == 0 {
		return dto.Fail("没有需要更新的内容")
	}
	// 不存在则创建（user_id 主键）
	var count int64
	if err := a.DB.WithContext(ctx).Model(&repo.UserInfo{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if count == 0 {
		// 仅插入主键列，create_time/update_time 走数据库默认值
		info := repo.UserInfo{UserId: &userId}
		if err := a.DB.WithContext(ctx).Select("user_id").Create(&info).Error; err != nil {
			a.Log.Error("创建用户详情失败", "err", err)
			return dto.Fail("服务器异常")
		}
	}
	if err := a.DB.WithContext(ctx).Model(&repo.UserInfo{}).Where("user_id = ?", userId).Updates(fields).Error; err != nil {
		a.Log.Error("更新用户详情失败", "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.Ok()
}
