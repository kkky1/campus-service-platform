# Spec Delta

## Purpose

提供店铺详情、按类型与名称检索、附近店铺搜索以及店铺类型列表，并以 Redis 逻辑过期缓存支撑热点数据的高性能读取。

## ADDED Requirements

### Requirement: 店铺详情（逻辑过期缓存）

系统 SHALL 提供 `GET /shop/{id}` 接口。读取 Redis 键 `cache:shop:{id}`，其值为 JSON `{data, expireTime}`（逻辑过期结构）。键不存在时返回 `success=false`、`errorMsg="店铺不存在！"`；存在且未过逻辑过期时间时返回缓存数据；已逻辑过期时仍返回旧数据，同时以 `lock:shop:{id}`（TTL 10 秒）为互斥锁异步重建缓存：抢到锁者查数据库并重写缓存（逻辑过期 30 分钟），未抢到锁者仅返回旧数据。

#### Scenario: 缓存未预热

- **WHEN** 键 `cache:shop:{id}` 不存在（店铺从未缓存）
- **THEN** 返回 `success=false`，`errorMsg="店铺不存在！"`

#### Scenario: 缓存命中且未过期

- **WHEN** `cache:shop:{id}` 存在且 `expireTime` 晚于当前时间
- **THEN** 返回 `success=true`，`data` 为缓存中的店铺数据，不触发数据库查询

#### Scenario: 缓存已逻辑过期

- **WHEN** `cache:shop:{id}` 存在但 `expireTime` 不晚于当前时间
- **THEN** 立即返回旧店铺数据，且后台仅有一个竞争者（持有 `lock:shop:{id}` 者）查询数据库并重写缓存，逻辑过期时间为 30 分钟后

### Requirement: 店铺新增

系统 SHALL 提供 `POST /shop` 接口，将请求体店铺数据插入 `tb_shop`，返回 `data=店铺id`。

#### Scenario: 新增店铺

- **WHEN** 提交合法店铺数据
- **THEN** 数据库中新增一条店铺记录，返回其 id

### Requirement: 店铺更新

系统 SHALL 提供 `PUT /shop` 接口。id 为空时返回 `success=false`、`errorMsg="店铺id不能为空"`；否则更新数据库记录并删除缓存键 `cache:shop:{id}`。

#### Scenario: 更新店铺

- **WHEN** 提交含 id 的店铺数据
- **THEN** 数据库记录被更新，且 `cache:shop:{id}` 被删除

#### Scenario: 缺少 id

- **WHEN** 请求体无 id
- **THEN** 返回 `success=false`，`errorMsg="店铺id不能为空"`

### Requirement: 按类型分页查询

系统 SHALL 提供 `GET /shop/of/type?typeId=&current=&x=&y=` 接口。x 或 y 任一为空时，按 `type_id` 过滤并分页（每页 5 条）查询数据库；x、y 均提供时，从 Redis GEO 键 `shop:geo:{typeId}` 搜索以 (x, y) 为圆心、半径 5000 米内的店铺（按距离升序），取第 current 页（前 `current*5` 条跳过前 `(current-1)*5` 条），再按该顺序以 `ORDER BY FIELD(id, ...)` 查询数据库，且每项附带 `distance` 字段（单位为米）。

#### Scenario: 无坐标按类型查询

- **WHEN** 请求不含 x 或 y
- **THEN** 返回按 `type_id` 过滤、每页 5 条的第 current 页数据

#### Scenario: 按坐标附近搜索

- **WHEN** 请求提供 x 与 y
- **THEN** 返回 5000 米内按距离升序、分页后的店铺列表，每项含 `distance`

### Requirement: 按名称搜索

系统 SHALL 提供 `GET /shop/of/name?name=&current=` 接口，按店铺名模糊匹配（LIKE）分页查询，每页 10 条。

#### Scenario: 按关键字搜索

- **WHEN** 请求提供 name 关键字
- **THEN** 返回名称包含该关键字的店铺分页列表，每页 10 条

### Requirement: 店铺类型列表

系统 SHALL 提供 `GET /shop-type/list` 接口。优先读取 Redis List `shop_type:` 的全部元素；List 不存在时查询 `tb_shop_type` 按 `sort` 升序排列，逐条 RPUSH 回填该 List（不设 TTL）后返回；数据库无数据时返回 `success=false`、`errorMsg="没有分类数据"`。

#### Scenario: 缓存命中

- **WHEN** Redis 中 `shop_type:` List 非空
- **THEN** 按 List 顺序返回类型列表，不查询数据库

#### Scenario: 缓存未命中

- **WHEN** `shop_type:` 不存在但数据库有数据
- **THEN** 返回按 `sort` 升序的类型列表，并将各元素依次 RPUSH 到 `shop_type:`

#### Scenario: 无分类数据

- **WHEN** 缓存与数据库均无数据
- **THEN** 返回 `success=false`，`errorMsg="没有分类数据"`
