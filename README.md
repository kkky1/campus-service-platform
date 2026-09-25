# Campus Service Platform

面向高校学生的校园服务平台：校园店铺查询、校园福利券抢购、学生动态分享、关注互动、签到积分。后端为 Go 实现（由 Spring Boot 版本重写而来，对外行为等价），前端为静态资源（Vue + Element UI + axios），经 Nginx 反向代理访问后端。

## 技术栈

- 后端：Go 1.24、Gin、GORM、go-redis、segmentio/kafka-go、slog
- 存储与中间件：MySQL 8、Redis（6.2+）、Kafka（KRaft 单节点即可）
- 前端：Vue、Element UI、Axios、Nginx 静态资源服务
- 部署：Docker Compose、Nginx 反向代理、多阶段构建（~20MB 镜像）

## 核心功能

- 手机验证码登录：Redis 存储登录 Token（Hash，30 分钟），双中间件（Token 刷新 / 登录校验），401 语义不变。
- 校园服务查询：店铺类型列表（Redis List 缓存）、店铺详情（逻辑过期缓存 + 互斥锁异步重建）、附近店铺（Redis GEO 5000m 排序分页）。
- 校园福利券抢购：Redis + Lua 原子完成库存校验、防重复抢券、扣减库存。
- 异步下单削峰：抢券资格校验通过后发布 Kafka 消息，消费者幂等建单（`INSERT ... ON DUPLICATE KEY`）并条件扣库存；失败重试 3 次（指数退避）后进入 DLT 死信主题由兜底消费者处理。
- 社交动态：发布动态推送粉丝 feed（ZSET）、滚动分页、点赞切换、热门榜、关注与共同关注。
- 签到积分：Redis BITFIELD 位图签到与连续天数统计。
- 图片上传：hash 目录存储，路径格式与旧版一致（可配置上传根目录）。

## 本地构建与运行

前置：Go 1.24+。

```bash
go build -o bin/campus-server ./cmd/server
./bin/campus-server            # 默认读取 ./config.yaml
```

配置：`config.yaml`，支持环境变量覆盖：`SERVER_PORT`、`MYSQL_DSN`、`REDIS_ADDR`、`REDIS_PASSWORD`、`KAFKA_BROKERS`、`UPLOAD_DIR`、`CONFIG_PATH`。

## Docker Compose 一键起全套

```bash
docker compose -f docker-compose.local.yml up -d --build
```

- MySQL：初始化脚本 `db/hmdp.sql`（需首次导入：`docker exec -i hmdp-mysql mysql -uroot -p123456 hmdp < db/hmdp.sql`）
- Redis：`:6379`
- Kafka：`:9092`（KRaft 单节点，主题自动创建）
- 后端：`:8081`
- 前端 + Nginx：`http://localhost:8080`（`/api` 反向代理到后端）

## 测试

```bash
go test ./...                 # 单元 + 契约测试（内存 Redis/SQLite，无需外部依赖）
go test ./internal/service/   # 含集成测试：需要本地 Redis 127.0.0.1:6379 与 MySQL hmdp 库，不可用时自动跳过
python3 scripts/contract_check.py   # HTTP 契约基线：37 项断言（需服务已启动；登录分支需预置验证码，见脚本注释）
```

## 目录结构

```
cmd/server/          入口装配
internal/config/     配置加载
internal/httpapi/    路由与处理器（契约测试）
internal/middleware/ 认证双中间件 + 异常恢复
internal/service/    业务层（登录/店铺/动态/关注/秒杀/上传）
internal/repo/       GORM 模型（11 张表）与自定义 SQL
internal/mq/         Kafka 生产者 + 主消费者 + DLT 兜底消费者
internal/pkg/dto/    响应契约（JSON null 省略、时间格式）
internal/pkg/rds/    Redis 键与 TTL 常量
internal/pkg/cache/  （预留）
scripts/lua/         内嵌 Lua 脚本
db/                  hmdp.sql 初始化脚本
front/               前端静态资源（不变）
openspec/            OpenSpec 规格与变更
```

## OpenSpec

项目使用 OpenSpec 进行规格驱动开发。当前变更：`openspec/changes/rewrite-backend-in-go`（Spring Boot → Go 重写），其中 `proposal.md` / `design.md` / `specs/` 记录了行为契约与全部技术决策。
