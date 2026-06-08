# Request Archive 异步写入改造技术留底

> 适用目的: 在 sub2api 上游 main 大版本升级后, 能快速迁移本次改造, 或按同一方案重新实现.
> 当前实现形态: `默认关闭 + 异步有界队列 + 后台单 writer 持久文件句柄 + 按日期轮转 + 响应不落正文`.

## 1. 背景与问题

`gateway.request_archive` 把网关请求/响应体写入本地 JSONL, 供短期排障使用。生产环境观察到:

- `data/request-archive/2026-05-29.jsonl` 单日增长到约 1GB。
- 当天约 1900+ 对 request/response 归档记录。
- request 行平均约 361KB, response 行平均约 172KB, 最大单行约 2.7MB。
- 最近 2000 行大量为流式响应归档。
- 运行配置未显式配置 `gateway.request_archive`, 实际吃源码默认值。

### 根因

1. 默认值与注释不符: 注释写「默认关闭」, 但默认值实际是 `enabled=true`, `capture_response=true`。
2. 旧实现位于请求热路径且全同步:
   - 请求开始 `io.ReadAll(c.Request.Body)` 读完整请求体。
   - 发往上游前同步写 request 记录。
   - 响应过程中 wrapper 对每个 `Write`/`WriteString` 做 hash 和 body 缓存。
   - 请求完成后同步写 response 记录。
3. 文件写入非异步队列: 每条记录都 `os.MkdirAll` + `json.Marshal` + 全局 `sync.Mutex` + `os.OpenFile(O_CREATE|O_WRONLY|O_APPEND)` + `Write` + `Close`。
4. 高并发 / 大 body / 流式响应场景造成锁竞争与同步 IO 阻塞, 拖慢请求尾延迟。

## 2. 最终方案

### 配置默认值 (`backend/internal/config/config.go`)

- `gateway.request_archive.enabled` 默认改为 `false`, 与注释一致。
- `gateway.request_archive.capture_response` 默认改为 `false`, response 捕获独立开关。
- 新增 `gateway.request_archive.queue_size`, 默认 `1024`, 异步队列容量。
- `dir`, `max_request_body_bytes`(8MiB), `max_response_body_bytes`(2MiB) 默认不变。

`GatewayRequestArchiveConfig` 新增字段:

```go
QueueSize int `mapstructure:"queue_size"`
```

### 中间件 (`backend/internal/server/middleware/request_archive.go`)

1. `enabled=false` 时直接返回纯透传 handler:
   - 不读 body, 不包装 `ResponseWriter`, 不创建后台 goroutine。
2. `enabled=true` 时创建 `asyncRequestArchiveWriter`, handler 由 `newRequestArchiveHandler` 构造。
3. 热路径只做非阻塞 `Enqueue`:
   - `select` 写入有界 channel, 满则 `default` 分支丢弃并累计 `dropped`(原子计数), 每丢 256 条采样告警。
4. 后台单 goroutine `run()`:
   - 从 channel 取记录, `json.Marshal` 后写盘。
   - `fileForToday()` 持有当日文件句柄, 跨天时关闭旧句柄重开新文件, 避免每条记录 open/close。
   - channel 关闭时收尾关闭文件句柄。
5. `capture_response` 为独立开关; 为 `false` 时不包装 `ResponseWriter`; 为 `true` 时只记录响应大小、hash、流式标记和 token usage, 不保存响应正文。
6. 响应 usage 从非流式 JSON 的 `usage` / `response.usage` / `usageMetadata`(Gemini)/ `message.usage`(Anthropic `message_start` 兜底)提取; 流式 SSE 从 `data:` JSON 事件中提取最后一次非空 usage, 请求结束时冲洗残留行 buffer 兜底无尾换行的终止事件; 单行超过 256KB 被裁剪后以碎片行降级 fragment 提取兜底。非流式大响应仅保留 256KB 尾部解析窗口, usage 提取推迟到请求结束执行一次, Write 热路径只做窗口追加, 不写入归档正文。
7. 保留对外层中间件 `ResponseWriter` 的还原逻辑 (defer 恢复 `c.Writer`)。
8. 归档目录支持后台热切换: writer 的 `dir` 为 `atomic.Value`, 中间件每请求以运行态配置调用 `SetDir`(值未变化时一次原子读即返回); `fileForToday` 在跨天或目录变化时轮转句柄, 切换后下一条记录写入新目录。自定义目录持久化于 settings(`request_archive_settings` 的 `dir` 字段), 保存前经磁盘存在/可写校验, 历史文件不自动迁移。详见 `docs/features/request-archive-dir-runtime-config-design-cn.md`。

### 关键类型

- `asyncRequestArchiveWriter`: `dir`, `ch chan requestArchiveRecord`, `done chan struct{}`, `dropped atomic.Int64`, `file *os.File`, `fileDate string`。
  - `Enqueue(record)`: 非阻塞入队。
  - `run()`: 后台消费循环。
  - `Close()`: 关闭 channel 并等待 `done`; 生产随进程常驻无需调用, 主要供测试释放文件句柄。
- `archiveEnqueuer` 接口: 抽象 `Enqueue`, 便于测试注入可关闭 writer。
- `newRequestArchiveHandler(cfg, writer)`: 与 writer 解耦的 handler 构造函数。

## 3. 设计取舍

| 维度 | 旧实现 | 新实现 | 理由 |
| --- | --- | --- | --- |
| 默认开关 | 默认开启 | 默认关闭 | 与注释一致, 避免无意中长期归档 |
| 写入方式 | 同步 + 全局锁 + 每条 open/close | 异步有界队列 + 单 writer 持久句柄 | 消除热路径锁竞争与同步 IO 阻塞 |
| 队列满策略 | 不适用 | 丢弃 + 采样告警 | 排障数据非关键, 优先保证请求不阻塞 |
| 文件句柄 | 每条 open/close | 后台常驻句柄按日期轮转 | 减少 syscall 与目录创建开销 |
| response 捕获 | 跟随 enabled 且保存正文 | 独立开关默认关闭, 开启后只存元信息和 usage | 降低磁盘占用, 保留 token 排障信号 |

### 为什么队列满选择 drop 而非阻塞

归档是排障辅助数据, 不是业务关键路径。阻塞会把磁盘 IO 抖动直接传导到请求尾延迟, 与本次改造目标相悖。因此队列满时丢弃并采样告警, 保证请求永不被归档拖慢。

## 4. 测试覆盖

`backend/internal/server/middleware/request_archive_test.go`:

- `TestRequestArchiveDisabledDoesNotWriteFiles`: disabled 时不读 body(handler 仍能读完整 body), 不写文件。
- `TestRequestArchiveCaptureResponseDisabledDoesNotWrapWriter`: `capture_response=false` 时不包装 `ResponseWriter`, response 记录无 body。
- `TestRequestArchiveCaptureResponseStoresUsageWithoutResponseBody`: 非流式 JSON response 提取 `usage`, 但不写响应 `body`。
- `TestRequestArchiveCaptureResponseExtractsSSEUsageWithoutResponseBody`: SSE `data:` response 提取 `response.usage`, 但不写响应 `body`。
- `TestRequestArchiveCaptureResponseExtractsSSEUsageWithoutTrailingNewline`: 终止事件缺少结尾换行时仍能提取 usage。
- `TestRequestArchiveCaptureResponseExtractsGeminiUsageMetadata`: Gemini 响应从 `usageMetadata` 提取 usage。
- `TestRequestArchiveCaptureResponseExtractsAnthropicMessageStartUsage`: 流死在 `message_delta` 前时从 `message_start` 的 `message.usage` 兜底提取。
- `TestRequestArchiveCaptureResponseExtractsUsageFromOversizedSSELine`: 单个 `data:` 行超过 256KB 缓冲上限被裁剪后, 碎片行降级 fragment 提取仍能拿到 usage。
- `TestAsyncRequestArchiveWriterDropsWhenQueueFull`: 队列满时 `Enqueue` 不阻塞且 `dropped` 累计。
- 既有用例适配异步写入(`readArchiveRecords` 改为 `require.Eventually` 轮询), 覆盖关联写入 / 截断 / 身份关联 / 跳过非模型请求 / 外层 writer 还原。

`backend/internal/config/config_test.go`:

- `TestLoadDefaultGatewayRequestArchiveConfig`: 默认 `enabled=false`, `capture_response=false`, `queue_size=1024`。

测试辅助 `useRequestArchive(t, cfg)` 在用例结束 `t.Cleanup` 中调用 `writer.Close()`, 释放文件句柄(Windows 下避免 TempDir 清理失败)。

### 验证命令

```bash
cd backend
go build ./...
go test ./internal/server/middleware/ ./internal/config/
```

## 5. 运维约束

- 归档仅用于短期排障, 排障结束后必须在管理后台关闭请求归档开关。
- 示例配置见 `deploy/config.example.yaml` 的 `gateway.request_archive` 段, 已标注「仅短期排障」与磁盘/尾延迟风险。
- 生产止血: 优先在 `/admin/settings` 的 Gateway 标签页关闭「请求归档」; 如 settings DB 不可用, 在实际运行配置显式加 `gateway.request_archive.enabled: false` 后重启服务。

### 开关位置与操作方式

当前 `request_archive` 支持**管理后台运行态开关**:

- 页面入口: 管理后台 `/admin/settings` -> Gateway 标签页 -> 「请求归档」。
- 后端 API: `GET/PUT /api/v1/admin/settings/request-archive`。
- 页面可热切换: `enabled` 和 `capture_response`, 保存后写入 settings 表, 中间件通过缓存 provider 在请求热路径读取, 无需重启。
- 配置文件仍控制: `dir`, `max_request_body_bytes`, `max_response_body_bytes`, `queue_size`; 这些参数修改后需要重启。
- 本地/源码配置位置: `backend/config.yaml` 的 `gateway.request_archive`。
- 部署示例位置: `deploy/config.example.yaml` 的 `gateway.request_archive`。
- 生产实际位置: 以运行进程加载的配置文件为准; 当前生产观察路径对应通常是 `/opt/sub2api/backend/config.yaml` 或部署脚本挂载到容器内的同名配置。

配置片段:

```yaml
gateway:
   request_archive:
      enabled: false
      capture_response: false
```

## 6. 涉及文件

- `backend/internal/config/config.go`: 默认值与 `QueueSize` 字段。
- `backend/internal/config/config_test.go`: 默认配置断言。
- `backend/internal/server/middleware/request_archive.go`: 异步 writer 与 handler。
- `backend/internal/server/middleware/request_archive_test.go`: 测试覆盖。
- `deploy/config.example.yaml`: 示例配置段。
- `llm-wiki/wiki/backend.md`: 中间件知识更新。
