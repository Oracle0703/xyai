# Token 分析：归档请求与计费记录关联设计

更新：2026-11-04。状态：**本地四轮独立复核均已通过（最后一轮 2026-10-25）；2026-10-25 已部署到生产（`777960134`，migration 247/248 随应用启动执行）。10-26 生产窗口核对：已关联的 57,043 条全部是 `request_id` 精确键、没有重复关联；另有 3,022 条未关联、2,703 条待复核（§9）。受控历史回填和原始缺陷样本的精确关联验收还没完成（§13）。**

本文是归档请求与 usage 计费记录关联功能的唯一文档，覆盖缺陷背景、关联键与 schema、复核版本与晚到账单重试、关联流程、展示合同、存量恢复、部署记录和验证。审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看）。涉及的路径有：`docs/reviews/2026-10-24-token-origin-human-input-investigation-cn.md`、`docs/reviews/2026-10-24-token-analysis-usage-link-{implementation-review,rereview,third-review}-cn.md`、`docs/reviews/2026-10-25-token-analysis-usage-link-fourth-review-cn.md`、`docs/delivery/2026-10-24-token-origin-human-input/`（`usage-link-review*` 复现源码与输出）、`docs/delivery/2026-10-25-token-analysis-deployment.md`。来源分类是另一个功能，见 [请求来源识别设计](token-analysis-request-origin-design-cn.md)。

阅读顺序建议：
- 修改关联逻辑：先读 §4、§5 和 §10；
- 修改 schema 或仓储写入：读 §3、§4；
- 修改页面或接口：读 §6；
- 运维、回填和核对：读 §8、§9；
- 定位代码或跑测试：读 §7、§12。

## 1. 背景与缺陷链

2026-10-24 生产上有一条真实人工请求（archive `73eb6d29496d6c677054f5c797996d66`，`/v1/responses`，模型 gpt-6-astra）在默认请求明细里不可见，勾选"包含未匹配归档"后又显示为 0 Token / 0 费用。来源字段本身是对的（human/user_input/medium/v1）。缺陷链如下：

**14:11:12.654 请求开始 → 14:11:31.202 完成（HTTP 200，耗时 18,547 ms）→ 14:11:31 产生账单 $0.186578（非缓存输入 294、输出 547、缓存约 156.3K）→ 旧关联器只在请求开始时间 ±10 秒内（14:11:02–14:11:22）找同用户/Key/模型的最近账单 → `usage_log_id` 为空 → 仓储在 `include_unmatched=false` 时追加 `usage_log_id IS NOT NULL` → 页面默认排除 → 未关联行的 Token/费用被 SQL 用 COALESCE 补成 0。**

旧规则的结构性问题：
- 归档 request 行记录的是请求开始时间（中间件在 `c.Next()` 之前写入），而 `usage_logs.created_at` 是转发完成后，计费任务里取的 `time.Now()`。长请求的账单必然落在窗口之外。
- 顺序执行的 agent 轮次里，下一请求的开始时间紧挨着上一请求账单的落库时间，"离开始时间最近"会系统性地取到上一请求的账单。
- 一笔账单可以同时被多个请求关联，聚合时会重复累计；请求行入库时请求多半还在进行，之后到账的账单不会再关联（文件水位已经越过）。
- 生产配置没有覆盖 `usage_match_window_seconds`，用的是代码默认值 10 秒。

## 2. 验收口径（用户确认）

| 必须做到 | 不算完成的做法 |
| --- | --- |
| 已计费请求无论勾选与否都能在明细中看到，仍标为人工 | 让用户自己去勾选；只改默认勾选值；把"分类字段正确"当作问题已解决 |
| 请求和真实账单正确关联；账单暂时没落库时不误导 | 把未关联显示成确认的 0 Token / 0 费用；为了显示一条记录去改来源标签 |
| 预览能看到真实问题（剥离客户端注入包装） | 用客户端注入的前 300 字代替用户问题；按预览文字判断来源 |
| 新请求和存量未关联记录都有处理方案 | 只修新请求，不说明旧摘要怎么恢复 |
| 并发、重试、异步落库下不串单、不重复累计 | 扩大最近时间窗口 |

## 3. 关联键与 schema

- **`client_request_id`**：网关所有链路中，`ClientRequestID` 中间件都先于归档执行。归档的 request/response 行新增 `client_request_id`，它和计费 `usage_logs.request_id` 中的 `client:<id>` 同源（`resolveUsageBillingRequestID`）。`(request_id, api_key_id)` 在 usage_logs 上唯一。
- **`response_id`**：WS 上游（`forwardOpenAIWSV2`、HTTP→WS 桥）计费时用上游 response.id 作为 request_id。所以归档 response 行新增 `response_id`：流式取 SSE 事件里最后出现的 `response.id`，非流式取 Responses 对象顶层的 `id`；尾部窗口（256 KB）不是完整 JSON 时放弃。
- **`247_token_analysis_usage_link.sql`**：摘要表新增 `client_request_id`、`response_id`、`completed_at`、`response_status`、`response_output_tokens`、`usage_match_method`，以及复核字段 `usage_link_version`、`usage_link_checked_version`、`usage_link_retry_until`；新建 `token_analysis_pending_responses`。只加可空列或带常量默认值的列，不回填、不重写存量行。
- **`248_token_analysis_usage_link_keys_index_notx.sql`**：用 CONCURRENTLY 为两个关联键和待复核行（两个版本不等）建部分索引，存量行不进入索引。migration runner 会清理无效索引。
- 247/248 在部署前改写过一次：布尔列 `usage_link_pending` 换成了版本字段（§4）。部署时生产执行的是改写后的版本。这两个文件现在受 checksum 校验，不能再改。
- 后来的 `250_token_analysis_forward_facts.sql` 给暂存表增加了转发侧事实列，属于风险分类功能，不在本文范围内。
- **写回完成事实**：索引器读到 response 行时，写回完成时间、HTTP 状态、上游 response.id 和归档 usage 里的输出 token（Responses/Messages 取 `output_tokens`，Chat 取 `completion_tokens`）。如果请求行还没入库（例如跨日文件逆序回填），先暂存到 `token_analysis_pending_responses`，每轮关联前合并；请求行一直不出现的暂存数据 7 天后清理。

## 4. 复核版本与晚到账单重试

- **登记复核**：写回或合并改变了某个未精确关联行的完成事实（状态、完成时间、response.id），或者该行的账单被精确关联夺走时，`usage_link_version` 加一，`usage_link_retry_until` 设为数据库当前时间加 48 小时。相同完成事实重放时不续期。
- **待复核行**：版本与 `usage_link_checked_version` 不等的行。下一轮关联处理它时**不受日期范围限制**：升序分日回填时，D 日跨夜请求的 response 在 D+1 日的文件里，只索引 D+1 也能关联到它；被另一日请求夺走账单的旧行也会在同一次索引里重算风险。
- **只能确认读到的版本**：关联写入和"无需改动"的确认都必须带上查找前读到的版本，并按它做条件更新，确认时只写回读到的版本。查找之后又登记的复核会让这次结果作废：陈旧的时序写入会被拒绝，陈旧的确认也吞不掉新任务。
- **晚到账单**：带关联键、响应未失败的请求，复核后仍找不到账单时**不确认版本**，之后每轮都不受日期范围限制地继续精确查找，直到账单出现或超过 `usage_link_retry_until`；期满仍没有证据，就确认为未关联、不再重试。以后显式选择请求日期时仍可重新查找；"未关联"不表示零费用。
- **无关联键的存量行**：账单早已落库，一次查不到就确认，避免历史回填后每轮重查大量歧义行。

## 5. 关联流程

文件扫描阶段的请求行只写摘要，不找账单；风险分先按 body 侧信号计算，并把计入风险的同 body 重复数写进 `summary_json.same_body_recent_count`。每轮索引（自动索引覆盖昨天和今天，手动"索引当前范围"覆盖所选日期）在文件扫描后合并暂存的完成事实，然后在库内跑关联轮次，不重读 JSONL：

1. **候选**（按 id 续读，排除 count_tokens）：范围内的未关联行、旧窗口关联行（method 为空）、账单被夺走的行（`released`）、有关联键且可被精确证据纠正的时序关联行，以及任意日期的待复核行。
2. **精确关联**（方式 `request_id`）：在同一 `api_key_id` 下按 `client:<client_request_id>` 和上游 response.id 查找。精确证据优先：账单只被时序或旧窗口关联占用时直接夺回，原占用行置为 `released` 并登记复核，随后重新关联或解除并重算风险。已被另一条精确关联占用的账单不动。精确认领的目标行条件更新成功后才会释放他行；失败则整个事务回滚，不会因陈旧目标误释放别人的账单。
3. **只接受精确证据的情形**：失败请求（响应非 2xx；真实计费证据仍可关联）；已确认的时序关联行（只有出现精确证据才改变）；带上游 response.id 的新请求；有关联键但还没完成的新请求（等下一轮重试）。完成事实变化后的时序关联行不算已确认，要按新事实完整复核，例如晚到的 502 响应会解除没有精确证据的时序关联。
4. **时序兜底**（方式 `timing`）只用于两类行：无关联键的存量行；有关联键、没有 response.id、已完成的新请求（后者只看不含 `:` 的上游原生 ID 账单）。必须同时满足：
   - 账单合理：同用户/Key/模型；推算的转发开始时间 `created_at - duration_ms` 不早于请求开始（同进程时钟，留 500 ms 余量；计费排队只会让它变晚，所以它只能当下界）。有完成时间时，账单耗时不超过请求耗时，且落库时间在完成前 2 秒到完成后 `usage_match_window_seconds`（默认 10 秒）之间；没有完成时间时，转发必须在开始后 window 内启动。
   - 账单没有被他行占用；不是附加计费（`web_search:`、`grok-video:`、`grok_audio:`、`grok_realtime:`、`gcache:`）；也不是另一条带关联键请求的精确账单（`client:<他行 id>` 或他行 response.id）。
   - 归属唯一：没有其他尚未确认关联、同 Key 同模型的请求也能解释这笔账单。计费排队会让"首发失败后重试"和"首发账单排队"在时序上无法区分，此时两条都保持未关联，不猜测。
5. **认领**：在事务内按账单 ID 取 `pg_advisory_xact_lock`，锁内复核占用后再写入，多个索引实例并发也不会把同一笔账单关联给两条请求。所有写入（关联与解除）都按查找前读到的 `usage_log_id`、`usage_match_method` 和复核版本做条件更新；如果同一行已被另一实例改过，或之后登记了新复核，就放弃这次写入，留待下一轮。
6. **风险重算**：关联变化时按真实 usage 重算风险分和风险原因；解除关联时去掉依赖 usage 的风险原因，保留 body 自身的风险；旧行没有记录重复数时现查，并扣掉自身。
7. 释放或夺回账单后，同一次索引最多再跑两轮，让本轮已处理过的请求重新认领。

`usage_match_window_seconds` 的语义已改为时序兜底的余量，不再是"开始时间 ±窗口"。

## 6. 状态与展示（接口合同）

- 列表 API 新增 `usage_link_status`（`linked`/`unlinked`/`failed`/`not_billable`）、`usage_match_method`（`request_id`/`timing`/`legacy_window`，最后一种表示尚未复核的旧窗口关联）、`completed_at`、`response_status`。状态由 `decorateTokenAnalysisUsageLink` 推导。
- 非 `linked` 行的 Token 与费用字段仍由 SQL 补 0，只是占位。页面表格和抽屉显示 `—`，并说明"未知，不代表零消耗"；合法的零消耗只出现在已关联行。抽屉还显示关联方式、账单 ID 和完成时间。
- **请求明细是完整的归档视角**：服务端始终包含未关联行并标注状态，因此已正确分类的人工请求无论是否勾选都可见。原来的勾选项改名为"排行含未关联请求"，只影响用户/项目排行的请求数，默认值不变。排行和概览的 Token/费用只统计已关联行；概览始终包含未关联行。
- 来源列改称"本次调用来源"，预览列改称"最近用户输入"：工具续执行可以和发起它的人工首发共享同一句预览，来源仍按本次调用判断。预览开头完整闭合的 `<in-app-browser-context>` 等注入包装和紧随的 `## My request:` 会被剥离（规则见来源设计 §5）。

## 7. 代码地图

| 关注点 | 位置 |
| --- | --- |
| 归档关联键 | `backend/internal/server/middleware/request_archive.go`（request 行取开始时间，response 行取 `c.Next()` 之后的时间，并记录 `client_request_id` 和上游 response.id） |
| 计费 request_id | `backend/internal/service/gateway_usage_billing.go`：`resolveUsageBillingRequestID`；WS 上游 `backend/internal/service/openai_ws_forwarder_v2.go`：`forwardOpenAIWSV2` |
| 服务层关联 | `backend/internal/service/token_analysis_usage_link.go`：`recordTokenAnalysisResponse`、`tokenAnalysisArchiveOutputTokens`、`linkUsageRange`、`linkUsageCandidate`、`decorateTokenAnalysisUsageLink` |
| 类型 | `backend/internal/service/token_analysis_types.go`：`TokenAnalysisUsageLinkState`（含 `Version`、`KeepPending`）、`TokenAnalysisUsageLinkCandidate`、`TokenAnalysisUsageLinkQuery`、`TokenAnalysisUsageMatch`、`TokenAnalysisResponseFacts` |
| 仓储 | `backend/internal/repository/token_analysis_repo.go`：`RecordResponse`、`MergePendingResponses`、`ListUsageLinkCandidates`、`FindUsageLink`（内部是 `findUsageByRequestID`、`findUsageByTiming`）、`SetUsageLink`（带 `expected` 状态）、`ClearUsageLinkPending`（带 `version`） |
| 索引入口 | `backend/internal/service/token_analysis_indexer.go`：`indexArchiveLine` |
| 前端 | `frontend/src/components/admin/token-analysis/usageLink.ts`、`TokenAnalysisRequestTable.vue`、`TokenAnalysisRequestDrawer.vue` |
| 测试 | `backend/internal/repository/token_analysis_usage_link_integration_test.go`（16 个顶层用例）；服务单测 `backend/internal/service/token_analysis_indexer_test.go`（仓储 stub）；仓储 SQL 形态 `backend/internal/repository/token_analysis_repo_test.go` |

## 8. 存量数据恢复

部署之前的行没有关联键和完成时间，`usage_match_method` 为空。

- **自动恢复**：新版本部署后，自动索引每轮复核昨天和今天的存量行（未关联行重试；旧窗口关联行确认、改判或解除），只读写数据库。
- **历史日期（DB-only，不需要 JSONL）**：在页面上对目标日期执行"索引当前范围"。文件水位已经追平时只跑关联轮次；JSONL 已被删除的日期同样可用。这是生产写操作（会更新关联和风险分），需要单独授权。
- **更强的历史恢复（需要 JSONL 仍在）**：与已挂起的 NUL/multipart 受控重索引、来源 v2 重算合并成一次执行。重置目标文件水位后重读，让 response 行补回完成时间（旧归档没有关联键和 response.id），存量行就能用带完成时间的约束，消除一部分只看开始时间时的歧义。单日归档有上百 GiB，只扫一遍；升序或逆序回填都可以，逆序时完成事实先暂存再合并。核对完成前不删 JSONL。
- **只读核对**（在授权终端执行）：

```sql
-- 关联状态分布(按日)
SELECT date_trunc('day', event_time) AS day, usage_match_method,
       count(*) FILTER (WHERE usage_log_id IS NOT NULL) AS linked,
       count(*) FILTER (WHERE usage_log_id IS NULL) AS unlinked
FROM token_analysis_request_summaries
WHERE event_time >= :since
GROUP BY 1, 2 ORDER BY 1, 2;

-- 同一账单被多个请求关联(期望 0)
SELECT usage_log_id, count(*) FROM token_analysis_request_summaries
WHERE usage_log_id IS NOT NULL AND event_time >= :since
GROUP BY 1 HAVING count(*) > 1;

-- 待复核与过期(版本不等的行; retry_until 已过仍未确认的应为 0)
SELECT count(*) AS pending,
       count(*) FILTER (WHERE usage_link_retry_until < now()) AS expired
FROM token_analysis_request_summaries
WHERE usage_link_version <> usage_link_checked_version;

-- 原始缺陷样本
SELECT archive_id, usage_log_id, usage_match_method, completed_at, summary_json->'request_origin'
FROM token_analysis_request_summaries WHERE archive_id = '73eb6d29496d6c677054f5c797996d66';
```

## 9. 部署记录与生产核对

**部署（2026-10-25，生产 118 服务器，`/opt/sub2api`）**：
- 从 `98151012b` 升级到 `777960134`（main，`git pull --ff-only`），Version 0.2.17，构建方式为 `CGO_ENABLED=0 go build -tags embed`。8 份关键源码的 SHA-256 与第四轮复核快照一致。
- 只执行了一次 `systemctl restart sub2api`；Redis、两个 PostgreSQL 15 实例和 Nginx 都没有重启。migration 只随应用启动执行，没有手工执行 SQL、没有改配置、没有重置水位或手动触发重索引。`/health` 和 `/admin/token-analysis` 均返回 200，页面资源包含 `usage_link_status`。
- 运维注意：
  - 构建脚本经 Windows 管道执行时，附加的 CR 行会导致返回 127，这不是构建失败，脚本入口要过滤 CR；
  - 就绪检查要带重试；
  - 在 pipefail 下用 `head` 截断输出会误报失败；
  - 旧进程退出时打印的 `TokenAnalysisAutoIndex succeeded` 不能当作新版本索引完成的证据。

**上线后核对**：
- 部署当晚，用户新发的测试请求 `d5a496e050ad7291aa0c2f4f8f40eece` 显示为人工，预览已去掉包装，勾选与否都可见；同轮后续工具调用显示为 AI。观察期间它仍是"未关联计费"，Token/费用显示未知而不是零。
- 10-26 审计时，该请求已精确关联（`usage_log_id=4304734`，方式 `request_id`，两个版本都是 1）。原始缺陷样本 `73eb6d29…` 显示人工且已关联，但方式是 `timing`，并且缺少完成事实，**不能算作精确关联验收通过**。

**10-26 窗口（`[10-26 09:00, 10-27 09:00)`，生产 `25bedc620`，60,065 条请求）的关联统计**：

| 检查 | 结果 |
| --- | ---: |
| 已关联 | 57,043，全部按 `request_id` 精确关联 |
| 精确键关联但用户/API Key 不一致 | 0 |
| 同一账单被重复关联 | 0 |
| 未关联 | 3,022（5.031%） |
| 未关联但其实已有匹配的精确账单 | 0 |
| 待复核 | 2,703，全部未关联；过期 0 |

未关联集合中有失败响应 288、未记录完成事实 12、count_tokens 19、compact 20，这些条件可能重叠，不能相加。5.031% 不是来源误判率；57,043/60,065 也不是以全量账单为分母的覆盖率。审计 SQL 全程只读：`default_transaction_read_only=on`、`BEGIN READ ONLY`、statement timeout 45 秒、lock timeout 2 秒、每次 ROLLBACK；账单 JOIN 只取窗口前后各 2 小时。

## 10. 关键约束与易错点

以下每条约束都对应一个曾在独立 PostgreSQL 18 空库中复现过的失败场景，现在都有永久回归。

- **精确证据可以夺回弱关联，时序关联不能当作永久确认。** 依据：A 的 `client:A` 账单晚到，A 先按时序认领了 WS 邻居 B 的 resp_* 账单；之后精确账单到达，重复索引仍保留错误账单。（`TestTokenAnalysisUsageLinkLateExactBillBeatsNeighborTiming`、`...ResponseIDCorrectsTimingClaim`）
- **时序兜底不能占用他行的精确账单。** 依据：无键的旧行 A 按 id 先处理，按时序认领了新请求 B 唯一的 `client:b` 账单，B 的精确查询被占用检查挡住，永远无法关联。（`...LegacyCannotTakeExactOwnersBill`）
- **推算的转发开始时间只能当下界，归属必须唯一。** 依据：A 在 t0 开始，转发耗时 1 s，计费任务排队到 t4；B 在 t2.1 开始。`created_at - duration` 推算出 t3，于是 A 的账单被关联给了 B。（`...LegacyAmbiguousBillStaysUnlinked`、`...ConcurrentAndRetriedRequests`）
- **认领必须原子。** 依据：两个索引实例都在写入前读到同一笔未占用账单，两次 UPDATE 都成功，同一 `usage_log_id` 永久关联两条请求。查询时的 NOT EXISTS 约束不了之后的 UPDATE，服务内 mutex 也只保护单个实例。（`...ConcurrentClaimsAreAtomic`，用持行锁制造"检查后、提交前"的交错）
- **完成事实先于请求行到达时必须暂存。** 依据：先索引 D+1 再索引 D，第一次 RecordResponse 的 UPDATE 影响 0 行却正常返回，response 水位已经前移，D 日请求永远没有 completed_at。（`...CrossDayRequestInAnyIndexOrder`，覆盖逆序、升序分日、整段三种方式）
- **跨日请求要按复核版本处理，不能只看本轮文件日期。** 依据：单独先索引 D、再索引 D+1，response.id 已经补回，但候选只有 D+1 开始的请求，D 日行一直是 NULL。（同上）
- **失败响应要触发复核。** 依据（两种复现）：旧窗口错误关联的 502 行被候选 SQL 直接排除（`...FailedRequestsKeepOnlyExactBills`）；已经变成 timing 的行之后才收到 502，服务层 `match == nil && weakLink` 提前返回，错误账单保留（`...LateFailedResponseClearsTimingLink`，分有无 response.id 两种）。有精确账单的失败请求仍要关联，不能按失败状态一刀切清空。
- **写入必须按查找前读到的状态做条件更新。** 依据：A 查到邻居账单后暂不写入，B 精确绑定了 A 的真实账单，A 再提交旧结果，仍返回 Applied=true 并把关联回退；旧 nil 结果也能清空 B 的精确关联。账单锁只互斥同一笔账单，挡不住同一行被不同结果覆盖。（`...StaleWritesCannotOverrideNewerLink`）
- **账单被夺回的行必须在本次索引内重算风险，并保留 body 自身的风险。** 依据：D 日旧行的账单被 D+1 的精确 owner 夺走后，usage_log_id 已为 NULL，但风险仍是 70 分（`huge_input_tiny_output` 45 + `low_cache_hit_large_input` 25）。修复后的回归验证 90→20，只剩 `oversized_system_prompt`。（`...DisplacedRowOnOtherDayIsRescored`）
- **一次查不到账单不等于复核完成。** 依据：response 先到，账单后到，服务在 `match == nil` 时清掉了唯一不受日期限制的标记，之后在请求日之外的索引里再也关联不上。（`...LateBillIsRetriedUntilDeadline`，含截止后确认未关联）
- **复核状态必须版本化，Set 和 Clear 都只能确认读到的版本。** 依据：T2a，A 查到时序结果后，B 写入首个 502 响应，A 的旧写入仍然成功，还顺带清掉了标记；T2b，A 准备确认"无需改动"时，B 夺回账单登记了新任务，A 无条件的 Clear 把它吞掉，旧行 70 分风险残留。（`...NewRecheckRejectsStaleResults`）
- **已否决：把 `usage_match_window_seconds` 调大。** 原因：窗口越大，并发和顺序请求越容易串单；长请求问题的根源是时间口径不对，不是窗口太小。
- **已否决：只把"包含未匹配"的默认值改成 true。** 原因：用户验收要求勾选与否都可见；这个勾选现在只影响排行请求数。
- **已否决：给 `usage_log_id` 加唯一约束。** 原因：存量数据里可能已经有重复，迁移前必须先定义清理方式；改为用咨询锁加关联轮次复核来消化。
- **已否决：用布尔 `usage_link_pending` 标记复核。** 原因：布尔值无法区分 true→true 的新任务（T2b），已在部署前改成版本字段。
- **不要假设不同模块的 request_id、archive_id、hash 属于同一命名空间。** 只有 `client:<client_request_id>` 和 WS 上游 response.id 被证实可以贯通归档与账单。
- 跑集成测试时，DSN 只能指向新建的空库。harness 会执行 migration 并写入测试数据。

## 11. 已知限制

- 存量行没有关联键，只在归属唯一时按时序关联。以下歧义场景保持未关联（显示"未关联计费"，不猜测）：顺序请求间隔短于 window、同 Key 并发、计费排队；预处理超过 window 才开始转发的存量请求同样保持未关联。受控重索引补回完成时间后，可以解开其中一部分。
- 新请求的主账单都能精确关联：HTTP 转发按 `client:`，WS 上游按 response.id。只有 response.id 捕获不到时（非流式响应超过 256 KB 尾部窗口、非 Responses 协议经 WS 转发）才走受限的时序兜底，有歧义时同样不关联。
- 如果 response 行被归档队列丢弃，新请求只能按 `client:` 关联，WS 上游的账单保持未关联。
- 未关联不等于未计费：状态提示会说明账单可能还在落库，或者无法唯一确定，后续索引会重试。

## 12. 验证

```bash
# backend/ 下执行
go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/middleware -run 'TokenAnalysis|RequestArchive'
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN=... go test -tags integration -p 1 -count=1 ./internal/repository -run 'TokenAnalysisUsageLink|TokenAnalysisListRequestsFiltersByOrigin|TokenAnalysisUserInputQuality' -v
```

**永久回归**：`token_analysis_usage_link_integration_test.go` 共 16 个顶层用例：
- 原始缺陷时序（`TestTokenAnalysisUsageLinkLongRunningLegacyRequest`：18.4 秒账单、前一请求的账单紧邻开始时间、旧窗口错误关联被改判，开关两种状态下都可见且为人工）；
- `LateBillingByRequestID`、`ConcurrentAndRetriedRequests`、`LegacyAmbiguousBillStaysUnlinked`；
- §10 列出的各约束用例；
- `TestTokenAnalysisListRequestsFiltersByOrigin`。

**变异检查**：以下改动各自都会让对应用例失败：
- 去掉归属唯一约束、他行精确账单保护、账单咨询锁、精确证据夺回弱关联；
- 去掉关联状态条件更新、复核版本条件更新；
- 把"确认读到的版本"改成"确认当前版本"；
- 夺回时或完成事实变化时不登记复核；
- 不跨日期处理待复核行；
- 不等待晚到账单。

单独去掉确认时的版本条件不会让测试失败，因为确认只写回读到的版本，陈旧的确认覆盖不了新版本，这一项不构成缺口。

**服务、仓储单测和前端**：
- 服务/仓储单测覆盖：精确与兜底的切换，失败行和已确认时序行只认精确证据，登记复核后完整复核，无需改动时按版本确认，等待晚到账单，被夺回行解除并重算风险，释放后下一轮认领；SQL 形态和参数；列表强制完整视角和状态推导；归档中间件记录关联键。
- 前端覆盖未关联/失败/不计费/旧接口行显示 `—` 加状态标签，以及抽屉中的关联信息。

**最近结果（2026-10-25 第四轮复核，本机 PostgreSQL 18.3 新空集群）**：
- 第三轮的 3 个原始复现加 17 个正式用例，共 20 个数据库顶层用例通过，无跳过；
- Go 四个包的专项通过；
- 前端 4 个文件 49 项通过，typecheck 和相关 ESLint 通过。

各轮的原始复现（`TestReview*` 6 个、`TestRereview*` 5 个顶层/6 个场景、`TestReview3*` 3 个）都已改写为上述永久回归，不再单独保留。适配时只补了新签名需要的"查找前读到的状态与版本"参数，并把旧的 `usage_link_pending` 列读取改成了版本比较；触发顺序和预期都没变。

## 13. 状态与待办

| 项 | 状态 |
| --- | --- |
| 实现与本地复核 | 完成，四轮复核均已通过 |
| 部署 | 2026-10-25 完成（`777960134`） |
| 新请求关联 | 10-26 窗口：已关联 57,043 条均为精确键、无重复；未关联 3,022 条（5.031%），待复核 2,703 条 |
| 原始缺陷样本 `73eb6d29…` | 已关联，但方式为 timing，缺完成事实；要等受控重索引补回 response 后复核 |
| 历史日期 DB-only 恢复 | 未执行，需单独授权（§8） |
| 受控重索引（合并 NUL/multipart、来源 v2、补完成时间） | 未执行，需授权；完成核对前不删 JSONL |
