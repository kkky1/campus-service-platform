package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

func shopPtr(s *repo.Shop) *repo.Shop { return s }

// 写入逻辑过期缓存（模拟预热/重建）。
func (a *App) seedShopCache(t *testing.T, id int64, shop *repo.Shop, expireIn time.Duration) {
	t.Helper()
	ctx := context.Background()
	rd := redisData{Data: json.RawMessage(mustJSON(shop)), ExpireTime: time.Now().Add(expireIn).Format("2006-01-02 15:04:05")}
	if err := a.RDB.Set(ctx, rds.CacheShopKey+itoa(id), mustJSON(rd), 0).Err(); err != nil {
		t.Fatal(err)
	}
}

func itoa(id int64) string { return json.Number(intToString(id)).String() }

func intToString(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}

func TestQueryShopByIdCacheHit(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	shop := repo.Shop{Id: int64Ptr(1), Name: strPtr("一食堂")}
	app.seedShopCache(t, 1, &shop, 10*time.Minute)
	res := app.QueryShopById(ctx, 1)
	if !res.Success {
		t.Fatalf("缓存命中应成功: %v", res)
	}
	data, _ := json.Marshal(res.Data)
	if string(data) != `{"id":1,"name":"一食堂"}` {
		t.Fatalf("缓存数据不对: %s", data)
	}
}

func TestQueryShopByIdNotWarmed(t *testing.T) {
	app, _ := newAppEnv(t)
	res := app.QueryShopById(context.Background(), 2)
	if res.ErrorMsg == nil || *res.ErrorMsg != "店铺不存在！" {
		t.Fatalf("未预热应返回店铺不存在: %v", res)
	}
}

func TestQueryShopByIdLogicalExpireRebuild(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	shop := repo.Shop{Id: int64Ptr(1), Name: strPtr("旧名")}
	app.DB.Create(&shop)
	app.seedShopCache(t, 1, &shop, -time.Minute) // 已逻辑过期
	// 1) 过期缓存立即返回旧数据（不等重建）
	res := app.QueryShopById(ctx, 1)
	if !res.Success {
		t.Fatalf("过期应返回旧数据: %v", res)
	}
	data, _ := json.Marshal(res.Data)
	if string(data) != `{"id":1,"name":"旧名"}` {
		t.Fatalf("过期返回的不是旧数据: %s", data)
	}
	// 2) 等待异步重建完成：缓存被重写且处于未过期状态（确定性轮询）
	deadline := time.Now().Add(2 * time.Second)
	for {
		raw, err := app.RDB.Get(ctx, rds.CacheShopKey+"1").Result()
		var rd redisData
		if err == nil && json.Unmarshal([]byte(raw), &rd) == nil {
			if exp, e := parseExpireTime(rd.ExpireTime); e == nil && exp.After(time.Now()) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("异步重建未在期限内完成")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 3) 更新店铺（内部删除缓存）→ 再次查询回源得到新值
	if r := app.UpdateShop(ctx, &repo.Shop{Id: int64Ptr(1), Name: strPtr("新名")}); !r.Success {
		t.Fatalf("更新失败: %v", r)
	}
	res = app.QueryShopById(ctx, 1)
	data, _ = json.Marshal(res.Data)
	if string(data) != `{"id":1,"name":"新名"}` {
		t.Fatalf("更新后查询应回源得到新值: %s", data)
	}
}

func TestUpdateShopDeletesCache(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	shop := repo.Shop{Id: int64Ptr(1), Name: strPtr("一食堂")}
	app.DB.Create(&shop)
	app.seedShopCache(t, 1, &shop, 10*time.Minute)
	// 更新
	upd := repo.Shop{Id: int64Ptr(1), Name: strPtr("二食堂")}
	if res := app.UpdateShop(ctx, &upd); !res.Success {
		t.Fatalf("更新失败: %v", res)
	}
	exists, _ := app.RDB.Exists(ctx, rds.CacheShopKey+"1").Result()
	if exists != 0 {
		t.Fatalf("更新后缓存未删除")
	}
	// id 为空
	if res := app.UpdateShop(ctx, &repo.Shop{}); res.ErrorMsg == nil || *res.ErrorMsg != "店铺id不能为空" {
		t.Fatalf("空 id 应拒绝: %v", res)
	}
}

func TestShopTypeList(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	// DB 无数据 → 没有分类数据
	res := app.QueryShopTypeList(ctx)
	if res.ErrorMsg == nil || *res.ErrorMsg != "没有分类数据" {
		t.Fatalf("空数据文案不对: %v", res)
	}
	// 插入数据 → 回填 Redis List
	app.DB.Create(&repo.ShopType{Id: int64Ptr(1), Name: strPtr("美食"), Icon: strPtr("i1"), Sort: intPtr(2)})
	app.DB.Create(&repo.ShopType{Id: int64Ptr(2), Name: strPtr("健身"), Icon: strPtr("i2"), Sort: intPtr(1)})
	res = app.QueryShopTypeList(ctx)
	if !res.Success {
		t.Fatalf("查询失败: %v", res)
	}
	// Redis List 已回填
	n, _ := app.RDB.LLen(ctx, rds.ShopTypeKey).Result()
	if n != 2 {
		t.Fatalf("回填数量 = %d, want 2", n)
	}
	// 第二次命中缓存（排序为 DB 升序：健身、美食）
	res = app.QueryShopTypeList(ctx)
	list := res.Data.([]repo.ShopType)
	if len(list) != 2 || *list[0].Name != "健身" {
		t.Fatalf("缓存路径顺序不对: %v", list)
	}
}

func int64Ptr(v int64) *int64 { return &v }
func intPtr(v int) *int       { return &v }

// B1 修复回归：缓存未命中回源 DB 并写缓存
func TestQueryShopByIdFallsBackToDB(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	app.DB.Create(&repo.Shop{Id: int64Ptr(1), Name: strPtr("回源店")})
	res := app.QueryShopById(ctx, 1)
	if !res.Success {
		t.Fatalf("缓存未命中应回源成功: %v", res)
	}
	data, _ := json.Marshal(res.Data)
	if string(data) != `{"id":1,"name":"回源店"}` {
		t.Fatalf("回源数据不对: %s", data)
	}
	// 缓存已写入（逻辑过期结构）
	raw, err := app.RDB.Get(ctx, rds.CacheShopKey+"1").Result()
	if err != nil || !strings.Contains(raw, "expireTime") {
		t.Fatalf("回源后应写缓存: %q %v", raw, err)
	}
}

// B1 修复回归：DB 不存在写空值缓存（防穿透）
func TestQueryShopByIdNullCache(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	res := app.QueryShopById(ctx, 999)
	if res.ErrorMsg == nil || *res.ErrorMsg != "店铺不存在！" {
		t.Fatalf("不存在店铺文案: %v", res)
	}
	val, err := app.RDB.Get(ctx, rds.CacheShopKey+"999").Result()
	if err != nil || val != "" {
		t.Fatalf("应写空值缓存: %q %v", val, err)
	}
	ttl := app.RDB.TTL(ctx, rds.CacheShopKey+"999").Val()
	if ttl <= 0 {
		t.Fatalf("空值缓存应有 TTL: %v", ttl)
	}
}

// B2 修复回归：新增/更新维护 GEO 索引，类型变更清理旧集合
func TestShopGeoIndexMaintenance(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	x, y := 120.1, 30.2
	res := app.SaveShop(ctx, &repo.Shop{Name: strPtr("GEO店"), TypeId: int64Ptr(1), X: &x, Y: &y})
	if !res.Success {
		t.Fatalf("新增失败: %v", res)
	}
	id := res.Data.(int64)
	if n, _ := app.RDB.ZCard(ctx, rds.ShopGeoKey+"1").Result(); n != 1 {
		t.Fatalf("类型1 GEO 应有 1 个成员: %d", n)
	}
	// 换类型
	if r := app.UpdateShop(ctx, &repo.Shop{Id: &id, TypeId: int64Ptr(2)}); !r.Success {
		t.Fatalf("更新失败: %v", r)
	}
	if n, _ := app.RDB.ZCard(ctx, rds.ShopGeoKey+"1").Result(); n != 0 {
		t.Fatalf("旧类型 GEO 应清空: %d", n)
	}
	if n, _ := app.RDB.ZCard(ctx, rds.ShopGeoKey+"2").Result(); n != 1 {
		t.Fatalf("新类型 GEO 应有成员: %d", n)
	}
}
