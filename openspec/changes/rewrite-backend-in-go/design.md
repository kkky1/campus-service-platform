# Design

## Context

后端由 Spring Boot 2.3 / Java 8 / MyBatis-Plus 重写为 Go（动机见 proposal.md - Why）。约束：前端静态资源、nginx、MySQL 表结构（hmdp.sql）、Redis 键名与 TTL、HTTP API 契约（见 specs/）全部不变；RabbitMQ 替换为 Kafka。目标 Go 版本 1.22+（工程用 1.24），单二进制部署。

## Goals / Non-Goals

**Goals**
- 单 Go 模块，标准布局（`cmd/` + `internal/`），`go build` 产出单二进制
- 对外行为与 specs/ 逐条一致（含 null 省略、时间格式、401、错误文案）
- docker compose 一键起全套：mysql / redis / kafka / backend / nginx

**Non-Goals**
- 不改前端、不改 SQL、不改 Redis 数据结构与键名
- 不新增业务功能（登出仍保留"功能未完成"占位行为）
- 不做微服务拆分、不做水平扩展设计（单实例部署即可）

## Decisions

### D1. 工程布局

```
campus-service-platform/
├── go.mod                        # module campus-service-platform
├── cmd/server/main.go            # 装配：config → db → redis → kafka → router → consumer
├── internal/
│   ├── config/                   # YAML + 环境变量覆盖
│   ├── httpapi/                  # handler（按模块分文件）+ 路由注册
│   ├── middleware/               # TokenRefresh / LoginAuth / Recovery
│   ├── service/                  # 业务层（shop/blog/seckill/user）
│   ├── repo/                     # GORM 模型 + 自定义 SQL
│   ├── mq/                       # Kafka producer + 主消费者 + DLT 消费者
│   └── pkg/
│       ├── cache/                # CacheClient（逻辑过期/空值/互斥重建）、RedisIdWorker
│       ├── rds/                  # Redis 键常量、Lua 脚本（go:embed）
│       └── dto/                  # Result/ScrollResult/UserDTO 等
├── scripts/lua/seckill.lua       # 嵌入用（去除 XADD 行）
├── Dockerfile                    # 多阶段构建
└── front/、nginx.local.conf      # 不动
```

选择原因：模块边界与 Java 包结构一一对应（controller→httpapi、service→service、mapper→repo、utils→pkg），迁移时逐文件对照，降低遗漏风险。备选 flat layout 对 40+ 路由规模不够清晰。

### D2. HTTP 框架与路由

用 **Gin**。理由：中间件模型与双拦截器（refresh/auth）一一对应；`gin.Context.Set/Get` 承载用户上下文；静态路由优先规则解决 `/blog/hot` 与 `/blog/{id}` 冲突（Gin 树中静态节点优先于参数节点，与 Spring 一致）。

- 自定义 `Recovery` 中间件等价 `WebExceptionAdvice`：panic 时记录日志，返回 200 + `{"success":false,"errorMsg":"服务器异常"}`。
- 401 由 `LoginAuth` 中间件 `c.AbortWithStatus(401)`（空响应体）。
- 白名单集合在路由注册处声明（`/user/login`、`/user/code`、`/shop/*`、`/shop-type/*`、`/voucher/*`、`/upload/*`、`/blog/hot`），未命中白名单的其余路由均需登录。

### D3. ORM 与数据访问

用 **GORM**。理由：链式查询（`Where/Order/Raw`）与 MyBatis-Plus 用法最接近；`ORDER BY FIELD(id,...)` 用 `Order("field(id, ?)", ids)` 表达；自增 id 写入回填自动完成。备选 sqlx 更显式但样板多，收益有限。

- 每模型显式 `TableName()`（GORM 默认蛇形复数会得到 `tb_users`，必须显式指定 `tb_user` 等）。
- 保留的自定义 SQL：券列表 LEFT JOIN（`queryVoucherOfShop`）；`FIELD()` 排序三处（附近店铺、点赞榜、feed）；原子库存扣减 `UPDATE tb_seckill_voucher SET stock=stock-1 WHERE voucher_id=? AND stock>0`。
- 秒杀券发布用 GORM 事务（等价 `@Transactional`）。
- 时间字段统一 `time.Time`，读写格式见 D6。

### D4. Redis（go-redis v9）

- **键与 TTL 常量表**（来自 RedisConstants，全部保留）：`login:code:{phone}` 2min、`login:token:{token}` 30min、`cache:shop:{id}` 逻辑过期 30min、`lock:shop:{id}` 10s、`seckill:stock:{voucherId}`、`seckill:order:{voucherId}`、`icr:order:{yyyy:MM:dd}`、`shop:geo:{typeId}`、`shop_type:`（List 无 TTL）、`blog:liked:{id}`、`feed:{userId}`、`follows:{userId}`、`sign:{userId}:{yyyyMM}`。
- **逻辑过期缓存**：`cache:shop:{id}` 存 `{"data":{...},"expireTime":"..."}`；未命中直接返回失败（与原行为一致，缓存需预热）；已过期返回旧值并异步重建（互斥锁 `lock:shop:{id}` SETNX EX 10 + 重建后 DEL）。重建线程池用有界 goroutine 池（10 worker，等价原 fixedThreadPool(10)）。
- **Lua 脚本**：`seckill.lua` 去除末尾 `XADD` 行后 `go:embed` 进二进制，`Eval` 一次往返。注意 Java 对 `-1` 的分支处理为"库存不足"（本变更已修复文案映射）。
- **ID worker**：`INCR icr:order:{yyyy:MM:dd}` → `(nowSeconds-1640995200)<<32 | count`，count 取低 32 位。
- **GEO**：`GEOSEARCH` 半径 5000m 带距离，参数语义与原 `search(key, from(x,y), 5000m, limit(end))` 一致：先按距离排序截取前 `current*5`，再跳过 `(current-1)*5`。
- **BITFIELD**：签到统计 `BITFIELD GET u{dayOfMonth} 0`，统计低位连续 1。

### D5. Kafka（segmentio/kafka-go）

选 **segmentio/kafka-go** 而非 sarama：消费者组 API 更简单、无 CGO、文档清晰，本项目规模下足够。备选 sarama 生态更老牌，但样板多，不做取舍。

- 主题：`seckill.order`（3 分区）、`seckill.order.dlt`。消息体 JSON `{"id":...,"userId":...,"voucherId":...}`（与原 RabbitMQ 消息一致）。
- 生产者：`Writer` 同步写，`RequiredAcks=All`；发送失败 → 抛出（等价原"发送消息失败"路径，经 Recovery 中间件返回"服务器异常"）。
- 主消费者：`ReaderGroup`（组 `seckill-order-group`），手动 commit。处理失败重试 3 次（100ms/500ms/1s 指数退避），仍失败投递 `seckill.order.dlt` 并 commit（保证不阻塞后续消息）。
- DLT 消费者：独立 group，同一处理逻辑，幂等兜底。
- **幂等下单**：`INSERT INTO tb_voucher_order (id,user_id,voucher_id) VALUES ... ON DUPLICATE KEY UPDATE id=id`，`RowsAffected==1` 才执行库存扣减（`... WHERE voucher_id=? AND stock>0`）。这修复了原 RabbitMQ auto-ack 下重投可能造成的重复扣库存缺陷。
- 配置：`KAFKA_BROKERS`（默认 `localhost:9092`），compose 内单节点 KRaft。

### D6. JSON 契约（最关键的一致性风险）

Java 的 Jackson `non_null` 语义 = "null 省略、零值输出"。Go 默认 `omitempty` 会把零值也省略，**必须用指针策略复刻**：

- DTO/实体中所有可空字段声明为指针类型（`*int64`、`*string`、`*bool`、`*float64`、`*time.Time`）并加 `omitempty`。nil → 省略；非 nil 零值（如 `liked=0`、`isLike=false`）→ 正常输出。string 字段同规则（区分 null 与 ""，如 `icon=""` 必须输出空串）。
- 时间格式复刻 Spring Boot 2.3 Jackson 默认：`LocalDateTime` → `"2006-01-02T15:04:05"`、`LocalDate` → `"2006-01-02"`，自定义 `MarshalJSON`（不带时区后缀）。
- `Result` 结构：`success` 恒输出；`errorMsg`/`data`/`total` 各自 `omitempty`（`total` 用 `*int64`）。本系统无接口返回 total，保持可选。
- 契约测试：对每个接口用 httptest 断言 JSON 键集合（而非仅值），防止意外多键/少键。

### D7. 认证中间件

- `TokenRefresh`（全部路由）：读 `authorization` 头 → `HGETALL login:token:{token}` → 命中则写入 context 并 `EXPIRE` 30min；未命中放行。
- `LoginAuth`（非白名单路由）：context 无用户 → 401 空响应体。
- 用户上下文结构 `{Id, NickName, Icon}`，token hash 字段同名小写驼峰（`id`/`nickName`/`icon`）。

### D8. 上传与静态资源

- 上传目录从配置读取（`UPLOAD_DIR`，compose 挂载 volume），默认 `/data/uploads`。
- 目录哈希复刻 Java `String.hashCode`（int32 溢出语义，`h = 31*h + c`），保证同一 UUID 生成与 Java 版相同的 `/blogs/{d1}/{d2}/{uuid}.{ext}` 路径。
- 删除接口同样拒绝目录路径（"错误的文件名称"）。

### D9. 配置与部署

- `config.yaml` 默认值与原 `application.yaml` 对齐并修正矛盾：MySQL `hmdp`/`123456`（与 compose 一致）、Redis `127.0.0.1:6379`、端口 8081；环境变量覆盖（`MYSQL_DSN`、`REDIS_ADDR`、`KAFKA_BROKERS`、`UPLOAD_DIR`）。
- `Dockerfile`：golang:1.24-alpine 构建（CGO_ENABLED=0）→ distroless/static 运行，约 20MB。
- `docker-compose.local.yml`：移除 rabbitmq，新增 kafka（bitnami/kafka KRaft 单节点）与 backend 服务（依赖 mysql/redis/kafka 健康检查）；nginx 代理目标改为 backend 服务名；mysql/redis 配置不变。

### D10. 测试策略

- 单元测试（`go test`）：ID worker 位运算、Java hashCode 目录哈希、feed 滚动 offset 计算、签到位统计、Result JSON 键集合序列化。
- 集成测试（可选跑，依赖 compose 起的 mysql/redis）：Lua 抢券资格校验 + 幂等消费 + 库存扣减；逻辑过期缓存重建。Kafka 部分用 mock 或真实 broker。
- 契约测试：httptest 覆盖全部 40+ 路由的 JSON 键集合与状态码。

## Risks / Trade-offs

- [JSON 序列化差异（null/零值/时间/长整型）导致前端解析差异] → 指针字段策略 + 自定义时间 MarshalJSON + 契约测试逐接口断言键集合
- [Java String.hashCode 复刻错误导致图片路径不一致] → 单元测试用固定 UUID 断言路径（对照 Java 实现）
- [GORM 与 MyBatis-Plus 语义差异（默认表名、更新时间等）] → 显式 TableName、仅依赖显式 SQL/更新，避免隐式自动行为
- [Kafka at-least-once 重复投递造成重复下单/重复扣库存] → ON DUPLICATE KEY 幂等插入 + RowsAffected 判定后再扣库存
- [GEO 搜索语义偏差（半径、分页 skip/limit 顺序）] → 保持"先 limit end 再 skip from"原语义，集成测试比对
- [逻辑过期缓存未预热时接口返回失败（原系统同样行为）] → 保留行为，提供预热辅助（测试/脚本），文档注明

## Migration Plan

1. 建 Go 工程骨架 + 配置加载 + 路由空壳，`go build` 通过。
2. 按依赖顺序迁移模块：http-contract/auth → user-profile → shop → social → seckill（每个模块完成后跑对应契约测试）。
3. 更新 docker-compose（kafka + backend），`docker compose up` 全链路自测（登录 → 逛店 → 发动态 → 抢券 → 查订单）。
4. 契约基线比对：对原 Java 版（可临时在宿主机跑）与 Go 版跑同一批 curl 断言，确认响应一致。
5. 切换：删除 `src/`、更新 README 部署说明。
6. 回滚：git 历史保留 Java 完整代码；若需回退，`git revert` 即可（MQ 侧 Kafka 为一次性替换，无双向同步要求）。

## Open Questions

无。已在本轮探索中确认：保真度 B、技术栈、Kafka 语义 (b)、工程形态、数据库配置统一为 hmdp/123456。
