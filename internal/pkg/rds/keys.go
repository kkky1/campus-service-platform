// Package rds Redis 键与 TTL 常量（对齐原 RedisConstants，全部保留）。
package rds

import "time"

// 键前缀
const (
	LoginCodeKey   = "login:code:"
	LoginUserKey   = "login:token:"
	CacheShopKey   = "cache:shop:"
	LockShopKey    = "lock:shop:"
	SeckillStockKey = "seckill:stock:"
	SeckillOrderKey = "seckill:order:"
	IcrOrderKey    = "icr:order:"
	BlogLikedKey   = "blog:liked:"
	FeedKey        = "feed:"
	ShopGeoKey     = "shop:geo:"
	ShopTypeKey    = "shop_type:"
	UserSignKey    = "sign:"
	FollowsKey     = "follows:"
)

// TTL（对齐原 RedisConstants 与实际生效值）
const (
	LoginCodeTTL = 2 * time.Minute
	LoginUserTTL = 30 * time.Minute // 原常量声明 36000L，但代码实际使用 30 分钟，统一为实际行为
	CacheShopTTL = 30 * time.Minute // 逻辑过期
	LockShopTTL  = 10 * time.Second
)

// 业务常量（对齐原 SystemConstants）
const (
	DefaultPageSize = 5
	MaxPageSize     = 10
	FeedPageCount   = 2
	GeoRadiusM      = 5000
	TopLikers       = 5
	NickNamePrefix  = "user_"
)

// 雪花式订单号起点（对齐原 RedisIdWorker.BEGIN_TIMESTAMP）
const BeginTimestamp int64 = 1640995200

// 抢券 Lua 返回值约定
const (
	SeckillOK      = 0
	SeckillNoStock = 1
	SeckillRepeat  = 2
)
