package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/repo"
)

func newServiceEnv(t *testing.T, migrate ...any) (*App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(nil, rdb, nil, nil, t.TempDir()), mr
}

func TestJavaHashCode(t *testing.T) {
	cases := []struct {
		in   string
		want int32
	}{
		{"abc", 96354},
		{"4f8b1e9a-2c3d-4e5f-6a7b-8c9d0e1f2a3b", -2104092736},
		{"12345678-1234-1234-1234-123456789012", 1540141310},
		{"hello", 99162322},
	}
	for _, c := range cases {
		if got := javaHashCode(c.in); got != c.want {
			t.Errorf("javaHashCode(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func nowYM() string {
	return time.Now().Format("200601")
}

func TestSign(t *testing.T) {
	app, _ := newServiceEnv(t)
	ctx := context.Background()
	res := app.Sign(ctx, 7)
	if !res.Success {
		t.Fatalf("签到失败: %v", res)
	}
	key := fmt.Sprintf("sign:7:%s", nowYM())
	v, err := app.RDB.GetBit(ctx, key, int64(time.Now().Day()-1)).Result()
	if err != nil || v != 1 {
		t.Fatalf("签到位未写入: %v %v", v, err)
	}
}

func TestUploadPathHash(t *testing.T) {
	app, _ := newServiceEnv(t)
	dir := app.UploadDir
	// 用固定 uuid 计算路径（newUUID 不可控，直接验证 hash 目录规则）
	uuid := "4f8b1e9a-2c3d-4e5f-6a7b-8c9d0e1f2a3b"
	h := javaHashCode(uuid)
	d1, d2 := int(h)&0xF, (int(h)>>4)&0xF
	if d1 != 0 || d2 != 12 {
		t.Fatalf("hash 目录不对: %d/%d", d1, d2)
	}
	rel := fmt.Sprintf("/blogs/%d/%d/%s.png", d1, d2, uuid)
	if !strings.HasPrefix(rel, "/blogs/0/12/") {
		t.Fatalf("路径格式不对: %s", rel)
	}
	_ = dir
}

func TestUploadAndDelete(t *testing.T) {
	app, _ := newServiceEnv(t)
	ctx := context.Background()
	res := app.UploadImage(ctx, "photo.png", []byte("PNGDATA"))
	if !res.Success {
		t.Fatalf("上传失败: %v", res)
	}
	rel, _ := res.Data.(string)
	if !strings.HasPrefix(rel, "/blogs/") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("返回路径不对: %q", rel)
	}
	full := filepath.Join(app.UploadDir, filepath.FromSlash(rel))
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("文件未保存: %v", err)
	}
	// 删除
	if res := app.DeleteImage(ctx, rel); !res.Success {
		t.Fatalf("删除失败: %v", res)
	}
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Fatalf("文件未被删除")
	}
	// 目录路径拒绝
	if res := app.DeleteImage(ctx, "/blogs"); res.ErrorMsg == nil || *res.ErrorMsg != "错误的文件名称" {
		t.Fatalf("目录应拒绝: %v", res)
	}
}

var _ = gorm.Expr
var _ = repo.User{}
