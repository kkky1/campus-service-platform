#!/usr/bin/env bash
# 从旧 Docker Compose 迁移数据到 K8s。
#   ./deploy/k8s/scripts/migrate.sh backup    # 旧 compose 仍在运行时：导出 MySQL + 打包 uploads
#   ./deploy/k8s/scripts/migrate.sh restore   # K8s 就绪后：导入 MySQL + 解包 uploads
set -euo pipefail

cd "$(dirname "$0")/../../.."
ENV_FILE=".env"
BACKUP_DIR="${BACKUP_DIR:-/root/k8s-migration}"
NS="campus"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

set -a; source "$ENV_FILE"; set +a

backup() {
  mkdir -p "$BACKUP_DIR"
  echo "==> 导出 MySQL（容器 hmdp-mysql → $BACKUP_DIR/hmdp-live-dump.sql）"
  docker exec hmdp-mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases hmdp \
    --default-character-set=utf8mb4 --no-tablespaces --single-transaction --set-gtid-purged=OFF' \
    > "$BACKUP_DIR/hmdp-live-dump.sql"
  echo "    大小：$(du -h "$BACKUP_DIR/hmdp-live-dump.sql" | cut -f1)"

  echo "==> 打包 uploads（卷 hmdp-upload-data → $BACKUP_DIR/hmdp-uploads.tar.gz）"
  docker run --rm -v hmdp-upload-data:/data:ro -v "$BACKUP_DIR":/backup alpine:3.20 \
    tar czf /backup/hmdp-uploads.tar.gz -C /data . 2>/dev/null || true
  echo "    大小：$(du -h "$BACKUP_DIR/hmdp-uploads.tar.gz" | cut -f1)"
  echo "✅ 备份完成，产物在 $BACKUP_DIR"
}

restore() {
  echo "==> 等待 MySQL 就绪"
  kubectl -n "$NS" rollout status statefulset/mysql --timeout=300s

  echo "==> 导入 MySQL 数据（$BACKUP_DIR/hmdp-live-dump.sql）"
  if [[ -f "$BACKUP_DIR/hmdp-live-dump.sql" ]]; then
    kubectl -n "$NS" exec -i mysql-0 -- mysql -uroot -p"$MYSQL_ROOT_PASSWORD" \
      < "$BACKUP_DIR/hmdp-live-dump.sql"
    echo "✅ MySQL 导入完成"
  else
    echo "⚠️  未找到 $BACKUP_DIR/hmdp-live-dump.sql，跳过（使用种子数据 hmdp.sql）"
  fi

  echo "==> 等待 backend 就绪"
  kubectl -n "$NS" rollout status deployment/backend --timeout=300s

  if [[ -f "$BACKUP_DIR/hmdp-uploads.tar.gz" ]]; then
    echo "==> 恢复 uploads 到共享卷"
    cat "$BACKUP_DIR/hmdp-uploads.tar.gz" | kubectl -n "$NS" exec -i deploy/backend -- \
      tar xzf - -C /data/uploads
    echo "✅ uploads 恢复完成"
  else
    echo "⚠️  未找到 $BACKUP_DIR/hmdp-uploads.tar.gz，跳过"
  fi

  echo "✅ 迁移完成。访问状态页 http://<宿主机>:8080/status.html"
}

case "${1:-}" in
  backup)  backup ;;
  restore) restore ;;
  *) echo "用法: $0 {backup|restore}"; exit 1 ;;
esac
