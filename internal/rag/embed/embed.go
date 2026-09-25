// Package embed 文本向量化：本地确定性哈希（离线、测试友好）与 OpenAI 兼容接口。
package embed

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"campus-service-platform/internal/config"
	"campus-service-platform/internal/rag/chunker"
)

// Embedder 文本向量化接口。
type Embedder interface {
	Dim() int
	Encode(ctx context.Context, texts []string) ([][]float32, error)
}

// New 按配置构造 Embedder。
func New(cfg config.RAGConfig) Embedder {
	if cfg.Embedding.Provider == "openai" && cfg.Embedding.BaseURL != "" {
		dim := cfg.Embedding.Dim
		if dim <= 0 {
			dim = 1536
		}
		return &openAIEmbedder{
			baseURL: strings.TrimRight(cfg.Embedding.BaseURL, "/"),
			apiKey:  cfg.Embedding.APIKey,
			model:   cfg.Embedding.Model,
			dim:     dim,
			client:  &http.Client{Timeout: 30 * time.Second},
		}
	}
	dim := cfg.Embedding.Dim
	if dim <= 0 {
		dim = 384
	}
	return &LocalEmbedder{Dimension: dim}
}

// ---- 本地哈希 Embedding ----

// LocalEmbedder 基于特征哈希的确定性向量器：无需外部服务，离线可用。
type LocalEmbedder struct {
	Dimension int
}

func (e *LocalEmbedder) Dim() int { return e.Dimension }

// Encode 特征哈希 + L2 归一化。
func (e *LocalEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	dim := e.Dimension
	if dim <= 0 {
		dim = 384
	}
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec := make([]float32, dim)
		for _, tok := range chunker.Tokenize(text) {
			h := fnv.New64a()
			_, _ = h.Write([]byte(tok))
			sum := h.Sum64()
			idx := int(sum % uint64(dim))
			sign := float32(1)
			if sum&(1<<63) != 0 {
				sign = -1
			}
			vec[idx] += sign
		}
		// L2 归一化
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		if norm > 0 {
			n := float32(math.Sqrt(norm))
			for i := range vec {
				vec[i] /= n
			}
		}
		out = append(out, vec)
	}
	return out, nil
}

// ---- OpenAI 兼容 Embedding ----

type openAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	client  *http.Client
}

func (e *openAIEmbedder) Dim() int { return e.dim }

type openAIEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (e *openAIEmbedder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{"model": e.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed openAIEmbeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("embedding 响应解析失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := string(body)
		if parsed.Error != nil {
			msg = parsed.Error.Message
		}
		return nil, fmt.Errorf("embedding 接口错误 %d: %s", resp.StatusCode, msg)
	}
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	for i, v := range out {
		if len(v) == 0 {
			return nil, fmt.Errorf("embedding 第 %d 条缺失", i)
		}
	}
	if len(out) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}

// ---- 向量序列化 ----

// EncodeVector float32 小端序列化（存库用）。
func EncodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DecodeVector 反序列化。
func DecodeVector(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// Cosine 余弦相似度（0 向量返回 0）。
func Cosine(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
