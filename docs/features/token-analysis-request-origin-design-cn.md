# Token Analysis 请求来源识别

更新：2026-10-28。本文汇总最终合同，历史过程见证据入口。**v2 已在 `feature/hy/10222_token_origin_v2` 实现并通过独立复审，尚未部署或历史重索引。** 基线为 `02cf7a07c` 加未提交变更。

## 目标与存储合同

来源回答“本次模型调用为何发起”，不判断文字作者或账号是否为机器人，仅用于管理分析，不参与鉴权、计费或绩效判断。客户端 role/metadata 可伪造，人工标签不能证明本人点击发送。

| 展示 | 合同 | 含义 |
| --- | --- | --- |
| 人工 | human / user_input / medium | 新用户输入结构，未命中明确机器用途；普通脚本仍可能显示人工 |
| AI | ai / high | subagent、automation、tool_result、agent_message、assistant_continuation、compaction |
| AI | ai / compaction_resume / medium | v2 按恢复摘要结构推断的续执行 |
| — | unknown / none | no_signal、unsupported_shape、body_truncated、count_tokens；旧行缺来源也显示未识别 |

复用 `summary_json.request_origin={kind,reason,confidence,version}`，只保存固定枚举，不保存正文或会话标识。当前写 version 2；前端兼容 v1/v2，非法组合、未知版本及 v1 携带 compaction_resume 显示未识别。分类本身无新增 migration；人工计数依赖已有指纹迁移 251/252。

## v2 修订

### 分类规则与优先级

1. 索引器按原始路径覆盖：count_tokens 固定未识别，优先于截断；其余截断请求（含可解析 JSON/multipart）未识别；完整 `/responses/compact` 判压缩。
2. 检查客户端元数据白名单：子 Agent 标记优先，其次 request_kind=compaction，再检查自动 trigger 与专用 thread_source。x-codex-turn-metadata 支持字符串/对象，生产只验证过字符串。
3. 从协议尾部找有效事件，跳过任意类型的 system/developer（含 additional_tools）及完整纯注入 user 消息；未知尾项不能跳过后用历史用户消息补判。
4. 工具输出、Agent 消息、assistant/model 续执行和压缩条目判 AI；普通 user 文本/图片/文件判人工推断。混合工具结果与独立新用户内容以新输入为准。

| 元数据字段 | 有效白名单 |
| --- | --- |
| x-openai-subagent | collab_spawn、guardian、memory_consolidation → subagent |
| 嵌套 subagent_kind | thread_spawn、guardian → subagent；不能与上项混用 |
| 嵌套 request_kind | compaction → compaction |
| 嵌套 turn_trigger | automation_cron_scheduled、automation_heartbeat_scheduled、goal、memory_consolidation、guardian_review、guardian_classifier、ambient_suggestions、ambient_suggestion_safety、thread_title、thread_description → automation；app_tool_send_message → agent_message |
| 嵌套 thread_source | thread_title、thread_description、ambient_suggestions、ambient_suggestion_safety、guardian_review、guardian_classifier、memory_consolidation → automation |

composer/user/queue/edit_user_message/resume_interrupted_task trigger、user/automation/system source、history_initialization、User-Agent、工具声明/数量、历史正文、重复数、短间隔及 previous_response_id 均不单独证明来源。

| user 内容形态 | 当前处理 |
| --- | --- |
| 纯注入包装 | environment_context、in-app-browser-context、system-reminder、skill-context、system-update：完整闭合、无外部正文时跳过，可带属性/连续多段；未闭合、正文引用或包装加新问题保留用户内容 |
| 固定压缩模板 | 仅正规化 CRLF 与首尾空白后完整匹配；Codex 用常量，Copilot 两种/hertz 一种用全文长度和 SHA-256，不按关键词猜测 |
| Codex 恢复摘要 | 有效末项以完整 tokenAnalysisCodexResumePrefix 开头，且无独立新文本块、媒体或后续有效 user 输入 → compaction_resume/medium。无状态，不关联前驱，不保存会话标识 |
| 子 Agent / goal 通知 | 完整 subagent_notification，内层 JSON 含非空 agent_path/status 对象 → agent_message；完整 codex_internal_context source="goal" → automation。包装外有新正文则保留人工 |
| WorkBuddy 后台通知 | task-notification 含 task-id/status 标记，后面只能是带同一 task id 的固定 NOT USER INPUT 尾注 → tool_result |
| WorkBuddy 工具图片 | `Attached image(s) from tool result:` 加图片、无新正文，前一有效项是工具结果 → tool_result。纯注入消息可跳过，真实正文、机器通知及未知结构阻断 |
| hertz 纯图片 | 仅“tool 后 user 图片”不足以证明工具来源，暂不覆盖，等待客户端显式标记 |

### 人工计数口径（已定）

- 标签、Token、费用和 request_count 仍按请求；新增 human_input_count 为**估算**。失败/中断也可能有账单，不因失败忽略费用。
- 仅归并当前筛选集合内的 human 行：双方 fp_state=ok、API Key/hash 非空、同 Key/指纹版本/prefix_end_hash，按 `(event_time, archive_id)` 排序，前后间隔 ≤15 分钟。连续链只计链首（0/14/28 分钟计 1），NULL Key/无有效指纹逐条计。
- 前驱必须满足相同的时间、用户/邮箱/Key/账号/分组/模型/endpoint、项目、风险、来源与 include_unmatched 筛选；未指定账号/分组不额外限制相等。跨时间边界的链在相邻范围各计一次，已接受。
- 概览与请求明细强制含未关联，排行使用自身最终筛选。失败 A 后成功 B：概览含 A/B 计 1，默认排行只有 B 也计 1。邮箱条件覆盖概览主查询、风险原因、人工次数及同期计费分母。
- 三个统计接口共用 tokenAnalysisHumanInputSQL，按 Key/版本/fp_state/hash 分区取 LAG(event_time)，等价于存在 15 分钟内前驱；计数归入链首所在桶。项目按原始 client_project 关联，不能用展示名称 unattributed 作键。
- prefix_end_hash 包含协议族、保留的顶层字段与全部条目，排除生成参数、client_metadata、prompt_cache_key 等，不是严格请求身份。不同会话可能合并；不能按末条文字去重（历史不同的 27 次“A”应分开），不做跨压缩/子线程的整轮归集。

窗口函数替代最初 EXISTS 方案，无新增 migration。实施方记录：PG18、100.5 万行/14 天合成数据（含 5,000 行长重复桶），概览约 3,761 ms → 620 ms，计数一致；人工部分索引约 455 ms，暂未采用。独立复审未重跑该基准，生产性能仍待验收。

## 页面与预览

请求表显示“本次调用来源”，抽屉显示 reason/confidence/version；直接使用列表 summary_json，前端不猜测或逐行取分类。概览、用户/项目排行展示人工输入估算及说明，前后端须同批发布。

origin=human/ai/unknown 作用于概览、排行和明细，非法值返回 400；unknown 含旧行缺来源。来源/风险/项目等归档筛选不作用于计费趋势或同期计费请求数，因此筛选后覆盖率仅供参考。

预览/净输入跳过工具、assistant/Agent 文本及开头完整注入包装，剥离后移除紧随的 `## My request:`；无新正文的工具轮次沿用前面的输入。机器通知、压缩指令和恢复摘要仍可能进入预览，这是现存边界，不能用预览或 duplicate_count 反推来源。净输入变化清空旧 quality_*，同哈希保留；只有完整解析确认无正文才删除旧净输入，采集关闭/截断/降级不删。

人工首发曾因计费关联失败被默认明细隐藏，属于关联问题而非来源误判；现明细始终含未关联，以“—”表示 Token/费用未知。另见 [计费关联设计](token-analysis-usage-link-design-cn.md)。

## 审计方补证与实施边界

| 证据 | 结论与限制 | 材料 |
| --- | --- | --- |
| 初次采样 | 272 条，Messages 仅 5；有界窗口不是随机样本，不推算准确率或客户端份额 | [采样包](../delivery/2026-10-24-token-origin-samples/README.md) |
| 10-26 09:00 至 10-27 09:00 生产审计 | 60,065 请求；3,591 条人工中检出 478（压缩 232、包装 91、元数据 87、无标记辅助调用 66、工具图片 2）。13.31% 为保守检出比例，不是真实误判率；AI 抽查 470 未见可确认反例，不代表全量为零 | [审计报告](../reviews/2026-10-27-token-analysis-production-origin-audit-cn.md) |
| 早期 v2 补证 | 186 条摘要候选仅 2 条原文可关联；hertz 22 条纯图缺原文；可读 64 条 Messages 未见 tool_result+reminder 同条，不能外推全部。后者关闭补证，保留防御规则 | [结构证据](../delivery/2026-10-27-token-origin-v2-evidence/README.md) |
| 10-27 压缩快照 | 截至 18:17:24，固定前 110,200,243,008 bytes、49,781 请求，非完整日。286 次显式压缩（233 mid_turn、53 pre_turn、manual 0）；268 条模板中 267 有元数据，另 19 次压缩不用模板 | [压缩补证](../delivery/2026-10-27-compaction-followup/README.md) |
| 恢复续接 | 213 条摘要尾项中 212 可关联 mid_turn/auto（205 直接后继），同 turn/root_turn、前驱 200 且先完成；1 条缺身份。pre_turn 后摘要尾项 0/53，摘要后独立 user 正文 19；32 个后继本身再次压缩，归“其他” | 完整四项统计及逐条定位见压缩补证 |

**已定取舍（2026-10-27）：** 审计建议可靠前驱或显式恢复标记；用户决定采用无状态恢复前缀规则，以 medium 表示推断。证据仅覆盖单日 Codex 0.153–0.164、manual 无样本、同块追加新问题无可靠边界；风险已接受，需新窗口复验，不得把强相关写成来源证明。

已知限制：无标记辅助任务/脚本可能显示人工；排队新消息若被包在纯 system-reminder 内会被跳过。旧 291 条重复候选（≤15 分钟 255 条）不是真实重试真值；186 条摘要候选中已有 2 条计入 478，追加候选不可未经逐条核对直接累加。

## 历史回填与待验收

新规则只对之后索引的请求生效；自动索引默认每 300 秒增量处理昨天/今天，“索引当前范围”不会重算已读行。

| 待办 | 要求 |
| --- | --- |
| 原文保留 | v2 验收前保留 10-27 或之后至少一个完整日。10-27 16:35 预检时生产为 25bedc620、无指纹列，归档目录无 10-26 原文/压缩副本；这是历史快照，执行前重新核验 |
| 发布与受控重索引 | 另行授权、备份、明确范围；确认索引互斥且自动索引不会抢跑，同时重置目标文件 offset 与累加计数，一次重算来源/净输入/指纹，再关联计费及重算风险。不可只清 offset、不改原始归档；无原文不能凭预览补判 |
| 新窗口验收 | 按完整指纹逐对说明归并原因，不预设 255 候选全命中；覆盖同时间戳、15 分钟边界、长链、不同 Key/版本、NULL Key、失败首发、筛选内仅有后继及 v1/v2 混合；按端点抽查来源和未知结构 |
| 性能与页面 | 在真实规模数据检查 EXPLAIN (ANALYZE, BUFFERS)，完成浏览器验收；合成测试不替代生产结果 |
| hertz | 客户端团队说明纯图生成方式并提供显式来源字段 |

## 实现与审核入口

分类/预览/索引：`backend/internal/service/token_analysis_{origin,summary,indexer}.go`；计数：`backend/internal/repository/token_analysis_repo.go`；前端：`frontend/src/components/admin/token-analysis/` 与 `frontend/src/api/admin/tokenAnalysis.ts`。reason 合同由 tokenAnalysisOriginReasons 与 requestOrigin.ts 对照维护。

2026-10-28 复审关闭 F1 邮箱遗漏、F2 项目键碰撞、F3 工具图片前驱遗漏；未改动原失败用例，后端 140 项、PG18.3 集成 60 项、前端 50 项通过，无新阻塞性编码遗漏。见 [精简审核结论](../reviews/2026-10-28-token-origin-v2-implementation-review-cn.md) 与 [修复后证据](../delivery/2026-10-28-token-origin-v2-review/rereview/README.md)。不代表已部署或已回填历史。
