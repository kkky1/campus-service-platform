package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"campus-service-platform/internal/repo"
)

// 集成测试：依赖本地真实 Redis 与 MariaDB（127.0.0.1）。
// 不可用时自动跳过（CI 或无基础设施环境）。

const (
	itRedisAddr = "127.0.0.1:6379"
	itMySQLDSN  = "root:123456@tcp(127.0.0.1:3306)/hmdp?charset=utf8mb4&parseTime=True&loc=Local"
)

func itRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: itRedisAddr, DB: 13})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("本地 Redis 不可用，跳过集成测试: %v", err)
	}
	rdb.FlushDB(ctx)
	return rdb
}

func itDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(itMySQLDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("本地 MySQL 不可用，跳过集成测试: %v", err)
	}
	return db
}

func itApp(t *testing.T) (*App, *gorm.DB) {
	t.Helper()
	db := itDB(t)
	app := New(db, itRedis(t), fakePub{}, nil, t.TempDir())
	return app, db
}

type fakePub struct{}

func (fakePub) PublishSeckillOrder(_ context.Context, _ SeckillOrderMsg) error { return nil }

func TestSignCountIntegration(t *testing.T) {
	app, _ := itApp(t)
	ctx := context.Background()
	now := time.Now()
	if now.Day() < 4 {
		t.Skip("月初前 4 天内无法构造 3 天连续窗口")
	}
	key := fmt.Sprintf("%s7:%s", rds.UserSignKey, now.Format("200601"))
	// 连续签到：昨天、前天、大前天（含今天共 3 天）
	for d := 0; d < 3; d++ {
		if err := app.RDB.SetBit(ctx, key, int64(now.Day()-1-d), 1).Err(); err != nil {
			t.Fatal(err)
		}
	}
	res := app.SignCount(ctx, 7)
	if n := res.Data.(int); n != 3 {
		t.Fatalf("连续签到 = %d, want 3", n)
	}
	// 再往前补一天 → 4 天连续
	if err := app.RDB.SetBit(ctx, key, int64(now.Day()-4), 1).Err(); err != nil {
		t.Fatal(err)
	}
	res = app.SignCount(ctx, 7)
	if n := res.Data.(int); n != 4 {
		t.Fatalf("补签后连续签到 = %d, want 4", n)
	}
}

func TestSeckillLuaIntegration(t *testing.T) {
	app, _ := itApp(t)
	ctx := context.Background()
	// 预载库存
	app.RDB.Set(ctx, fmt.Sprintf("%s1", rds.SeckillStockKey), "5", 0)
	// 三次成功
	for i := 1; i <= 3; i++ {
		res := app.SeckillVoucher(ctx, int64(i), 1)
		if !res.Success {
			t.Fatalf("第 %d 次抢券失败: %v", i, res)
		}
	}
	// 重复下单
	res := app.SeckillVoucher(ctx, 2, 1)
	if res.ErrorMsg == nil || *res.ErrorMsg != "不能重复下单" {
		t.Fatalf("重复抢券应报不能重复下单: %v", res)
	}
	// 库存耗尽（5 份，已卖 3，再卖 2 后不足）
	for i := 4; i <= 5; i++ {
		if res := app.SeckillVoucher(ctx, int64(i), 1); !res.Success {
			t.Fatalf("第 %d 次抢券失败: %v", i, res)
		}
	}
	res = app.SeckillVoucher(ctx, 99, 1)
	if res.ErrorMsg == nil || *res.ErrorMsg != "库存不足" {
		t.Fatalf("库存耗尽应报库存不足: %v", res)
	}
	// 库存 key 缺失 → 库存不足（修复后的行为）
	res = app.SeckillVoucher(ctx, 100, 999)
	if res.ErrorMsg == nil || *res.ErrorMsg != "库存不足" {
		t.Fatalf("库存未预载应报库存不足: %v", res)
	}
}

func TestSeckillMissingKeyMessage(t *testing.T) {
	app, _ := itApp(t)
	ctx := context.Background()
	app.RDB.Del(ctx, fmt.Sprintf("%s777", rds.SeckillStockKey))
	res := app.SeckillVoucher(ctx, 1, 777)
	if res.ErrorMsg == nil || *res.ErrorMsg != "库存不足" {
		t.Fatalf("key 缺失文案不对: %v", res)
	}
}

func TestOrderIdempotentIntegration(t *testing.T) {
	app, db := itApp(t)
	ctx := context.Background()
	// 准备秒杀券
	app.DB.Exec("DELETE FROM tb_voucher_order WHERE voucher_id = 5001")
	app.DB.Exec("DELETE FROM tb_seckill_voucher WHERE voucher_id = 5001")
	app.DB.Exec("INSERT INTO tb_voucher (id, shop_id, title, status) VALUES (5001, 1, 'IT券', 1)")
	app.DB.Exec("INSERT INTO tb_seckill_voucher (voucher_id, stock) VALUES (5001, 10)")
	msg := SeckillOrderMsg{Id: 90001, UserId: 1001, VoucherId: 5001}
	// 第一次：插入成功 + 扣库存
	inserted, err := CreateOrderIdempotent(ctx, db, msg)
	if err != nil || !inserted {
		t.Fatalf("首次建单失败: %v %v", inserted, err)
	}
	var stock int
	app.DB.Raw("SELECT stock FROM tb_seckill_voucher WHERE voucher_id = 5001").Scan(&stock)
	if stock != 9 {
		t.Fatalf("库存 = %d, want 9", stock)
	}
	// 重复投递：不重复建单、不重复扣库存
	inserted, err = CreateOrderIdempotent(ctx, db, msg)
	if err != nil || inserted {
		t.Fatalf("重复投递应幂等: %v %v", inserted, err)
	}
	app.DB.Raw("SELECT stock FROM tb_seckill_voucher WHERE voucher_id = 5001").Scan(&stock)
	if stock != 9 {
		t.Fatalf("重复投递后库存 = %d, want 9", stock)
	}
	var cnt int64
	app.DB.Model(&repo.VoucherOrder{}).Where("id = 90001").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("订单数 = %d, want 1", cnt)
	}
	// 清理
	app.DB.Exec("DELETE FROM tb_voucher_order WHERE id = 90001")
	app.DB.Exec("DELETE FROM tb_seckill_voucher WHERE voucher_id = 5001")
	app.DB.Exec("DELETE FROM tb_voucher WHERE id = 5001")
}

func TestIdWorkerIntegration(t *testing.T) {
	app, _ := itApp(t)
	ctx := context.Background()
	id1, err := app.NextOrderId(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := app.NextOrderId(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id1 >= id2 {
		t.Fatalf("订单号应递增: %d >= %d", id1, id2)
	}
	// 高位应为时间戳差值（近 0），低位为序列
	ts := time.Now().Unix() - rds.BeginTimestamp
	if id1>>32 < ts-60 || id1>>32 > ts+60 {
		t.Fatalf("订单号高 32 位不是时间戳: %d vs %d", id1>>32, ts)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// feed 滚动分页：同分去重 + offset 计算（5.3，依赖 MySQL 的 field() 排序）
func TestQueryBlogOfFollowScrollIntegration(t *testing.T) {
	app, db := itApp(t)
	ctx := context.Background()
	db.Exec("DELETE FROM tb_blog WHERE id IN (1,2,3,4)")
	db.Exec("INSERT INTO tb_blog (id, shop_id, user_id, title, images, content) VALUES (1,1,9,'b1','','c'),(2,1,9,'b2','','c'),(3,1,9,'b3','','c'),(4,1,9,'b4','','c')")
	key := rds.FeedKey + "10"
	app.RDB.Del(ctx, key)
	app.RDB.ZAdd(ctx, key,
		redis.Z{Score: 4000, Member: "4"}, redis.Z{Score: 3000, Member: "3"},
		redis.Z{Score: 3000, Member: "2"}, redis.Z{Score: 1000, Member: "1"})
	u := &dto.UserDTO{Id: int64Ptr(10)}
	// 第一页：score<=999999 → 取 4,3
	res := app.QueryBlogOfFollow(ctx, 10, 999999, 0, u)
	if !res.Success {
		t.Fatalf("feed 查询失败: %v", res)
	}
	sr := res.Data.(dto.ScrollResult)
	list := sr.List.([]repo.Blog)
	if len(list) != 2 || *list[0].Id != 4 || *list[1].Id != 3 {
		t.Fatalf("第一页顺序不对: %v", list)
	}
	if *sr.MinTime != 3000 || *sr.Offset != 1 {
		t.Fatalf("第一页滚动参数 minTime=%d offset=%d, want 3000/1", *sr.MinTime, *sr.Offset)
	}
	// 第二页：score<=3000 offset=1 → 取 2,1
	res = app.QueryBlogOfFollow(ctx, 10, *sr.MinTime, *sr.Offset, u)
	sr = res.Data.(dto.ScrollResult)
	list = sr.List.([]repo.Blog)
	if len(list) != 2 || *list[0].Id != 2 || *list[1].Id != 1 {
		t.Fatalf("第二页顺序不对: %v", list)
	}
	if *sr.MinTime != 1000 || *sr.Offset != 1 {
		t.Fatalf("第二页滚动参数 minTime=%d offset=%d, want 1000/1", *sr.MinTime, *sr.Offset)
	}
	// 第三页：无更多 → data 省略
	res = app.QueryBlogOfFollow(ctx, 10, *sr.MinTime, *sr.Offset, u)
	if res.Data != nil {
		t.Fatalf("第三页应为空: %v", res.Data)
	}
	db.Exec("DELETE FROM tb_blog WHERE id IN (1,2,3,4)")
}

// 附近店铺搜索：GEO 5000m 排序 + 分页（4.4）
func TestShopGeoIntegration(t *testing.T) {
	app, db := itApp(t)
	ctx := context.Background()
	db.Exec("DELETE FROM tb_shop WHERE id IN (1001,1002,1003,1004,1005,1006)")
	db.Exec("INSERT INTO tb_shop (id,name,type_id,images,address,x,y,sold,comments,score) VALUES (1001,'近A',1,'','a',120.00010,30.00010,0,0,0),(1002,'近B',1,'','a',120.00030,30.00030,0,0,0),(1003,'远C',1,'','a',120.01000,30.01000,0,0,0),(1004,'近D',1,'','a',120.00020,30.00020,0,0,0),(1005,'超远',1,'','a',121.00000,31.00000,0,0,0),(1006,'近E',1,'','a',120.00040,30.00040,0,0,0)")
	key := rds.ShopGeoKey + "1"
	app.RDB.Del(ctx, key)
	for _, m := range []struct {
		lng, lat float64
		id       string
	}{
		{120.00010, 30.00010, "1001"},
		{120.00030, 30.00030, "1002"},
		{120.01000, 30.01000, "1003"},
		{120.00020, 30.00020, "1004"},
		{121.00000, 31.00000, "1005"},
		{120.00040, 30.00040, "1006"},
	} {
		app.RDB.GeoAdd(ctx, key, &redis.GeoLocation{Longitude: m.lng, Latitude: m.lat, Name: m.id})
	}
	x, y := 120.0, 30.0
	// 第一页 5 条（1005 超远被 5000m 排除）
	res := app.QueryShopByType(ctx, 1, 1, &x, &y)
	if !res.Success {
		t.Fatalf("GEO 查询失败: %v", res)
	}
	shops := res.Data.([]repo.Shop)
	if len(shops) != 5 {
		t.Fatalf("第一页 = %d, want 5", len(shops))
	}
	// 距离升序：1001 < 1004 < 1002 < 1006 < 1003
	wantOrder := []int64{1001, 1004, 1002, 1006, 1003}
	for i, s := range shops {
		if *s.Id != wantOrder[i] {
			t.Fatalf("第 %d 个 = %d, want %d", i, *s.Id, wantOrder[i])
		}
		if s.Distance == nil || *s.Distance <= 0 {
			t.Fatalf("distance 未附加: %v", s)
		}
	}
	// 第二页：半径内共 5 条，from=5 → size<=from → 空列表（对齐 Java list.size()<=from 语义）
	res = app.QueryShopByType(ctx, 1, 2, &x, &y)
	shops = res.Data.([]repo.Shop)
	if len(shops) != 0 {
		t.Fatalf("第二页应为空（对齐原分页语义）: %v", shops)
	}
	// 无坐标 → DB 分页（页大小 5）
	res = app.QueryShopByType(ctx, 1, 1, nil, nil)
	shops = res.Data.([]repo.Shop)
	if len(shops) != 5 {
		t.Fatalf("无坐标分页 = %d, want 5", len(shops))
	}
	db.Exec("DELETE FROM tb_shop WHERE id IN (1001,1002,1003,1004,1005,1006)")
}
