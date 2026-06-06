# Request Archive 响应存储瘦身设计

## 背景

`gateway.request_archive` 用于短期排障, 会把网关请求和响应以 JSONL 写入本地文件。当前开启 `capture_response` 后, 响应体会被完整读取并按截断上限写入 `response` 归档记录。生产排障中响应归档体积增长很快, 尤其是流式响应和大模型输出, 单日文件可能达到 GB 级。

本次改造目标是降低响应归档存储体积, 同时保留排障和用量分析所需的 token usage 信息。

## 现状

- 核心实现: `backend/internal/server/middleware/request_archive.go`。
- 路由接入: `backend/internal/server/routes/gateway.go`。
- 配置开关: `gateway.request_archive.enabled` 和 `gateway.request_archive.capture_response`。
- 输出文件: `data/request-archive/YYYY-MM-DD.jsonl`。
- 当前每次网关请求通常写入两条记录:
  - `event=request`: 请求元信息和请求体。
  - `event=response`: 响应元信息和响应体。

## 问题

| 问题 | 影响 |
| --- | --- |
| 响应体存储体积大 | 长输出和流式 SSE 会显著放大 JSONL 文件大小 |
| 响应体排障价值有限 | 多数日常分析只需要状态、耗时、模型、用户/API Key 和 token usage |
| `capture_response` 成本高 | 需要缓存响应内容, 对热路径内存和磁盘都有压力 |

## 设计目标

| 目标 | 说明 |
| --- | --- |
| 不再归档响应正文 | `response` 记录不写完整 `body`, 降低磁盘占用 |
| 保留 token 使用量 | 从响应 JSON 或 SSE 事件中提取 `usage` 并写入轻量字段 |
| 保留关联能力 | 继续使用同一个 `archive_id` 关联 request 和 response |
| 保持请求归档不变 | `event=request` 仍按现有逻辑保存请求体, 供 token_analysis 解析 |
| 兼容现有后台开关 | `capture_response` 继续控制是否捕获响应侧额外信息, 但不再表示保存响应正文 |

## 方案

### 1. response 记录不再写响应正文

`requestArchiveResponseWriter` 继续包裹 `gin.ResponseWriter`, 但不再把响应片段累积为完整 `Body`。它只统计:

- `body_size`
- `body_sha256`
- `body_truncated`
- `stream`
- `usage`

`body_sha256` 可保留响应内容指纹, 便于判断相同响应或排查重复输出, 但不暴露正文。

### 2. 新增 response usage 摘要字段

在 `requestArchiveRecord` 增加轻量 usage 字段, 建议命名为:

```go
Usage map[string]any `json:"usage,omitempty"`
```

提取时保留上游原始 usage 结构中的常用 token 字段, 不写完整响应正文。兼容来源包括:

| 协议/形态 | usage 位置 |
| --- | --- |
| OpenAI Chat Completions 非流式 | `usage` |
| OpenAI Responses 非流式 | `usage` 或 `response.usage` |
| Anthropic Messages 非流式 | `usage` |
| Anthropic Messages SSE 提前中断 | `message_start` 事件的 `message.usage`(流死在 `message_delta` 前时保留 input/cache tokens) |
| Gemini generateContent / streamGenerateContent | `usageMetadata`(SSE 事件与 JSON 数组分片均支持) |
| SSE 流式 | `data:` JSON 事件中的 `usage` / `response.usage` / `usageMetadata`, 取最后一次非空 usage |

### 3. SSE usage 提取策略

流式响应不能保存完整 body, 但可以在 `Write` / `WriteString` 时按增量扫描 `data:` 行:

- 忽略空行和 `data: [DONE]`。
- 对每个可解析 JSON 的 `data:` payload 检查 usage。
- 若发现 usage, 覆盖为最新 usage。
- 保留少量未完成行 buffer, 避免跨 chunk 的 `data:` 行解析失败。
- 请求结束后在 `Usage()` 中冲洗残留 buffer: 最后一个事件缺少结尾换行(如上游在终止事件后立即断开)时, usage 仍可被提取。
- 单个 `data:` 行超过 256KB buffer 上限时(如 Responses 协议巨大的 `response.completed` 终止事件), 裁剪会砍掉行首前缀; 对仍带 usage 标记的碎片行降级做 fragment 提取, 避免 usage 静默丢失。

### 4. `capture_response` 语义调整

保持配置项不改名, 避免前后端和部署配置大改。新语义:

- `capture_response=false`: 不包裹 response writer, `response` 记录只含状态、耗时等基础信息。
- `capture_response=true`: 包裹 response writer, 统计响应大小/hash/stream, 并提取 usage, 但不保存响应正文。

前端文案和 `llm-wiki` 需要同步说明: 该开关用于捕获响应元信息和 token usage, 不再保存响应正文。

## 影响范围

| 文件 | 改动 |
| --- | --- |
| `backend/internal/server/middleware/request_archive.go` | 调整 response writer 缓存逻辑, 新增 usage 提取和归档字段 |
| `backend/internal/server/middleware/request_archive_test.go` | 增加响应正文不落盘、非流式 usage、SSE usage 的回归测试 |
| `frontend/src/views/admin/SettingsView.vue` / i18n | 更新 `capture_response` 文案 |
| `llm-wiki/wiki/backend.md` | 更新 RequestArchive 行为说明 |
| `docs/features/request-archive-async-writer-technical-notes-cn.md` | 追加本次行为变化说明 |

## 验证计划

| 验证 | 命令/方式 |
| --- | --- |
| 中间件单测 | `cd backend && go test ./internal/server/middleware/` |
| 配置相关回归 | `cd backend && go test ./internal/config/ ./internal/service/ -run RequestArchive` |
| 前端类型检查 | 如修改前端文案或类型, 执行 `pnpm --dir frontend run typecheck` |

## 最终落地

- `event=response` 记录不再包含 `body` 字段。
- `capture_response=true` 时仍记录 `body_size`、`body_sha256`、`body_truncated`、`stream`。
- 新增 `usage` 字段, 从非流式 JSON 的 `usage` / `response.usage` / `usageMetadata`(Gemini)/ `message.usage`(Anthropic `message_start` 兜底)或 SSE `data:` JSON 事件中提取。
- 非流式大响应只保留有限尾部解析窗口(256KB)用于捕获末尾 usage, 不把该窗口写入归档; usage 提取推迟到请求结束后执行一次, `Write` 热路径只做尾部窗口追加, 不做重复解析。
- SSE 路径在请求结束时冲洗残留行 buffer, 兜底最后一个事件缺少结尾换行的情况; 超过 256KB 的单行被裁剪后以碎片行降级 fragment 提取兜底。
- 中间件测试覆盖非流式 JSON、SSE(含无尾换行、超长单行裁剪、`message_start` 提前中断)、截断尾窗、Gemini `usageMetadata` 的 usage 提取, 并断言响应正文不落盘。
- 经 2026-06-05 全天真实流量回放验证(10,858 条响应): `/v1/responses` 99.5%、`/v1/chat/completions` 99.4%、`/v1/messages` 99.8%、非流式 100% 提取成功, 失败样本均为流中断导致上游未下发 usage; `message_delta` 实测携带全量累计 usage, 覆盖语义正确。

## 非目标

- 本次不改变 request 记录的请求体保存逻辑。
- 本次不改变 JSONL 文件目录和日期轮转方式。
- 本次不新增数据库 schema。
- 本次不修改 token_analysis 对 request 行的索引逻辑, 除非实现中发现与新字段存在冲突。
