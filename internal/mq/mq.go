// Package mq Kafka 生产者与消费者。
package mq

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"

	"campus-service-platform/internal/service"
)

const (
	TopicSeckillOrder    = "seckill.order"
	TopicSeckillOrderDLT = "seckill.order.dlt"
	groupMain            = "seckill-order-group"
	groupDLT             = "seckill-order-dlt-group"
)

// Producer Kafka 生产者（实现 service.OrderPublisher）。
type Producer struct {
	w      *kafka.Writer
	log    *slog.Logger
	closed bool
}

// NewProducer 创建同步生产者（acks=all）。
func NewProducer(brokers []string, log *slog.Logger) *Producer {
	if log == nil {
		log = slog.Default()
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicSeckillOrder,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
	}
	return &Producer{w: w, log: log}
}

// PublishSeckillOrder 发送秒杀订单消息。
func (p *Producer) PublishSeckillOrder(ctx context.Context, msg service.SeckillOrderMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.w.WriteMessages(ctx, kafka.Message{Key: []byte(seckillKey(msg)), Value: body})
}

func seckillKey(msg service.SeckillOrderMsg) string {
	return strconv.FormatInt(msg.VoucherId, 10)
}

// Close 关闭生产者。
func (p *Producer) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	return p.w.Close()
}

// OrderProcessor 订单处理逻辑（主消费者与 DLT 消费者共用）。
type OrderProcessor interface {
	ProcessSeckillOrder(ctx context.Context, msg service.SeckillOrderMsg) error
}

// retryBackoffs 指数退避（3 次重试）。
var retryBackoffs = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second}

// Consumer Kafka 消费者：手动 commit，失败重试 3 次后投 DLT。
type Consumer struct {
	r     *kafka.Reader
	dltW  *kafka.Writer
	proc  OrderProcessor
	log   *slog.Logger
	topic string
	isDLT bool
}

// NewMainConsumer 主消费者（seckill.order）。
func NewMainConsumer(brokers []string, proc OrderProcessor, log *slog.Logger) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  groupMain,
		Topic:    TopicSeckillOrder,
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	dltW := &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: TopicSeckillOrderDLT, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll}
	return &Consumer{r: r, dltW: dltW, proc: proc, log: log, topic: TopicSeckillOrder}
}

// NewDLTConsumer 死信消费者（seckill.order.dlt）。
func NewDLTConsumer(brokers []string, proc OrderProcessor, log *slog.Logger) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  groupDLT,
		Topic:    TopicSeckillOrderDLT,
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	return &Consumer{r: r, proc: proc, log: log, topic: TopicSeckillOrderDLT, isDLT: true}
}

// Run 消费循环（阻塞）。主消费者失败重试后投 DLT；DLT 消费者失败仅告警。
func (c *Consumer) Run(ctx context.Context) {
	for {
		m, err := c.r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			c.log.Error("拉取消息失败", "topic", c.topic, "err", err)
			time.Sleep(time.Second)
			continue
		}
		var msg service.SeckillOrderMsg
		if err := json.Unmarshal(m.Value, &msg); err != nil {
			c.log.Error("消息反序列化失败", "topic", c.topic, "err", err)
			c.r.CommitMessages(ctx, m)
			continue
		}
		if err := c.processWithRetry(ctx, msg); err != nil {
			if !c.isDLT && c.dltW != nil {
				if e := c.dltW.WriteMessages(context.Background(), kafka.Message{Key: m.Key, Value: m.Value}); e != nil {
					c.log.Error("投递 DLT 失败", "err", e)
				} else {
					c.log.Warn("订单处理失败已投 DLT", "orderId", msg.Id, "err", err)
				}
			} else {
				c.log.Error("DLT 处理失败", "orderId", msg.Id, "err", err)
			}
		}
		if err := c.r.CommitMessages(ctx, m); err != nil {
			c.log.Error("提交 offset 失败", "topic", c.topic, "err", err)
		}
	}
}

func (c *Consumer) processWithRetry(ctx context.Context, msg service.SeckillOrderMsg) error {
	err := c.proc.ProcessSeckillOrder(ctx, msg)
	for i := 0; err != nil && i < len(retryBackoffs); i++ {
		time.Sleep(retryBackoffs[i])
		err = c.proc.ProcessSeckillOrder(ctx, msg)
	}
	return err
}

// Close 关闭消费者。
func (c *Consumer) Close() error {
	err := c.r.Close()
	if c.dltW != nil {
		_ = c.dltW.Close()
	}
	return err
}
