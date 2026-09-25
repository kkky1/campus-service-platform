# Spec Delta

## Purpose

提供校园福利券的发布、秒杀抢券的原子资格校验与基于 Kafka 的异步下单，保证高并发下库存不超卖且一人一单。

## ADDED Requirements

### Requirement: 普通券发布

系统 SHALL 提供 `POST /voucher` 接口，仅插入 `tb_voucher`（不写秒杀表、不预载库存），返回 `data=voucherId`；标题为空时返回 `success=false`、`errorMsg="券标题不能为空"`。

#### Scenario: 发布普通券

- **WHEN** 提交含标题的券数据（可不含 stock）
- **THEN** `tb_voucher` 新增记录并返回其 id；`tb_seckill_voucher` 与 `seckill:stock:{id}` 均无变化

#### Scenario: 标题为空

- **WHEN** 请求体不含标题
- **THEN** 返回 `success=false`，`errorMsg="券标题不能为空"`，不产生任何记录

### Requirement: 秒杀券发布

系统 SHALL 提供 `POST /voucher/seckill` 接口，事务内插入 `tb_voucher` 与 `tb_seckill_voucher`（含 `voucher_id`、`stock`、`begin_time`、`end_time`），并将库存写入 Redis 键 `seckill:stock:{voucherId}`；`stock` 缺失时返回 `success=false`、`errorMsg="服务器异常"` 且事务回滚。

#### Scenario: 发布秒杀券

- **WHEN** 提交含 stock、beginTime、endTime 的券数据
- **THEN** 两张表各新增一条记录，Redis 键 `seckill:stock:{voucherId}` 等于 stock，返回 `data=voucherId`

#### Scenario: 缺少 stock

- **WHEN** 请求体无 stock
- **THEN** 返回 `success=false`、`errorMsg="服务器异常"`，两张表均无新增记录

### Requirement: 店铺券列表

系统 SHALL 提供 `GET /voucher/list/{shopId}` 接口，返回该店铺 `status=1` 的券列表，LEFT JOIN 秒杀表附带 `stock`、`beginTime`、`endTime` 字段（非秒杀券这些字段为 null 并省略）。

#### Scenario: 查询店铺券列表

- **WHEN** 请求指定 shopId
- **THEN** 返回 `tb_voucher LEFT JOIN tb_seckill_voucher` 中 `shop_id=shopId AND status=1` 的记录，含 `stock`/`beginTime`/`endTime`

### Requirement: 抢券

系统 SHALL 提供 `POST /voucher-order/seckill/{id}` 接口（需登录）。处理流程：

1. 生成订单号：对 Redis 键 `icr:order:{yyyy:MM:dd}` 执行 INCR，`orderId = (当前秒 - 1640995200) << 32 | 序列值`。
2. 执行 Lua 脚本原子完成资格校验：读取 `seckill:stock:{voucherId}`；键缺失或 `stock<=0` 返回结果 1；`seckill:order:{voucherId}` 集合已含当前 userId 返回结果 2；否则 INCRBY -1 并 SADD userId，返回结果 0。
3. 结果 1 返回 `success=false`、`errorMsg="库存不足"`；结果 2 返回 `success=false`、`errorMsg="不能重复下单"`。
4. 结果 0 时向 Kafka 主题 `seckill.order` 发送消息 `{id, userId, voucherId}`，并立即返回 `data=orderId`；发送失败时按全局异常处理返回 `errorMsg="服务器异常"`。

#### Scenario: 库存不足

- **WHEN** Lua 校验时库存键缺失或库存不大于 0
- **THEN** 返回 `success=false`、`errorMsg="库存不足"`，不生成订单

#### Scenario: 重复抢券

- **WHEN** 当前用户已存在于 `seckill:order:{voucherId}` 集合
- **THEN** 返回 `success=false`、`errorMsg="不能重复下单"`

#### Scenario: 抢券成功

- **WHEN** 库存充足且用户未抢过
- **THEN** Redis 库存减一、用户加入防重集合，Kafka 收到订单消息，接口立即返回 `data=orderId`

### Requirement: 异步下单（幂等消费）

消费者 SHALL 消费 `seckill.order` 消息并幂等处理：插入 `tb_voucher_order`（`id` 已存在时不重复插入、不报错）；仅当插入真正成功时执行 `UPDATE tb_seckill_voucher SET stock = stock - 1 WHERE voucher_id = ? AND stock > 0`。

#### Scenario: 正常消费

- **WHEN** 消费者收到订单消息且订单 id 不存在
- **THEN** `tb_voucher_order` 新增订单，`tb_seckill_voucher` 库存减一

#### Scenario: 消息重复投递

- **WHEN** 消费者再次收到相同订单 id 的消息
- **THEN** 不产生重复订单，库存不重复扣减

### Requirement: 失败重试与 DLT 兜底

消费失败 SHALL 重试 3 次（指数退避）；3 次仍失败 MUST 将消息投递至死信主题 `seckill.order.dlt`；DLT 消费者以与主消费者相同的幂等逻辑兜底处理。

#### Scenario: 消费最终失败进入 DLT

- **WHEN** 某消息连续 3 次处理失败
- **THEN** 消息被投递至 `seckill.order.dlt`，主消费者继续处理后续消息

#### Scenario: DLT 兜底成功

- **WHEN** DLT 消费者成功处理死信消息
- **THEN** 订单被创建（或按幂等规则跳过），库存按幂等规则扣减

### Requirement: 一人一单

同一用户对同一券 SHALL 只能成功抢购一次，防重校验在 Lua 脚本内原子完成；订单号在抢券请求内生成并同步返回给用户。

#### Scenario: 并发重复抢购

- **WHEN** 同一用户并发发起多次抢同一券的请求
- **THEN** 仅一次请求返回订单号，其余返回「不能重复下单」
