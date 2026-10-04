#!/usr/bin/env bash
# 一键部署到 K8s：从仓库根目录 .env 生成 Secret → 生成 mysql-initdb ConfigMap → kubectl apply -k
# 用法：
#   ./deploy/k8s/scripts/apply.sh                # 复用 .env 中的密钥
#   可选环境变量：
#     GITHUB_USER / GITHUB_PAT   # 用于创建 ghcr.io 镜像拉取凭据（read:packages 权限的 PAT）
#     KUBECONFIG                  # 默认 /etc/rancher/k3s/k3s.yaml（k3s 单节点）
set -euo pipefail

cd "$(dirname "$0")/../../.."   # 仓库根目录
ENV_FILE=".env"
NS="campus"
K8S_DIR="deploy/k8s"

export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "❌ 未找到 .env，请先在仓库根目录准备密钥文件（参考 deploy/k8s/scripts/secrets.env.example）"
  exit 1
fi
set -a; source "$ENV_FILE"; set +a

: "${MYSQL_ROOT_PASSWORD:?.env 缺少 MYSQL_ROOT_PASSWORD}"
: "${MYSQL_APP_PASSWORD:?.env 缺少 MYSQL_APP_PASSWORD}"
: "${REDIS_PASSWORD:?.env 缺少 REDIS_PASSWORD}"

echo "==> 创建命名空间"
kubectl apply -f "$K8S_DIR/00-namespace.yaml"

echo "==> 生成 campus-secrets"
kubectl -n "$NS" create secret generic campus-secrets \
  --from-literal=MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
  --from-literal=MYSQL_APP_PASSWORD="$MYSQL_APP_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=MYSQL_DSN="campus:${MYSQL_APP_PASSWORD}@tcp(mysql:3306)/hmdp?charset=utf8mb4&parseTime=True&loc=Local" \
  --from-literal=RAG_LLM_API_KEY="${RAG_LLM_API_KEY:-}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> 生成 mysql-initdb ConfigMap（app 账号 + hmdp.sql）"
kubectl -n "$NS" create configmap mysql-initdb \
  --from-file=01-app-user.sh="$K8S_DIR/initdb/01-app-user.sh" \
  --from-file=02-hmdp.sql=db/hmdp.sql \
  --dry-run=client -o yaml | kubectl apply -f -

if [[ -n "${GITHUB_USER:-}" && -n "${GITHUB_PAT:-}" ]]; then
  echo "==> 生成 ghcr.io 镜像拉取凭据 ghcr-secret"
  kubectl -n "$NS" create secret docker-registry ghcr-secret \
    --docker-server=ghcr.io \
    --docker-username="$GITHUB_USER" \
    --docker-password="$GITHUB_PAT" \
    --dry-run=client -o yaml | kubectl apply -f -
else
  echo "⚠️  未设置 GITHUB_USER/GITHUB_PAT，生成占位 ghcr-secret（本地导入镜像无需拉取；启用 Actions 后请用真实 PAT 重建）"
  kubectl -n "$NS" create secret docker-registry ghcr-secret \
    --docker-server=ghcr.io \
    --docker-username=placeholder \
    --docker-password=placeholder \
    --dry-run=client -o yaml | kubectl apply -f -
fi

echo "==> 应用全部资源"
kubectl apply -k "$K8S_DIR"

echo ""
echo "==> 等待关键 Deployment 就绪"
kubectl -n "$NS" rollout status deployment/backend --timeout=300s
kubectl -n "$NS" rollout status deployment/frontend --timeout=120s

echo ""
echo "✅ 部署完成。状态页：http://<宿主机>:8080/status.html"
kubectl -n "$NS" get deploy,sts,pods -o wide
