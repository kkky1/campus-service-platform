package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8081 {
		t.Errorf("默认端口 = %d, want 8081", cfg.Server.Port)
	}
	if cfg.MySQL.DSN == "" || cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("默认 DSN/Redis 不对: %s %s", cfg.MySQL.DSN, cfg.Redis.Addr)
	}
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "localhost:9092" {
		t.Errorf("默认 brokers 不对: %v", cfg.Kafka.Brokers)
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("MYSQL_DSN", "root:p@tcp(127.0.0.1:3306)/test")
	os.Setenv("REDIS_ADDR", "redis:6379")
	os.Setenv("KAFKA_BROKERS", "k1:9092,k2:9092")
	os.Setenv("UPLOAD_DIR", "/data/uploads")
	defer func() {
		for _, k := range []string{"SERVER_PORT", "MYSQL_DSN", "REDIS_ADDR", "KAFKA_BROKERS", "UPLOAD_DIR"} {
			os.Unsetenv(k)
		}
	}()
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("端口覆盖失败: %d", cfg.Server.Port)
	}
	if cfg.MySQL.DSN != "root:p@tcp(127.0.0.1:3306)/test" {
		t.Errorf("DSN 覆盖失败: %s", cfg.MySQL.DSN)
	}
	if cfg.Redis.Addr != "redis:6379" {
		t.Errorf("Redis 覆盖失败: %s", cfg.Redis.Addr)
	}
	if len(cfg.Kafka.Brokers) != 2 || cfg.Kafka.Brokers[1] != "k2:9092" {
		t.Errorf("brokers 覆盖失败: %v", cfg.Kafka.Brokers)
	}
	if cfg.Upload.Dir != "/data/uploads" {
		t.Errorf("上传目录覆盖失败: %s", cfg.Upload.Dir)
	}
}

func TestYamlFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"
	content := "server:\n  port: 9000\nmysql:\n  dsn: x:y@tcp(1.2.3.4:3306)/db\nredis:\n  addr: r:6379\nkafka:\n  brokers:\n    - b1:9092\nupload:\n  dir: /tmp/u\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9000 || cfg.Upload.Dir != "/tmp/u" || cfg.Kafka.Brokers[0] != "b1:9092" {
		t.Errorf("yaml 解析不对: %+v", cfg)
	}
}
