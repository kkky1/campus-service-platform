package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

// redisData 逻辑过期缓存结构（对齐 RedisData {data, expireTime}）。
type redisData struct {
	Data       json.RawMessage `json:"data"`
	ExpireTime string          `json:"expireTime"`
}

const rebuildWorkers = 10

// QueryShopById 店铺详情：仅依赖逻辑过期缓存，未预热返回失败。
func (a *App) QueryShopById(ctx context.Context, id int64) dto.Result {
	key := fmt.Sprintf("%s%d", rds.CacheShopKey, id)
	raw, err := a.RDB.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		// 键不存在/为空：与原 queryWithLogicalExpire 一致，返回失败（缓存需预热）
		return dto.Fail("店铺不存在！")
	}
	var rd redisData
	if err := json.Unmarshal([]byte(raw), &rd); err != nil {
		return dto.Fail("服务器异常")
	}
	var shop repo.Shop
	if err := json.Unmarshal(rd.Data, &shop); err != nil {
		return dto.Fail("服务器异常")
	}
	expire, err := parseExpireTime(rd.ExpireTime)
	if err != nil || expire.After(time.Now()) {
		// 未过期：直接返回
		return dto.OkData(shop)
	}
	// 已逻辑过期：返回旧数据，异步重建（互斥锁 lock:shop:{id} EX 10s）
	a.rebuildShopCacheAsync(context.Background(), key, id)
	return dto.OkData(shop)
}

func parseExpireTime(s string) (time.Time, error) {
	for _, l := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析 expireTime %q", s)
}

// rebuildShopCacheAsync 互斥锁保护的后台缓存重建（等价 CacheClient.queryWithLogicalExpire 重建分支）。
func (a *App) rebuildShopCacheAsync(ctx context.Context, cacheKey string, id int64) {
	lockKey := fmt.Sprintf("%s%d", rds.LockShopKey, id)
	ok, err := a.RDB.SetNX(ctx, lockKey, "1", rds.LockShopTTL).Result()
	if err != nil || !ok {
		return // 未抢到锁：仅返回旧数据
	}
	go func() {
		defer a.RDB.Del(context.Background(), lockKey)
		var shop repo.Shop
		if err := a.DB.First(&shop, id).Error; err != nil {
			a.Log.Error("缓存重建查库失败", "id", id, "err", err)
			return
		}
		a.setShopWithLogicalExpire(context.Background(), cacheKey, &shop)
	}()
}

// setShopWithLogicalExpire 写入逻辑过期缓存（expireTime 用 hutool 空格格式，兼容 Java 读取）。
func (a *App) setShopWithLogicalExpire(ctx context.Context, cacheKey string, shop *repo.Shop) {
	now := time.Now().Add(rds.CacheShopTTL)
	rd := redisData{
		Data:       json.RawMessage(mustJSON(shop)),
		ExpireTime: now.Format("2006-01-02 15:04:05"),
	}
	a.RDB.Set(ctx, cacheKey, mustJSON(rd), 0)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// SaveShop 新增店铺，返回 id。
func (a *App) SaveShop(ctx context.Context, s *repo.Shop) dto.Result {
	cols := shopColumns(s)
	if err := a.DB.WithContext(ctx).Select(cols).Create(s).Error; err != nil {
		a.Log.Error("新增店铺失败", "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.OkData(*s.Id)
}

// shopColumns 非空字段列（等价 MyBatis-Plus 动态 insert）。
func shopColumns(s *repo.Shop) []string {
	var cols []string
	if s.Name != nil {
		cols = append(cols, "name")
	}
	if s.TypeId != nil {
		cols = append(cols, "type_id")
	}
	if s.Images != nil {
		cols = append(cols, "images")
	}
	if s.Area != nil {
		cols = append(cols, "area")
	}
	if s.Address != nil {
		cols = append(cols, "address")
	}
	if s.X != nil {
		cols = append(cols, "x")
	}
	if s.Y != nil {
		cols = append(cols, "y")
	}
	if s.AvgPrice != nil {
		cols = append(cols, "avg_price")
	}
	if s.Sold != nil {
		cols = append(cols, "sold")
	}
	if s.Comments != nil {
		cols = append(cols, "comments")
	}
	if s.Score != nil {
		cols = append(cols, "score")
	}
	if s.OpenHours != nil {
		cols = append(cols, "open_hours")
	}
	return cols
}

// UpdateShop 更新店铺并删除缓存。
func (a *App) UpdateShop(ctx context.Context, s *repo.Shop) dto.Result {
	if s.Id == nil {
		return dto.Fail("店铺id不能为空")
	}
	cols := shopColumns(s)
	if err := a.DB.WithContext(ctx).Model(&repo.Shop{}).Where("id = ?", *s.Id).Select(cols).Updates(s).Error; err != nil {
		a.Log.Error("更新店铺失败", "err", err)
		return dto.Fail("服务器异常")
	}
	a.RDB.Del(ctx, fmt.Sprintf("%s%d", rds.CacheShopKey, *s.Id))
	return dto.Ok()
}

// QueryShopByType 按类型分页；带坐标时走 GEO 5000m。
func (a *App) QueryShopByType(ctx context.Context, typeId int64, current int, x, y *float64) dto.Result {
	if x == nil || y == nil {
		var shops []repo.Shop
		if err := a.DB.WithContext(ctx).Where("type_id = ?", typeId).
			Offset((current - 1) * rds.DefaultPageSize).Limit(rds.DefaultPageSize).
			Find(&shops).Error; err != nil {
			return dto.Fail("服务器异常")
		}
		if shops == nil {
			shops = []repo.Shop{}
		}
		return dto.OkData(shops)
	}
	key := fmt.Sprintf("%s%d", rds.ShopGeoKey, typeId)
	from := (current - 1) * rds.DefaultPageSize
	end := current * rds.DefaultPageSize
	// GEORADIUS：与 Java 版 GEOSEARCH 语义一致（距离升序 + 5000m），兼容 Redis 6.0 与 7.x
	res, err := a.RDB.GeoRadius(ctx, key, *x, *y, &redis.GeoRadiusQuery{
		Radius:  rds.GeoRadiusM,
		Unit:    "m",
		WithDist: true,
		Sort:    "ASC",
		Count:   end,
	}).Result()
	if err != nil {
		return dto.Fail("服务器异常")
	}
	if len(res) <= from {
		return dto.OkData([]repo.Shop{})
	}
	var ids []int64
	distMap := map[int64]float64{}
	for _, loc := range res[from:] {
		var id int64
		fmt.Sscanf(loc.Name, "%d", &id)
		ids = append(ids, id)
		distMap[id] = loc.Dist
	}
	var shops []repo.Shop
	idStr := joinInt64(ids)
	if err := a.DB.WithContext(ctx).Where("id IN ?", ids).
		Order("field(id, " + idStr + ")").
		Find(&shops).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	for i := range shops {
		if shops[i].Id != nil {
			if d, ok := distMap[*shops[i].Id]; ok {
				shops[i].Distance = &d
			}
		}
	}
	if shops == nil {
		shops = []repo.Shop{}
	}
	return dto.OkData(shops)
}

func joinInt64(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ",")
}

// QueryShopByName 名称模糊搜索，页大小 10；名称为空时不加过滤条件（对齐原行为）。
func (a *App) QueryShopByName(ctx context.Context, name string, current int) dto.Result {
	q := a.DB.WithContext(ctx)
	if name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var shops []repo.Shop
	if err := q.Offset((current - 1) * rds.MaxPageSize).Limit(rds.MaxPageSize).Find(&shops).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if shops == nil {
		shops = []repo.Shop{}
	}
	return dto.OkData(shops)
}

// QueryShopTypeList 类型列表：Redis List 优先，DB 回填（无 TTL）。
func (a *App) QueryShopTypeList(ctx context.Context) dto.Result {
	items, err := a.RDB.LRange(ctx, rds.ShopTypeKey, 0, -1).Result()
	if err == nil && len(items) > 0 {
		types := make([]repo.ShopType, 0, len(items))
		for _, it := range items {
			var t repo.ShopType
			if json.Unmarshal([]byte(it), &t) == nil {
				types = append(types, t)
			}
		}
		return dto.OkData(types)
	}
	var types []repo.ShopType
	if err := a.DB.WithContext(ctx).Order("sort ASC").Find(&types).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if len(types) == 0 {
		return dto.Fail("没有分类数据")
	}
	for _, t := range types {
		a.RDB.RPush(ctx, rds.ShopTypeKey, mustJSON(t))
	}
	return dto.OkData(types)
}

var _ = gorm.ErrRecordNotFound
