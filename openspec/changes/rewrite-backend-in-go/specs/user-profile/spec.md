# Spec Delta

## Purpose

提供用户资料与摘要查询、每日签到位图与连续签到统计、博客图片上传与删除，保持个人中心相关页面行为不变。

## ADDED Requirements

### Requirement: 用户资料

系统 SHALL 提供 `GET /user/info/{id}` 接口。按 `user_id` 查询 `tb_user_info`：不存在时返回 `success=true` 且 data 省略；存在时返回资料对象且 `createTime`、`updateTime` 为 null（序列化时省略）。

#### Scenario: 资料存在

- **WHEN** 请求存在的用户 id
- **THEN** 返回 `{userId, city, introduce, fans, followee, gender, birthday, credits, level}`，不含 createTime、updateTime

#### Scenario: 资料不存在

- **WHEN** 请求无资料的用户 id
- **THEN** 返回 `success=true` 且不含 data

### Requirement: 用户摘要

系统 SHALL 提供 `GET /user/{id}` 接口。返回用户摘要 `{id, nickName, icon}`；用户不存在时返回 `success=true` 且不含 data。

#### Scenario: 查询用户摘要

- **WHEN** 请求存在的用户 id
- **THEN** 返回 `{id, nickName, icon}`

### Requirement: 签到

系统 SHALL 提供 `POST /user/sign` 接口（需登录）。对 Redis 键 `sign:{userId}:{yyyyMM}` 执行 SETBIT，偏移量为当月第几天减一，值为 1。

#### Scenario: 每日签到

- **WHEN** 已登录用户在当月某天签到
- **THEN** 该用户在 `sign:{userId}:{yyyyMM}` 位图中对应天位的 bit 被置 1

### Requirement: 连续签到统计

系统 SHALL 提供 `GET /user/sign/count` 接口（需登录）。对 `sign:{userId}:{yyyyMM}` 执行 BITFIELD GET（无符号、宽度为当月已过天数，偏移 0），统计结果低位连续 1 的个数并作为 data 返回；无签到记录时返回 0。

#### Scenario: 有连续签到

- **WHEN** 用户最近 n 天连续签到且第 n+1 天未签到
- **THEN** 返回 data=n

#### Scenario: 无签到记录

- **WHEN** 当月无任何签到
- **THEN** 返回 data=0

### Requirement: 图片上传

系统 SHALL 提供 `POST /upload/blog` 接口（multipart，字段名 `file`）。生成 UUID 文件名并保留原扩展名，计算 Java `String.hashCode` 算法（对 UUID 字符串）得到哈希，目录为 `/blogs/{d1}/{d2}/`（`d1 = hash & 0xF`、`d2 = (hash >> 4) & 0xF`），文件保存至可配置上传根目录下的该路径，返回 `data="/blogs/{d1}/{d2}/{uuid}.{ext}"`。目录不存在时自动创建。

#### Scenario: 上传图片

- **WHEN** 提交 multipart 图片文件
- **THEN** 文件保存到上传根目录下 hash 目录中，返回相对路径字符串，路径格式与 Java 版一致

### Requirement: 图片删除

系统 SHALL 提供 `GET /upload/blog/delete?name=` 接口。删除上传根目录下对应文件；路径指向目录时返回 `success=false`、`errorMsg="错误的文件名称"`。

#### Scenario: 删除图片

- **WHEN** 请求携带已上传图片的相对路径
- **THEN** 对应文件被删除，返回 `success=true`

#### Scenario: 非法名称

- **WHEN** 请求名称对应目录而非文件
- **THEN** 返回 `success=false`，`errorMsg="错误的文件名称"`
