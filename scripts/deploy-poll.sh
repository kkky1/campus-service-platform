#!/bin/bash
# 服务器端拉取式部署（pull-based CD）——安全版：
# 只允许 fast-forward，绝不 reset，绝不动本地未推送的提交。
set -e

REPO=/root/ykwork/campus-service-platform
cd "$REPO"

# 读取部署密钥（.env 不入库）
if [ -f .env ]; then
    set -a; . ./.env; set +a
fi

# 1. 工作区有未提交改动 → 跳过，保护开发中的工作
if [ -n "$(git status --porcelain)" ]; then
    echo "[deploy] 工作区有本地改动，跳过本次部署"
    exit 0
fi

# 2. 拉取远端
git fetch origin main -q
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)
if [ "$LOCAL" = "$REMOTE" ]; then
    echo "[deploy] 无新提交（$LOCAL）"
    exit 0
fi

# 3. 本地存在未推送提交（HEAD 不是 origin/main 的祖先）→ 跳过，防止覆盖本地开发
if ! git merge-base --is-ancestor HEAD origin/main; then
    echo "[deploy] 本地存在未推送提交（$(git rev-parse --short HEAD)），跳过部署以保护代码"
    exit 0
fi

# 4. 仅允许快进合并
git merge --ff-only origin/main -q
echo "[deploy] 已快进到 $(git rev-parse --short HEAD)"

# 5. 首次导入数据库（tb_user 表不存在时）
if ! docker exec hmdp-mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" hmdp -N -e 'SHOW TABLES LIKE "tb_user"' 2>/dev/null | grep -q tb_user; then
    echo "[deploy] 首次导入数据库 db/hmdp.sql"
    (echo "SET SESSION sql_mode='NO_ENGINE_SUBSTITUTION';"; cat db/hmdp.sql) | docker exec -i hmdp-mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" hmdp
fi

# 6a. 修正上传卷属主（容器以 uid 10001 运行）
docker run --rm -v hmdp-upload-data:/data alpine:3.20 chown -R 10001:10001 /data 2>/dev/null || true

# 6. 宿主机编译二进制（容器内下载依赖过慢），再构建瘦镜像并启动
export PATH=$PATH:/usr/local/go/bin
echo "[deploy] 编译 Go 二进制"
go build -o bin/campus-server ./cmd/server
echo "[deploy] docker compose 构建启动"
docker compose -f docker-compose.local.yml up -d --build --remove-orphans
docker image prune -f > /dev/null

# 7. 健康检查（轮询 60 秒）
echo "[deploy] 健康检查"
for i in $(seq 1 30); do
    if curl -sf http://127.0.0.1:8081/shop-type/list > /dev/null 2>&1; then
        echo "[deploy] 部署成功 ✅ ($(git rev-parse --short HEAD))"
        exit 0
    fi
    sleep 2
done
echo "[deploy] 健康检查超时，打印 backend 日志："
docker compose -f docker-compose.local.yml logs backend --tail 50
exit 1
