package httpapi

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"campus-service-platform/internal/pkg/buildinfo"
)

var startTime = time.Now()

// kafkaPinger 用于探测 Kafka 连通性（由 mq.Producer 实现）。
type kafkaPinger interface {
	Ping(ctx context.Context) error
}

// DepStatus 单个依赖的探测结果。
type DepStatus struct {
	Name      string `json:"name"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// versionPayload 版本与运行信息（状态页 / 探针共用）。
type versionPayload struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"build_time"`
	Pod           string `json:"pod"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func currentPod() string {
	if p := os.Getenv("HOSTNAME"); p != "" {
		return p
	}
	return os.Getenv("POD_NAME")
}

func (h *Handlers) versionInfo() versionPayload {
	return versionPayload{
		Status:        "ok",
		Version:       buildinfo.Version,
		Commit:        buildinfo.Commit,
		BuildTime:     buildinfo.Time,
		Pod:           currentPod(),
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
	}
}

// checkDeps 探测 MySQL / Redis / Kafka，返回每个依赖的状态。
func (h *Handlers) checkDeps(ctx context.Context) []DepStatus {
	deps := make([]DepStatus, 0, 3)

	// MySQL
	{
		d := DepStatus{Name: "mysql"}
		begin := time.Now()
		if sqlDB, dbErr := h.App.DB.DB(); dbErr != nil {
			d.Error = dbErr.Error()
		} else if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
			d.Error = pingErr.Error()
		} else {
			d.Healthy = true
			d.LatencyMs = time.Since(begin).Milliseconds()
		}
		deps = append(deps, d)
	}

	// Redis
	{
		d := DepStatus{Name: "redis"}
		begin := time.Now()
		if err := h.App.RDB.Ping(ctx).Err(); err != nil {
			d.Error = err.Error()
		} else {
			d.Healthy = true
			d.LatencyMs = time.Since(begin).Milliseconds()
		}
		deps = append(deps, d)
	}

	// Kafka
	{
		d := DepStatus{Name: "kafka"}
		begin := time.Now()
		if p, ok := h.App.Pub.(kafkaPinger); ok {
			if err := p.Ping(ctx); err != nil {
				d.Error = err.Error()
			} else {
				d.Healthy = true
				d.LatencyMs = time.Since(begin).Milliseconds()
			}
		} else {
			d.Healthy = true // 未实现 Ping 时视为不可判定，避免误报
			d.LatencyMs = time.Since(begin).Milliseconds()
		}
		deps = append(deps, d)
	}

	return deps
}

func allHealthy(deps []DepStatus) bool {
	for _, d := range deps {
		if !d.Healthy {
			return false
		}
	}
	return true
}

// Healthz 存活探针：进程存活即 200，不探测依赖（避免依赖抖动导致容器被反复重启）。
func (h *Handlers) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, h.versionInfo())
}

// Readyz 就绪探针：依赖全部健康才返回 200。
// 滚动更新时，旧 Pod 在新 Pod 就绪前持续服务，实现零中断。
func (h *Handlers) Readyz(c *gin.Context) {
	deps := h.checkDeps(c.Request.Context())
	code := http.StatusOK
	status := "ok"
	if !allHealthy(deps) {
		code = http.StatusServiceUnavailable
		status = "degraded"
	}
	payload := h.versionInfo()
	payload.Status = status
	c.JSON(code, gin.H{
		"status":  status,
		"version": payload.Version,
		"commit":  payload.Commit,
		"pod":     payload.Pod,
		"deps":    deps,
	})
}

// DeployStatus 状态页数据源：始终 200（即使降级也返回依赖详情，便于页面展示）。
func (h *Handlers) DeployStatus(c *gin.Context) {
	deps := h.checkDeps(c.Request.Context())
	ready := allHealthy(deps)
	payload := h.versionInfo()
	payload.Status = map[bool]string{true: "ok", false: "degraded"}[ready]
	c.JSON(http.StatusOK, gin.H{
		"ready":   ready,
		"status":  payload.Status,
		"version": payload.Version,
		"commit":  payload.Commit,
		"build_time": payload.BuildTime,
		"pod":       payload.Pod,
		"uptime_seconds": payload.UptimeSeconds,
		"deps":     deps,
	})
}
