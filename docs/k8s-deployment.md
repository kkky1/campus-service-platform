# Kubernetes 部署（k3s 单节点）

本文档说明校园服务平台从 Docker Compose 迁移到 Kubernetes（k3s 单节点）后的部署方式，
覆盖三个关键步骤：**可视化打包（镜像构建）→ 微服务打包 → 微服务滚动更新**，以及**可视化状态页**。

## 1. 架构

```
GitHub(main) ──push──> GitHub Actions ──打包(可视化流水线)──> ghcr.io 镜像仓库
                                                                   │
                                        kubectl set image + rollout status（SSH）
                                                                   ▼
                              本机 k3s 单节点（namespace: campus）
     ┌─────────┬─────────┬──────────┬────────────────┬──────────────────────┐
     │ mysql-0 │ redis   │ kafka-0  │ backend ×2     │ frontend ×2          │
     │ (Stateful│(Deploy) │(Stateful │ (Deployment,   │ (Deployment,         │
     │  Set)   │         │  Set)    │  RollingUpdate)│  RollingUpdate)      │
     └─────────┴─────────┴──────────┴────────────────┴──────────────────────┘
                                              ▲
                              frontend NodePort :8080 ← 对外入口（同旧方案）
```

- 有状态组件用 StatefulSet + PVC（`local-path` 动态供给）：`mysql`、`kafka`。
- 无状态微服务用 Deployment + 滚动更新策略：`backend`、`frontend`、`redis`。
- 对外入口：`frontend` Service 为 `NodePort: 8080`（与旧 compose 方案宿主机端口一致）。

## 2. 三个关键步骤

### 2.1 可视化打包（镜像构建 + 推送）

`GitHub Actions` 工作流 `.github/workflows/k8s-cd.yml` 提供**可视化构建流水线**（Actions UI 中可看到每个阶段的执行状态）：

| 阶段 | 内容 |
|---|---|
| 打包后端镜像 | `docker build -f Dockerfile`，注入 `VERSION/COMMIT/BUILD_TIME`（供状态页展示），推送到 `ghcr.io/<owner>/<repo>/backend` |
| 打包前端镜像 | `docker build -f Dockerfile.front`，推送到 `ghcr.io/<owner>/<repo>/frontend` |

### 2.2 微服务打包（镜像产物）

| 镜像 | Dockerfile | 说明 |
|---|---|---|
| `backend` | `Dockerfile`（多阶段：golang → alpine，约 15MB） | Go 后端，`-ldflags` 注入版本信息 |
| `frontend` | `Dockerfile.front`（nginx:1.25-alpine） | Vue 静态资源 + nginx 反代 `/api` |

本地（无 Actions）也可手动打包：

```bash
./scripts/build.sh                          # 编译带版本信息的 bin/campus-server
docker build -f Dockerfile.prebuilt -t ghcr.io/<owner>/<repo>/backend:latest .
docker build -f Dockerfile.front  -t ghcr.io/<owner>/<repo>/frontend:latest .
docker save <镜像> | k3s ctr images import -   # 导入 k3s containerd（离线/加速）
```

### 2.3 微服务滚动更新（零中断）

`backend` / `frontend` Deployment 配置：

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0     # 更新期间不允许“可用副本”少于目标值
    maxSurge: 1           # 先多起 1 个新副本，就绪后再摘旧副本
```

配合就绪探针：

- `backend` 就绪探针 `GET /readyz`：依赖 MySQL/Redis/Kafka **全部健康**才返回 200。
- `frontend` 就绪探针 `GET /`。

效果：新 Pod 就绪 → 加入 Service 后端 → 旧 Pod 才被终止，全程可用副本数不下降。
GitHub Actions 中用 `kubectl set image` + `kubectl rollout status` 驱动并等待完成：

```bash
kubectl -n campus set image deployment/backend backend=ghcr.io/<owner>/<repo>/backend:<sha>
kubectl -n campus rollout status deployment/backend --timeout=180s
kubectl -n campus set image deployment/frontend nginx=ghcr.io/<owner>/<repo>/frontend:<sha>
kubectl -n campus rollout status deployment/frontend --timeout=120s
```

## 3. 可视化状态页

后端新增三个免登录接口（`internal/httpapi/health.go`）：

| 接口 | 用途 | 返回 |
|---|---|---|
| `GET /healthz` | 存活探针（进程存活即 200） | 版本/提交/构建时间/Pod/运行时长 |
| `GET /readyz` | 就绪探针（依赖健康才 200） | 版本 + 各依赖健康状态 |
| `GET /deploy/status` | 状态页数据源（恒 200） | 版本 + 依赖 + 就绪标志 + Pod |

前端新增 `front/html/hmdp/status.html`：访问 `http://<宿主机>:8080/status.html`，
每 5 秒轮询 `/api/deploy/status`，可视化展示：

- 当前版本 / Git 提交 / 构建时间 / 服务 Pod / 运行时长
- MySQL、Redis、Kafka 依赖健康状态（绿/红点 + 延迟）
- 滚动更新提示：检测到版本变化时高亮显示「滚动更新进行中」

## 4. 目录结构

```
deploy/k8s/
├── 00-namespace.yaml      # 命名空间 campus
├── 01-configmap.yaml      # backend config.yaml + nginx.conf
├── 10-mysql.yaml          # StatefulSet + headless Svc（首次导入 hmdp.sql）
├── 20-redis.yaml          # Deployment + Svc + PVC
├── 30-kafka.yaml          # StatefulSet + headless Svc（KRaft 单节点）
├── 40-backend.yaml        # Deployment（RollingUpdate + 探针）+ Svc
├── 50-frontend.yaml       # Deployment（RollingUpdate + 探针）+ NodePort Svc
├── 60-pvcs.yaml           # uploads 共享卷（backend rw / frontend ro）
├── kustomization.yaml
├── initdb/01-app-user.sh  # MySQL 首次初始化：创建 campus 账号
└── scripts/
    ├── apply.sh           # 从 .env 生成 Secret → apply -k
    ├── migrate.sh         # 旧 compose → K8s 数据迁移（backup/restore）
    └── secrets.env.example
```

## 5. 部署操作

```bash
# ① 准备密钥（.env 已存在于仓库根目录，勿提交）
# ② 一键部署
./deploy/k8s/scripts/apply.sh

# 查看状态
kubectl -n campus get deploy,sts,pods,svc
curl http://127.0.0.1:8080/api/deploy/status
```

从旧 compose 迁移数据（已执行一次，供参考）：

```bash
./deploy/k8s/scripts/migrate.sh backup    # 旧 compose 运行时导出 MySQL + uploads
# …… 停掉旧栈、安装 k3s、apply.sh 部署完成 ……
./deploy/k8s/scripts/migrate.sh restore   # 导入 MySQL + 恢复 uploads
```

## 6. 接通 GitHub Actions（一次性配置）

### 6.1 仓库 Secrets（GitHub → Settings → Secrets and variables → Actions）

| Secret | 值 |
|---|---|
| `K8S_SSH_HOST` | VPS 公网地址（如 `103.236.98.137`） |
| `K8S_SSH_PORT` | SSH 端口（NAT 映射端口，如 `64540`） |
| `K8S_SSH_USER` | `root` |
| `K8S_SSH_KEY` | 部署专用私钥（见下） |

生成 SSH 部署密钥（本机）：

```bash
ssh-keygen -t ed25519 -f /root/.ssh/gh-actions -N '' -C 'github-actions-deploy'
cat /root/.ssh/gh-actions.pub >> /root/.ssh/authorized_keys   # VPS 侧授权
cat /root/.ssh/gh-actions        # 把私钥内容粘到 GitHub Secret K8S_SSH_KEY
```

### 6.2 ghcr.io 镜像拉取凭据（集群侧）

GitHub Actions 用 `GITHUB_TOKEN` 推送镜像到 ghcr.io（仓库私有包）。集群拉取需要
`read:packages` 权限的 **PAT**：

```bash
# 在 GitHub 生成 PAT（勾选 read:packages），然后：
GITHUB_USER=<用户名> GITHUB_PAT=ghp_xxx ./deploy/k8s/scripts/apply.sh
# 或单独重建：
kubectl -n campus create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io --docker-username=<用户名> --docker-password=ghp_xxx \
  --dry-run=client -o yaml | kubectl apply -f -
```

> 当前 `ghcr-secret` 为占位值（本地导入镜像 + `imagePullPolicy: IfNotPresent` 无需拉取）。
> 接通 Actions 后务必用真实 PAT 重建，否则 `kubectl set image` 到新 `<sha>` 时无法拉取。

## 7. 验证滚动更新

```bash
# 观察更新过程中新旧 Pod 交替（maxSurge=1 → 短暂出现 3 个 Pod → 旧 Pod 终止）
kubectl -n campus rollout restart deployment/backend
kubectl -n campus rollout status deployment/backend --timeout=90s

# 状态页上滚动更新期间可看到「版本/Pod」随流量在旧新副本间切换
# 打开 http://<宿主机>:8080/status.html
```

## 8. 回滚

```bash
kubectl -n campus rollout undo deployment/backend      # 回滚上一次发布
kubectl -n campus rollout undo deployment/frontend
# 回滚到旧 compose：k3s 不影响 docker，旧容器/卷仍保留，可 docker compose -p campus-service-platform start
```

## 9. 运维要点

1. **内存**：3.8G 机器已停掉 Jenkins 与旧 compose 栈，k3s（禁用 traefik/servicelb）+ 全套工作负载约 1.5G，余量充足。
2. **镜像加速**：`/etc/rancher/k3s/registries.yaml` 配置了 Docker Hub 镜像源（daocloud/1panel/xuanyuan），否则 k3s containerd 拉不动 `docker.io`。
3. **NodePort 8080**：k3s 安装参数 `--service-node-port-range=8080-32767` 允许低端口 8080，保持对外 NAT 映射不变。
4. **kafka headless**：KRaft 组合模式 broker 需通过 `kafka:9093` 连接自身 controller，必须用 `clusterIP: None` + `publishNotReadyAddresses: true`，否则起不来。
5. **上传卷共享**：单节点下 `ReadWriteOnce` 的 `uploads` PVC 由 backend（读写）与 frontend（只读）同时挂载，配合 `fsGroup: 10001` 保证容器 uid 10001 可写。
