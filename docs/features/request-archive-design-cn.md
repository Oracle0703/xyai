# 网关请求本地归档设计

更新：2026-11-04。状态：**已合入 main 并在生产使用。默认关闭；当前生产按 Token Analysis 需要常开，开关、归档目录与请求体截断上限可在管理后台热改。剩余改进项（优雅停机 drain、去全局 writer 注册表、按大小切片与保留策略）未做（§13）。**

本文是网关请求本地 JSONL 归档（`gateway.request_archive`，中间件 `RequestArchive`）的唯一文档，包含规则、配置、记录格式、代码位置、易错点和验证方式。审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看，原路径：`docs/superpowers/specs/2026-05-15-local-request-archive-design.md`、`docs/superpowers/plans/2026-05-15-local-request-archive.md`、`docs/features/request-archive-async-writer-technical-notes-cn.md`、`docs/reviews/request-archive-async-writer-review-cn.md`、`docs/features/request-archive-response-usage-storage-design-cn.md`、`docs/superpowers/plans/2026-06-07-request-archive-response-usage-storage.md`、`docs/features/request-archive-dir-runtime-config-design-cn.md`、`docs/reviews/request-archive-dir-runtime-config-code-review-cn.md`）。凡是对后续修改有约束的审核结论都已写入正文，主要在 §11。

归档的下游消费者是 Token Analysis（索引器按日读取 JSONL），其规则见 `docs/features/token-analysis-*.md`，本文只描述归档本身，不复制分析侧规则。

阅读顺序建议：
- 改中间件或写入路径：先读 §3、§4、§11；
- 改响应捕获或 usage 提取：读 §6、§11；
- 改后台设置或目录切换：读 §7、§8；
- 运维止血或切盘：读 §9；
- 定位代码：读 §10；跑测试读 §12。

## 1. 目标与演进

把 AI 网关模型调用的请求体、客户端设备信息和响应元信息写入本地 JSONL，用同一个 `archive_id` 关联一次交互，用于排障和 Token Analysis。不影响转发、计费、限流和错误处理。

| 阶段 | 变化 | 原因 |
| --- | --- | --- |
| 首版（2026-05-15） | 路由层中间件，同步写；默认开启，保存请求体与响应正文 | 内部小范围测试 |
| 异步写入（2026-05-30） | 默认关闭；异步有界队列 + 后台单 writer 持久句柄 + 按日轮转；`capture_response` 独立开关；管理后台热切换 `enabled`/`capture_response` | 生产 2026-05-29 单日文件约 1GB（1900+ 对记录，request 行均约 361KB、response 行均约 172KB、最大单行约 2.7MB），且源码默认值实际是开启，与注释“默认关闭”不符；旧实现每条记录 `MkdirAll` + 全局锁 + open/write/close，拖慢尾延迟 |
| 响应只存 usage（2026-06-07） | response 记录不再写正文，只存大小、hash、流式标记和 token usage | 响应正文体积大、排障价值低；日常分析只需状态、耗时、身份和 usage |
| 目录可配置（2026-06-07） | 后台可编辑归档目录，保存前校验，热切换 | 归档需常开，部署盘容量有限，扩盘后要把归档放到新盘 |
| 请求体上限可热改 | `max_request_body_bytes` 默认 8MiB → 16MiB，后台可热改（提交 `cfdf430a0`） | 生产观测到 Codex 大上下文请求达 10.7MB，8MiB 截断使 Token Analysis 只能降级入库 |
| Token Analysis 字段 | 记录增加 `client_request_id`、`response_id`、`token_analysis` | 与 `usage_logs` 和上游计费关联，见 Token Analysis 文档 |

## 2. 覆盖范围

仅 POST，且路径（去尾 `/`）属于以下之一（`shouldArchiveGatewayRequest`）；登录、后台管理、支付等接口不归档：

| 路径 | 说明 |
| --- | --- |
| `/v1/messages`、`/antigravity/v1/messages` | Claude/Antigravity Messages |
| `/v1/messages/count_tokens`、`/antigravity/v1/messages/count_tokens` | token 计数 |
| `/v1/responses`、`/responses`、`/backend-api/codex/responses` 及其子路径 | Responses API |
| `/v1/chat/completions`、`/chat/completions` | Chat Completions |
| `/v1beta/models/*`、`/antigravity/v1beta/models/*` | Gemini 原生 |
| `/v1/images/generations`、`/v1/images/edits`、`/images/generations`、`/images/edits` | 图片 |

WebSocket（GET `/responses` 建连无 body，后续消息在 WS 数据通道）不在范围内，需另行设计。

## 3. 写入模型

- `enabled=false` 且没有 provider（未注入 `SettingService`）时，`RequestArchiveWithProvider` 返回纯透传 handler：不读 body、不包装 `ResponseWriter`、不创建 goroutine。注入 provider 时即使配置默认关闭也会创建 writer，以便后台开启后生效；运行态关闭时 `shouldArchiveGatewayRequest` 与 `runtimeCfg.Enabled` 判断先于读 body，零额外开销。
- 每请求：`io.ReadAll` 请求体后立即用 `io.NopCloser(bytes.NewReader(body))` 还原给下游；生成 `archive_id`；`writer.SetDir(runtimeCfg.Dir)`；入队 `request` 记录；按 `capture_response` 决定是否包装 writer；`c.Next()` 后入队 `response` 记录。defer 中若 `c.Writer` 仍是包装器则还原为外层 writer。
- `asyncRequestArchiveWriter`：有界 channel（`queue_size`，默认 1024）。`Enqueue` 用 `select` 非阻塞入队，满则丢弃并累加 `dropped`，每丢 256 条（`dropped%256==1`）采样告警 `request_archive.queue_full_dropped`。后台单 goroutine `run()` 消费，`json.Marshal` 后写当日文件。
- `fileForToday()` 持有当日句柄，跨天或目录变化时关闭旧句柄重开。文件名 `<dir>/YYYY-MM-DD.jsonl`，日期按进程本地时区；记录内 `timestamp` 为 UTC RFC3339Nano。目录权限 `0o700`，文件 `0o600`。
- `Close()` 关闭独立的 `quit` channel 并等待 `done`，`run()` 收到后尽力排空队列再关闭句柄；数据 channel `ch` 永不关闭。当前生产不调用 `Close`（只有测试经 `CloseRequestArchiveWritersForTest` 调用），进程退出时已入队未落盘的记录会丢失，对排障数据可接受。
- 写入失败只记日志 `request_archive.write_failed`，不阻断用户请求。

## 4. 记录格式

每次请求通常两行：`event=request` 与 `event=response`，共用 `archive_id`。字段（`requestArchiveRecord`）：

| 字段 | 含义 |
| --- | --- |
| `archive_id`、`event`、`timestamp` | 关联 ID、`request`/`response`、UTC 时间 |
| `client_request_id` | 请求上下文 `ctxkey.ClientRequestID`，与 `usage_logs.request_id` 的 `client:` 键同源 |
| `method`、`path`、`endpoint` | 方法、原始路径、归一化入站端点（`_gateway_inbound_endpoint`） |
| `user_id`、`api_key_id`、`group_id`、`account_id` | 认证主体 / API Key（无主体时回落 Key 的 user_id）/ 分组 / 调度账号（`ops_account_id`，回落 `_gateway_selected_account_id`） |
| `model` | 上下文 `ops_model`，request 行回落 body 的 `model` |
| `client_ip`、`user_agent`、`headers` | headers 只取 allowlist（`archiveHeaderAllowlist`）：Content-Type、Accept、User-Agent、X-Request-Id、X-Client-Request-Id、X-Forwarded-For、X-Real-Ip、Openai-Organization、Anthropic-Version、Anthropic-Beta |
| `body`、`body_size`、`body_sha256`、`body_truncated` | request 行：正文按 `max_request_body_bytes` 截断，`body_size`/`body_sha256` 按完整正文计算。response 行：**无 `body`**，大小与 hash 按完整响应计算，超 `max_response_body_bytes` 时 `body_truncated=true` |
| `status`、`duration_ms`、`stream` | 仅 response 行 |
| `usage` | 仅 response 行且 `capture_response=true` 时（§6） |
| `response_id` | 上游 Responses 对象 id（SSE 中最后出现的 `response.id`，非流式取顶层 `object=response` 的 `id`），WS 上游计费的 request_id |
| `token_analysis` | `service.TokenAnalysisForwardFactsFromContext` 的最后一次转发事实，仅原生 OpenAI Responses 路径写入 |

`Authorization`、`Cookie`、`Set-Cookie`、`X-Api-Key` 等敏感头不在 allowlist，不落盘。请求正文本身不脱敏，可能含 PII。

## 5. 配置

`gateway.request_archive`（`backend/internal/config/config.go` 的 `GatewayRequestArchiveConfig`，示例 `deploy/config.example.yaml`）：

| 配置 | 默认 | 后台热改 | 说明 |
| --- | ---: | --- | --- |
| `enabled` | `false` | 是 | 总开关 |
| `capture_response` | `false` | 是 | 捕获响应元信息与 usage，不保存正文 |
| `dir` | `data/request-archive` | 是（自定义绝对路径优先） | 配置值作为默认目录 |
| `max_request_body_bytes` | 16777216（16MiB） | 是（64KiB–512MiB；0 或等于默认值即恢复默认） | 截断的请求体无法被 Token Analysis 解析，只能降级为元数据入库 |
| `max_response_body_bytes` | 2097152（2MiB） | 否，需重启 | 只影响 `body_truncated` 标记，正文不写入 |
| `queue_size` | 1024 | 否，需重启 | writer 构造时固化 |

中间件内部兜底（`normalizeRequestArchiveConfig`、`defaultRequestArchiveSettings`）在配置显式为 0 时使用 64KiB 的 body 上限；正常部署走 Viper 默认值。

运行态配置链路：settings 表键 `request_archive_settings`（`SettingKeyRequestArchiveSettings`）→ `SettingService.GetRequestArchiveRuntimeConfig`（`atomic.Value` 缓存 5 秒，`singleflight` 合并回源，Do 内二次检查，`context.WithoutCancel` + 2 秒 DB 超时；读库失败沿用旧缓存 1 秒，无旧缓存回落配置默认）→ `NewRequestArchiveConfigProvider` → 中间件每请求读取并经 `mergeRequestArchiveRuntimeConfig` 合并（`Dir`、`Max*BodyBytes` 为空/≤0 时取基础配置，`QueueSize` 恒取基础配置）。保存后本进程立即更新缓存，多实例最长 5 秒生效。

持久化内容（`persistedRequestArchiveSettings`）只有 `enabled`、`capture_response`、`dir`（空=默认）、`max_request_body_bytes`（0=默认）。JSON 损坏时 `slog.Warn` 并回落配置默认（即关闭）。

## 6. 响应捕获与 usage 提取

`capture_response=false`：不包装 writer，response 行只有状态、耗时、身份等基础字段。

`capture_response=true`：`requestArchiveResponseWriter` 包装 writer，统计 `body_size`、`body_sha256`、`body_truncated`、`stream`（Content-Type `text/event-stream` 或调用过 `Flush`），提取 `usage` 与 `response_id`，**不保存响应正文**。

| 协议/形态 | usage 位置（`archiveUsageJSONPaths` 顺序） |
| --- | --- |
| OpenAI Chat / Anthropic Messages 非流式 | `usage` |
| OpenAI Responses | `usage` 或 `response.usage` |
| Gemini（JSON 与 SSE、JSON 数组分片） | `usageMetadata` |
| Anthropic SSE 死在 `message_delta` 前 | `message_start` 的 `message.usage`（放最后，避免抢占 `message_delta` 顶层累计 usage） |

- SSE：`Write` 时增量按行扫描 `data:`，忽略空行与 `[DONE]`，取最后一次非空 usage；行缓冲上限 256KB。请求结束时 `Usage()` 冲洗残留缓冲，兜底终止事件缺少结尾换行。单行超 256KB 被裁掉行首后，带 `"usage"`/`"usageMetadata"` 标记的碎片走 `extractArchiveUsageFromFragment`（取最后一次出现的键，括号平衡截取对象）。
- 非流式：`Write` 热路径只维护最后 256KB 尾部窗口（`appendJSONTail`），提取推迟到请求结束执行一次；窗口不是完整 JSON 时降级 fragment 提取。
- 生产回放（2026-06-05 全天 10,858 条响应）：`/v1/responses` 99.5%、`/v1/chat/completions` 99.4%、`/v1/messages` 99.8%、非流式 100% 提取成功；失败样本均为流中断导致上游未下发 usage。`message_delta` 实测携带全量累计 usage，覆盖语义正确。

## 7. 管理后台接口与页面

- 接口：`GET/PUT /api/v1/admin/settings/request-archive`（`backend/internal/server/routes/admin.go`，handler 在 `backend/internal/handler/admin/setting_handler_runtime.go`），admin 专属。
- 响应 DTO（`dto.RequestArchiveSettings`）：`enabled`、`capture_response`、`dir`（当前生效）、`default_dir`、`dir_customized`、`disk_total_bytes`、`disk_free_bytes`（当前生效目录所在磁盘，查询失败为 0，不阻塞接口）、`max_request_body_bytes`、`max_response_body_bytes`、`queue_size`。
- PUT 请求（`UpdateRequestArchiveSettingsRequest`）：`enabled`、`capture_response` 必填；`dir` 可选（省略=不改，空串=恢复默认，非空=自定义绝对路径）；`max_request_body_bytes` 可选（省略=不改，0=恢复默认）。只接收这些字段，`max_response_body_bytes`、`queue_size` 从协议层不可改。
- 校验失败返回 400 与具体 code：`REQUEST_ARCHIVE_DIR_NOT_ABSOLUTE`、`REQUEST_ARCHIVE_DIR_VOLUME_NOT_FOUND`、`REQUEST_ARCHIVE_DIR_IS_FILE`、`REQUEST_ARCHIVE_DIR_CREATE_FAILED`、`REQUEST_ARCHIVE_DIR_NOT_WRITABLE`、`REQUEST_ARCHIVE_MAX_REQUEST_BODY_INVALID`；失败不落库。
- 页面：`/admin/settings` → Gateway 标签页 → “请求归档”卡片（`frontend/src/views/admin/SettingsView.vue`，i18n `admin.settings.requestArchive.*`；API `frontend/src/api/admin/settings.ts` 的 `getRequestArchiveSettings`/`updateRequestArchiveSettings`）。目录输入、恢复默认、默认值、磁盘剩余（文案明确为“当前生效目录所在磁盘剩余”）、切换须知 `dirSwitchWarning` 位于 `enabled` 条件之外，关闭归档时也可改目录；`capture_response` 等在启用后显示。

## 8. 归档目录热切换

- 校验 `ValidateRequestArchiveDir`（`backend/internal/service/request_archive_dir.go`）：① 必须绝对路径（相对路径含义随部署方式漂移）；② 卷存在（`os.Stat(VolumeName + 分隔符)`，典型为 Windows 盘符填错或新盘未挂载）；③ 不能是已存在文件，不存在则 `MkdirAll(0o700)`；④ 写探针（创建并删除临时文件）。保存会产生文件系统副作用。
- 保存前 `filepath.Clean`；与默认目录等价（`requestArchiveDirEquals`，Windows 下 `EqualFold`）时按“恢复默认”处理，避免把默认值固化为自定义。未做 `EvalSymlinks`。
- 写入端：`asyncRequestArchiveWriter.dir` 为 `atomic.Value`，`SetDir` 值未变化时一次原子读即返回；`fileForToday` 在目录变化时轮转，下一条记录写入新目录。
- 索引端：`TokenAnalysisService.archiveDir(ctx)` 优先取运行态目录，未注入 `SettingService` 时回落配置（`tokenAnalysisArchiveDir`），与写入端同源。
- 磁盘容量：`backend/internal/pkg/diskspace` 的 `Usage(path)`，Windows 走 `GetDiskFreeSpaceEx`，其他平台 `unix.Statfs`，build tag 分文件。
- 自定义目录持久化在 DB，重启后仍生效；config.yaml 的 `dir` 只是默认值。

## 9. 运维

- 本地/源码配置 `backend/config.yaml`；部署示例 `deploy/config.example.yaml`；生产以运行进程加载的配置为准（通常 `/opt/sub2api/backend/config.yaml` 或容器内挂载的同名文件）。
- 止血：优先在后台 Gateway 标签页关闭“请求归档”；settings DB 不可用时，在运行配置显式写 `gateway.request_archive.enabled: false` 并重启。
- 切换目录：建议深夜低峰、先关归档再切换。历史 JSONL 不自动迁移；切换当日文件会分裂在新旧两个目录，而索引只读当前目录。旧文件如需继续被索引，手动移到新目录后重新触发索引（Token Analysis 按 `archive_id` 幂等，不会重复）。
- 磁盘：只按日轮转，单日文件无大小上限、无自动清理，靠容量规划与人工清理。Token Analysis 对 JSONL 的保留/删除约束见其文档，删除前先确认。
- 默认归档目录已在 `.gitignore`（`data/request-archive/`，无前导斜杠，任意层级匹配），避免误提交；自定义到仓库外的目录不受影响。

## 10. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/internal/server/middleware/request_archive.go` | `RequestArchive`、`RequestArchiveWithProvider`、`NewRequestArchiveConfigProvider`、`archiveEnqueuer`、`newRequestArchiveHandler`、`mergeRequestArchiveRuntimeConfig`、`shouldArchiveGatewayRequest`、`normalizeRequestArchiveConfig`、`requestArchiveRecord`、`buildArchiveRecord`、`archiveHeaderAllowlist`、`captureBytes`；writer `asyncRequestArchiveWriter`（`Enqueue`、`SetDir`、`run`、`Close`、`writeRecord`、`fileForToday`）、`registerRequestArchiveWriter`、`CloseRequestArchiveWritersForTest`；响应 `requestArchiveResponseWriter`（`capture`、`Usage`、`ResponseID`、`appendJSONTail`、`captureSSEUsage`、`captureSSELineUsage`）、`archiveUsageJSONPaths`、`extractArchiveUsageFromJSON`、`extractArchiveUsageFromFragment` |
| `backend/internal/server/routes/gateway.go` | 构造 `requestArchive` 中间件（注入 `settingService`）并挂到各 AI 网关路由组 |
| `backend/internal/service/setting_request_archive.go` | `GetRequestArchiveSettings`、`SetRequestArchiveSettings`、`GetRequestArchiveRuntimeConfig`、`defaultRequestArchiveSettings`、`getRequestArchiveSettingsUncached`；常量 `requestArchiveRuntimeCacheTTL`、`minRequestArchiveBodyBytes`、`maxRequestArchiveBodyBytes` |
| `backend/internal/service/settings_view.go` | `RequestArchiveSettings`、`persistedRequestArchiveSettings` |
| `backend/internal/service/request_archive_dir.go` | `ValidateRequestArchiveDir`、`requestArchiveDirEquals` |
| `backend/internal/service/token_analysis_indexer.go` | `(*TokenAnalysisService).archiveDir` |
| `backend/internal/pkg/diskspace/` | `Usage` |
| `backend/internal/handler/admin/setting_handler_runtime.go`、`backend/internal/handler/dto/settings.go` | `GetRequestArchiveSettings`、`UpdateRequestArchiveSettings`、`UpdateRequestArchiveSettingsRequest`、`dto.RequestArchiveSettings` |
| `backend/internal/config/config.go` | `GatewayRequestArchiveConfig` 与默认值 |
| 前端 | `frontend/src/views/admin/SettingsView.vue`、`frontend/src/api/admin/settings.ts` |

## 11. 关键约束与易错点

1. **热路径不得阻塞**：队列满丢弃 + 采样告警，不改成阻塞或加锁。依据：归档是可丢的排障数据，阻塞会把磁盘抖动传导到请求尾延迟；旧同步实现正是事故根因。
2. **不得关闭数据 channel `ch`**：停止只能关 `quit`。曾复现：旧 `Close()` 执行 `close(w.ch)`，并发 `Enqueue` 时 `send on closed channel`，32 goroutine 压测单轮 34,907 次 panic；回归用例 `TestAsyncWriterCloseDuringConcurrentEnqueueDoesNotPanic`。
3. **默认值必须与注释一致（默认关闭）**。依据：首版注释写“默认关闭”而默认值为开启，生产未显式配置即长期归档，单日约 1GB。
4. **response 行不得写正文**；`capture_response` 的含义是“捕获响应元信息和 usage”，前端文案、示例配置与 wiki 不得再写成“保存响应体”。
5. **usage 提取只在请求结束执行一次**，`Write` 热路径只做窗口追加和 SSE 行切分；不要在每次 `Write` 时解析整个窗口。
6. **SSE 结束时必须冲洗残留缓冲**；超长行裁剪后必须走 fragment 降级。依据：终止事件无尾换行或 Responses `response.completed` 超 256KB 时 usage 会静默丢失。
7. **`message.usage` 必须排在提取路径最后**，否则 `message_start` 的部分 usage 会覆盖 `message_delta` 的累计全量值。
8. **`QueueSize` 不参与运行态合并**：writer 构造时固化，合并它会制造“已生效”的错觉（审核时曾确认 `Dir`/`QueueSize` 合并是死逻辑；后来 `Dir` 通过 `SetDir` 真正支持热切换，`QueueSize` 仍不支持）。
9. **改目录前必须校验并在失败时不落库**；与默认等价的路径按恢复默认处理。
10. **写入端与索引端目录必须同源**（都取 `GetRequestArchiveRuntimeConfig`）；新增读取归档的代码不得直接读 `cfg.Gateway.RequestArchive.Dir`。
11. **目录输入不能放在 `enabled` 条件内**。曾复现：输入框只在开启时显示，与“先关归档再切换”的运维建议矛盾。
12. **磁盘容量只代表当前已保存目录**，不是目标盘预览；文案不得暗示预览。
13. 测试中 writer 异步落盘：读取记录要 `require.Eventually` 轮询；`useRequestArchive(t, cfg)` 在 `t.Cleanup` 中 `Close()`，否则 Windows 下 TempDir 清理失败。
14. Windows + `-race` 偶发 `fork/exec ... Access is denied`（Defender 拦截临时测试 exe），属环境问题；去掉 `-race` 或用新的 `GOTMPDIR` 重跑。

已否决 / 未采纳：
- 已否决：后台写入加 `bufio.Writer`。原因：基准显示无缓冲 12.8µs/op、带缓冲 1.3µs/op，但单 writer 端到端约 70,000 条/秒，而当时生产约 1900 对/天，没有瓶颈；缓冲还会增加崩溃丢失窗口。
- 已否决：`Enqueue` 加 `recover` 或用读写锁包 send/close。原因：前者掩盖问题且热路径每条 defer，后者引入锁竞争。
- 暂不采纳：`dropped` 进指标。原因：仓库无 Prometheus/metrics 注册体系，采样告警足够。
- 不采纳：自定义目录 allowlist/敏感目录黑名单。原因：从简；目录配置与 `enabled`/`capture_response` 同为 admin 权限，能开归档的人本就能让敏感请求体落盘，不扩大影响面。若要收紧，加配置级 `gateway.request_archive.allowed_dirs` 白名单。管理员需自行避免指向 Web 静态目录或系统目录。
- 不采纳：多目录索引 / 目录变更历史。原因：历史文件不迁移是已确认的产品决策，页面已提示手动移动。
- 不采纳：新增目录“校验/预览”接口。原因：保存即校验并返回新目录容量，满足低峰运维场景。

## 12. 验证

### 12.1 命令

```bash
cd backend
go build ./...
go test -tags=unit ./internal/server/middleware/ ./internal/config/ ./internal/server/routes/ ./internal/handler/admin/
go test -tags=unit ./internal/service/ -run 'RequestArchive|TokenAnalysisIndexerFollowsRuntimeArchiveDir|TokenAnalysisListArchiveFilesFollowsRuntimeDir'
go test ./internal/pkg/diskspace/
go test -tags=unit -race ./internal/server/middleware/ -run TestAsyncWriterCloseDuringConcurrentEnqueueDoesNotPanic -count=20
pnpm --dir ../frontend run typecheck
```

`internal/service` 的设置用例在 `unit` 标签下才编译。

### 12.2 测试清单

- `backend/internal/server/middleware/request_archive_test.go`：关联写入、`client_request_id`、`response_id`、关闭不写/运行态关闭不读 body、`capture_response=false` 不包装 writer、非流式与 SSE usage 不写正文、无尾换行、Gemini `usageMetadata`、Anthropic `message_start` 兜底、超长 SSE 行、截断前缀后的尾窗提取、目录热切换 `TestRequestArchiveHotSwitchesDirViaRuntimeConfig`、队列满丢弃、跳过非模型请求、截断、外层 writer 还原、身份字段与 API Key user_id 回落、并发 Close 不 panic、`token_analysis` 序列化。
- `backend/internal/service/request_archive_settings_test.go`：默认值来自配置、持久化只覆盖可热改字段、写库后缓存即时更新、自定义目录持久化、空串恢复默认、自定义请求体上限及越界拒绝、非法目录拒绝、索引器与归档文件列表跟随运行态目录。
- `backend/internal/config/config_test.go`：`TestLoadDefaultGatewayRequestArchiveConfig`。
- `backend/internal/pkg/diskspace/diskspace_test.go`、`backend/internal/server/routes/gateway_test.go`。

### 12.3 最近结果

- 2026-05-30（异步写入修复）：middleware 包全绿；并发 Close 回归用例 `-race -count=20` 全过；middleware 包 `-race` 全绿（32 秒）。
- 2026-06-07（目录热切换）：service、middleware、diskspace（需新 `GOTMPDIR`，`-p 1`）、`go test ./cmd/server`（Wire 编译）、前端 typecheck、`git diff --check` 均通过。
- 本次文档合并未重跑。

## 13. 状态与待办

- [ ] 优雅停机 drain 与去全局 writer 注册表（合并为一个小迭代）：让路由注册方持有 writer 的 `io.Closer`，在 `cmd/server/main.go` 的 `Server.Shutdown` 后调用 `Close`，删除包级 `requestArchiveActiveWriters`。`Close` 已可安全用于生产。
- [ ] 按大小切片 + 保留 N 天自动清理，从根因防止开启期磁盘膨胀（须先与 Token Analysis 的 JSONL 保留约束对齐）。
- [ ] （可选）落盘前对请求正文中的已知敏感字段脱敏（可复用 `logredact`）。
- [ ] （可选）目录路径白名单 `allowed_dirs`。
