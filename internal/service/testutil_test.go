package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/repo"
)

// newSQLiteDB 内存 SQLite（纯 Go 驱动，共享缓存避免连接池各自独立库），自动迁移指定模型。
func newSQLiteDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:test_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// newAppEnv App + miniredis + sqlite（全模型）。
func newAppEnv(t *testing.T) (*App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db := newSQLiteDB(t,
		&repo.User{}, &repo.UserInfo{}, &repo.Shop{}, &repo.ShopType{},
		&repo.Blog{}, &repo.BlogComments{}, &repo.Follow{},
		&repo.Voucher{}, &repo.SeckillVoucher{}, &repo.VoucherOrder{},
	)
	app := New(db, rdb, fakePub{}, nil, t.TempDir())
	return app, mr
}
