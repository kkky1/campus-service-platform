# Spec Delta

## Purpose

提供手机验证码登录、基于 Redis 的会话 token、登录态刷新与接口访问控制，保证前端登录流程与权限边界行为不变。

## ADDED Requirements

### Requirement: 发送验证码

系统 SHALL 提供 `POST /user/code?phone={phone}` 接口。手机号不匹配正则 `^1([38][0-9]|4[579]|5[0-3,5-9]|6[6]|7[0135678]|9[89])\d{8}$` 时返回失败；匹配时生成 6 位随机数字验证码，写入 Redis 字符串键 `login:code:{phone}`，TTL 为 2 分钟。验证码仅记录日志，不真实发送。

#### Scenario: 手机号格式非法

- **WHEN** 请求 phone 参数不匹配手机号正则
- **THEN** 返回 `success=false`，`errorMsg="手机号格式错误"`

#### Scenario: 手机号格式合法

- **WHEN** 请求 phone 参数匹配手机号正则
- **THEN** 返回 `success=true`，且 Redis 中存在键 `login:code:{phone}`，其值为 6 位数字，约 2 分钟后过期

### Requirement: 登录

系统 SHALL 提供 `POST /user/login` 接口，请求体为 `{phone, code}`。手机号非法时行为与发送验证码一致；验证码缺失或不一致时返回 `success=false`、`errorMsg="验证码不一致，请重新输入"`；验证通过时按手机号查询用户，不存在则自动创建（`nick_name = "user_" + 10 位随机字符串`），随后生成无连字符的 UUID 作为 token，将用户摘要 `{id, nickName, icon}` 以 Redis Hash 写入 `login:token:{token}`，TTL 为 30 分钟，返回 `data=token`。

#### Scenario: 验证码错误

- **WHEN** 提交的 code 与 Redis 中 `login:code:{phone}` 不一致或已过期
- **THEN** 返回 `success=false`，`errorMsg="验证码不一致，请重新输入"`

#### Scenario: 登录成功

- **WHEN** 手机号与验证码均正确
- **THEN** 返回 `success=true`、`data` 为 token；Redis Hash `login:token:{token}` 包含 `id`、`nickName`、`icon` 字段且 30 分钟后过期；不存在的手机号同时完成用户注册

### Requirement: Token 刷新

任意请求携带 authorization 头时，若 Redis 中 `login:token:{token}` 存在，系统 SHALL 将该用户摘要载入请求上下文，并刷新该键 TTL 为 30 分钟；token 无效或缺失时不阻断请求本身。

#### Scenario: 携带有效 token

- **WHEN** 请求 authorization 头对应的 `login:token:{token}` 存在
- **THEN** 后续业务可通过上下文取得用户 `{id, nickName, icon}`，且该键 TTL 被重置为 30 分钟

### Requirement: 登录校验与白名单

除白名单路径外，请求上下文中无用户时系统 SHALL 返回 HTTP 401。白名单路径为：`/user/login`、`/user/code`、`/shop/**`、`/shop-type/**`、`/voucher/**`、`/upload/**`、`/blog/hot`。白名单路径即使携带无效 token 也不返回 401。

#### Scenario: 白名单路径匿名访问

- **WHEN** 无 token 访问 `/shop/1` 或 `/blog/hot` 等白名单路径
- **THEN** 请求正常处理并返回 HTTP 200

#### Scenario: 受保护路径匿名访问

- **WHEN** 无 token 访问 `/user/me` 或 `/blog` 等受保护路径
- **THEN** 返回 HTTP 401 且响应体为空

### Requirement: 当前用户

系统 SHALL 提供 `GET /user/me` 接口，返回当前登录用户摘要 `{id, nickName, icon}`。

#### Scenario: 已登录查询当前用户

- **WHEN** 携带有效 token 请求 `/user/me`
- **THEN** 返回 `success=true`，`data` 为 `{id, nickName, icon}`

### Requirement: 登出（保留原行为）

系统 SHALL 提供 `POST /user/logout` 接口，恒返回 `success=false`、`errorMsg="功能未完成"`，保持与原系统一致。

#### Scenario: 调用登出

- **WHEN** 请求 `POST /user/logout`
- **THEN** 返回 `{"success": false, "errorMsg": "功能未完成"}`，HTTP 状态码 200
