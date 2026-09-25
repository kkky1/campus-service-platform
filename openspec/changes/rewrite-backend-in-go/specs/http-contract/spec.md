# Spec Delta

## Purpose

定义所有 HTTP 接口共用的响应包结构、JSON 序列化规则与错误约定，确保 Go 重写后前端与调用方无需任何改动即可正常工作。

## ADDED Requirements

### Requirement: 统一响应包结构

所有业务接口 SHALL 返回 HTTP 200，响应体为 JSON 对象 `{"success": boolean, "errorMsg": string, "data": any, "total": number}`。成功时 `success=true` 且 `errorMsg` 与 `total` 省略；失败时 `success=false`、`errorMsg` 为具体文案，且 `data` 与 `total` 省略。

#### Scenario: 成功响应

- **WHEN** 业务处理成功且携带数据
- **THEN** 响应体包含 `success: true` 与 `data`，且不包含 `errorMsg`、`total` 键

#### Scenario: 业务失败响应

- **WHEN** 业务校验失败（如参数错误、库存不足）
- **THEN** 响应体包含 `success: false` 与对应 `errorMsg`，且不包含 `data`、`total` 键

### Requirement: JSON null 字段省略

响应对象中值为 null 的字段 SHALL 在序列化时被省略；值为 `0`、`false`、空字符串的字段 MUST 正常输出，不得被当作 null 省略。

#### Scenario: 字段为 null

- **WHEN** 响应对象的某个字段值为 null
- **THEN** 输出 JSON 中不包含该键

#### Scenario: 字段为零值

- **WHEN** 响应对象的数值字段为 0 或布尔字段为 false
- **THEN** 输出 JSON 中正常包含该键及值

### Requirement: 运行时异常响应

未捕获的运行时异常 SHALL 返回 HTTP 200，响应体 `success=false`、`errorMsg="服务器异常"`，不泄露堆栈信息。

#### Scenario: 处理过程抛异常

- **WHEN** 接口处理过程中抛出未捕获异常（如消息发送失败、文件保存失败）
- **THEN** 返回 HTTP 200，响应体 `{"success": false, "errorMsg": "服务器异常"}`

### Requirement: 未认证响应

受保护接口在请求无有效登录态时 SHALL 返回 HTTP 401，且响应体为空。

#### Scenario: 未登录访问受保护接口

- **WHEN** 请求未携带有效 authorization 头且访问受保护接口
- **THEN** 返回 HTTP 401，响应体为空
