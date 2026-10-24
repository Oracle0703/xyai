# 给 Claude：人工请求被用量关联过滤隐藏的缺陷与修复交接

日期：2026-10-24。生产代码：main `98151012b1ce107fbbbbc67d93aafd08aa40bbf6`。两名 subagent 独立核对分类规则与页面/计费调用链；主代理使用应用内浏览器和 SSH 只读取证。未执行 SQL、修改生产数据、触发索引或重启服务；本轮未修改业务代码。

## 结论

**这是需要修复的功能缺陷。** 用户已经明确：这次真实发生且已计费的请求，勾选或不勾选“包含未匹配归档”都应该展示。让用户手动勾选是定位手段，不是修复方案；仅把默认值改为 true 也不能满足这个验收条件。

用户本人提交的“我之前请求的来源是不是没有办法归类了,只有上线后的才能归类?”已经入库，并正确分类为 **人工 / user_input / medium / v1**；但是计费用量关联失败，列表随后排除了该行。客户端附带上下文还占满了预览，进一步影响定位。

完整缺陷链：**请求 14:11:12 开始 → 14:11:31 产生账单 → 关联器只查开始时间 ±10 秒 → 没关联到账单 → `usage_log_id` 为空 → 默认列表排除 → 正确的人工记录不可见。** 来源字段正确不等于页面功能正确。

原始证据和可重放脚本集中在 `docs/delivery/2026-10-24-token-origin-human-input/`，请与本说明一起复核。

## 用户验收口径与修复范围

| 必须达到的结果 | 不构成完成的做法 |
| --- | --- |
| 本例请求在勾选、取消勾选时都可见，仍标为人工 | 让用户自己勾选；仅修改默认勾选值；把“分类字段正确”当作整个问题已解决 |
| 请求与实际计费记录正确关联；来源展示不因账单暂未落库而误导 | 将未关联记为真实 0 Token / 0 费用；为显示一条记录强行改来源标签 |
| 预览能看到这句真实问题，详情保留必要的上下文边界 | 用前 300 字客户端注入说明替代用户问题；按预览文字判断请求来源 |
| 新请求与存量未关联记录都有明确处理方案 | 仅修新请求，未说明同一 archive_id 旧摘要如何恢复 |

若调整“包含未匹配归档”的产品语义或移除该控件，应同步 UI、API 合同和测试；本交接不预设扩大共享仓储过滤器对其他聚合接口的影响范围。

## 原始归档与页面证据

| 项目 | 事实 |
| --- | --- |
| archive_id | `73eb6d29496d6c677054f5c797996d66` |
| 请求开始 | 2026-10-24 14:11:12.654557512（北京时间） |
| 文件 | `/opt/sub2api/backend/data/request-archive/2026-10-24.jsonl` |
| 请求行偏移 / 长度 | 18155124865 / 942377 bytes |
| 请求行 SHA-256（含行尾） | `a2e65ddfab9a846d1efc7c1ef6fc4f338071823aad2554d9aa938ffedcee74e1` |
| 协议 / 模型 | `/v1/responses` / `gpt-6-astra`，未截断 |
| 元数据 | `turn_trigger=composer`，无 `x-openai-subagent` / `subagent_kind` |
| 请求尾部 | input 共 119 项；末项 index 118 是 role=user、type=message、content=input_text，包含上述问题；117 为历史 assistant |
| UI 分类 | 在原有用户筛选下勾选“包含未匹配归档”，第 2 页打开同一 archive_id；详情显示人工、置信度中、规则版本 1 |
| UI 关联状态 | 匹配置信度 0，Token/费用显示 0；原文抽屉包含用户的完整问题 |

首次读取只扫描当日文件末尾有界快照：快照大小 18507566175 bytes，从完整行偏移 17976392800 起读取 531173375 bytes、239 行，其中 request 120 行。随后按已知偏移定向读取同一请求的 response；不全扫当日或前日归档。只导出目标相关结构、元数据枚举和哈希，不导出完整请求、凭据或其他用户原文。

## 为什么未匹配

| 项目 | 事实 |
| --- | --- |
| response 行偏移 | 18166371053 |
| 请求完成 | 14:11:31.202047834，HTTP 200，归档 duration_ms=18547 |
| 原始 usage | input_tokens=156582，其中 cached_tokens=156288；output_tokens=547 |
| 管理端实际账单 | `/admin/usage` 筛选同一用户，14:11:31 的 gpt-6-astra/cx 行：非缓存输入 294、输出 547、缓存约 156.3K、总耗时 18.41 秒、费用 $0.186578，与归档用量吻合 |
| 匹配窗口 | 生产 `/var/lib/sub2api/config.yaml` 没有显式覆盖 `usage_match_window_seconds`，相关环境变量无覆盖，采用代码默认 10 秒 |
| 该请求查找区间 | 14:11:02.654—14:11:22.654，实际账单时间在窗口之外 |

调用链：`middleware/request_archive.go` 在 `c.Next()` 前按请求开始时间归档；`openai_gateway_handler.go` 在 Forward 完成后提交用量任务；`openai_gateway_usage.go` 的 `RecordUsage` 用 `time.Now()` 设置 CreatedAt。`repository/token_analysis_repo.go:FindNearestUsageLog` 却按请求开始时间前后 10 秒寻找相同用户/密钥/模型的最近账单。

账单行本轮通过已登录管理页面读取，未查询数据库。页面时间精度为秒，归档时间保留纳秒；response 归档时间与实际 CreatedAt 不要求完全相等。

## 精确代码位置（基线 98151012b）

| 链路 | 位置与行为 |
| --- | --- |
| 默认隐藏 | `frontend/src/views/admin/TokenAnalysisView.vue:304-313`：defaultFilters 设置 `include_unmatched:false`；311 行注释称请求明细默认展示全部请求，但实际仍有计费关联过滤 |
| 勾选提交 | 同文件 `:78`：checkbox 更新 include_unmatched 并 reloadAll；handler `backend/internal/handler/admin/token_analysis_handler.go:204` 解析该参数 |
| 实际排除行 | `backend/internal/repository/token_analysis_repo.go:907-908`：`!filters.IncludeUnmatched` 时添加 `s.usage_log_id IS NOT NULL`；不是前端偶发漏渲染 |
| 关联时间窗 | 同文件 `:119-123`：以 eventTime ± window 限制 usage_logs.created_at；`:160-161` 按时间距离最近取一条；`backend/internal/service/token_analysis_indexer.go:472-478` 默认 window=10 秒 |
| 开始/结束时间不同 | `backend/internal/server/middleware/request_archive.go:101,114,131-134`：request 取 startedAt，response 取 c.Next 完成后的 time.Now |
| 计费时间 | `backend/internal/handler/openai_gateway_handler.go:817-831,999`：Forward 完成后提交任务；`backend/internal/service/openai_gateway_usage.go:448`：CreatedAt=time.Now |
| 旧未关联行不自动重试 | `backend/internal/service/token_analysis_indexer.go:154-158` 从 LastOffset 续读；`:252-254` 跳过 response 行；`:304` 只在当前 request 行处理时尝试关联。之后到账的 usage 不会使已经跨过水位的同一 request 自动再关联 |
| 零值误导 | `backend/internal/repository/token_analysis_repo.go:514-515`：LEFT JOIN 未命中时 COALESCE 为 0；请求表/抽屉未把这些零值与真实零用量区分 |
| 分类与预览不同口径 | `backend/internal/service/token_analysis_origin.go:122` 从尾部判断本次调用；`token_analysis_summary.go:308-309,357` 保留最近有效用户输入，工具续执行仍可显示相同历史人工句子 |

关联修复应优先确认能贯通归档、转发结果和 usage 的稳定标识；不要假设现有不同模块的 request_id、archive_id 或 hash 天然属于同一命名空间。时间兜底必须兼顾并发、重试和异步落库；单纯扩大最近时间窗口会增加串单风险。

## 三个需要改善的点

1. **请求完整性与计费关联被混在一起。** `TokenAnalysisView.vue:defaultFilters` 默认 `include_unmatched=false`，仓储 `buildTokenAnalysisWhere` 排除 `usage_log_id IS NULL`，导致已正确识别来源的人工记录不可见。来源分析应提供完整归档视角，并明确标记“未关联用量”。
2. **计费关联时间口径不一致。** 应优先使用稳定的请求标识关联归档与账单，必要时结合响应完成信息做有约束的兜底，并支持对尚未关联的行后续补关联。仅依赖较宽的最近时间窗口会增加并发请求串单风险。
3. **预览与零值表达误导。** 本例输入先包含 `<in-app-browser-context source="ambient-ui-state">…</in-app-browser-context>`，实际问题位于 `## My request:` 后；当前 300 字预览被前置上下文占满。应剥离已知完整客户端上下文包装、保留真实用户正文；未关联的 Token/费用应表示未知，不能让用户误认为请求没有消耗。来源文案也应明确是“本次调用来源”，预览是“最近用户输入”。

本例排除了分类器 metadata 提前返回、历史工具覆盖新 user、前端错行这三个假设。没有依据把全部携带相同人工原文的后续 Agent 调用改为人工。

## 之前为什么判断不充分，以及哪些错误已有证据

### 本例的判断更正

| 之前的判断/解释 | 更正与证据 |
| --- | --- |
| 起初将“本人句子 + AI”作为可能的来源误判或工具续执行预览来调查 | 这些只是候选假设。精确 archive_id 的末项是新 user、metadata=composer、无子 Agent 标记，线上同 ID 也显示人工，已排除本条被分类器判 AI |
| 找到“人工”后，重点解释勾选即可看到 | 这个解释没有充分承认缺陷。用户正常使用时记录不可见本身就是问题；直接原因是关联失败和默认排除，不应让用户承担绕过缺陷的操作 |
| 部署后看到 AI 标记，结合既有专项测试通过，认为主链路可用 | 当时没有验证“真实人工请求耗时超过匹配窗 + 开关两种状态 + 实际账单”这个端到端场景。分类测试通过不能证明记录一定可见、用量一定关联正确 |
| 排查过程中尚不能确定“未关联”是否伴随“没有账单” | 后续只读 `/admin/usage` 找到相符的 14:11:31 账单；归档 response 也有非零 usage。这个待排除假设已被证据排除，此处 0 是缺关联后的 SQL 填充值 |

没有取得用户当时指认为 AI 的那条行的确切 archive_id，因此不声称已证明用户看到的 AI 行就是这条首发的后续调用。已确证的是：指定原话的人工首发存在、分类正确、默认被过滤。

### 此前来源/净输入实现中确实出现过的错误（与本例分开）

| 历史问题 | 原因、证据与当前状态 |
| --- | --- |
| 纯 Claude Code reminder 续执行被判人工 | 原规则把 reminder-only 的 user 当作新用户正文。生产 Messages 历史证实“assistant 后纯 reminder”和“reminder + 正文”两种形状；前者现跳过包装后判 AI，后者仍人工。证据见 `docs/delivery/2026-10-24-token-origin-samples/messages-history-audit.json`、`messages-audit.md`；已修复 |
| 曾以 tool_result + reminder 同条作为主要依据 | 生产样本没有命中该组合，只能列防御用例，不能写成生产已发生。曾把影响称为高优先级/主力客户端影响也缺乏统计依据，已在来源设计文档更正；不能用非随机的 272 条样本或含 response 的 1246 行推全天占比 |
| 老预览/净输入混入 assistant 文本和工具输出 | 旧 Responses 摘要取到了 assistant，Anthropic 递归取到了 tool_result；据此看页面会误认输入作者。提取已修复，但历史已索引行未全量回填，旧预览仍可能保留旧口径。证据见来源设计文档“审核修订”与旧实现审核 F1 |
| 新抽取为空后旧全文残留、换正文后评分仍保留 | 上轮 F1 已用真实 indexArchiveLine + 预存旧输入复现；F2 由 SQL 确认并补真实 PG 状态转换验证。两项已修复，历史重索引尚未执行。它们不是本例默认隐藏人工首发的根因 |

旧报告：`docs/reviews/2026-10-24-token-analysis-request-origin-implementation-review-cn.md`；来源设计：`docs/features/token-analysis-request-origin-design-cn.md`。这些已关闭问题和采样局限不能替代本例的可见性修复。

## 给 Claude 的修复验收清单

1. 用本证据的 14:11:12.654 开始 / 14:11:31 账单时间建立回归：用户首发保持 human，正确关联账单；两种 include_unmatched 状态下都能找到同 archive_id。
2. 补异步时序：首次索引时账单未落库，随后账单出现；在不全量扫 123 GiB 历史归档的情况下，旧未关联行有可验证的补关联路径。
3. 补同用户、同 API key、同模型的并发和重试，验证不串到相邻请求，不重复累计同一计费记录。时间窗口放大本身不算通过。
4. 对真正未关联/失败/取消请求明确来源与计费状态，未知用量不能表现成确认的免费请求；保留合法零消耗与未知的区别。
5. 本例 ambient wrapper 后的 `## My request:` 正文要直接出现在预览；整块注入、注入与真实正文混合、普通用户引用标签、包装未闭合均需覆盖，不能误删真实输入。
6. 来源判断仍按本次调用：本例末尾 user 为人工，随后工具返回继续执行为 AI；两者可以共享最近用户输入，不据重复数改标签。
7. 更新中英文说明和 wiki；交付明确区分本地修复、已部署、新行生效、旧行修复。当前生产只读权限不包含 SQL、回填或部署；这些操作另按用户明确授权执行。

## 验证与交付边界

- 原始请求结构、线上同 archive_id 的人工分类、实际计费行三方交叉核对完成。
- 浏览器保留目标人工请求的详情，包含未匹配归档筛选已开启，便于用户复核。
- 未修改或重新部署分类算法；上述问题留待单独修复与回归验收。
- 本次交接补读两条原始记录共 969403 bytes，请求行 SHA-256 与首次定位完全一致；证据 JSON 和最小样例均完成解析校验，`git diff --check` 通过。没有把文档/证据校验写成业务修复测试通过。

## 处理结果（2026-10-24，本地修复，未提交、未部署）

修复设计与存量恢复已回写 `docs/features/token-analysis-usage-link-design-cn.md`；来源设计文档补充“人工首发被计费关联隐藏”一节。

| 验收项 | 处理 | 证据 |
| --- | --- | --- |
| 1 账单关联 | 归档行记录 `client_request_id`，与 `usage_logs.request_id='client:<id>'` 精确关联；不能精确关联时的时序兜底按账单推算的转发开始时间（`created_at - duration_ms`）、完成时间与耗时约束，并排除已被他行占用的账单；不靠扩大窗口 | 集成测试按本例时序（开始 14:11:12.654、账单 14:11:31.202、耗时 18.41 秒）关联到正确账单，前一请求账单紧邻开始时间也不会被取走 |
| 2 开关两态可见 | 请求明细服务端强制完整视角并标注关联状态；勾选改为只影响排行请求数（“排行含未关联请求”），默认值未改 | 同一集成用例在 `include_unmatched=false/true` 下均返回该 archive_id，来源仍为 human，Token/费用为真实账单值 |
| 3 未关联状态 | 新增 `usage_link_status`（linked/unlinked/failed/not_billable）与关联方式；非 linked 行页面显示 `—` 与“未知，不代表零消耗” | 前端用例覆盖未关联/失败/不计费/旧接口行和抽屉 |
| 4 预览清理 | 剥离文本开头完整闭合的 `<in-app-browser-context …>` 等注入包装及其后 `## My request:` 标题；正文中引用、未闭合、正文之后的包装和无包装标题保留 | `classifier-fixture.json` 同构样例：human/user_input/medium/v1，预览即原问题；8 种边界用例 |
| 5 存量恢复 | 关联轮次每次索引对范围内未关联行重试、对旧窗口关联复核；历史日期可用“索引当前范围”DB-only 恢复，JSONL 仍在时可并入受控重索引补完成时间 | 设计文档“存量数据恢复”，含只读核对 SQL |
| 异步/并发/重试 | 未完成的有关联键请求只做精确关联，后续轮次重试；失败请求只认精确账单；WS 上游按捕获的 response.id 精确关联；时序兜底只在归属唯一时生效，存量“失败后重试/计费排队”等歧义保持未关联；认领按账单加锁 | PostgreSQL 18 集成用例与变异检查；随后独立复核的五类缺口与修复见 `2026-10-24-token-analysis-usage-link-implementation-review-cn.md` 末节 |
| 来源口径 | 分类仍按本次调用；后续工具续执行仍判 AI，可与首发共享“最近用户输入”预览，不按重复数改标签 | 既有分类用例保持原期望并全部通过（另增只含包装的 user 判 AI、同前缀不同标签判人工两例） |

区分：此前已修复的历史问题（纯 reminder 续执行、预览/净输入混入助手与工具文本、净输入残留与评分绑定）不在本轮改动范围；本轮新增的是计费关联、展示口径和 `<in-app-browser-context>` 包装处理。新代码对新索引行与关联轮次覆盖的日期生效，旧行需按设计文档恢复。

仍需单独授权的生产操作：部署（含 migration 247，只加列）；部署后历史日期的“索引当前范围”DB-only 恢复（更新关联与风险分）；JSONL 仍在的日期与 NUL/multipart 修复合并的受控重索引；部署后的只读核对 SQL。本轮未连接生产数据库、未执行 SQL、未重索引、未重启或部署。
