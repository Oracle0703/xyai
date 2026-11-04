# Token Analysis 请求来源识别设计

更新：2026-11-04。状态：**v2 规则已实现，并已通过独立复审（2026-10-28，F1–F3 已关闭）；当前 main（`26f6363af`）中 `tokenAnalysisOriginVersion = 2`。生产窗口验收、受控历史重索引、真实规模下的性能验收和浏览器验收都还没完成（§12）。**

本文是请求来源识别功能的唯一文档，覆盖人工/机器来源分类（v1→v2）、净输入与预览提取、人工输入次数估算、样本与生产审计结论，以及历史回填边界。审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看，路径见 §6）。审核中被采纳、且对后续修改有约束的结论都已写入正文。计费关联是另一个功能，见 [归档请求与计费记录关联设计](token-analysis-usage-link-design-cn.md)。

阅读顺序建议：
- 修改分类规则：先读 §3 和 §9，再读 §6、§7 的证据边界；
- 修改人工次数或统计 SQL：读 §4、§9；
- 修改预览/净输入或页面：读 §5；
- 定位代码：读 §8；
- 部署、回填或验收：读 §10、§11、§12。

## 1. 目标与非目标

来源只回答一个问题：**本次模型调用是因为什么发起的**。它不判断文字是谁写的，也不判断账号是不是机器人；只用于管理分析，不参与鉴权、计费或绩效判断。

- 客户端的 role 和 metadata 可以伪造，所以"人工"标签证明不了某个人真的点了发送；高置信的机器形态也不能绝对排除有人手动粘贴了同一份完整客户端模板。
- 同一个人工首发之后会有很多工具续执行调用。它们和首发共享"最近用户输入"预览，但来源要按本次调用判 AI（§5）。
- 已否决：按"摘要/标题/JSON/Python/User-Agent"等关键词、工具声明数量、历史正文、重复次数或请求间隔判断来源。原因：这些信号在 10-26 生产审计中两类请求都有出现，会误伤真实人工请求（§7.3）。

## 2. 存储与展示合同

复用 `summary_json.request_origin = {kind, reason, confidence, version}`，只存固定枚举，不存正文或会话标识。分类本身没有新增 migration；人工次数依赖已有的指纹迁移 251/252。

| 展示 | kind / reason / confidence | 含义 |
| --- | --- | --- |
| 人工 | human / user_input / medium | 有新用户输入结构，且没命中明确的机器用途；普通脚本仍可能显示为人工 |
| AI | ai / subagent、automation、tool_result、agent_message、assistant_continuation、compaction / high | 显式元数据或协议结构证明是机器发起 |
| AI | ai / compaction_resume / medium | v2 按恢复摘要结构推断的续执行（推断，不是证明） |
| 未识别 | unknown / no_signal、unsupported_shape、body_truncated、count_tokens / none | 无信号、结构未知、归档截断或计数探测；没有来源字段的旧行也显示未识别 |

- 当前写入 version 2。前端同时兼容 v1 和 v2；组合不合法、版本未知，或 v1 带 `compaction_resume` 时一律显示未识别。
- reason → kind/confidence 的映射以后端 `tokenAnalysisOriginReasons` 为准，前端 `requestOrigin.ts` 必须与之对照维护（测试 `TestTokenAnalysisRequestOriginContractFollowsReason`）。

## 3. 分类规则（v2 修订）

v1（2026-10-24 实现、2026-10-25 部署）只有末条 user 文本兜底判人工这条主线，白名单和包装标签全部来自 10-23 的 272 条样本（§6.1）。10-26 的生产审计在人工标签里检出了 478 条机器请求（§7），v2 据此补充规则。本节是现行规则。

### 3.1 判定顺序

1. **按原始路径覆盖**：count_tokens 固定判未识别，优先级高于截断；其余截断请求（即使 JSON 或 multipart 能解析）判未识别；完整的 `/responses/compact` 判压缩。
2. **检查客户端元数据白名单**（§3.2）：子 Agent 标记优先，然后是 `request_kind=compaction`，再看自动 trigger 和专用 thread_source。`x-codex-turn-metadata` 支持字符串和对象两种形式，但生产上只验证过字符串（§6.1）。
3. **从协议尾部向前找有效事件**：跳过任意类型的 system/developer（包括 `additional_tools`），以及完整的纯注入 user 消息。尾部出现未知项时不能跳过，也不能拿更早的历史用户消息补判。
4. **判定**：工具输出、Agent 消息、assistant/model 续执行、压缩条目判 AI；普通 user 文本/图片/文件推断为人工。如果工具结果和独立的新用户内容混在一起，以新输入为准。

### 3.2 元数据白名单

| 字段 | 有效值 → reason |
| --- | --- |
| `x-openai-subagent` | collab_spawn、guardian、memory_consolidation → subagent |
| 嵌套 `subagent_kind` | thread_spawn、guardian → subagent。与上一项是两套值域，不能互相替换 |
| 嵌套 `request_kind` | compaction → compaction |
| 嵌套 `turn_trigger` | automation_cron_scheduled、automation_heartbeat_scheduled、goal、memory_consolidation、guardian_review、guardian_classifier、ambient_suggestions、ambient_suggestion_safety、thread_title、thread_description → automation；app_tool_send_message → agent_message |
| 嵌套 `thread_source` | thread_title、thread_description、ambient_suggestions、ambient_suggestion_safety、guardian_review、guardian_classifier、memory_consolidation → automation |

以下信号都**不能单独**证明来源：composer/user/queue/edit_user_message/resume_interrupted_task 这些 trigger，user/automation/system 这些 source，history_initialization，User-Agent，工具声明及数量，历史正文，重复次数，短间隔，previous_response_id。依据：10-26 审计中这些值在带真实用户正文的请求上也出现过。

### 3.3 user 内容形态

| 形态 | 处理 |
| --- | --- |
| 纯注入包装 | environment_context、in-app-browser-context、system-reminder、skill-context、system-update：必须完整闭合、包装外没有正文才跳过，可以带属性，可以连续多段。未闭合、正文中引用、包装后追加新问题时保留为用户内容 |
| 固定压缩模板 | 只把 CRLF 正规化并去掉首尾空白，然后要求完全匹配。Codex 用常量 `tokenAnalysisCodexCompactionPrompt`；Copilot 的两种和 hertz 的一种用全文长度加 SHA-256 存在 `tokenAnalysisCompactionPromptDigests`。不按关键词猜测 |
| Codex 恢复摘要 | 有效末项以完整的 `tokenAnalysisCodexResumePrefix` 开头，且后面没有独立的新文本块、媒体或有效 user 输入 → compaction_resume/medium。无状态判断，不关联前驱请求，不保存会话标识 |
| 子 Agent / goal 通知 | 完整的 `subagent_notification`，内层 JSON 有非空 agent_path 和对象类型的 status → agent_message；完整的 `codex_internal_context source="goal"` → automation。包装外有新正文时仍是人工 |
| WorkBuddy 后台通知 | `task-notification` 带 task-id/status 标记，后面只能跟同一 task id 的固定 NOT USER INPUT 尾注 → tool_result |
| WorkBuddy 工具图片 | `tokenAnalysisToolImageMarker`（`Attached image(s) from tool result:`）加图片、没有新正文，且前一个有效项是工具结果 → tool_result。中间的纯注入消息可以跳过；真实正文、机器通知和未知结构会阻断 |
| hertz 纯图片 | "tool 之后出现只有图片的 user 消息"不足以证明图片来自工具，暂不覆盖，等客户端提供显式标记 |

### 3.4 v2 已知不覆盖的情形

- **依赖专用 system 文本的辅助调用**（10-26 窗口 66 条：会话概况生成 20、命名 13、system 定义的子 Agent 调用 11、Copilot 进度文案 8、工具分组 7、补丁修复 7）。这些请求没有显式标记，v2 仍显示人工。已否决：按 system 提示关键词识别，原因同 §1。
- 普通脚本、没有标记的内部任务：只能推断，可能显示人工。
- 客户端把排队的新用户消息包进纯 `system-reminder` 时会被跳过。生产上还没见过这种形态（§6.1、§6.4），属于已接受的风险。
- 恢复摘要和新问题写在同一个文本块里时没有可靠的分界，会判 compaction_resume。已接受（§9）。

## 4. 人工计数口径（human_input_count）

标签、Token、费用和 request_count 仍然按请求统计。新增的 `human_input_count` 是**估算**的人工输入次数，用于抵消重试副本（10-26 窗口里人工标签中有 291 条疑似同请求的重试副本，间隔 1–5 秒，多为 404/503/403/400 失败）。失败或中断的请求也可能产生账单，费用不因请求失败而忽略。

- **归并范围**：只归并当前筛选集合内的 human 行。要求双方 `fp_state=ok`、API Key 和 hash 非空、同一 Key、同一指纹版本、同一 `prefix_end_hash`；按 `(event_time, archive_id)` 排序，前后间隔 ≤15 分钟。连续链只计链首（0/14/28 分钟的三条计 1）。Key 为 NULL 或指纹无效的行逐条计数。
- **前驱也要过筛选**：前驱必须满足相同的时间、用户/邮箱/Key/账号/分组/模型/endpoint、项目、风险、来源和 include_unmatched 条件；未指定账号或分组时不额外要求相等。跨时间边界的链在相邻两个范围里各计一次，这一点已接受。
- **各视图**：概览和请求明细强制包含未关联行；排行用它自己的最终筛选。例如失败的 A 之后成功的 B：概览包含 A、B，计 1；默认排行只有 B，也计 1。邮箱条件要覆盖概览主查询、风险原因、人工次数和同期计费分母（复审 F1）。
- **SQL**：三个统计接口共用 `tokenAnalysisHumanInputSQL`，按 Key/版本/fp_state/hash 分区取 `LAG(event_time)`，等价于"15 分钟内存在前驱"；计数归入链首所在的桶。项目必须按原始 `client_project` 关联，不能用展示名 `unattributed` 当键（复审 F2）。
- **指纹的含义**：`prefix_end_hash` 包含协议族、保留的顶层字段和全部条目，排除生成参数、client_metadata、prompt_cache_key 等，所以它不是严格的请求身份，不同会话也可能被合并。不能按末条文字去重（历史不同的 27 次"A"应该分开计），也不做跨压缩或跨子线程的整轮归集。

性能：窗口函数替代了最初的 EXISTS 方案，没有新增 migration。实施方在 PG18、100.5 万行/14 天的合成数据（含 5,000 行的长重复桶）上测得概览约 3,761 ms → 620 ms，两种写法计数一致；为人工行建部分索引约 455 ms，暂未采用。独立复审没有重跑这个基准，生产性能仍待验收。

## 5. 净输入、预览与页面

- **预览/净输入提取**：跳过工具输出、assistant/Agent 文本，以及开头完整闭合的注入包装；剥离包装后，紧跟着的 `## My request:` 标题也去掉。正文中的引用、未闭合包装、正文之后的包装、没有包装的同名标题都保留。没有新正文的工具轮次沿用之前的输入。
- 机器通知、压缩指令和恢复摘要仍可能进入预览，这是现存边界。因此**不能用预览或 duplicate_count 反推来源**。
- **净输入的持久化**：净输入变化时清空旧的 `quality_*`，哈希不变时保留（`UpsertUserInput` 按更新前的 `content_sha256` 判断）。只有在留存开启、未截断、未降级且完整解析确认没有正文时，才调用 `DeleteUserInput` 删除旧净输入；采集关闭、截断或降级时不删。
- **页面**：请求表显示"本次调用来源"，抽屉显示 reason/confidence/version；前端直接用列表返回的 summary_json，不自行推断，也不逐行请求分类。概览和用户/项目排行展示人工输入估算及说明，前后端必须同批发布。
- **origin 筛选**：`origin=human/ai/unknown` 作用于概览、排行和明细，非法值返回 400；unknown 包含缺来源字段的旧行。来源、风险、项目等归档筛选不作用于计费趋势和同期计费请求数，所以筛选后的覆盖率只能参考。
- **筛选快照**：概览、各列表、趋势、排序/翻页续查和索引触发都读取已提交的 `appliedFilters`，不读草稿。
- 真实人工首发曾因为计费关联失败被默认明细隐藏。那是关联问题，不是来源误判；现在明细始终包含未关联行，Token/费用用"—"表示未知。详见计费关联文档。

## 6. 样本与证据集

以下证据目录都已移出仓库，最后存在于提交 `26f6363af`。代码和测试注释里提到的证据都指向本节。各证据集共同的脱敏原则：不导出正文、工具参数/结果、图片字节、凭据、用户 ID/邮箱/IP/headers 或原始会话 ID；保留 archive_id、源文件、行偏移和行 SHA-256（含行尾 LF），供授权人员回到生产原文复查。生产原文位于 `/opt/sub2api/backend/data/request-archive/<日期>.jsonl`，有保留期，定位信息不保证永久可读。

### 6.1 2026-10-23 结构样本（`docs/delivery/2026-10-24-token-origin-samples/`）

- **来源与窗口**：生产文件 `2026-10-23.jsonl`（132,582,213,280 bytes），2026-10-24 02:10 只读采样。按 `int(size*i/16)` 取 16 个字节锚点，每个窗口最多 20 条 request，单行上限 20 MiB，实际读取 532,218,034 bytes；采样前后文件的 stat 完全一致。
- **样本量**：272 条请求，Responses 259、Chat 8、Messages 5，`body_truncated` 为 0。末项结构是互斥计数：custom_tool_call_output 156、function_call_output 54、Chat tool 6、agent_message 22、Messages system 尾注 5、developer 1、user 28。子 Agent 标记：collab_spawn 88、guardian 6、memory_consolidation 3、无标记 175。
- **脱敏**：`text` 只保留长度、标签和包装布尔值；15 条重点案例只导出最后 6 个事件的结构。完整性由 `checksums.json`（SHA-256）固定。
- **局限**：这不是随机样本，也不是人工逐条标注的真值，不能推算准确率或客户端份额。样本里没有 Gemini、multipart、count_tokens 和截断请求，这些边界只靠构造用例覆盖。`x-codex-turn-metadata` 在 252 条中是字符串，另外 20 条的 `{}` 是采集器兜底，不能当作对象格式的证据。
- **得出的规则**：子 Agent 白名单；`subagent_kind=thread_spawn` 是独立值域；system/developer（包括 `additional_tools`）先于 item type 跳过；纯 environment 注入跳过；composer 不能证明人工；#193（文本+图片，JSON 带 preflight，没有元数据）和 #175（翻译模板）结构上无法判定，保持人工推断。
- **Messages 追加检查**（同目录的 `messages-audit.md`、`messages-history-audit.json`）：在同一文件做 1 GiB 有界读取，得到 1,246 个完整行（含 response 行），其中 Messages 11 条，**tool_result 和纯 reminder 同在一条消息里的情况 0 条**。另对 5 条完整历史逐条检查，确认存在"assistant 之后纯 reminder"和"reminder + 正文"两种形态。前者现在跳过包装后判 AI，后者仍判人工。
- **对应用例**：`TestTokenAnalysisRequestOriginUsesLatestEffectiveInput` 中带 `#N` 的用例（#2/#5/#15/#23/#153/#184/#193/#210/#230）和带 `prod history` 的用例；`TestTokenAnalysisRequestOriginMetadataIsAllowlisted`。tool_result+reminder 的混合形态只作防御用例（`tool result with stacked reminders` 等），不能说成生产已经发生。

### 6.2 人工首发被隐藏的现场证据（`docs/delivery/2026-10-24-token-origin-human-input/`）

- **对象**：一条请求，archive `73eb6d29496d6c677054f5c797996d66`，2026-10-24 14:11:12.654 开始，`/v1/responses`，input 共 119 项，末项（index 118）是 user/input_text，metadata 为 `turn_trigger=composer`。请求行偏移 18155124865、长 942,377 bytes、SHA-256 `a2e65ddf…e74e1`；response 行偏移 18166371053，HTTP 200，duration 18547 ms。
- **`classifier-fixture.json`**：最小同构样例。末条 user 内容原样取自归档，即 `<in-app-browser-context source="ambient-ui-state">…</in-app-browser-context>` 包装，后接 `## My request:` 和真实问题；历史删减成一条 assistant。当时预期 human/user_input/medium/v1，预览应等于原问题。包装里有一个内部管理页 URL，没有凭据。
- **得出的规则**：`in-app-browser-context` 加入纯注入包装；预览剥离开头完整包装和紧随的 `## My request:`。这条请求在分类上从来是对的，它被隐藏是计费关联缺陷造成的，缺陷链和该目录里的 usage-link 复现文件属于计费关联文档。
- **对应用例**：`TestTokenAnalysisRequestOriginProductionAmbientContextTurn`，以及 `ambient context only after assistant`、`different tag sharing the prefix`。

### 6.3 10-26 生产来源审计（`docs/delivery/2026-10-27-token-origin-audit/`）

- **窗口**：北京时间 `[2026-10-26 09:00, 2026-10-27 09:00)`，生产版本 `25bedc620`，所有行的来源版本都是 1。
- **样本量**：60,065 条已归档且已索引的请求（人工 3,591、AI 56,387、未识别 87，其中截断 68、count_tokens 19）。人工标签全量核查；AI 按"标签×端点"分层抽样，排序键为 `md5('origin-audit-20261027:' || archive_id)`，Chat/Responses/Messages 各取前 150 条，compact 全取 20 条，共 470 条。去重后共 4,061 条唯一记录，累计读取 8,872,057,311 bytes，提取错误 0。
- **脱敏**：只保留 ID、类别、元数据枚举、行位置和行 SHA，没有正文、工具输出、用户或 API Key 标识；完整性由 `SHA256SUMS` 固定。`source_offset` 是行**末**偏移，复查时必须向前找行首，并核对 event、archive_id 和行 SHA。
- **得出的规则**：§3.2 中新增的 trigger 和 thread_source；固定压缩模板（Responses 205 条加 Chat 27 条，其中 Chat 的 27 条来自 Copilot 和 hertz 的长提示，只以全文长度加 SHA-256 保存：6857 hertz、6156 Copilot 0.56–0.71、5115 Copilot 0.71）；skill-context、subagent_notification、system-update、task-notification、工具图片、goal 等包装。
- **对应用例**：`TestTokenAnalysisRequestOriginV2ProductionShapes`、`TestTokenAnalysisRequestOriginMetadataIsAllowlisted`、`TestTokenAnalysisCompactionPromptDigestsAreWellFormed`。结论见 §7。

### 6.4 v2 设计补证（`docs/delivery/2026-10-27-token-origin-v2-evidence/`）

- **预检**：2026-10-27 16:35，生产仍是 `25bedc620`，还没有指纹列；归档目录里只剩 `2026-10-27.jsonl`（另有 05-28、08-09），**10-26 的原文已不存在**。
- **覆盖**：摘要续接候选 186 条，只读到 2 条；固定压缩 207 条，读到 3 条；hertz 纯图 22 条，0 条；AI Messages 1,053 条，读到 64 条。共 74 次定点读、121,058,550 bytes，错误 0。身份比较使用一次性随机 HMAC key，只导出"是否相等"。
- **结论**：读到的 2 对摘要/压缩请求同属一个 API Key 和 session/thread/turn，父请求是 `request_kind=compaction`、auto、mid_turn，紧跟在工具输出之后。另有 1 条压缩是 pre_turn，所以不能把摘要之后的请求一律当作 AI。64 条 Messages 的完整历史中 tool_result+reminder 同条为 0，**这项补证就此关闭**，只保留防御规则。压缩计数换成一致的分母：207 条中前接工具输出 200 条；去掉与元数据层重叠的 2 条后为 205/198。重复候选 291/255 没有经过完整指纹验证。

### 6.5 10-27 压缩后继补证（`docs/delivery/2026-10-27-compaction-followup/`）

- **窗口**：`2026-10-27.jsonl` 的固定前缀 110,200,243,008 bytes，快照时间北京时间 18:17:24.690，不是完整的一天。完整扫描了 99,536 行（49,781 request / 49,755 response），外层 JSON 错误为 0；其中 63 个 body 无法完整解析（43 个截断的 Responses、20 个 multipart）。
- **脱敏**：没有正文、图片或真实 ID；身份关联同样用临时 HMAC，只导出相等布尔；完整性由 `SHA256SUMS` 固定。
- **计数**：显式压缩 286 次（auto/mid_turn 233、auto/pre_turn 53、manual 0），客户端版本为 Codex 0.153.4–0.164.0（desktop、tui、vscode）。紧接的下一请求以恢复摘要为有效末项的：mid_turn 205/233，**pre_turn 0/53**；pre_turn 之后"摘要后跟 user 正文"的有 19 条；32 个直接后继本身又是压缩，归入"其他"。
- **反查**：以恢复前缀结尾的请求 213 条，其中 212 条能找到同 turn/root_turn、HTTP 200 且先完成的 mid_turn/auto 前驱（205 条是直接后继），1 条缺 session/thread 无法关联（`f7e10bc2…`）。固定模板 268 条中 267 条带 `request_kind=compaction`（99.63%），唯一没标记的是 `e9d72fa4…`；另有 19 次显式压缩不用该模板。
- **得出的规则**：显式 `request_kind=compaction` 优先，模板作兜底；恢复续接改用无状态前缀规则，判 medium（取舍见 §9）。
- **对应用例**：`TestTokenAnalysisRequestOriginV2ProductionShapes` 中的 `codex compaction prompt*` 和 `resume summary*` 系列，以及 `new request after resume summary`、`resume prefix quoted` 等防误伤用例。

### 6.6 v2 实现复审证据（`docs/delivery/2026-10-28-token-origin-v2-review/`，含 `rereview/`）

- 全部使用本机合成数据和独立的 PostgreSQL 18.3 临时库，没有生产数据。基线为 `02cf7a07c` 加未提交修改，源文件哈希记在 `source-hashes.json`。
- 原始复现通过 Go `-overlay` 加载，复现文件内容不改：`TestCodexV2ReviewSummaryEmail`（F1，邮箱不匹配时概览仍计 1）、`TestCodexV2ReviewProjectBucketCollision`（F2，2 次计成 4 次）、`TestCodexV2ReviewToolImageInjectedPredecessor`（F3，被判 human）。修复后三者都通过，并已转成正式回归：`TestTokenAnalysisHumanInputCountFollowsFilters`（邮箱）、`TestTokenAnalysisHumanInputCountKeepsRawProjectBuckets`、用例 `tool images after injected context`。
- 修复后结果：service/repository/handler/admin 共 140 个顶层测试通过；PG18.3 共 60 个顶层测试通过（58 个正式用例加原 F1/F2）；前端 4 个文件、50 项通过；0 失败、0 跳过。

## 7. 生产审计结论（v1，10-26 窗口）

### 7.1 总体

在人工标签里检出 **478 条机器请求**：显式元数据 87 条，高置信客户端结构 391 条，两层互斥。478/3,591 = **13.31%**，这是按协议语义得出的保守检出比例，**不是真实误判率**；最严格的元数据口径是 87/3,591 = 2.42%。478 占全部 60,065 条请求的 0.80%。剩下的 3,113 条人工标签不能视为已验证正确。AI 抽查 470 条，没有发现能确认的新人工输入被误判为 AI；但 0/470 不能外推为 AI 全量零误判。

| 端点（人工标签） | 全量 | 检出 | 比例 |
| --- | ---: | ---: | ---: |
| Responses | 2,766 | 342（87 元数据 + 255 结构） | 12.36% |
| Chat Completions | 702 | 127 | 18.09% |
| Messages | 79 | 9 | 11.39% |
| Images | 44 | 0 条可确认 | 不能据此说全部正确 |

### 7.2 分类与根因

- **元数据 87**：`thread_source=thread_title` 37（全部来自 codex-tui 0.158–0.164，只写 thread_source，turn_trigger 为空；10-23 的样本里两个字段都有）、`ambient_suggestion_safety` 27、`thread_description` 16、`app_tool_send_message` 4、`automation_heartbeat_scheduled` 3。
- **结构 391**：固定压缩指令 232（Responses 205 + Chat 27）、工具之后的纯 skill-context 58、子 Agent 通知 29、依赖专用 system 的辅助调用 66（见 §3.4）、后台任务事件 2、工具结果图片 2、system-update 1、goal 续执行 1。
- **根因**：实现和设计一致，存储也没错；问题在于 v1 的白名单和包装标签只来自 272 条样本，客户端把机器内容放进普通 user 消息后落到了"末条 user 判人工"的兜底分支。
- **报告之外的发现**：207 次本地压缩中 198 次紧跟工具输出，属于轮中自动压缩，所以 186 条以恢复摘要结尾的请求（审计保守地仍算人工）大概率是 AI，v2 用 compaction_resume 覆盖。hertz 有 22 条在 tool 之后只带 image_url 的 user 消息被判人工，v2 仍不覆盖。人工标签里有 291 条重试副本（§4）。

### 7.3 排除项与反向抽查

- 以下情况不自动算作误判：普通恢复摘要、只用自然语言描述的父任务、只有标题提示、脚本/研究/结构化 JSON 模板；Responses 中 87 条 question-reply 保留为人工。Chat 中普通标题提示 18 条、恢复节选 6 条、没有严格外部边界的终端通知 2 条没有计入。
- AI 抽查的 470 条中，末有效事件为：工具结果 443、Agent 消息 18、Guardian 审查 7、子 Agent 委派 1、assistant 之后纯 reminder 续执行 1。
- 分类器回放结果与入库一致，**不能**证明分类准确；旧的人工输入预览也不能证明当前的工具续执行应判人工。

## 8. 代码地图

| 关注点 | 位置 |
| --- | --- |
| 分类入口与版本 | `backend/internal/service/token_analysis_origin.go`：`classifyTokenAnalysisRequestOrigin`、`tokenAnalysisOriginVersion`、`tokenAnalysisOriginReasons` |
| 压缩、恢复、工具图片常量 | 同文件：`tokenAnalysisCodexCompactionPrompt`、`tokenAnalysisCompactionPromptDigests`、`tokenAnalysisCodexResumePrefix`、`tokenAnalysisToolImageMarker` |
| 预览/净输入、multipart | `backend/internal/service/token_analysis_summary.go`：`SummarizeTokenAnalysisMultipartRequest` 等 |
| 索引接入 | `backend/internal/service/token_analysis_indexer.go`：`indexArchiveLine` |
| 净输入持久化 | `backend/internal/repository/token_analysis_repo.go`：`UpsertUserInput`（按哈希保留或清空 quality_*）、`DeleteUserInput` |
| 人工次数 SQL | 同文件：`tokenAnalysisHumanInputSQL`，被 `GetSummary` 路径、`ListUserUsage`、`ListProjectUsage` 共用 |
| 前端 | `frontend/src/components/admin/token-analysis/requestOrigin.ts`（合同对照）、`TokenAnalysisRequestTable.vue`、`TokenAnalysisRequestDrawer.vue`、`TokenAnalysisOverview.vue`；页面 `frontend/src/views/admin/TokenAnalysisView.vue`；API `frontend/src/api/admin/tokenAnalysis.ts` |
| 后端测试 | `backend/internal/service/token_analysis_origin_test.go`；`backend/internal/repository/token_analysis_human_input_integration_test.go`、`token_analysis_user_input_integration_test.go`、`token_analysis_repo_test.go`（`TestTokenAnalysisHumanInputSQLReusesFilters`） |

## 9. 关键约束与易错点

- **包装必须完整闭合且包装外无正文才能跳过。** 依据：生产历史中同时存在"纯 reminder"和"reminder + 正文"（§6.1），用户也会引用标签；未闭合或追加新问题的情况都必须保留为人工。
- **固定模板只做 CRLF 和首尾空白正规化，然后全文比对；长模板只存长度加 SHA-256。** 依据：模板加追加问题、被引用、拆到多个块里都是真实可能的形态（用例 `compaction prompt plus user request`、`quoted`、`split across blocks`）。
- **未知尾项判 unsupported_shape，不能跳过后用历史 user 补判。** 依据：历史 user 属于更早的轮次，用它补判会把续执行误标为人工。
- **工具图片规则要找前一个有效项，中间的纯注入消息可以跳过。** 依据：曾复现的失败场景是 tool → 纯 `system-reminder` user → 带标记图片，被判 human（复审 F3）。
- **人工次数的前驱必须过同一套筛选，项目按原始 `client_project` 关联。** 依据：曾复现邮箱不匹配时排行为空、概览仍计 1（F1）；空项目和真实的 "unattributed" 项目按展示名关联后，2 次被计成 4 次（F2）。
- **抽取结果为空时，只有在完整解析确认无正文的情况下才删除旧净输入。** 依据：曾复现 tool_use → 只有 tool_result 的 user 重索引后，旧的"tool output"净输入仍然残留（2026-10-24 实现审核 F1）。采集关闭、截断、降级时不能误删。
- **净输入换了哈希就清空 quality_*。** 依据：旧版保存的是 assistant 文本，回填后变成用户正文，旧评分会挂到新正文上（同审核 F2，由 `TestTokenAnalysisUserInputQualityFollowsContentHash` 覆盖）。
- **排序/翻页/索引触发只读 `appliedFilters`。** 依据：曾复现"已应用 model-a，草稿改成 model-b 未提交，点击排序后实际请求了 model-b"（同审核 F3）。
- **证据结论不能放大。** 272 条样本和 1,246 行（含 response）都不是随机样本；13.31% 不是误判率；0/470 不代表零误判；强相关不等于来源证明。
- **已定取舍（2026-10-27，用户决定）：恢复续接用无状态前缀规则判 compaction_resume/medium。** 依据：pre_turn 后继以摘要结尾为 0/53；能关联上的 212 条摘要尾项全部来自 mid_turn。已否决：审计方建议的"可靠前驱关联或客户端显式恢复标记"，原因是前驱关联需要保存会话关联状态，用户选择不做，并用 medium 表示这是推断。已接受的风险：证据只覆盖单日的 Codex 0.153–0.164，manual 压缩没有样本，同一文本块里追加的新问题没有可靠边界，需要用新窗口复验。
- **已否决：用 EXISTS 子查询求人工次数。** 原因：100 万行/14 天的合成数据上概览约 3.8 s，LAG 窗口函数约 0.6 s，计数一致。
- 旧的 291 条重复候选（≤15 分钟的 255 条）不是真实的重试真值；186 条摘要候选中有 2 条已计入 478，追加候选不能不逐条核对就直接累加。

## 10. 历史回填与重索引边界

新规则只对之后索引的请求生效。自动索引默认每 300 秒增量处理昨天和今天；"索引当前范围"不会重算已经读过的行。

- **原文保留**：v2 验收前至少保留 10-27 或之后的一个完整日的原文。10-26 的原文已经不在，那个窗口既不能补判，也不能验收；没有原文时不能凭预览补判。
- **受控重索引**需要另行授权、先备份、明确范围。要确认索引互斥、自动索引不会抢先运行，同时重置目标文件的 offset 和累加计数，一次性重算来源、净输入和指纹，然后再关联计费、重算风险。不能只清 offset，也不能改动原始归档。这次重索引要和 NUL/multipart 补录、计费关联补完成时间合并成一次扫描（见计费关联文档 §8）；完成核对之前不删 JSONL。
- 10-27 16:35 预检时生产为 `25bedc620`、没有指纹列。这是历史快照，执行前要重新核验当前部署和 schema。

## 11. 验证

```bash
# 在 backend/ 下执行。CI 单测门禁使用 -tags=unit，默认 tag 跑不到 unit 用例
go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run 'TokenAnalysis'
# 真实 PostgreSQL：DSN 必须指向新建的空库（harness 会执行 migration 并写数据，禁止指向生产或业务库）
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN=... go test -tags integration -p 1 -count=1 ./internal/repository -run 'TokenAnalysisHumanInput|TokenAnalysisUserInputQuality|TokenAnalysisListRequestsFiltersByOrigin' -v
```

前端专项是 TokenAnalysisView、API、locale 完整性和文案编译 4 个 Vitest 文件，另外跑 `pnpm run typecheck` 和相关 ESLint（在 `frontend/` 下执行）。

最近结果（2026-10-28 独立复审，本机）：Go 专项 140 个顶层测试、PG18.3 空库 60 个顶层测试、前端 4 个文件 50 项全部通过；gofmt、locale ESLint、`git diff --check` 通过。没有独立重跑整仓构建和百万行基准。

## 12. 状态与待办

| 项 | 状态 / 要求 |
| --- | --- |
| v2 规则与人工计数 | 已实现，已通过复审；代码在 main |
| 生产部署 | 本文没有记录 v2 的部署结果，执行和验收前要核对生产版本和 schema（251/252 指纹列） |
| 原文保留与受控重索引 | 未执行，需授权（§10） |
| 新窗口验收 | 按完整指纹逐对说明归并原因，不预设 255 条候选全部命中；要覆盖同时间戳、15 分钟边界、长链、不同 Key/版本、NULL Key、失败首发、筛选内只有后继、v1/v2 混合；按端点抽查来源和未知结构 |
| 性能与页面 | 在真实规模数据上做 EXPLAIN (ANALYZE, BUFFERS)，完成浏览器验收；合成测试不能代替生产结果 |
| hertz 纯图 | 需要客户端团队说明生成方式，并提供显式来源字段 |
| 专用 system 辅助调用 | 66 条/日量级仍显示人工，等客户端提供显式标记后再处理 |
| P3 索引完成自动提交草稿 | 未修：`TokenAnalysisView.vue` 的 `pollIndexStatus()` 在索引结束后调用 `reloadAll()`，而 `reloadAll()` 第一步会把草稿复制进 `appliedFilters`。应把"提交草稿"和"按现有快照刷新"拆开 |
