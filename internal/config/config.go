// Package config 配置加载：config.yaml + 环境变量覆盖。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 服务配置。默认值与原 application.yaml 对齐（并统一为 compose 的 hmdp/123456）。
type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	MySQL struct {
		DSN string `yaml:"dsn"`
	} `yaml:"mysql"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
	} `yaml:"redis"`
	Kafka struct {
		Brokers []string `yaml:"brokers"`
	} `yaml:"kafka"`
	Upload struct {
		Dir string `yaml:"dir"`
	} `yaml:"upload"`
}

// Load 读取 yaml 文件（不存在则用默认值），再用环境变量覆盖。
// 支持：SERVER_PORT、MYSQL_DSN、REDIS_ADDR、REDIS_PASSWORD、KAFKA_BROKERS、UPLOAD_DIR。
func Load(path string) (*Config, error) {
	cfg := defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
		}
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
			}
		}
	}
	applyEnv(cfg)
	return cfg, nil
}

func defaults() *Config {
	cfg := &Config{}
	cfg.Server.Port = 8081
	cfg.MySQL.DSN = "root:123456@tcp(127.0.0.1:3306)/hmdp?charset=utf8mb4&parseTime=True&loc=Local"
	cfg.Redis.Addr = "127.0.0.1:6379"
	cfg.Kafka.Brokers = []string{"localhost:9092"}
	cfg.Upload.Dir = "./data/uploads"
	return cfg
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = splitComma(v)
	}
	if v := os.Getenv("UPLOAD_DIR"); v != "" {
		cfg.Upload.Dir = v
	}
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
