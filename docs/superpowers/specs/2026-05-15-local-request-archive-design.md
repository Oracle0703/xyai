# 本地请求归档设计

## 目标

在内部小范围测试中，按配置保存 AI 网关模型调用的请求体、用户设备信息，以及返回给用户的响应内容，并用同一个 `archive_id` 关联一次问答交互。该能力默认开启，可通过配置关闭，不影响现有转发、计费、限流和错误处理流程。

## 覆盖范围

仅覆盖 AI 网关模型调用路径，不覆盖登录、注册、后台管理、支付等管理接口。

| 路径 | 说明 |
|---|---|
| `/v1/messages`、`/antigravity/v1/messages` | Claude/Antigravity Messages |
| `/v1/messages/count_tokens`、`/antigravity/v1/messages/count_tokens` | Claude token 计数 |
| `/v1/responses`、`/responses`、`/backend-api/codex/responses` | Responses API |
| `/v1/chat/completions`、`/chat/completions` | Chat Completions |
| `/v1beta/models/*`、`/antigravity/v1beta/models/*` | Gemini 原生 API |
| `/v1/images/generations`、`/v1/images/edits`、`/images/*` | OpenAI 图片请求 |

WebSocket 首条消息本期不作为主要目标；GET `/responses` 建连本身无请求 body，后续消息在 WS 数据通道中，需单独设计。

## 数据模型

归档以 JSONL 写入本地文件，一行一个事件。每次 HTTP 请求生成一个 `archive_id`，对应至少一条 `request` 事件和一条 `response` 事件。

| 字段 | 含义 |
|---|---|
| `archive_id` | 一次请求响应交互的关联 ID |
| `event` | `request` 或 `response` |
| `timestamp` | RFC3339Nano 时间 |
| `method`、`path`、`endpoint` | 请求方法、原始路径、归一化端点 |
| `user_id`、`api_key_id`、`group_id`、`account_id` | 可获取到的身份和调度信息 |
| `model` | 从请求上下文或 body 中提取的模型名 |
| `client_ip`、`user_agent`、`headers` | 用户设备和客户端信息，headers 仅 allowlist |
| `body`、`body_size`、`body_sha256`、`body_truncated` | request 或 response body 及其元信息 |
| `status`、`duration_ms` | 响应状态和请求耗时，仅 response 事件 |
| `stream` | 是否疑似流式响应 |

## 配置

配置放在 `gateway.request_archive` 下：

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `enabled` | `true` | 总开关 |
| `dir` | `data/request-archive` | JSONL 输出目录 |
| `max_request_body_bytes` | `65536` | 请求体保存上限 |
| `max_response_body_bytes` | `65536` | 响应体保存上限 |
| `capture_response` | `true` | 是否捕获返回给用户的响应体 |

## 安全边界

不记录 `Authorization`、`Cookie`、`Set-Cookie`、`X-Api-Key` 等敏感头。文件写入失败只记录内部日志，不阻断用户请求。保存目录加入 `.gitignore`，避免误提交本地归档内容。

## 实现方式

在网关路由层增加中间件：

1. 匹配 AI 网关路径和 HTTP method。
2. 读取 request body 后立即还原给下游 handler。
3. 生成 `archive_id` 并写入 request 事件。
4. 用 response writer 包装器限量捕获写给用户的响应体。
5. `c.Next()` 后写入 response 事件，携带状态、耗时、捕获 body 和同一个 `archive_id`。

该实现集中在路由/中间件层，不逐个改动各网关 handler。

## 测试策略

| 测试 | 验证点 |
|---|---|
| 默认关闭 | 不创建归档文件 |
| 开启归档 | request/response 两行 JSONL 共用 `archive_id` |
| 敏感头过滤 | Authorization、Cookie 不落盘，User-Agent 保留 |
| 截断 | 超出上限的 request/response 标记 `truncated=true` |
| 下游 body 可读 | 中间件读取 body 后 handler 仍能读取同样 body |
