# Tasks

## 1. 工程骨架

- [x] 1.1 初始化 go.mod（module campus-service-platform，go 1.24）并建立目录结构（cmd/server、internal/{config,httpapi,middleware,service,repo,mq}、internal/pkg/{cache,rds,dto}），`go build ./...` 通过
- [x] 1.2 实现配置加载（config.yaml + 环境变量覆盖：端口 8081、MySQL hmdp/123456、Redis 127.0.0.1:6379、KAFKA_BROKERS、UPLOAD_DIR），单测验证环境变量覆盖与默认值
- [x] 1.3 编写 Dockerfile（多阶段构建）与更新 docker-compose.local.yml（rabbitmq→kafka KRaft、新增 backend 服务、nginx 代理指向 backend），`docker compose config` 校验通过
- [x] 1.4 迁移 seckill.lua 至 scripts/lua/（去除 XADD 行）并 go:embed，单测断言嵌入内容不含 XADD

## 2. HTTP 契约与认证（http-contract + auth specs）

- [x] 2.1 实现 Result/ScrollResult/UserDTO 等 DTO 的指针字段 + omitempty 序列化与时间自定义格式（"2006-01-02T15:04:05"/"2006-01-02"），单测断言 null 省略、零值输出、时间格式
- [x] 2.2 实现 Recovery 中间件（panic → 200 + {"success":false,"errorMsg":"服务器异常"}）与路由骨架，单测覆盖异常响应
- [x] 2.3 实现 TokenRefresh/LoginAuth 中间件（authorization 头 → `login:token:{token}` Hash → 上下文 + EXPIRE 30min；非白名单无用户 → 401 空响应体），单测覆盖白名单/401/TTL 刷新
- [x] 2.4 实现 /user/code、/user/login（验证码 Redis 2min、自动注册 user_ 前缀、token 30min）、/user/me、/user/logout（恒"功能未完成"），契约测试断言各场景 JSON 键集合与文案

## 3. 用户资料与上传（user-profile spec）

- [x] 3.1 实现 /user/info/{id} 与 /user/{id}（createTime/updateTime 置空省略、不存在返回空 data），契约测试通过
- [x] 3.2 实现 /user/sign 与 /user/sign/count（SETBIT/BITFIELD + 连续 1 计数），单测覆盖位图与连续天数计算
- [x] 3.3 实现 /upload/blog 与 /upload/blog/delete（Java String.hashCode 目录哈希、可配置上传根目录、目录路径拒绝），单测用固定 UUID 断言路径格式

## 4. 店铺模块（shop spec）

- [x] 4.1 实现 GORM 模型与 TableName 映射（11 张表），迁移验证 hmdp.sql 可直接导入
- [x] 4.2 实现 /shop/{id} 逻辑过期缓存（cache:shop:{id} 30min、lock:shop:{id} 10s 互斥异步重建、未预热返回失败），单测覆盖命中/过期重建/未预热三态
- [x] 4.3 实现 /shop POST/PUT（更新后删缓存）与 /shop/of/name（LIKE 分页 10 条），契约测试通过
- [x] 4.4 实现 /shop/of/type（无坐标 DB 分页 5 条；有坐标 GEO 5000m 先 limit 后 skip + ORDER BY FIELD + distance），集成测试比对分页语义
- [x] 4.5 实现 /shop-type/list（Redis List 缓存回填、无 TTL、"没有分类数据"），契约测试通过

## 5. 社交模块（social spec）

- [x] 5.1 实现 /blog 发布（粉丝 feed ZADD 毫秒时间戳）与 /blog/like/{id} 点赞切换（ZSET + liked±1），单测覆盖点赞切换与 feed 写入
- [x] 5.2 实现 /blog/hot、/blog/{id}、/blog/likes/{id}、/blog/of/me、/blog/of/user（分页 10 条、name/icon/isLike 附加、FIELD 排序），契约测试通过
- [x] 5.3 实现 /blog/of/follow 滚动分页（ZREVRANGEBYSCORE 2 条 + 同分 offset 计算 + ScrollResult），单测覆盖滚动去重
- [x] 5.4 实现 /follow 关注/取关（DB + follows:{userId} SADD/SREM）、/follow/or/not/{id}、/follow/common/{id}（SINTER），契约测试通过

## 6. 秒杀模块（seckill spec）

- [x] 6.1 实现 RedisIdWorker（icr:order:{yyyy:MM:dd} INCR、(now-1640995200)<<32|count），单测断言位运算与日期分键
- [x] 6.2 实现 /voucher、/voucher/seckill（事务双表 + seckill:stock:{voucherId} 预载）与 /voucher/list/{shopId}（LEFT JOIN），契约测试通过
- [x] 6.3 实现 /voucher-order/seckill/{id}（Lua 原子校验、-1/1→"库存不足"、2→"不能重复下单"、0→Kafka 发布 + 立即返回订单号），集成测试覆盖三态
- [x] 6.4 实现 Kafka 主消费者（ReaderGroup 手动 commit、重试 3 次指数退避、失败投 DLT）与 DLT 消费者，幂等 INSERT ON DUPLICATE KEY + RowsAffected 判定后条件扣库存，集成测试验证重复投递不重复扣库存

## 7. 集成与切换

- [x] 7.1 编写契约基线脚本（curl 断言全部 40+ 路由的 JSON 键集合与状态码），对 Go 版全量通过
- [ ] 7.2 docker compose 全链路自测（登录→逛店→发动态→关注→抢券→签到），验证与前端页面兼容
- [x] 7.3 删除 src/ Java 代码、更新 README（Go 构建与 compose 部署说明），git 状态干净且 Java 版保留于历史
