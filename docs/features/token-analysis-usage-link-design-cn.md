# Token 分析：归档请求与计费记录关联

日期：2026-10-24。缺陷定位与现场证据见 `docs/reviews/2026-10-24-token-origin-human-input-investigation-cn.md` 和 `docs/delivery/2026-10-24-token-origin-human-input/`。本文是修复后的关联设计与运维边界。

## 背景

生产一条真实人工请求（archive `73eb6d29…`）14:11:12 开始、耗时 18.5 秒，14:11:31 产生账单 $0.186578。来源正确分类为人工，但旧关联器只在“请求开始时间 ±10 秒”内找同用户/密钥/模型的最近账单，漏掉了它；请求明细默认排除未关联行，所以这条已计费的人工请求在默认页面不可见，勾选后又显示为 0 Token / 0 费用。

旧规则的结构性问题：

- 归档 request 行按请求开始时间记录，`usage_logs.created_at` 在转发完成后的计费任务里取 `time.Now()`，长请求的账单必然落在窗口外。
- 顺序 agent 轮次中，下一请求的开始时间紧挨上一请求的账单落库时间，“离开始时间最近”会系统性取到上一请求的账单。
- 账单可被多个请求同时关联，聚合会重复累计；请求行入库时请求多半仍在进行，之后到账的账单不会再关联。

## 关联键与入库字段

- 网关所有链路中 `ClientRequestID` 中间件先于归档执行。归档 request/response 行新增 `client_request_id`，与计费 `usage_logs.request_id` 的 `client:<id>` 同源（`resolveUsageBillingRequestID`），`(request_id, api_key_id)` 在 usage_logs 上唯一。
- WS 上游（`forwardOpenAIWSV2`、HTTP→WS 桥）计费改用上游 response.id 作为 request_id。归档 response 行新增 `response_id`：流式取 SSE 事件里最后出现的 `response.id`，非流式取 Responses 对象顶层 `id`（尾部窗口不是完整 JSON 时放弃）。
- `backend/migrations/247_token_analysis_usage_link.sql` 为摘要表增加 `client_request_id`、`response_id`、`completed_at`、`response_status`、`response_output_tokens`、`usage_match_method`、复核版本 `usage_link_version`/`usage_link_checked_version` 与等待截止 `usage_link_retry_until`，并新建 `token_analysis_pending_responses`；只加可空列或常量默认列，不回填、不重写存量行。`248_token_analysis_usage_link_keys_index_notx.sql` 以 CONCURRENTLY 为两个关联键和待复核行（两个版本不等）建部分索引（存量行不进索引）。
- 索引器读到 response 行时写回完成时间、HTTP 状态、上游 response.id 和归档 usage 中的输出 token（Responses/Messages 的 `output_tokens`、Chat 的 `completion_tokens`）。请求行还未入库（跨日文件逆序回填）时暂存到 `token_analysis_pending_responses`，每轮关联前合并；请求行一直缺失的暂存 7 天后清理。
- 复核事件：写回或合并改变了未精确关联行的完成事实、或账单被精确关联夺回时，`usage_link_version` 加一并把 `usage_link_retry_until` 置为当前时间加 48 小时。版本与 `usage_link_checked_version` 不等的行即待复核，下一轮关联不受日期范围限制地处理：升序分日回填时，D 日跨夜请求的 response 在 D+1 日文件里，只索引 D+1 的那一轮也会关联它；被另一日请求夺回账单的旧行也会在同一次索引里重算风险。
- 复核确认：处理结果只能确认查找前读到的版本（关联写入与“无需改动”的确认都按该版本条件更新）。查找之后又登记的复核使结果作废：陈旧的时序写入被拒绝，陈旧的确认也不能吞掉新任务。
- 晚到账单的重试合同：带关联键、响应未失败的请求在复核后仍找不到账单时不确认版本，此后每轮（不受日期范围限制）继续精确查找，直到账单出现或超过 `usage_link_retry_until`；期满仍无证据即确认为未关联、不再重试。无关联键的存量行账单早已落库，一次查不到即确认，避免历史回填后每轮重查大量歧义行。

## 关联流程

文件扫描阶段的请求行只写摘要，不找账单；风险分先按 body 侧信号计算，并把计入风险的同 body 重复数写进 `summary_json.same_body_recent_count`。每轮索引（自动索引覆盖昨天和今天，手动“索引当前范围”覆盖所选日期）在文件扫描后合并暂存完成事实，再执行库内关联轮次，不重读 JSONL：

1. 候选（按 id 续读，排除 count_tokens）：范围内的未关联行、旧窗口关联行（method 为空）、被夺回账单的行（`released`）、有关联键可被精确证据纠正的时序关联行，以及任意日期的待复核行（复核版本未确认）。
2. 精确关联（方式 `request_id`）：按 `client:<client_request_id>` 与上游 response.id 在同一 `api_key_id` 下查找。精确证据优先：账单只被时序或旧窗口关联占用时直接夺回，原占用行置为 `released`，随后重新关联或解除并重算风险；已被另一条精确关联占用的账单不动。
3. 只接受精确证据的情形：失败请求（响应非 2xx，真实计费证据仍可关联）；已确认的时序关联行（只在出现精确证据时改变）；带上游 response.id 的新请求；有关联键但尚未完成的新请求（下一轮重试）。完成事实变化后的时序关联行不算已确认，按新事实完整复核：例如晚到的 502 响应会解除没有精确证据的时序关联。
4. 时序兜底（方式 `timing`）只用于无关联键的存量行，以及有关联键、没有 response.id、已完成的新请求（后者只看不含 `:` 的上游原生 ID 账单）。必须同时满足：
   - 账单合理：同用户/密钥/模型；推算转发开始 `created_at - duration_ms` 不早于请求开始（同进程时钟，500 ms 余量；计费排队只会让它变晚，所以只作下界）。有完成时间时账单耗时不超过请求耗时，且在完成前 2 秒至完成后 `usage_match_window_seconds`（默认 10 秒）内落库；无完成时间时转发须在开始后 window 内启动。
   - 账单未被他行占用，不是附加计费（`web_search:`、`grok-video:`、`grok_audio:`、`grok_realtime:`、`gcache:`），也不是另一条带关联键请求的精确账单（`client:<他行 id>` 或他行 response.id）。
   - 归属唯一：没有其他尚未确认关联的同键同模型请求同样能解释这笔账单。计费排队会让“首发失败后重试”和“首发账单排队”在时序上无法区分，此时两条都保持未关联，不猜测。
5. 认领在事务内按账单 ID 取 `pg_advisory_xact_lock`，锁内复核占用后写入，多个索引实例并发也不会把同一账单关联给两条请求。所有写入（关联与解除）都按查找前读到的 `usage_log_id`/`usage_match_method`/复核版本条件更新：同一行已被另一实例改过、或其后登记了新复核时放弃，陈旧结果不能覆盖较新的关联；放弃的行留待下一轮。
6. 关联变化时按真实 usage 重算风险分与风险原因；解除关联时去掉依赖 usage 的风险原因；旧行没有记录重复数时现查并扣掉自身。
7. 释放或夺回账单后同一次索引最多再跑两轮，让本轮已处理过的请求认领。

`usage_match_window_seconds` 的语义随之改为时序兜底余量，不再是“开始时间 ±窗口”。不能用扩大窗口代替以上约束。

## 状态与展示

列表 API 新增 `usage_link_status`（`linked`/`unlinked`/`failed`/`not_billable`）、`usage_match_method`（`request_id`/`timing`/`legacy_window`，后者表示尚未复核的旧窗口关联）、`completed_at`、`response_status`。Token 与费用字段在非 `linked` 行仍由 SQL 补 0，只是占位；页面表格与抽屉显示 `—` 并说明“未知，不代表零消耗”，合法的零消耗只出现在已关联行。

请求明细是完整归档视角：服务端始终包含未关联行并标注状态，因此已正确分类的人工请求在“包含未匹配”勾选与否时都可见。该勾选改名为“排行含未关联请求”，只影响用户/项目排行的请求数；排行与概览的 Token/费用只统计已关联行，概览始终包含未关联行。来源列改称“本次调用来源”，预览列改称“最近用户输入”：工具续执行可以与发起它的人工首发共享同一句预览，来源仍按本次调用判断。

## 存量数据恢复

已部署代码之前的行没有关联键和完成时间，`usage_match_method` 为空：

- 自动恢复：新版本部署后，自动索引每轮复核昨天和今天的存量行（未关联行重试；旧窗口关联行确认、改判或解除），只读写数据库。
- 历史日期（DB-only，不需要 JSONL）：在页面对目标日期执行“索引当前范围”。文件水位已追平时只做关联轮次；JSONL 已删除的日期同样可用。这是生产写操作（更新关联、风险分），需单独授权。
- 更强的历史恢复（需 JSONL 仍在）：与已挂起的 NUL/multipart 受控重索引合并执行，重置目标文件水位后重读，response 行补回完成时间（旧归档没有关联键和 response.id），存量行改用有完成时间的约束，可消除部分只看开始时间时的歧义。单日归档上百 GiB，只扫一遍；按升序或逆序回填都可，逆序时完成事实先暂存后合并；完成核对前不删除 JSONL。
- 只读核对（部署后、授权终端执行）：

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

-- 本次缺陷样本
SELECT archive_id, usage_log_id, usage_match_method, completed_at, summary_json->'request_origin'
FROM token_analysis_request_summaries WHERE archive_id = '73eb6d29496d6c677054f5c797996d66';
```

## 已知限制

- 存量行没有关联键：只在归属唯一时按时序关联。顺序请求间隔短于 window、同键并发、计费排队等歧义场景保持未关联（显示“未关联计费”，不猜测）；预处理超过 window 才开始转发的存量请求同样保持未关联。受控重索引补回完成时间后可解开其中一部分。
- 新请求的主账单都能精确关联：HTTP 转发按 `client:`，WS 上游按 response.id。只有 response.id 无法捕获（非流式响应超过 256 KB 尾部窗口、非 Responses 协议经 WS 转发）时才走受限的时序兜底，歧义时同样保持未关联。
- response 行被归档队列丢弃时，新请求只能按 `client:` 关联，WS 上游账单保持未关联。
- 未关联不等于未计费：状态提示说明可能仍在落库或无法唯一确定，后续索引会重试。

## 验证

- 仓储集成测试（嵌入式 PostgreSQL 18 新建空库，`token_analysis_usage_link_integration_test.go`，16 个顶层用例）：生产时序复现（18.4 秒账单、前一请求账单紧邻开始时间、旧窗口错关联被改判并由正确请求认领，开关两态均可见且为人工）；账单晚到先未关联、后按 request_id 关联且不抢相邻 `client:` 账单；失败请求、客户端重试、两条 WS 并发请求各取各的账单；存量歧义（失败后重试、计费排队）保持未关联；精确账单晚到时不先抢 WS 邻居账单；存量行不能占用他行精确账单；上游 response.id 精确关联并夺回时序占用；持行锁制造交错的并发认领只成功一条；陈旧的时序/解除写入不能覆盖较新的精确关联；跨日请求按逆序、升序分日、整段三种回填方式都能关联；晚到的失败响应解除时序关联（有无 response.id 两种）；被另一日请求夺回账单的旧行在同一次索引里重算风险并保留 body 自身风险；跨日请求账单晚于 response 落库时在请求日之外的索引中仍能关联，账单一直不来的请求截止后确认未关联；查找后才登记的首次失败响应使陈旧时序写入被拒，陈旧确认不能吞掉夺回账单产生的新复核；旧失败行错误关联被清除、失败请求的精确账单仍关联；来源筛选。
- 变异检查：分别去掉归属唯一约束、他行精确账单保护、账单咨询锁、精确证据夺回弱关联、关联状态条件更新、复核版本条件更新、确认改为确认当前版本、夺回时登记复核、完成事实变化登记复核、跨日期处理待复核行、等待晚到账单，各自对应用例失败。确认只写回读到的版本，因此单独去掉确认的版本条件不会让陈旧确认生效，这一项不构成缺口。
- 三轮独立复核的复现：首轮六个中五个原样通过、并发认领那个改为持锁交错用例；第二轮六个场景与第三轮三个场景全部通过，其中直接调用仓储写入/确认的复现需要传入调用方查找前读到的状态与版本（新增参数，触发顺序与预期不变），第三轮复现读取的旧 `usage_link_pending` 列改为等价的版本比较。均已改写为上述永久回归。
- 服务/仓储单测：精确/兜底切换、失败行与已确认时序行只认精确证据、登记复核后完整复核、无需改动时按版本确认、等待晚到账单（带关联键/未失败/未过截止才保留，解除关联时也不确认）、被夺回行解除并重算风险、释放后下一轮认领；SQL 形态与参数（关联状态与版本条件更新、复核登记、跨日期候选）；列表强制完整视角与状态推导；归档中间件记录 `client_request_id` 与上游 response.id；`in-app-browser-context` 预览清理与生产同构样例分类。
- 前端：未关联/失败/不计费/旧接口行显示 `—` 与状态标签，抽屉显示关联方式、账单 ID、完成时间与“未知”说明。

## 第四次独立复核状态（2026-10-25）

本地审核通过：第三轮 3 个原始失败场景与当前正式数据库专项 17 项全部通过，本轮未发现新的实质遗漏。版本化确认、48 小时晚到账单重试及期限结束均已验证；证据见 `docs/reviews/2026-10-25-token-analysis-usage-link-fourth-review-cn.md`。此状态不代表生产验收完成：未部署、未执行受控历史回填或原始样本上线后核对。
