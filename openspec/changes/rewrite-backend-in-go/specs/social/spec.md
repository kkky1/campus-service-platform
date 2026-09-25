# Spec Delta

## Purpose

提供校园动态的发布、点赞、热门榜、关注 feed 滚动流，以及关注关系与共同关注查询，保持前端社交页面行为不变。

## ADDED Requirements

### Requirement: 发布动态

系统 SHALL 提供 `POST /blog` 接口（需登录）。插入 `tb_blog`（`user_id` 为当前用户），随后对每位关注者执行 `ZADD feed:{followerId} blogId {当前毫秒时间戳}`，返回 `data=blogId`。

#### Scenario: 发布成功

- **WHEN** 已登录用户提交动态
- **THEN** 数据库新增动态，且所有关注者的 feed ZSET 均加入该 blogId，返回其 id

### Requirement: 点赞切换

系统 SHALL 提供 `PUT /blog/like/{id}` 接口（需登录）。检查 `blog:liked:{id}` 中是否含当前用户：未赞时执行 `liked=liked+1` 并 `ZADD blog:liked:{id} userId {当前毫秒}`；已赞时执行 `liked=liked-1` 并 ZREM。返回 `success=true`。

#### Scenario: 首次点赞

- **WHEN** 用户未赞过该动态
- **THEN** `liked` 计数加一，用户进入点赞 ZSET

#### Scenario: 取消点赞

- **WHEN** 用户已赞过该动态
- **THEN** `liked` 计数减一，用户移出点赞 ZSET

### Requirement: 热门榜单

系统 SHALL 提供 `GET /blog/hot?current=` 接口。按 `liked` 倒序分页查询 `tb_blog`（每页 10 条），每条附带作者 `name`、`icon` 与 `isLike`（当前用户是否赞过；未登录时为 false）。

#### Scenario: 查询热门动态

- **WHEN** 请求第 current 页
- **THEN** 返回按 liked 倒序的第 current 页动态，每条含 name、icon、isLike

### Requirement: 动态详情

系统 SHALL 提供 `GET /blog/{id}` 接口。动态不存在时返回 `success=false`、`errorMsg="博客不存在"`；存在时附带作者 `name`、`icon` 与 `isLike`。

#### Scenario: 查询详情

- **WHEN** 请求存在的动态 id
- **THEN** 返回动态详情及 name、icon、isLike

#### Scenario: 动态不存在

- **WHEN** 请求不存在的动态 id
- **THEN** 返回 `success=false`，`errorMsg="博客不存在"`

### Requirement: 点赞榜

系统 SHALL 提供 `GET /blog/likes/{id}` 接口。取 `blog:liked:{id}` ZSET 前 5 名用户，按 `ORDER BY FIELD(id, ...)` 顺序返回用户摘要 `{id, nickName, icon}` 列表。

#### Scenario: 查询点赞用户

- **WHEN** 请求动态的点赞榜
- **THEN** 返回最多 5 名按点赞时间最早的用户的摘要列表

### Requirement: 我的动态

系统 SHALL 提供 `GET /blog/of/me?current=` 接口（需登录）。按 `user_id` 过滤分页查询（每页 10 条），仅返回原始字段（不附加 name/icon/isLike）。

#### Scenario: 查询我的动态

- **WHEN** 已登录用户请求
- **THEN** 返回其动态分页列表（每页 10 条）

### Requirement: 指定用户的动态

系统 SHALL 提供 `GET /blog/of/user?id=&current=` 接口。按指定 `user_id` 过滤分页查询（每页 10 条），仅返回原始字段。

#### Scenario: 查询某用户动态

- **WHEN** 请求提供用户 id 与页码
- **THEN** 返回该用户动态分页列表（每页 10 条）

### Requirement: 关注 feed 滚动流

系统 SHALL 提供 `GET /blog/of/follow?lastId=&offset=` 接口（需登录）。从 `feed:{userId}` ZSET 按 score 倒序取 `score <= lastId` 的前 2 条（`offset` 用于跳过与 lastId 同分的已读项），按 `ORDER BY FIELD(id, ...)` 查询动态并附带 name/icon/isLike，返回 `ScrollResult {list, minTime, offset}`，其中 minTime 为本批最小 score，offset 为该 score 同分项个数。

#### Scenario: 首次拉取 feed

- **WHEN** 请求提供 lastId=当前毫秒时间戳、offset=0
- **THEN** 返回 feed 中最新 2 条动态及滚动参数 minTime、offset

#### Scenario: 继续滚动

- **WHEN** 请求携带上次返回的 minTime 与 offset
- **THEN** 返回不重复的后续动态及新的滚动参数

### Requirement: 关注与取关

系统 SHALL 提供 `PUT /follow/{id}/{isFollow}` 接口（需登录）。`isFollow=true` 时插入 `tb_follow(user_id, follow_user_id)` 并 `SADD follows:{userId} followUserId`；`isFollow=false` 时删除对应记录并 SREM。

#### Scenario: 关注用户

- **WHEN** isFollow=true
- **THEN** 关注关系入库，目标用户进入关注集合

#### Scenario: 取消关注

- **WHEN** isFollow=false
- **THEN** 关注关系删除，目标用户移出关注集合

### Requirement: 是否关注

系统 SHALL 提供 `GET /follow/or/not/{id}` 接口（需登录）。按当前用户与目标用户查询 `tb_follow` 记录数，返回 `data=count>0`。

#### Scenario: 查询关注状态

- **WHEN** 请求目标用户 id
- **THEN** 返回布尔值表示当前用户是否关注了目标用户

### Requirement: 共同关注

系统 SHALL 提供 `GET /follow/common/{id}` 接口（需登录）。计算 `SINTER follows:{userId} follows:{id}`，返回共同关注用户的摘要 `{id, nickName, icon}` 列表。

#### Scenario: 查询共同关注

- **WHEN** 请求另一用户 id
- **THEN** 返回两人共同关注的用户摘要列表
