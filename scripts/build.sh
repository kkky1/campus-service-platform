#!/usr/bin/env bash
# 编译后端二进制，注入构建信息（版本/提交/时间）供状态页与探针展示。
# 用法：scripts/build.sh [输出路径，默认 bin/campus-server]
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="${1:-bin/campus-server}"

VERSION="${VERSION:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

mkdir -p "$(dirname "$OUT")"
echo "构建 ${OUT}  version=${VERSION} commit=${COMMIT} time=${BUILD_TIME}"
# systemd/面板环境下缺省设置 Go 缓存路径（有则沿用现有值）
export GOPATH="${GOPATH:-/root/go}"
export GOMODCACHE="${GOMODCACHE:-/root/go/pkg/mod}"
export GOCACHE="${GOCACHE:-/root/go/build-cache}"
# 小内存机器：限制并行编译与 Go 运行并发，避免 OOM
export GOMAXPROCS=${GOMAXPROCS:-1}
CGO_ENABLED=0 go build -p 1 -trimpath \
  -ldflags "-s -w \
    -X campus-service-platform/internal/pkg/buildinfo.Version=${VERSION} \
    -X campus-service-platform/internal/pkg/buildinfo.Commit=${COMMIT} \
    -X campus-service-platform/internal/pkg/buildinfo.Time=${BUILD_TIME}" \
  -o "$OUT" ./cmd/server
echo "完成：$(ls -lh "$OUT" | awk '{print $5, $9}')"
