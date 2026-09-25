# Proposal

## Why

现有后端基于 Spring Boot 2.3 + Java 8（均已停止维护），依赖重、部署镜像大，且代码中存在死代码与若干缺陷。为轻量化部署、简化维护并现代化技术栈，将后端整体重写为 Go，同时保持对外行为等价。

## What Changes

- 后端由 Spring Boot 2.3 / MyBatis-Plus / Java 8 重写为 Go（Gin + GORM + go-redis + Kafka + slog）。
- 前端静态资源、nginx 配置、MySQL 表结构（hmdp.sql）、Redis 键名与数据结构完全不变。
- RabbitMQ 替换为 Kafka（**BREAKING**：部署形态变更）。消息语义由「TTL 10s + 死信队列」改为「消费者重试 N 次 → DLT 主题 → 兜底消费者」，外部行为等价（订单最终被创建、存在兜底路径）。
- 清理死代码：seckill.lua 末尾 `XADD stream.orders`（无消费者）、Redisson / SimpleRedisLock / PasswordEncoder / AspectJ 全部死路径移除。
- 修复缺陷：登录 token TTL 统一为实际生效的 30 分钟；Lua 中库存 key 缺失时返回「库存不足」而非误报「不能重复下单」；订单消费幂等化（防重复建单与重复扣库存）；上传目录由硬编码 Windows 路径改为可配置。
- 数据库配置统一（**BREAKING**：本地环境）：原 application.yaml（库 `dingping`/密码 `290390`）与 docker-compose（库 `hmdp`/密码 `123456`）互相矛盾，统一为 `hmdp`/`123456`，保证 compose 一键跑通。
- 后端容器化并加入 docker-compose（多阶段构建），监听 8081，nginx 反向代理不变。
- `blog-comments` 无任何接口（仅表结构保留），不建立空路由。
- 对外契约保持不变：401 语义、免登录路径白名单、响应包 `{success, errorMsg, data, total}` 与 null 字段省略、分页尺寸（5/10）、GEO 搜索半径 5000m、订单号算法（`timestamp<<32|count`）、图片路径格式（`/blogs/{d1}/{d2}/{uuid}.{ext}`）、Redis 键名与 TTL。

## Capabilities

### New Capabilities

- `http-contract`: 全局响应包结构、JSON null 字段省略、异常响应与状态码契约
- `auth`: 手机验证码登录、Redis token 会话、双拦截器认证与 401 语义
- `shop`: 店铺/类型查询、逻辑过期缓存 + 空值缓存 + 互斥锁重建、GEO 附近搜索
- `seckill`: 优惠券管理、秒杀 Lua 原子校验、Kafka 异步下单与幂等、DLT 兜底
- `social`: 动态发布/点赞/热门/关注 feed 滚动分页、关注关系与共同关注
- `user-profile`: 用户信息查询、签到位图统计、图片上传与删除

### Modified Capabilities

无（项目首次建立 spec 基线）。

## Impact

- 代码：`src/` 下 76 个 Java 文件删除，替换为 Go 工程（`cmd/server` + `internal/`）。
- API：40+ HTTP 路由对外契约不变（JSON 形状、状态码、请求头）。
- 依赖：Spring Boot / MyBatis-Plus / Redisson / Spring AMQP → Gin / GORM / go-redis / kafka-go。
- 基础设施：docker-compose 中 rabbitmq 服务替换为 kafka（KRaft 单节点），新增 backend 服务（多阶段构建）。
- 数据：MySQL 使用原 hmdp.sql；Redis 键名、数据结构与 TTL 不变。
