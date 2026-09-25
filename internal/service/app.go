// Package service 业务层：等价原 service/impl 各实现类。
package service

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SeckillOrderMsg Kafka 秒杀订单消息（对齐原 RabbitMQ 消息体）。
type SeckillOrderMsg struct {
	Id        int64 `json:"id"`
	UserId    int64 `json:"userId"`
	VoucherId int64 `json:"voucherId"`
}

// OrderPublisher 秒杀订单发布器（实现见 internal/mq）。
type OrderPublisher interface {
	PublishSeckillOrder(ctx context.Context, msg SeckillOrderMsg) error
}

// App 服务依赖装配体。
type App struct {
	DB        *gorm.DB
	RDB       *redis.Client
	Pub       OrderPublisher
	Log       *slog.Logger
	UploadDir string
}

// ProcessSeckillOrder 实现 mq.OrderProcessor：幂等建单 + 条件扣库存。
func (a *App) ProcessSeckillOrder(ctx context.Context, msg SeckillOrderMsg) error {
	_, err := CreateOrderIdempotent(ctx, a.DB, msg)
	if err != nil {
		a.Log.Error("处理秒杀订单失败", "orderId", msg.Id, "err", err)
	}
	return err
}

// New 构造 App。
func New(db *gorm.DB, rdb *redis.Client, pub OrderPublisher, log *slog.Logger, uploadDir string) *App {
	if log == nil {
		log = slog.Default()
	}
	return &App{DB: db, RDB: rdb, Pub: pub, Log: log, UploadDir: uploadDir}
}

func strPtr(s string) *string { return &s }
