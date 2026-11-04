# Token Analysis 基础功能设计

更新：2026-11-04。状态：**页面、异步索引器、概览/排行、紧凑数字格式、选中用户计费趋势均已实现并在生产运行；2026-11-02 已对 `2026-10-31.jsonl` 做一次性受控补录。在线归档 writer 与索引器的故障恢复改造（§14）尚未实施；历史受控重索引（§13.3）仍待用户授权。**

本文是管理端 `/admin/token-analysis` 基础功能的唯一文档，包含页面结构、接口合同、归档 JSONL 索引器（水位、断点续扫、失败行处理、异步索引）、概览/排行口径、紧凑数字格式、选中用户趋势、生产故障排查与恢复方案。原始设计、执行计划、审核意见、三次线上索引错误调查和 2026-11-02 补录证据目录已并入本文并移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看，路径见 §17）。凡是对后续修改有约束的结论都已写入正文。

子功能文档（本文不重复其内容）：
- [风险分类](token-analysis-risk-classification-design-cn.md)：成本观察与缓存复用机会，取代固定风险分（数据层已实现，读接口/新页面未实施）；
- [请求来源](token-analysis-request-origin-design-cn.md)：`summary_json.request_origin`、人工输入计数；
- [计费关联](token-analysis-usage-link-design-cn.md)：摘要与 `usage_logs` 的关联、`token_analysis_pending_responses`；
- [项目归因](token-analysis-project-attribution-design-cn.md)：`client_project`/项目排行；
- [净输入留存](token-analysis-user-input-store-design-cn.md)：`token_analysis_user_inputs` 与全文懒加载。

阅读顺序建议：
- 改索引器或排查失败行：先读 §6 和 §12，再读 §13；
- 改页面或数字展示：读 §3、§7、§8；
- 改趋势图或 `/admin/dashboard/users-trend`：读 §9；
- 设计故障恢复：读 §13、§14；
- 定位代码：读 §11；跑测试：读 §15。

## 1. 目标与边界

目标：让管理员在不读原始 JSONL 的前提下，看到已归档请求的用量、缓存、费用、可疑请求与请求参数摘要，并按用户/项目/请求下钻。

固定边界（自原始设计起未变，后续修改不得突破）：

| 边界 | 规则 | 依据 |
| --- | --- | --- |
| 不改请求热路径 | 网关只写 JSONL 归档；摘要由后台异步索引生成，热路径不新增同步数据库写入 | 归档文件可达数十 GiB/天，同步写库会拖慢线上请求 |
| 不展示响应正文 | 只读 request 事件的 body 提取摘要；response 事件只取状态、usage、`response_id` 等事实 | 响应可能含敏感内容，体积与权限风险高 |
| 内容最小化 | 摘要表不存完整消息数组；预览默认 300 字符并脱敏（API Key、Bearer、Cookie、长 Base64、超长无空白串）；全文只进净输入留存表 | 降低误浏览与泄露风险 |
| 不做惩罚 | 风险分只用于排序与提示，不封禁、不扣费 | 规则存在误报 |
| 打开页面不扫文件 | 所有查询只读数据库摘要表 | 原始文件过大 |

## 2. 数据流

1. `server/middleware/request_archive.go` 在 `c.Next()` 前捕获原始请求体，按日写 `<归档目录>/YYYY-MM-DD.jsonl`；同一请求的 request/response 两个事件用 `archive_id` 关联。外层记录由 `json.Marshal` 生成，**外层合法不代表字符串字段 `body` 是合法 JSON**。归档目录、截断上限等配置见 `docs/features/request-archive-design-cn.md` 及 wiki backend 归档章节。
2. 后台索引器（§6）按文件水位续读，request 行生成摘要写 `token_analysis_request_summaries`（并做项目归因、净输入留存、指纹等子功能），response 行补完成事实/计费关联；找不到请求摘要的响应暂存 `token_analysis_pending_responses`。
3. 管理端接口（§4）只查询摘要表，按需 join `usage_logs` 取 token/费用。
4. 选中用户趋势（§9）是例外：直接查 `usage_logs`，不读摘要表。

## 3. 页面结构

路由 `/admin/token-analysis`，入口 `frontend/src/views/admin/TokenAnalysisView.vue`；展示组件在 `frontend/src/components/admin/token-analysis/`（目录 README 记录组件职责与验证入口，修改组件须同步）。

- **筛选卡**：共用 `DateRangePicker` + 邮箱/项目/模型/风险分/来源；ID 类筛选与「排行含未关联请求」收在“更多筛选”。已应用筛选是最近一次提交的快照 `appliedFilters`，以可移除标签显示；概览、列表、排序翻页、趋势、索引触发都只读该快照。默认日期取浏览器本地日（与请求 `timezone` 一致），不用 UTC 日期。
- **右侧索引状态胶囊 + “索引当前范围”按钮**（子管理员只读，不能触发索引）。
- **概览** `TokenAnalysisOverview.vue`：KPI 卡、Token 构成条、风险原因条形列表（点击切换全局 `risk_reason`）。
- **四个标签页**（`v-show`，列表在 `reloadAll` 中一次加载，计数显示在标签上）：用户排行 + 计费用量趋势（`TokenAnalysisUserRanking.vue`、`TokenAnalysisUserTrend.vue`）/ 项目（`TokenAnalysisProjectTable.vue`）/ 请求明细（`TokenAnalysisRequestTable.vue` + 抽屉 `TokenAnalysisRequestDrawer.vue`）/ 索引与归档（`TokenAnalysisIndexPanel.vue`）。
- 数据加载、竞态与索引轮询集中在 View，组件只做展示。共用格式与状态函数在 `tokenAnalysisFormat.ts`。
- 页面不提供“查看完整响应正文”。若将来需要，须单独做权限确认、审计和脱敏。

已知例外：索引完成后的轮询回调仍调用 `reloadAll` 并提交当前输入框草稿，待把“提交筛选”和“刷新数据”两个入口分离。

## 4. 接口合同

路由注册在 `server/routes/admin.go#registerTokenAnalysisRoutes`，前缀 `/api/v1/admin/token-analysis`。访问权限：完整管理员，或持有 `admin.token_analysis` 权限码的 `sub_admin`（只读）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/summary` | 概览（口径 §7.1） |
| GET | `/users` | 用户排行，默认 `sort_by=total_tokens desc`，仓储白名单映射排序字段 |
| GET | `/projects` | 项目 × 成员聚合（见项目归因文档） |
| GET | `/requests` | 请求明细；`sort_by` 仅 `event_time`/`risk_score`/`total_tokens`/`actual_cost`，默认 `risk_score`，其他值 400 |
| GET | `/requests/input?archive_id=` | 净输入全文懒加载，`archive_id` 必填 |
| POST | `/index` | 异步触发索引，body `{start_date,end_date,timezone}`，成功 202 |
| GET | `/index/status` | 索引状态（口径 §6.6） |
| GET | `/archive-files` | 归档目录文件与水位标签（§6.5） |

通用查询参数（`TokenAnalysisHandler.parseFilters`）：`start_date`/`end_date`（`YYYY-MM-DD`，按 `timezone` 解析，结束日为闭区间、内部 +1 天）、`user_id`/`api_key_id`/`account_id`/`group_id`（正整数）、`model`、`endpoint`、`risk_reason`、`risk_min`（0–100）、`project`、`user_email`、`origin`（`human|ai|unknown`）、`include_unmatched`（布尔）、`page`/`page_size`/`sort_by`/`sort_order`。非法值返回 400。

`POST /index` 错误码：日期非法或 `end_date < start_date` → 400 `TOKEN_ANALYSIS_INDEX_INVALID_RANGE`；已有索引在运行 → 409 `TOKEN_ANALYSIS_INDEX_RUNNING`。

## 5. 数据模型与迁移

| 迁移 | 内容 | 归属文档 |
| --- | --- | --- |
| 136 `token_analysis_request_summaries.sql` | 摘要表 `token_analysis_request_summaries`（`archive_id` 唯一）与水位表 `token_analysis_index_state` | 本文 |
| 145 | 项目归因列与 `token_analysis_project_roots` | 项目归因 |
| 146 | `token_analysis_user_inputs` | 净输入留存 |
| 247、248（_notx） | 计费关联列、`token_analysis_pending_responses`、关联键索引 | 计费关联 |
| 249–260 | 客户端族、转发事实、指纹、分类列与失效登记 | 风险分类 |

摘要表基础列（136）：`archive_id`、`usage_log_id`（FK `usage_logs`，`ON DELETE SET NULL`）、`match_confidence`、`event_time`、`user_id`/`api_key_id`/`account_id`/`group_id`、`model`、`endpoint`、`method`、`request_body_size`、`request_body_truncated`、`body_sha256`、`message_count`、`system_chars`、`user_chars`、`last_user_preview`、`tools_count`、`image_count`、`summary_json`、`risk_score`、`risk_reasons`、`indexed_at`、`source_file`、`source_offset`。索引覆盖 `(event_time DESC, id DESC)`、`(user_id|api_key_id|group_id, event_time DESC)`、`(risk_score DESC, event_time DESC)`、`(body_sha256, event_time DESC)`、`usage_log_id`。

- `source_offset` 是**该行结束后**的字节位置（与续读水位同口径），不是行首。
- `summary_json` 的约定标记：`degraded=body_truncated`、`multipart=true`/`source_image_count`、`degraded=multipart_body`（§6.3）；`request_origin` 见请求来源文档。

水位表 `token_analysis_index_state`（主键 `source_file`）：`last_offset`、`last_archive_id`、`processed_rows`、`failed_rows`（累加）、`last_error`、`started_at`、`finished_at`、`updated_at`。

`backend/migrations/*.sql` 受 checksum 校验，已应用文件不得修改。

## 6. 归档索引器

### 6.1 触发与互斥

- **自动**：服务启动即运行自动索引循环（`token_analysis_auto_index.go`），间隔 `token_analysis.auto_index_interval_seconds`（默认 300，0 = 仅手动）；每轮对 [昨天, 今天] 增量索引，跨天时补读昨日文件尾部。Wire cleanup 调用 `TokenAnalysisService.StopAutoIndex()`，合并 Wire 冲突时必须保留。
- **手动**：页面“索引当前范围”调用 `POST /index`。handler 同步校验日期与互斥后立即返回 202；索引在服务生命周期 goroutine 内执行（`lifecycleCtx` + 共享 WaitGroup，优雅停机时取消并等待）。前端每 3 秒轮询 `/index/status`，直到 `running=false` 再刷新页面。手动触发主要用于补历史日期。
- 自动与手动共用同一 running 互斥：撞车时自动轮静默跳过，手动返回 409。
- `running` 只来自服务内存标志，不按 `started_at` 非空且 `finished_at` 为空推断（依据：历史中断轮次会让该推断永远为真，前端轮询永不结束）。
- 互斥只在进程内，多实例之间没有跨进程锁（§14 列为待改造项）。

### 6.2 水位与断点续扫

`indexArchiveFile` 打开文件后读 `GetIndexState`，`Seek` 到 `last_offset` 续读；文件不存在则静默跳过（运维删除归档后不报错）。逐行处理：

- 每行结束后 `offset` 前移到该行末尾；有入库或失败时立即写水位（`UpdateIndexState`），并在开始/结束时写 `started_at`/`finished_at`。
- **EOF 处无换行的不完整尾行不推进 offset**（依据：写入端可能仍在追加当前行），下轮重读。
- 行级错误（解析失败或数据库写入失败）都会：`failed_rows` +1、`last_error` 记为该错误（先过 `sanitizeTokenAnalysisText`）、**offset 照常前移**、继续下一行。
- 所有 `UpdateIndexState` 返回值被忽略。
- `UpsertRequestSummary` 按 `archive_id` 幂等；重复索引同一行不会产生重复摘要（已知副作用：重扫会使 `duplicate_count` 自含 +1）。
- `UpdateIndexState` 用 `COALESCE(NULLIF(EXCLUDED.last_error,''), last_error)`：成功行与正常收尾**不清除**历史错误（粘性）。

### 6.3 行处理规则（`indexArchiveLine`）

| 行类型 | 处理 | 计数 |
| --- | --- | --- |
| 空行、非 request/response 事件 | 跳过 | skipped |
| request，body 为合法 JSON | `SummarizeTokenAnalysisRequest` 生成摘要 + 风险分 + 子功能写入 | processed |
| request，`body_truncated=true` 导致 JSON 不完整 | 降级为仅元数据入库，`summary_json.degraded=body_truncated`，保留 model/body_size/截断标记 | processed，不计 failed |
| request，multipart 体（如 `/v1/images/edits`，body 以 `--boundary` 开头） | `SummarizeTokenAnalysisMultipartRequest` 按 form 字段解出 prompt/model/n/size 与输入图数，标 `multipart=true`；boundary 优先取 `headers.content_type`，缺失从 body 首行兜底。截断的 multipart 因文本域在图片分片前同样能解（优先于截断降级）；完全解不出则 `degraded=multipart_body` | processed，不计 failed |
| request，未截断且非 multipart 的非法 JSON | 返回 `parse request body: ...`，不写摘要 | failed |
| count_tokens 探测（`tokenAnalysisIsCountTokensPath` 按**原始 path** 判断） | 不参与计费匹配，入库 endpoint 还原为 `/v1/messages/count_tokens` | processed |
| response | 补完成事实/计费关联；无对应摘要时写 `token_analysis_pending_responses`（失败报 `stash token analysis response`） | 失败计 failed |

count_tokens 必须用原始 path 判断，依据：归档 `endpoint` 已被 `NormalizeInboundEndpoint` 折叠成 `/v1/messages`；此前探测请求与紧邻真实请求匹配到同一条 `usage_logs`，概览把该笔 token 累加两次。

### 6.4 文本清洗（NUL 与非法 UTF-8）

归档 JSON 字符串中合法的 `\u0000` 解码后是真实 0x00 字节，PostgreSQL text/jsonb 拒收（`pq: invalid byte sequence for encoding "UTF8": 0x00`）。规则：**所有 body 衍生文本入库前必须过 `sanitizeTokenAnalysisText`**（`token_analysis_summary.go`，剥 NUL 并替换非法 UTF-8）。已接入：净输入留存（`RedactTokenAnalysisInputText`，哈希对剥离后文本计算）、预览（`SanitizeTokenAnalysisPreview`）、model 等字符串、shape、endpoint、归档行 method/endpoint/path/model、`archive_id`/`body_sha256` 兜底、`last_error`。项目归因中含 NUL 的 cwd 直接拒绝（`validAttributionCwd`），branch 剥离后保留。全字段回归：`TestTokenAnalysisIndexerNULInEveryFieldStillIndexesClean`。新增入库字段时必须接入并补该测试。

### 6.5 归档文件生命周期

`GET /archive-files`（`token_analysis_archive_files.go#ListArchiveFiles`）列出当前归档目录文件并按水位打标签：

| 状态常量 | 含义 |
| --- | --- |
| `writing` | 今日文件，写入端持有句柄，不可删 |
| `indexing` | 水位未追平文件大小 |
| `deletable` | 已读完且无失败行，可 gzip/删除 |
| `attention` | 已读完但有失败行，删除即放弃重处理机会 |
| `compressed` | `.gz`，不参与索引 |

删除由运维在服务器手动执行，接口和页面不提供删除动作。`deletable` 只表示水位读到 EOF 且 `failed_rows=0`，不证明所有子功能事实（响应、计费关联）都已齐全。

### 6.6 状态接口口径

`GetIndexStatus` 只汇总按 `updated_at DESC LIMIT 50` 的最近 50 个文件水位：`processed_rows`/`failed_rows` 为这 50 条累计，顶层 `last_error` 取其中首个非空历史错误。它**不跟随页面日期筛选，也不等于当前目录文件集合**。前端 `tokenAnalysisIndexState` 在非运行态只要 `last_error` 非空就判为 `error`，`TokenAnalysisIndexPanel.vue` 无条件显示该错误，最近水位只展示前 5 条；标签旁的数字是 `archiveFiles.length`（当前目录文件数），不是错误数。

## 7. 概览与排行口径

### 7.1 概览

- 统计对象是 `token_analysis_request_summaries`（已归档且已索引的请求），不是全量请求。
- service 层强制 `include_unmatched=true`（`GetSummary`、`ListRequests`），未匹配数/未匹配率卡片才有意义；`include_unmatched` 只影响用户/项目排行的请求数，前端勾选名为“排行含未关联请求”。
- 总 token/费用只含已关联 `usage_logs` 的行，未关联行贡献 0。
- 响应附 `billed_requests`（同期 `usage_logs` 计费请求数，只应用时间/用户/邮箱/密钥/账号/分组/模型过滤）与 `archive_coverage`（matched/billed），页面以“已归档请求数 / 同期计费请求数 / 归档覆盖率”呈现差距。
- 邮箱筛选同时作用于概览（含计费分母）。人工输入次数 `human_input_count` 口径见请求来源文档。

### 7.2 排行与请求明细

- 用户排行与项目排行是**归档样本口径**；“计费用量趋势”（§9）是 `usage_logs` 口径，两者数字可以不同，页面标题明确区分。
- 请求明细始终是完整归档视角（服务端忽略 `include_unmatched`）。非 linked 行的 Token/费用由服务端补 0，前端显示 `—` 并说明“未知，不代表零消耗”（见计费关联文档）。

### 7.3 当前风险分（旧规则）

`token_analysis_risk.go#ScoreTokenAnalysisRisk` 仍在生效，分值累加后截断到 [0,100]，每条原因携带指标：

| code | 分 | 条件 |
| --- | ---: | --- |
| `huge_input_tiny_output` | 45 | input ≥ 100000 且 output ≤ 200 |
| `repeat_uncached_body` | 35 | 同 body 近期 ≥ 3 次、input ≥ 20000、cache_read < input/10 |
| `low_cache_hit_large_input` | 25 | input ≥ 50000、cache_read < input/20 |
| `rapid_similar_requests` | 20 | 近期相似 ≥ 5 次、input ≥ 10000 |
| `oversized_system_prompt` | 20 | system_chars ≥ 20000，或 system_chars > 0 且 > user_chars |
| `tool_heavy_short_output` | 15 | tools ≥ 10、input ≥ 20000、output ≤ 500 |
| `large_tool_history` | 25 | 历史 tool 输出 ≥ 512 KiB 且 tool 消息 ≥ 10 |
| `giant_tool_output` | 25 | 单条 tool 输出 ≥ 512 KiB |

该固定分将被风险分类重设计取代，改规则前先读风险分类文档。

## 8. 紧凑数字格式

用户确认的页面专属规则（2026-07-13 需求，07-14 审核补全）。实现集中在 `tokenAnalysisFormat.ts`：

| 函数 | 规则 |
| --- | --- |
| `finiteNumber` | `null`/`undefined`/`NaN`/`Infinity` → 0 |
| `formatInteger` | `Intl.NumberFormat().format(Math.round(v))`，完整千分位整数，用于精确值与 `title` |
| `formatTokenMetric` | abs < 1,000,000 显示完整整数（**不用 K**）；≥ 1M 用 `M`，≥ 1B 用 `B` |
| `formatRequestMetric` | abs < 1000 原值；≥ 1000 用 `K`/`M`/`B` |
| `formatCost` | `$x.xxxx`（4 位小数） |
| `formatCompactCost` | abs < 1000 同 `formatCost`；≥ 1000 为 `$x.xK`，**永不升到 M/B**（`1_200_000` → `$1200.0K`） |

公共约定：紧凑值统一 `toFixed(1)`（边界值为 `1.0K`/`1.0M`/`1.0B`）；单位按格式化前绝对值选择，舍入后不晋级（`999_999_999` → `1000.0M`，请求数 `999_999` → `1000.0K`）；单位固定英文大写，不随 locale 变“万/亿”。`formatTokenMetric`/`formatRequestMetric` 内部调用共享 `utils/format.ts#formatCompactNumber`，但不扩展它的行为。

页面映射：

| 位置 | 展示 | `title` |
| --- | --- | --- |
| 概览请求数类（已归档、同期计费等） | `formatRequestMetric` | `formatInteger` |
| 概览 Token 类与构成条 | `formatTokenMetric` | `formatInteger` |
| 概览总费用主值 | 组件内 `formatHeroCost`：< $1 用 `formatCost`，≥ $1 为千分位 2 位小数 | `formatCost` |
| 概览风险费用 | `formatCost` | — |
| 用户排行请求数 / Token / 费用 | `formatRequestMetric` / `formatTokenMetric` / `formatCompactCost` | `formatInteger` / `formatInteger` / `formatCost` |
| 项目排行 Token、趋势纵轴 | 共享 `formatCompactNumber`（含 K） | 项目总 Token 为 `formatInteger` |

有意保留的同页差异：项目排行 Token 仍用 K/M/B，用户排行 Token 禁用 K。原生 `title` 满足桌面端查看精确值，不做移动端弹层。负金额不是合法业务输入，不为其写验收断言。

## 9. 选中用户计费趋势

需求：从用户排行选择最多 5 名用户，按北京时间日或小时比较每人的总 Token。

后端：扩展已有 `GET /api/v1/admin/dashboard/users-trend`（`dashboard_handler.go`、`usage_log_repo_trend.go`），不新增接口、表或迁移。

| 参数 | 规则 |
| --- | --- |
| `user_ids` | 逗号分隔正整数，去重升序，最多 5 个（`selectedUserTrendMaxUsers`）；显式空值、空片段、非正数、超限 → 400 |
| `start_date`/`end_date` | 精确选人模式必填，闭区间；`end < start` → 400 |
| `granularity` | `day` 最多 90 个自然日；`hour` 必须同一天 |
| `limit` | 仅未传 `user_ids` 的旧 Top N 模式生效；选人模式规范化为 0 |

- 不传 `user_ids` 保持 Dashboard 原 Top N 行为；显式 `user_ids=` 是 400，两者语义不同。
- 缓存键包含规范化后的 `user_ids`：集合相同顺序不同命中同一缓存。
- 数据源固定为 `usage_logs`；指标为 `input + output + cache_creation + cache_read`；时区固定 `Asia/Shanghai`；粒度格式走 `safeDateFormat` 白名单，`user_ids` 用 `pq.Array` 绑定。响应沿用 `UserTrendResponse`/`UserUsageTrendPoint`。
- 查询依赖已有 `idx_usage_logs_user_created (user_id, created_at)`；是否需要新索引只能由真实 `EXPLAIN ANALYZE` 决定。原始 `usage_logs` 默认保留 90 天，因此不承诺更早数据。

前端：用户排行首列复选框（无 `user_id` 的行不可选），选择状态保存用户 ID、邮箱、选择顺序，跨分页保留；满 5 人只禁用未选项。选择 ≥ 1 人后显示趋势面板（按日/按小时，小时仅在单日范围可用）。每人一条线，颜色跟随选择顺序（`USER_TREND_COLORS_*`）；前端按日期范围补齐横轴，缺失周期填 0。用户/粒度切换与刷新用请求序号丢弃迟到响应，失败显示错误并可重试。

已否决：方案 B（前端并发调 5 次单用户 `/admin/dashboard/trend`），原因是 5 次聚合与独立缓存、错误/取消处理更复杂；方案 C（新建 Token Analysis 专属趋势接口），原因是重复用量领域逻辑且易误用归档样本口径。

## 10. 配置

`config.go#TokenAnalysisConfig`（前缀 `token_analysis.`）：

| 键 | 默认 | 校验 / 说明 |
| --- | --- | --- |
| `index_enabled` | true | 关闭后不启动自动索引与分类作业 |
| `index_batch_size` | 1000 | 必须 > 0；**当前索引器未读取该值**（逐行写水位），见 §16 |
| `max_preview_chars` | 300 | 1–2000 |
| `auto_index_interval_seconds` | 300 | ≥ 0，0 = 仅手动 |
| `usage_match_window_seconds` | 10 | 1–120，计费关联时间窗 |
| `input_store_max_chars` | 8000 | ≥ 0，见净输入留存文档 |

`client_family_rules` 等分类配置见风险分类文档。归档目录可在后台热切换，索引器经 `archiveDir` 优先读运行态设置，未设置时回退 `config.yaml`。

## 11. 代码地图

后端（`backend/internal/`）：

| 位置 | 内容 |
| --- | --- |
| `server/routes/admin.go#registerTokenAnalysisRoutes` | 路由 |
| `handler/admin/token_analysis_handler.go` | `TokenAnalysisHandler`：`Summary`/`Users`/`Projects`/`Requests`/`RequestInput`/`TriggerIndex`/`IndexStatus`/`ArchiveFiles`、`parseFilters` |
| `service/token_analysis_service.go` | `GetSummary`、`ListRequests`（强制 `IncludeUnmatched`）、`IndexRangeAsync`、错误码 |
| `service/token_analysis_auto_index.go` | 自动索引循环、`StopAutoIndex` |
| `service/token_analysis_indexer.go` | `indexRange`、`indexArchiveFile`、`indexArchiveLine`、`tokenAnalysisIsCountTokensPath`、`tokenAnalysisIndexDates`、`archiveDir` |
| `service/token_analysis_summary.go` | `SummarizeTokenAnalysisRequest`、`SummarizeTokenAnalysisMultipartRequest`、`sanitizeTokenAnalysisText`、`SanitizeTokenAnalysisPreview`、`RedactTokenAnalysisInputText` |
| `service/token_analysis_risk.go` | `ScoreTokenAnalysisRisk` |
| `service/token_analysis_archive_files.go` | `ListArchiveFiles` 与状态常量 |
| `service/token_analysis_types.go` | DTO、过滤器、`TokenAnalysisRepository` 接口、风险 code 常量 |
| `repository/token_analysis_repo.go` | `UpsertRequestSummary`、`GetIndexState`、`UpdateIndexState`、`GetIndexStatus`、`RecordResponse`、`MergePendingResponses`、`tokenAnalysisHumanInputSQL` |
| `server/middleware/request_archive.go#writeRecord` | 归档写入（短写缺陷见 §12） |
| `handler/admin/dashboard_handler.go`、`repository/usage_log_repo_trend.go` | 选中用户趋势 |
| `config/config.go#TokenAnalysisConfig` | 配置与校验 |
| `service/admin_permission.go#AdminPermissionTokenAnalysis` | 权限码 `admin.token_analysis` |

前端（`frontend/src/`）：`views/admin/TokenAnalysisView.vue`（筛选、`appliedFilters`、`reloadAll`、轮询、选人）；`components/admin/token-analysis/` 下 `TokenAnalysisOverview.vue`、`TokenAnalysisUserRanking.vue`、`TokenAnalysisUserTrend.vue`、`TokenAnalysisProjectTable.vue`、`TokenAnalysisRequestTable.vue`、`TokenAnalysisRequestDrawer.vue`、`TokenAnalysisIndexPanel.vue`、`tokenAnalysisFormat.ts`（格式化、`tokenAnalysisIndexState`、`MAX_SELECTED_TREND_USERS`）；`api/admin/tokenAnalysis.ts`、`api/admin/dashboard.ts`（`user_ids` 序列化）；文案 `i18n/locales/{zh,en}/admin/tokenAnalysis.ts`。

## 12. 关键约束与易错点

1. **规则：失败计数不能当作“当前故障”或“请求数”。** 依据：2026-10-25 页面显示失败 11、10-27 显示 8，都是最近 50 个水位的历史累计；当前目录文件全部 `failed_rows=0`。10-27 全库实际 146 条水位、累计 14 条失败，数字下降只是 `2026-09-07.jsonl` 滑出 50 条窗口，并未恢复。失败数也包含 response 失败与坏行，2026-11-02 的 178 次失败不等于 178 条请求。
2. **规则：水位 `updated_at` 不是错误发生时间；`last_offset`/`last_archive_id` 不能定位失败行。** 依据：失败后 offset 继续前移，后续成功行覆盖这两个字段；每个文件只保留最后一个错误，不能断言同文件所有失败同因。
3. **规则：`parse request body:` 表示内层 body 不是合法 JSON，不代表文件损坏、归档失败、在线请求失败或未计费。** 依据：10-25 复现 `{model:"example","input":"hello"}` 精确得到 `invalid character 'm' looking for beginning of object key string`；历史还出现过 `'('`、`'\x00'`、请求体开头的 BOM（U+FEFF）。这只是语法机制验证，不能据此断言调用方。修复方向是发送端使用标准 JSON 序列化；**不在索引器里猜测补引号，不把 `body_truncated` 当作掩盖任意坏 JSON 的手段**。
4. **规则：重新点击“索引当前范围”不能修复已越过的失败行，也不会清除历史错误。** 依据：增量续读从 `last_offset` 开始，`last_error` 粘性保留。不要为消除提示而清空水位或失败统计。
5. **规则：数据库/基础设施错误同样会推进 offset 且水位写入错误被忽略，空间或连接恢复后不会自动重放。** 依据：2026-10-31 磁盘不足（PostgreSQL 日志 10:07:58–10:10:36）造成 178 次失败（含 `stash token analysis response` 写 `token_analysis_pending_responses` 失败，`pg_filenode_relation(0,982575)` 确认该关系；表本身仅 32 KiB，不是它占满磁盘）。摘要与净输入分步写入，还可能出现“摘要在、输入缺”。修复方案见 §14。
6. **规则：归档写入短写会污染后续记录。** 依据：`request_archive.go#writeRecord` 在 `os.File.Write` 部分成功后只返回错误，后台继续追加；磁盘满时留下无换行的半条 JSON，与下一条完整记录粘成一个物理行。2026-11-02 在 `2026-10-31.jsonl` 发现 5 处：每处都是截断 request + 后续完整 JSON。只修数据库重试解决不了这个问题。
7. **规则：部署修复不会让旧报错消失。** 依据：2026-06-11 部署 NUL 修复后仍显示 `upsert token analysis request summary: ... 0x00`，审计确认是修复前留下的粘性 `last_error`；只有删除该文件水位并重索引才会消失。判断修复是否有效看新增失败行，不看顶层 `last_error`。
8. **规则：按 `source_offset` 重读原文时要定位到上一行行首，并校验 request 事件、`archive_id` 与实际 body 哈希。** 依据：`source_offset` 是行尾位置；只比较行内哈希字段与摘要哈希检测不出正文漂移。文件失效或无法验证时保守降级，不能 `Seek` 后把下一行当本条。
9. **规则：新入库的 body 衍生文本必须过 `sanitizeTokenAnalysisText`。** 依据：2026-06-04、06-10 生产失败行均为 PostgreSQL 拒收 NUL（§6.4）。
10. **规则：count_tokens 判断用原始 path，不用 `endpoint` 字段。** 依据：§6.3 双计 token 事故。
11. **规则：状态接口的 `running` 只用内存标志。** 依据：§6.1 轮询永不结束事故。
12. **规则：本页精确值用 `formatInteger`/`formatCost`，不用共享 `@/utils/format#formatNumber`；不把本页规则写进共享 `formatCompactNumber`。** 依据：共享 `formatNumber` 在 abs ≥ 10000 时走 locale compact，中文环境显示“万/亿”；Token 禁用 K、费用只到 K 是本页专属合同，改共享函数会影响其他页面。用户排行 Token 小于 1M 不得显示 `0.0M`。
13. **规则：排行与计费趋势是两个口径，不要“修正”成一致。** 依据：§7.2、§9。
14. **规则：`.jsonl` 在受控重索引完成前不得删除。** 依据：失败行只能从原文恢复；`attention` 状态文件删除即放弃重处理。
15. **规则：文件仍在写入时不要对它做补录写入；补录必须拒绝源文件 size/mtime/行哈希/身份变化。** 依据：2026-11-02 补录保护（§13.2）。

已否决：
- 已否决：自动给坏 JSON 补引号或宽松解析，原因是会伪造用户正文与风险结论。
- 已否决：通过清空水位/失败统计“修复”提示，原因是丢失诊断证据且不恢复数据。
- 已否决：把 `token_analysis_pending_responses` 瘦身当作根治占盘，原因是占盘的是归档文件（2026-11-02 归档目录 294 GiB、库约 46 GiB）。
- 已否决（紧凑数字审核建议）：负金额显示为 `-$1.5K` 并写验收断言，原因是负费用不属于业务输入，当前输出 `$-1.5K` 不作合同；为 `NaN`/`Infinity` 写精确 `title` 测试，原因是 API 不会合法返回。

## 13. 运维排查与故障恢复

### 13.1 索引错误排查步骤

1. 读页面自身的 `GET /index/status` 与 `GET /archive-files`（只读），区分“最近 50 水位累计”与“当前目录文件状态”。
2. 只读 SQL（`default_transaction_read_only=on`、`BEGIN READ ONLY`、statement/lock timeout、结束 `ROLLBACK`）查全表：`SELECT source_file, processed_rows, failed_rows, last_error, updated_at FROM token_analysis_index_state WHERE failed_rows > 0 ORDER BY updated_at DESC;`，与页面窗口对比。
3. 按 `last_error` 前缀判阶段：`parse request body:`（内层 body 非法 JSON）；`upsert token analysis request summary`/`user input` + `0x00`（NUL，旧版本遗留）；`stash token analysis response`（响应暂存写库失败）；磁盘类错误再查 PostgreSQL 日志时间窗。
4. 确认原文件是否仍在归档目录（或备份）中；文件不在则无法补录，只能记录缺口。
5. 只有在原文件仍在、已部署对应修复且用户授权时，才按 §13.2 方法或 §13.3 方式重处理。

历史失败台账（截至 2026-11-02 补录后）：`2026-10-22`（1，`'m'`）、`2026-10-21`（1，`'('`）、`2026-10-14`（4，`'\x00'`）、`2026-09-09`（2，`'m'`）、`2026-09-07`（3，开头 BOM U+FEFF）、`2026-06-10`（2，摘要 NUL）、`2026-06-04`（1，用户输入 NUL）、`2026-10-31`（5，截断残留）。10-27 核对时前五个文件已不在归档目录，`/opt/sub2api/backups`、`/opt/backups` 不存在（未做全盘搜索），无法回放原文。

### 13.2 2026-11-02 生产补录（`2026-10-31.jsonl`）

用户授权“能恢复的就恢复”。只恢复可由原文确认的分析数据；未重发上游请求、未重新计费、未删除原始归档、未重启或部署（生产版本 `70e90d706`）。

- **原文**：78,020,680,200 bytes；完整扫描 883 秒（低 CPU/IO 优先级），扫描前后 size/mtime 不变。正常 request 25,238、response 25,224、损坏物理行 5。
- **坏行抢救**：5 处均为截断 request + 后续完整 JSON；只认可能独立解析的完整后缀，找回 2 条 request、3 条 response，按实际字节位置与 SHA-256 重放；5 条截断正文不可恢复。
- **写入**：新增 92 条摘要 + 92 条净输入、86 条响应事实（另 1 条已在 pending 表，未重复写）。388 条“无用户输入”的既有摘要经当前规则验证属正常，未改。71 条按 `request_id` 精确关联既有账单，21 条无确认账单，不推测补造；14 条无完成响应事实；3 条可读 response 因请求正文缺失留在 pending 表（沿用 7 天 TTL）。
- **全文件对账**：可读/抢救后 25,240 条 request 全部有摘要，`source_offset`/body hash 冲突 0；25,227 条 response 全部有完成事实或 pending 记录。再次只读预检 178 项全部 skip（未再执行写入测幂等）。
- **水位校正**：以原 checkpoint 的 offset/processed/failed/last_error 为 CAS 条件，只改 10-31 的 processed（25,148 → 25,240）、failed（178 → 5）、last_error（改为截断残留说明）、updated_at；未重置 offset、未删水位。最近 50 水位失败合计 184 → 11（5 + 更早 6）。
- **方法**：从生产 HEAD `git archive` 出隔离源码副本，桥接代码只加在副本中，复用现有解析、脱敏、指纹、候选边、response、usage-link 实现与运行时配置；指纹密钥只在服务器内从 `security_secrets` 读取。脚本用途：`scan.py` 逐行生成位置/长度/SHA-256 清单；`salvage_suffixes.py` 抢救完整后缀；`build_plan.py` 对照只读数据库快照生成候选（567 行计划）；`run_helper.py` 以服务进程环境运行 helper，默认只读，`--apply` 才写；`verify_repair.py` 只读全量对账。每项写前把既有派生记录写入 0600 journal 并 fsync；拒绝源文件 size/mtime/行哈希/身份变化及冲突响应。计费关联只处理本次 archive_id，`MergePendingResponses` 照常运行。
- **隔离测试**（基线 `70e90d706`，5 项通过）：原文漂移/错误身份/偏移不一致拒绝；摘要已写而输入失败时只补输入、不重写摘要与水位；response 写入故障可重试且保留输出 token/响应 ID；**复现现网缺陷**——response 失败后仍返回成功并越过水位（不代表长期修复完成）；计费关联只处理所选行。
- **证据位置**：私有前像 journal、偏移清单、不可恢复片段、SQL 快照留在服务器 0700 私有目录，不入库，也不得用于全表覆盖恢复。脚本与 Go helper 原文见 §17 提交。脚本固定该日期与路径，不是通用重试入口，不得未经审查对其他日期运行。
- 浏览器复核未完成（浏览器连接超时），结果由只读 SQL 验证。

### 13.3 待授权的受控重索引

- 内容：对仍在磁盘的历史文件一次扫描，同时完成 NUL/multipart 失败行补录、response 补完成时间、按更强约束重新计费关联（计费关联文档的存量恢复）。
- 做法：部署含修复的版本 → 只删 `failed_rows > 0` 的那些文件的水位行 → 页面选对应日期“索引当前范围” → 核对文件状态 `deletable`、失败 0、count_tokens 行 endpoint 带 `/count_tokens` 且未匹配、概览三卡合理。
- 约束：需用户单独授权；完成前不删 JSONL；多 GiB 文件全量重扫成本高，优先按 §13.2 的定点方式。

## 14. 故障恢复改造方案（未实施）

2026-11-02 事故后的修复设计。在线 writer、索引器、UI 均未修改或部署。

| 优先级 | 修改 | 验收合同 |
| --- | --- | --- |
| P1 | 归档写入处理短写/ENOSPC | 记录写前偏移并校验写入字节数；单 writer 独占且确认安全时回退未完成尾部，否则隔离当前文件/段并停止向坏尾追加；重启也检查尾部。网关请求不被归档故障阻塞，有界丢弃必须计数告警。多进程共享同一文件前先引入写入排他 |
| P1 | 区分内容错误与可重试基础设施错误 | SQL、连接、超时、水位保存失败停止本轮；可靠 checkpoint 不越过未持久化行。内容错误进入隔离清单后继续，不能无证据清除 |
| P1 | 持久化恢复任务 | 按源文件身份 + 字节区间 + 行哈希登记唯一任务，记录首次/最近错误时间、类型、尝试次数、退避、状态。故障文件队列独立于昨天/今天窗口，重启后继续。数据库完全不可用时保留旧 checkpoint，不依赖“成功写入错误表”才保证重试 |
| P1 | 幂等与并发 | 自动/手动/补录共用跨进程锁和 checkpoint CAS；摘要、用户输入与完成 checkpoint 尽量同事务，至少明确部分写入恢复协议。重试已有摘要须排除自身的重复计数，不重置质量评分/计费关联，不重复累计 processed/failed |
| P2 | 失败定位信息 | 持久化失败行的源文件、行起始 offset、archive_id、错误分类、失败时间、JSON 错误位置；不记录完整正文或凭据 |
| P2 | 管理端受控补录入口 | 预检 → 候选数与不可恢复数 → 执行 → 逐项验证；校验原文哈希与身份，保留原始 offset；只补缺失事实，遇冲突停止；请求/响应/输入分开计数；可取消续跑 |
| P2 | 状态展示 | 分开“当前运行状态 / 本轮失败 / 历史失败 / 已恢复 / 不可恢复”；标注最近 50 文件口径，或另做全历史失败聚合；历史错误显示源文件；不得把 response 错误加进请求行数；不因水位到 EOF 就宣称内容完整入库 |
| P2 | 容量管理 | 归档、PG、WAL 共盘时设阈值与告警；可配置保留期；有未解决故障的文件不自动清理 |
| 按需 | 非法请求仅元数据入库 | 若需在明细保留非法请求，可设计“仅元数据 + `parse_error` 标记”；来源保持 unknown，不伪造正文/风险结论，需单独测试计费关联与失败计数 |

回归测试矩阵：

| 场景 | 必须证明 |
| --- | --- |
| `Write` 返回 n < len 且 ENOSPC，空间恢复后再写 | 不产生粘连；故障记录可观测；后续完整记录仍可索引 |
| 短写无 error、尾部回退失败、重启遇坏尾、多 writer | 不静默接受短写；无法安全修尾时隔离；不截掉其他 writer 的有效数据 |
| 摘要 INSERT / 用户输入 UPSERT / 响应暂存失败 | 水位停在可恢复位置；恢复后三者齐全 |
| 初始/逐行/最终 checkpoint 写失败 | 错误上报；可靠水位不前进；重放不增加唯一成功数 |
| 提交成功但未收到确认、进程中断后重试 | 内容与计数一致，质量评分与计费关联不受破坏 |
| 故障超过两天、跨日请求响应乱序 | 历史任务自动重试；响应暂存按 archive_id 合并，不错配账单 |
| 非法 JSON、坏请求体、不完整 EOF、截断 body | 按规则隔离/等待/降级；基础设施错误不被当作永久坏数据跳过 |
| 原文件被改动/截断/移动、同日期新文件 | 哈希/身份不符拒绝重放，不拿另一行冒充原文 |
| 自动、手动、补录并发及锁租约变化 | 同一行不重复推进/记账；陈旧结果不覆盖新水位与完成事实 |
| 只打开今天页面但存在旧失败 | UI 标明历史来源；修复后显示恢复数量与残留原因 |

测试方式：真实 PostgreSQL 事务 + 断连/超时注入；ENOSPC/短写优先用可注入文件接口，另在隔离小文件系统验证一次，**禁止填满生产磁盘验证**。线上补录只验证分析数据，不重发上游请求、不重做计费。

## 15. 验证

### 15.1 命令

```powershell
# 后端（CI 单测门禁为 -tags=unit；以下测试文件无 build tag，默认也会编译）
cd backend
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run 'TokenAnalysis|ArchiveFile'
# 选中用户趋势 handler/缓存
go test -tags=unit -p 1 -count=1 ./internal/handler/admin -run 'TestDashboardHandler_GetUserUsageTrend'
# 真实 PostgreSQL（DSN 指向新建空库）：仓储趋势与执行计划
go test -tags=integration -p 1 -count=1 ./internal/repository -run 'TestUsageLogRepoSuite/TestGetUserUsageTrend' -v
$env:USER_USAGE_TREND_RUN_EXPLAIN = "1"
go test -tags=integration -p 1 -count=1 ./internal/repository -run '^TestSelectedUserUsageTrendExplainAnalyze$' -v

# 前端（回到仓库根目录）
cd ..
pnpm --dir frontend exec vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts src/api/__tests__/admin.tokenAnalysis.spec.ts src/api/__tests__/admin.dashboard.spec.ts src/utils/__tests__/formatCompactNumber.spec.ts
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
```

不要用 `pnpm --dir frontend run lint` 验证（含 `--fix`，会改工作区）。

关键回归测试：`TestTokenAnalysisIndexerCountsUntruncatedBadBodyAsFailed`、`TestTokenAnalysisIndexerDegradesTruncatedBodyToMetadata`、`TestTokenAnalysisIndexerIndexesMultipartImageEditRow`、`TestTokenAnalysisIndexerNULInEveryFieldStillIndexesClean`（`service/token_analysis_indexer_test.go`）；`TestTokenAnalysisRepositoryUpdateIndexStateKeepsLastError`、`TestTokenAnalysisRepositoryGetIndexStatusIgnoresStaleUnfinishedRows`（`repository/token_analysis_repo_test.go`）；`TestSelectedUserUsageTrendExplainAnalyze`（`repository/user_usage_trend_explain_integration_test.go`，需 `USER_USAGE_TREND_RUN_EXPLAIN=1`）。

### 15.2 最近结果

- 本次文档合并未运行测试（只改文档）。
- 2026-10-25：合成坏行诊断 + 上述 5 项既有索引器/仓储测试共 6 项通过；坏请求计失败 1 行，后续合法请求成功索引，水位推进到 EOF（494 bytes）。为本地合成数据，不是历史原文回放。
- 2026-11-02：补录隔离测试 5 项通过（§13.2），基线为生产 `70e90d706` 的隔离副本。
- 2026-07-14（选中用户趋势合入 0.1.155 后）：前端全量 Vitest 174 文件 1143/1143 通过，Token Analysis 页面 24/24，Dashboard API 2/2，typecheck、lint:check 通过；Handler、Service Go 测试通过。仓储 integration 只编译通过，**真实 `EXPLAIN ANALYZE` 与浏览器验收当时没有运行环境，至今无记录**。

## 16. 状态与待办

已完成并在生产运行：页面与四个标签页、异步索引（202/409/轮询）、截断/multipart 降级、NUL 清洗（2026-06-11 部署）、粘性 `last_error`、count_tokens 不参与匹配、概览口径与覆盖率卡、归档文件标签、紧凑数字格式、选中用户趋势。

待办：
1. §14 故障恢复改造（P1：writer 短写、错误分类停扫、持久化恢复任务、幂等/跨进程锁）——未开始。
2. 状态展示改造（区分当前/历史失败、标注 50 水位口径）——未开始。
3. §13.3 历史受控重索引——待用户授权；已不在磁盘的历史文件无法补录。
4. 筛选“提交”与“刷新”入口分离（§3 已知例外）。
5. 选中用户趋势的真实 `EXPLAIN ANALYZE` 与浏览器验收。
6. `token_analysis.index_batch_size` 已校验但索引器未使用：决定实现分批限速或在配置说明中标注无效。
7. 归档保留/清理策略（10-31 单日文件 72.66 GiB，2026-11-02 归档目录合计 294 GiB、数据库约 46 GiB，与 PG 共盘）。

## 17. 来源（已移出仓库，见提交 `26f6363af`）

- `docs/superpowers/specs/2026-05-19-token-waste-analysis-design.md`、`docs/superpowers/plans/2026-05-19-token-waste-analysis.md`、`docs/superpowers/plans/2026-05-21-token-analysis-production-data.md`
- `docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md`、`docs/superpowers/plans/2026-07-13-token-analysis-compact-number-format.md`、`docs/reviews/2026-07-14-token-analysis-compact-number-format-design-review.md`
- `docs/superpowers/specs/2026-07-13-token-analysis-selected-user-trend-design.md`、`docs/superpowers/plans/2026-07-13-token-analysis-selected-user-trend.md`
- `docs/features/token-analysis-recovery-design-cn.md`
- `docs/reviews/2026-10-25-token-analysis-index-errors-investigation-cn.md`、`docs/reviews/2026-10-27-token-analysis-eight-index-errors-investigation-cn.md`、`docs/reviews/2026-11-02-token-analysis-disk-errors-investigation-cn.md`
- `docs/delivery/2026-11-02-token-analysis-repair/`（README、`verification.json`、`plan-summary.json`、`scan-result.json`、脚本与 `repair_*.go.txt`）
