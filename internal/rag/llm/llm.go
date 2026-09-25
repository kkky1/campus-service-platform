// Package llm OpenAI 兼容的大模型对话客户端。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"campus-service-platform/internal/config"
)

// Client 对话客户端接口。
type Client interface {
	Chat(ctx context.Context, system, user string) (string, error)
	Model() string
}

// New 按配置构造 OpenAI 兼容客户端。
func New(cfg config.RAGConfig) Client {
	return &openAIClient{
		baseURL:     strings.TrimRight(cfg.LLM.BaseURL, "/"),
		apiKey:      cfg.LLM.APIKey,
		model:       cfg.LLM.Model,
		temperature: cfg.LLM.Temperature,
		maxTokens:   cfg.LLM.MaxTokens,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

type openAIClient struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	client      *http.Client
}

func (c *openAIClient) Model() string { return c.model }

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *openAIClient) Chat(ctx context.Context, system, user string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("未配置大模型 API Key（RAG_LLM_API_KEY）")
	}
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	if c.temperature > 0 {
		body["temperature"] = c.temperature
	}
	if c.maxTokens > 0 {
		body["max_tokens"] = c.maxTokens
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("大模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("大模型响应解析失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := string(data)
		if parsed.Error != nil {
			msg = parsed.Error.Message
		}
		return "", fmt.Errorf("大模型接口错误 %d: %s", resp.StatusCode, msg)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("大模型未返回内容")
	}
	return parsed.Choices[0].Message.Content, nil
}

// FuncClient 测试与自定义场景用的函数式客户端。
type FuncClient struct {
	Fn   func(ctx context.Context, system, user string) (string, error)
	Name string
}

func (c *FuncClient) Chat(ctx context.Context, system, user string) (string, error) {
	return c.Fn(ctx, system, user)
}

func (c *FuncClient) Model() string {
	if c.Name == "" {
		return "mock"
	}
	return c.Name
}
