# Jenkins 自动化部署（CD）说明

原「systemd 定时拉取」方案已停用，自动化部署迁移到 Jenkins。

```
GitHub(main) ──每2分钟 SCM 轮询──> Jenkins ──> 检出 → 构建测试 → 编译 → docker compose 部署 → 健康检查
                                        │
                                        └── 使用服务器 .env 密钥（只读挂载，不落日志）
```

## 组成

| 部件 | 说明 |
|---|---|
| `docker-compose.jenkins.yml` | Jenkins LTS 容器（独立 compose 项目 `jenkins`，避免与业务部署冲突） |
| `jenkins/init.groovy.d/` | 首次启动自动初始化：安全域/管理员、SSH 部署密钥、known_hosts、流水线任务 |
| `Jenkinsfile` | 流水线定义（检出/准备密钥/构建与测试/编译二进制/部署/健康检查） |
| `scripts/deploy-poll.sh` | 原 systemd 方案脚本，保留作为回退（定时器已 `disable`） |

## 访问与账号

- 地址：本机 `127.0.0.1:8090`（仅绑定本机）。远程访问走 SSH 隧道：
  `ssh -p 64540 -L 8090:127.0.0.1:8090 root@103.236.98.137`，然后打开 `http://localhost:8090`
- 管理员：`admin`，密码在服务器 `.env` 的 `JENKINS_ADMIN_PASSWORD`

## 触发

- **自动**：SCM 每 2 分钟轮询 `main`，检测到新提交即构建（等价原定时拉取语义）
- **手动**：Jenkins 页面「Build Now」，或用 REST：
  ```bash
  CRUMB=$(curl -s -c /tmp/jc -u admin:$PASS "http://127.0.0.1:8090/crumbIssuer/api/json" | jq -r '.crumb')
  curl -b /tmp/jc -u admin:$PASS -H "Jenkins-Crumb:$CRUMB" -X POST \
       "http://127.0.0.1:8090/job/campus-service-platform-cd/build"
  ```

## 运维要点（踩过的坑）

1. **内存**：3.8G 小机器需三件套——2G swap、`go build/test -p 1`、Kafka/Jenkins 堆限制（Kafka 384M、Jenkins 512M）。
2. **项目名隔离**：Jenkins 容器必须独立 compose 项目名（`name: jenkins`），否则业务部署的 `--remove-orphans` 会把它当孤儿删除。
3. **密钥不进日志**：流水线含密钥的 `sh` 块首行 `set +x`（Jenkins 默认 `-x` 会回显 `.env`）。
4. **集成测试**：Jenkins 容器接入业务网络 `campus-service-platform_default`，通过服务名访问 mysql/redis/backend。

## 启动 / 停止

```bash
docker compose -f docker-compose.jenkins.yml up -d     # 启动
docker compose -f docker-compose.jenkins.yml stop      # 停止
docker compose -f docker-compose.jenkins.yml logs -f   # 日志
```

---

# 构建流程详解

## 1. 组件拓扑

```
GitHub (main)                    服务器 (3.8G 小机器)
+-------------+   git ls-remote   +------------------------------------------+
|             | <---------------- | Jenkins 容器 (项目名: jenkins, 2G 上限)    |
|  仓库 main   |   每 2 分钟轮询    |  工作区: /var/jenkins_home/workspace/...   |
+-------------+                   |    |                                      |
      ^                           |    | 挂载:                                 |
      | git push (SSH 部署密钥)     |    |  /var/run/docker.sock  → 操作宿主Docker |
      |                           |    |  /usr/local/go (ro)    → Go 工具链      |
      |                           |    |  /root/go (rw)         → 模块/构建缓存   |
      |                           |    |  .env (ro)             → 部署密钥        |
      |                           |    |  id_ed25519 (ro)       → GitHub 拉取     |
      |                           |    +----|---------------------------------+
      |                           |         | docker compose -p campus-service-platform
      |                           |         v
      |                           |  +-------------------------------------+
      +---- SSH:22 ----< 拉取代码  |  | mysql  redis  kafka  backend  nginx |  ← 业务网络
                                  |  +-------------------------------------+
                                  +------------------------------------------+
```

要点：Jenkins 自己**不跑 Docker daemon**，而是通过挂载宿主机的 `/var/run/docker.sock` + docker CLI 直接操作宿主 Docker（Docker-outside-of-Docker）；构建产物（二进制/镜像）直接落到宿主机 Docker。

## 2. 触发机制（SCM 轮询）

- 任务配置 `SCMTrigger: H/2 * * * *`：Jenkins 每约 2 分钟用部署密钥执行一次 `git ls-remote`。
- 只有当远端 `main` 的 commit 与上次构建的 revision **不同**时才真正触发构建（所以叫"轮询"，不是定时无脑跑）。
- 等价于原 systemd 方案"每 2 分钟拉取一次"的语义，但由 Jenkins 统一管理、有构建历史与控制台日志。
- 也可手动：Jenkins 页面 **Build Now**，或 REST API（见上文）。
- 触发后 `Obtained Jenkinsfile from git ...`：Jenkins 先读仓库根的 `Jenkinsfile`，再按其执行。

## 3. 六个阶段（以真实构建 #7 为例，37 秒完成）

| 阶段 | 实际动作 | 为什么 |
|---|---|---|
| ① 检出 | `checkout scm`：浅克隆（depth=1）到 Jenkins 工作区；打印当前提交 | 拿最新代码；浅克隆适应国内网络 |
| ② 准备密钥 | `cp /run/secrets/app.env .env`（只读挂载 → 工作区） | compose 与脚本需要密钥，但不入库 |
| ③ 构建与测试 | `set +x` → `source .env` → 注入 `TEST_MYSQL_DSN/TEST_REDIS_ADDR/TEST_REDIS_PASSWORD` → `go build -p 1` → `go vet` → `go test -p 1`（14 包全绿） | 质量门禁；集成测试通过**业务网络服务名**连真实 mysql/redis；`-p 1` 防止小内存机器编译 OOM |
| ④ 编译二进制 | `go build -o bin/campus-server ./cmd/server` | 供瘦镜像 `Dockerfile.prebuilt` 直接 COPY（避免容器内下载依赖） |
| ⑤ 部署 | ①上传卷 chown ②首次检测 `tb_user` 不存在则导入 `db/hmdp.sql` ③`docker compose -p campus-service-platform up -d --build --remove-orphans` ④清理悬空镜像 | 接管既有容器（固定项目名）；compose 只重建变化的服务（日志可见 backend/nginx `Recreate`，mysql/redis/kafka `Running`） |
| ⑥ 健康检查 | `curl http://backend:8081/shop-type/list`，最多 30 次 × 2 秒 | 部署后门禁；失败则构建红，并触发 post 打印 backend 日志 |
| ⑦ RAG 评估（AI 随机数据） | 从知识库随机抽片段 → LLM 生成「问题+标准答案」→ 跑 `/rag/eval/run`（自动检索+生成）→ 输出报告与阈值判定 | 每次 CD 自动评测 RAG 效果；低于阈值构建失败 |

> RAG 评估可调参数：`RAG_EVAL_COUNT`（样本数，默认 3）、`RAG_EVAL_KB_ID`（默认 10）、
> `RAG_EVAL_ENFORCE=false`（只告警不失败）、`RAG_EVAL_MIN_*`（各指标阈值）。
> 报告归档在构建产物 `reports/rag-eval.md`；本地可执行 `go run ./cmd/rageval` 手动评测。

**部署阶段 Docker 具体发生了什么**：
1. `Dockerfile.prebuilt` 基于 `alpine` COPY `bin/campus-server` 二进制 → backend 镜像秒级构建；
2. `Dockerfile.front` 打包 `front/` 静态资源 + nginx 配置 → nginx 镜像；
3. compose 对比容器状态：配置/镜像未变的（mysql/redis/kafka）保持 `Running`；变化的（backend/nginx）`Recreate`；
4. 数据卷（mysql/redis/kafka/upload）使用固定名字，重建容器不丢数据。

## 4. 关键设计取舍（踩坑总结）

1. **Docker-outside-of-Docker**：Jenkins 容器直接驱动宿主 Docker，因此必须用固定项目名 `-p campus-service-platform` 才能接管既有容器；同时 Jenkins 自身用独立项目名 `jenkins`，否则业务部署的 `--remove-orphans` 会把 Jenkins 当孤儿容器杀掉。
2. **复用宿主工具链**：Go 装在宿主机（只读挂载），模块/构建缓存挂在 Jenkins 数据卷（`/root/go`）→ 第二次构建起全部命中缓存（`ok ... (cached)`）。
3. **密钥三防**：只读挂载（容器内无写权限）、`set +x` 阻止回显、`.env` 已 gitignore 不入库。
4. **小内存机器**：2G swap + Kafka 堆 384M + Jenkins 堆 512M/上限 2G + `-p 1` 串行编译，构建峰值内存从 ~1GB+ 压到安全线内。
5. **集成测试联网**：Jenkins 容器接入业务网络 `campus-service-platform_default`，通过 `mysql:3306`/`redis:6379`/`backend:8081` 访问，而不是宿主 127.0.0.1（容器内的 127.0.0.1 不是宿主机）。

## 5. 一次构建的完整时序（构建 #7 实测）

```
14:00:15  轮询检测到 main 有新提交 → 触发构建 #7 (Started by an SCM change)
14:00:15  读取 Jenkinsfile → 分配执行器（disableConcurrentBuilds，同一时刻只跑一个）
14:00:16  ① 检出：浅克隆 5ca70da → 工作区
14:00:18  ② 准备密钥：.env 就位
14:00:19  ③ 构建与测试：go build/vet/test（全部命中缓存，14 包）
14:00:40  ④ 编译二进制：bin/campus-server
14:00:42  ⑤ 部署：compose 重建 backend/nginx，其余保持 Running
14:00:50  ⑥ 健康检查：第 1 次探测即通过
14:00:52  SUCCESS（总耗时约 37 秒）
```

失败时的排查入口：Jenkins 页面构建号 → **Console Output**；post 阶段会自动附带 backend 最近 50 行日志。
