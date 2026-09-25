// campus-service-platform 后端入口：装配 config → db → redis → kafka → router → consumer。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/httpapi"
	"campus-service-platform/internal/mq"
	"campus-service-platform/internal/service"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfgPath := "config.yaml"
	if v := os.Getenv("CONFIG_PATH"); v != "" {
		cfgPath = v
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("加载配置失败", "err", err)
		os.Exit(1)
	}

	// MySQL
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Error("连接 MySQL 失败", "err", err)
		os.Exit(1)
	}

	// Redis
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Error("连接 Redis 失败", "err", err)
		cancel()
		os.Exit(1)
	}
	cancel()

	// Kafka
	producer := mq.NewProducer(cfg.Kafka.Brokers, log)
	app := service.New(db, rdb, producer, log, cfg.Upload.Dir)

	// 消费者（主 + DLT）
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go mq.NewMainConsumer(cfg.Kafka.Brokers, app, log).Run(rootCtx)
	go mq.NewDLTConsumer(cfg.Kafka.Brokers, app, log).Run(rootCtx)

	// HTTP
	gin.SetMode(gin.ReleaseMode)
	router := httpapi.NewRouter(httpapi.New(app))
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: router}
	go func() {
		log.Info("campus-service-platform 启动", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP 服务退出", "err", err)
			stop()
		}
	}()

	<-rootCtx.Done()
	log.Info("正在关闭…")
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer c2()
	_ = srv.Shutdown(shutdownCtx)
	_ = producer.Close()
}
