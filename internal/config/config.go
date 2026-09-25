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
	RAG   RAGConfig `yaml:"rag"`
	Login struct {
		// SkipCode 临时开关：登录不校验短信验证码（默认 false，生产请保持开启校验）
		SkipCode bool `yaml:"skip_code"`
	} `yaml:"login"`
}

// RAGConfig RAG 模块配置。
type RAGConfig struct {
	Chunk struct {
		TokenNum       int `yaml:"token_num"`
		OverlapPercent int `yaml:"overlap_percent"`
	} `yaml:"chunk"`
	Embedding struct {
		Provider string `yaml:"provider"` // local | openai
		BaseURL  string `yaml:"base_url"`
		APIKey   string `yaml:"api_key"`
		Model    string `yaml:"model"`
		Dim      int    `yaml:"dim"`
	} `yaml:"embedding"`
	LLM struct {
		BaseURL     string  `yaml:"base_url"`
		APIKey      string  `yaml:"api_key"`
		Model       string  `yaml:"model"`
		Temperature float64 `yaml:"temperature"`
		MaxTokens   int     `yaml:"max_tokens"`
	} `yaml:"llm"`
	Retrieval struct {
		TopK          int     `yaml:"top_k"`
		VectorWeight  float64 `yaml:"vector_weight"`
		KeywordWeight float64 `yaml:"keyword_weight"`
		MinSimilarity float64 `yaml:"min_similarity"`
	} `yaml:"retrieval"`
	Citation struct {
		MinSentenceLen int     `yaml:"min_sentence_len"`
		Threshold      float64 `yaml:"threshold"`
	} `yaml:"citation"`
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
	// RAG 默认值
	cfg.RAG.Chunk.TokenNum = 256
	cfg.RAG.Chunk.OverlapPercent = 10
	cfg.RAG.Embedding.Provider = "local"
	cfg.RAG.Embedding.Dim = 384
	cfg.RAG.LLM.BaseURL = "https://api.deepseek.com"
	cfg.RAG.LLM.Model = "deepseek-chat"
	cfg.RAG.LLM.Temperature = 0.2
	cfg.RAG.LLM.MaxTokens = 2048
	cfg.RAG.Retrieval.TopK = 8
	cfg.RAG.Retrieval.VectorWeight = 0.7
	cfg.RAG.Retrieval.KeywordWeight = 0.3
	cfg.RAG.Retrieval.MinSimilarity = 0.1
	cfg.RAG.Citation.MinSentenceLen = 5
	cfg.RAG.Citation.Threshold = 0.63
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
	// RAG 环境变量覆盖
	if v := os.Getenv("RAG_EMBEDDING_PROVIDER"); v != "" {
		cfg.RAG.Embedding.Provider = v
	}
	if v := os.Getenv("RAG_EMBEDDING_BASE_URL"); v != "" {
		cfg.RAG.Embedding.BaseURL = v
	}
	if v := os.Getenv("RAG_EMBEDDING_API_KEY"); v != "" {
		cfg.RAG.Embedding.APIKey = v
	}
	if v := os.Getenv("RAG_EMBEDDING_MODEL"); v != "" {
		cfg.RAG.Embedding.Model = v
	}
	if v := os.Getenv("RAG_EMBEDDING_DIM"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.RAG.Embedding.Dim)
	}
	if v := os.Getenv("RAG_LLM_BASE_URL"); v != "" {
		cfg.RAG.LLM.BaseURL = v
	}
	if v := os.Getenv("RAG_LLM_API_KEY"); v != "" {
		cfg.RAG.LLM.APIKey = v
	}
	if v := os.Getenv("RAG_LLM_MODEL"); v != "" {
		cfg.RAG.LLM.Model = v
	}
	if v := os.Getenv("LOGIN_SKIP_CODE"); v != "" {
		cfg.Login.SkipCode = v == "true" || v == "1"
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
