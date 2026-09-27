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
