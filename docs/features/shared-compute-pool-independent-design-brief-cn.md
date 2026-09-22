# 共享算力池普通用户保障需求与独立设计评审说明

日期：2026-09-21

请基于以下业务需求和当前项目实现，独立评估最小可行方案。可以否定已有设计、修正估算或提出替代方案；不需要证明现有三种方案合理。优先回答：**用什么衡量资源，怎样保障普通组，怎样避免增加请求延迟，以及怎样尽量少改现有调度逻辑。**

本文可单独交给 Claude 或其他评审者阅读。没有仓库访问权限时，请区分已提供的实现线索与尚未核实的事实，不要声称已完成源码审查。

## 一 本轮任务与范围

| 项目 | 要求 |
| --- | --- |
| 本轮目标 | 需求澄清、源码只读核对、独立设计和取舍评估；不实施业务功能 |
| 优先顺序 | 普通组获得约定保障；避免过度限制高用量组；优先低侵入、可回退、可验证的实现 |
| 已有反馈 | 原方案得到管理侧正面反馈，但技术方案尚未锁定，也没有完成生产验收 |
| 工作方式 | 主要由 AI 辅助编码，人工确认口径、审查和验收；不能将 AI 编码速度等同于验证速度 |
| 当前状态 | 已完成需求讨论与交互汇报文件；本文描述的整组动态保护尚未实现。2026-09-21 已对第四节、第八节所列入口做过一轮只读源码核对，结论标注为“已证实”；仍标“待核实”的项需评审者自行确认 |
| 操作边界 | 不改业务源码、生产配置或真实账号绑定，不启动服务、不执行真实上游消耗，不提交、推送或部署。保持其他任务的工作区修改 |
| 输出方式 | 可直接回复完整评审；如需落盘，仅新增独立评审报告，不覆盖原汇报或现有设计 |

有仓库权限时，先遵循 `AGENTS.md`，阅读 `llm-wiki/wiki/README.md`，再读相关 backend、data-and-domain、security-and-reliability、ops 页。源码和现有测试优先于文档。

编写本文时仓库为 `E:\allsite\xyai`，工作分支 `feature/hy/10207_department_usage`，HEAD 短标识 `927c164d9`。这是定位快照，不是要求切换分支或恢复旧状态；开始评审时应复核当前工作区，存在其他任务的修改时不得覆盖。

## 二 业务背景与目标

多个上游账号共同提供 AI 服务，普通用户和高用量用户通过不同 API 分组使用同一批上游资源。高用量组可能在短时间内大量消耗，导致普通组在后续需要使用时没有可用额度。

希望做到：资源充足时，高用量组正常使用；接近普通组保障范围时，只约束高用量组；资源恢复后可重新放行。需要同时识别两种问题：

| 问题 | 表现 | 不能混淆的边界 |
| --- | --- | --- |
| 累计额度不足 | 周额度被提前消耗，后续请求无法执行 | 降低并发只能减缓消耗，不会增加总额度 |
| 瞬时并发不足 | 周额度还有，但共享账号槽位已满，普通请求排队或失败 | 保留周额度不等于预留并发；请求数也不等于 Token 或费用 |

首先要缓解高用量组耗尽额度的影响，并评估普通组是否还需要独立的并发保障。**保障的具体服务水平、允许等待多久、是否必须硬预留并发，尚待确定。** 不要在未确认时把所有问题扩展成一个完整资源调度平台。

## 三 已确认的业务约束

| 事项 | 已确认内容 |
| --- | --- |
| 分组角色 | 可以在现有分组上标记普通组、高用量组，或由一份保护策略保存对应分组 ID；不要求建设多级分组体系 |
| 动态公式 | 第二种候选方案采用 `普通组最近 7 天累计用量 × 1.05`。5% 是普通组七天用量的 5%，不是整个池容量的 5% |
| 统计范围 | 应是同一实际共享资源范围内普通组的消耗，不能混入不相关平台或模型资源 |
| 账号窗口 | 用户计划把含 5 小时限额的账号移出目标号池；方案二、三按周额度目标池设计，移出尚未被验证完成，是其上线前置条件。第六节的方案四沿用既有逐窗口（5h / 7d）账号过滤，不以移出为前置条件；移出仍是用户决定，但不再是该路线的阻塞项 |
| 周重置时刻 | 即便均为周账号，实际重置时刻、账号容量和消耗速度仍可能不同；不假定同一时刻恢复 |
| 性能 | 每个请求可以检查状态，但不能同步遍历全部上游账号或重新查询七天日志；优先后台计算、缓存准入 |
| 路由 | 尽量保留既有优先级、负载感知、粘性会话、模型能力、账号健康和故障切换逻辑 |
| 暂停对象 | 针对高用量组的新算力请求，不能为了保护普通组而全局停用两组共享的账号 |
| 在途请求 | 已发往上游的请求仍可能继续消耗，暂停不能撤销已经发生的用量 |
| 推广 | 可以先限定单平台、明确的资源范围试点；实际生产平台与资源规模仍需核实 |

最近 7 天按滚动 168 小时统计，是当前设计建议，请核对是否应采用这一边界。若认为上述公式本身不合适，请明确说明适用条件、反例和替代公式，不要静默改成“日均用量”“池总量的 5%”或任意预测模型。

## 四 已发现的项目能力

以下是当前工作区已核对的实现线索，评审者仍应自行验证调用范围和运行配置。标注“已证实”的行在 2026-09-21 按第八节入口逐函数核对过。

| 能力 | 当前行为 | 对方案的影响 |
| --- | --- | --- |
| 账号绑定分组 | 一个上游账号可绑定多个分组 | 能配置专用保底账号，但同一上游配额不会因复制凭证或绑定多个组而增加 |
| 分组日周月限额 | 是每个用户订阅的额度模板，按用户与分组累计 | 不是整个分组共用预算 |
| 分组 RPM | 按用户 × 分组计数 | 不能直接作为全组请求速度上限 |
| 分组容量摘要 | 汇总关联账号的并发、会话和 RPM | 用于观测；多个分组共享账号时，摘要不能直接相加 |
| 分组日用量汇总（已证实） | 日桶表 `usage_group_daily_rollups` 仅有 `(bucket_date, group_id, actual_cost)` 三个业务列，只按分组累加倍率后的 `actual_cost`；无 `account_id`、无倍率前金额 | 不能按“同一实际共享资源范围”切分。按资源范围统计必须回到 `usage_logs`（含 `account_id`、`group_id`、`total_cost`、`actual_cost`、`rate_multiplier`、`account_rate_multiplier`）按 `account_id IN 池` 汇总，物理口径宜用倍率前 `total_cost`；`ActualCost` 是 `TotalCost` 应用倍率后的值 |
| 上游额度快照（已证实） | `OpenAICodexUsageSnapshot` 仅含 `*_used_percent`、`*_reset_after_seconds`、`*_window_minutes`；写入账号 `extra` 的键为 `codex_7d_used_percent`、`codex_7d_reset_at`、`codex_usage_updated_at` 等。没有绝对额度字段 | 已确认只有百分比。任何“池级绝对余量”都需要新建每账号容量换算，当前不存在；不能直接与历史费用比较 |
| 快照来源（已证实） | 转发响应头经 `updateCodexUsageSnapshot` 更新；主动查询 `OpenAIQuotaService.QueryUsage` 只由管理端 `admin/account_handler.go` 的 `GetUsage / GetUsageBatch`（经 `AccountUsageService.getOpenAIUsage`，受 `openAIProbeCacheTTL` 节流）和自动用卡流程触发 | 没有为已暂停或无流量账号服务的后台周期刷新。任何依赖账号快照的方案都要新增有界刷新任务，不可只靠业务请求或管理员打开页面自愈 |
| 账号级额度自动暂停（已证实） | `openai_gateway_scheduling.go#shouldAutoPauseOpenAIAccountByQuota` 在候选过滤阶段按账号 `extra.auto_pause_5h_threshold / auto_pause_7d_threshold`（缺省取全局 `OpsOpenAIAccountQuotaAutoPauseSettings.DefaultThreshold5h/7d`）跳过越线账号；支持 `auto_pause_{5h,7d}_disabled` 逐窗口豁免；过滤原因 `quota_auto_pause_{5h,7d}` 进入诊断日志。当前对所有分组一视同仁 | 是“只约束高用量组”最合适的扩展缝：按分组角色注入不同阈值即可，不改评分、TopK、粘性。旧汇报写“不能直接用来只暂停高用量组”对现状成立，但应视为拟扩展点而非死路 |
| 快照过期与重置语义（已证实） | `resolveOpenAIQuotaUtilization` 在 `codex_usage_updated_at` 超过 `openAICodexAutoPauseStaleAfter`（2 小时）或 `openAIQuotaWindowReset` 判定 `codex_<window>_reset_at` 已过时返回“无信号”，账号不再被暂停（fail-open），靠下一次响应头自愈 | 与本文“过期保守暂停”“预计重置不能提前记为可用”的要求相反。沿用则有一批在途请求的有界泄漏；改为对高用量角色保守暂停则必须配后台刷新，否则永久卡住 |
| 分组作用域调度上下文（已证实） | `SelectAccount*` 入口在调用 `selectAccountForModelWithExclusions` 前用 `withOpenAIQuotaAutoPauseContext`、`withOpenAIGroupPrivacyRequirement(ctx, groupID)`、`WithOpenAIProfitControlSuppressed` 装门 | 按分组角色改变候选过滤有现成模式，不需要另建路由 |
| WebSocket 后续轮次（已证实） | `openai_gateway_handler.go` 的 `OpenAIWSIngressHooks.BeforeTurn` 只做利润门 `ProfitControlVetoLatest` 复核、定价冻结、重抢用户与账号槽位；`BeforeRequest` 做模型白名单与内容审核。都不重跑 `CheckBillingEligibility`，也不重跑账号额度过滤；注释明确“连接内不换号” | 只在最外层加检查不够；账号级候选过滤也不覆盖已建连 WS。两条路线都要在 `BeforeTurn` 补检，越线时关闭连接要求重连 |
| 公共准入调用点（已证实） | `CheckBillingEligibility` 在约 20 个 handler 调用点执行（messages、responses、chat、images、embeddings、count-tokens、live、web search、grok、gemini、WS 握手），兜底分组在 `gateway_handler.go` 对兜底 Key 单独再调一次 | 外层准入需逐入口或在公共位置接入，并单独处理兜底分组；账号级过滤对重试、failover、兜底自动生效 |
| 账号并发 | 现有服务获取和释放账号并发槽位；非正上限表示不限制 | 0 不能当作零容量求和；组级并发保障不是现成能力 |
| 负载评分 | `EffectiveLoadFactor` 可作为评分分母，实际抢槽使用账号 `Concurrency` | 配置权重、真实槽位和实际吞吐需要分别理解 |
| 余量加权 | OpenAI 的 `quota_headroom` 默认关闭，偏向周剩余额度比例更高的账号 | 是相对比例，不是绝对余量；也不是普通组保障策略 |
| 优先级与余量 | 候选打分中 Priority、Load、Queue、ErrorRate、TTFT、QuotaHeadroom 等加权相加，再做 TopK 与加权选择 | 调高余量权重可能改变优先级取舍，不保证严格优先级分层 |
| 粘性与连续会话 | 部分路径可在普通负载评分前选定账号 | 不能宣称每次请求都能自由改选余量最多的账号 |
| 配置缓存 | `setting_gateway_runtime.go#GetOpenAIQuotaAutoPauseSettings` 使用本机缓存与异步 singleflight 刷新 | 可参考性能设计，不能直接把现有过期放行语义复制成保底政策 |

## 五 独立评审最需要回答的问题

### 1 资源到底用什么衡量

请明确区分账号数量、当前健康账号数、周绝对剩余额度、剩余比例、配置并发、空闲槽位、实际吞吐和等待时间。

| 待判断问题 | 希望得到的结论 |
| --- | --- |
| 不同套餐账号都是 50% 剩余，是否可以直接相加 | 同单位容量如何取得；只能得到百分比时有哪些可信替代办法，误差是什么 |
| 普通组历史是费用，上游是额度百分比 | 能否可靠换算，所需字段是什么；不能换算时是否应该放弃“精确池总量”控制 |
| 用户或渠道倍率变化 | 怎样避免收费策略变化被误认为真实资源消耗变化 |
| 账号数量与并发不同 | 哪些进入额度判断，哪些进入瞬时服务能力评估；是否需要两套独立约束 |
| 只有部分账号支持目标模型 | 怎样避免其他模型的余量掩盖目标模型短缺 |
| 同一上游配额被多凭证、多分组复用 | 配额去重、重叠资源归属如何定义，避免重复承诺同一容量 |

### 2 如何尽量少改现有路由

请分别评估以下路线，不要把它们混成一个大改造：

1. 仅通过现有账号绑定、权限和并发配置建立隔离保障。
2. 新增独立保护服务，目标解析后读取缓存状态，放行后继续调用原调度器。
3. 调整已有余量评分权重，不修改代码，但接受其对原排序的影响。
4. 明确要求严格优先级分层、同层优先高余量账号时，必要的调度改动。
5. 共享账号上硬预留普通组并发时，新增原子计数、租约释放和排队公平性的范围。
6. 在既有账号级自动暂停 `shouldAutoPauseOpenAIAccountByQuota` 上按分组角色区分阈值：普通组沿用账号阈值，高用量组用更低的 7d（及 5h）阈值，越线账号只对高用量组不可见，重置后自动放回。评审者需回答：它是否满足“只约束高用量组、资源恢复自动放行”的目标；“普通组近 7 天用量 × 1.05”在这条路线中只能作为阈值的离线校准目标而不是运行时比较量，请说明该转换的适用条件与反例；为何账号级过滤已覆盖重试、failover 与兜底，却仍需在 WebSocket `BeforeTurn` 补检；以及它不能提供什么（池级绝对量保底、并发预留）。

指出每条路线涉及的真实入口，特别是 HTTP、WebSocket 后续轮次、粘性账号、重试、fallback 和选中账号复核。只在最外层增加中间件是否足够，应给出源码依据。已证实的覆盖非对称：外层准入检查覆盖 `CheckBillingEligibility` 所在的 HTTP 入口，但需逐入口接入并单独处理兜底分组，且不覆盖 WS 后续轮次；账号级候选过滤自动覆盖所有经 `selectAccountForModelWithExclusions` 的选号（含重试、failover、兜底），但同样不覆盖已建连 WS。

### 3 怎样控制性能与数据陈旧风险

- 历史统计、上游采集、状态计算和请求检查分别在哪里执行，复杂度和依赖是什么？
- 多实例使用本机快照、共享状态、原子计数各有什么边界？通知丢失、重启、缓存失效时怎么办？
- 消耗越快，允许的快照滞后越小。刷新频率与上游查询负担如何平衡？
- 如何核对在途消耗和已完成但快照未反映的消耗，避免重复扣减？无法精确估计时怎样表述保障能力？
- 预计周重置时间到达，但额度尚未确认恢复时，是否放行？暂停后靠什么继续刷新？已证实现状：`openAIQuotaWindowReset` 在 `codex_7d_reset_at` 到达即视为已重置并放行；快照超过 2 小时无更新也放行；主动刷新只由管理端 `GetUsage / GetUsageBatch` 与自动用卡流程触发，没有为已暂停账号服务的后台任务。请分别对方案二、三的独立状态和方案四的账号级过滤回答：沿用放行语义（有界泄漏，约一批在途请求）还是对高用量角色改保守暂停；改保守暂停时后台刷新的频率、并发上限与上游查询负担。
- 7d `used_percent` 在窗口内单调上升、到重置时刻一次归零，恢复是离散事件。请说明这对状态机复杂度的影响：账号级阈值是否还需要迟滞与多次观测；E 模型中“渐进回补 + 稳定观测”是否只在含在途修正 D、需要防抖时才有意义。
- 请提出可测的延迟与正确性验收方法，不使用“零延迟”“绝对不超额”等未经验证的承诺。

### 4 原公式能保障什么 不能保障什么

- 周账号错峰恢复时，始终保留完整七天需求是否过于保守？不改变既定公式的首期是否仍有价值？
- 新普通组、不足七天样本、新增大量用户、节假日、此前已被限制使用，会如何影响历史基线？
- 保底线高于当前有效容量时如何处置？不能为了放行而静默调低保底。
- 5% 是需求缓冲，并不自动等于统计延迟、长请求和并发突发的安全上界。
- 有周额度但所有共享槽位被高用量组占满时，普通组体验如何保障？需要增加哪些能力，哪些可以暂缓？

## 六 已讨论的候选方案

以下是候选设计，不是评审必须接受的结论。请先独立分析业务与代码，再评价这些方案；方案四是 2026-09-21 源码核对后新增的替代路线，同样允许否定；也可以提出更简单的第五种方案。

| 方案 | 描述 | 明显收益 | 已知代价或未决项 |
| --- | --- | --- | --- |
| 一 独立保底加自动保护 | 普通组绑定专用与共享账号，高用量组只绑定共享账号；共享部分再建设自动保护。也可先只实施账号隔离配置 | 保底资源不被高用量组直接使用，同时隔离一部分账号并发 | 资源可能闲置；必须配置足够的额度、并发与模型覆盖；完整版本仍需自动控制 |
| 二 全共享动态触停 | 现有分组标记角色；后台计算普通组七天用量 × 1.05；有效余量触线时暂停高用量组新请求，满足恢复条件后放开 | 软件改动集中，沿用现有路由，资源共享程度高 | 同单位计量未确认；历史可能低估；不保证普通组空闲并发 |
| 三 动态保底加分级限速 | 在二上增加人工最低保底；临近保底线逐级减少高用量组可接纳请求，最后整组暂停 | 临界消耗更平缓，异常与恢复状态更明确 | 新增组级限速计数和状态测试；限请求数不等于限资源，也不等于硬预留并发；与二共用“同单位换算”门槛 |
| 四 分组感知的账号额度阈值（源码核对后新增） | 为分组打普通 / 高用量角色标记；在既有 `shouldAutoPauseOpenAIAccountByQuota` 候选过滤中按角色注入不同的 5h / 7d 阈值（示例高用量组 7d 80%）；补 WS `BeforeTurn` 复核与后台有界刷新；为“高用量组无可用账号”给独立原因码、429 与 Retry-After | 不需要费用到额度的换算；上游百分比是事实来源，自动吸收账号大小不等、重置错峰、池外消耗；复用陈旧与重置处理；候选过滤自然覆盖重试、failover、兜底；不改评分、TopK、粘性 | 保留量是每账号百分比，不是池级绝对量；不预留并发；普通组粘性集中在少数账号时保留可能不够；过期 / 预计重置沿用放行还是改保守暂停需决策；分级只能表现为逐级下调阈值 |

此前 AI 辅助工作量粗估：方案一完整版本 4–7 人日，配置子集 1–2 人日；方案二 2–4 人日；方案三 3–5 人日。这些数字建立在单平台、可互换资源、数据已能可靠换算、采集可复用的前提下；其中“数据已能可靠换算”经源码核对当前不成立，方案二、三还需一个容量校准工作包（每账号 `total_cost` 增量 / `used_percent` 增量关系、套餐分档、漂移复核）和至少一个周重置周期的采样。方案四的条件性估算为 1.5–3 人日（分组角色 / 阈值配置、候选过滤注入、`BeforeTurn` 复核、后台有界刷新、独立原因码与测试），阈值校准观察另计一个周重置周期。**请对四条路线分别重新估算，不要为了与这些数字一致而压缩必要工作。**

把新增数据采集、容量校准、跨池预算协调、严格额度预占、严格优先级排序、硬并发预留分别作为可能扩大的范围列出。区分 AI 生成代码时间、人工审查与测试的人日、等待真实周恢复的日历时间。

## 七 需要覆盖的反例与验收场景

| 场景 | 评审应验证什么 |
| --- | --- |
| 10 个小容量账号与 2 个大容量账号 | 不能按个数误判谁的池容量更大 |
| 低并发高余量与高并发低余量账号并存 | 额度和瞬时服务能力独立；高余量排序不应导致热点拥堵 |
| 周额度尚足但共享槽位全部占满 | 当前方案能否服务普通组，或必须增加独立并发保障 |
| 账号甲、乙在不同时间重置 | 只计实际确认的可用恢复量；重复读取同一快照不算多次观测 |
| 快速并发与长输出同时发生 | 暂停后的追加消耗是否可估计、可接受，是否需要原子预占 |
| 高用量组已暂停且没有新响应头 | 后台仍能发现恢复，不永久卡住 |
| 只有某个模型的账号耗尽 | 其他模型余量不能掩盖该模型的不可用状态 |
| 普通组历史曾因资源不足而受限 | 不把已发生用量直接当作全部真实需求 |
| 新组或历史日志不足七天 | 不把缺失数据默认为零需求；明确启动规则 |
| 同一额度账号在池外被使用 | 上游真实余量减少能够被发现，不能只看本站账单 |
| 更换 Key、分组授权、有效订阅或兜底目标 | 高用量用户不能切换身份或路由绕过保护 |
| Redis 故障、节点重启或版本通知丢失 | 状态有界陈旧；请求不触发全池查询风暴；人工停用不自动解除 |
| 重试、断流和失败后切号 | 限速计数、预估修正与实际结算没有重复累计或漏释放 |
| 调高 `quota_headroom` 权重 | 验证对优先级、会话粘性、成本、错误率和热点的实际影响 |
| 高用量组被全部账号过滤 | 现有候选为空时返回“no available OpenAI accounts”，诊断含 `quota_auto_pause_7d` 等原因但不区分角色过滤与真实耗尽。应给独立原因码、429 与 Retry-After（可取最近 `codex_7d_reset_at`），不被记为上游故障，不触发无意义切号 |
| 普通组会话粘性集中在少数账号 | 按账号百分比保留时，普通组常用账号先被自己耗尽，其它账号保留的份额用不上。评审应说明如何观察普通组在目标模型上的账号分布，以及提高阈值、放宽粘性或补少量专用账号的触发条件 |
| 快照过期或预计重置时刻到达 | 现有语义为放行。方案沿用时泄漏上界应可测（约一批在途请求）；改保守暂停时无业务请求也必须能恢复，且不引发对上游的查询风暴 |
| WebSocket 长连接跨越阈值 | 建连时账号未越线，后续轮次越线。`BeforeTurn` 复核应关闭连接要求重连，不能让连接绑定的账号继续为高用量组服务 |

## 八 建议源码阅读入口

路径均相对仓库根目录；按函数和调用链核对，不要求全仓库扫描。

| 关注点 | 路径及定位符 |
| --- | --- |
| 分组与账号关系 | `backend/ent/schema/group.go`、`backend/ent/schema/account_group.go`；`backend/internal/repository/account_repo.go#queryAccountsByGroup` |
| 分组服务字段 | `backend/internal/service/group.go`、`backend/internal/handler/admin/group_handler.go` |
| 七天统计能否复用 | `backend/internal/repository/custom_group_usage_rollup_repo.go`（日桶仅 `bucket_date, group_id, actual_cost`）、`backend/migrations/222_group_usage_daily_rollups.sql`、`223_group_usage_rollup_timezone.sql`、`backend/internal/repository/usage_log_repo_trend.go`、`backend/internal/service/usage_log.go`；按资源范围统计所需字段见 `backend/ent/schema/usage_log.go`（`account_id`、`group_id`、`total_cost`、`actual_cost`、`rate_multiplier`、`account_rate_multiplier`） |
| 用量与收费倍率 | `backend/internal/service/gateway_usage_billing.go`，关注 `ActualCost`、`TotalCost` 和账号倍率的不同用途；`backend/internal/service/billing_service.go#CostBreakdown`、`applyCostBreakdownMultiplier` |
| 上游周额度 | `backend/internal/service/openai_quota_service.go#OpenAIRateLimitWindow`、`QueryUsage`；快照结构 `backend/internal/service/openai_gateway_service.go#OpenAICodexUsageSnapshot`、`Normalize`（仅百分比、窗口分钟、重置秒数） |
| 响应头额度更新 | `backend/internal/service/openai_gateway_usage.go#updateCodexUsageSnapshot`（写 `codex_7d_used_percent`、`codex_7d_reset_at`、`codex_usage_updated_at`） |
| 主动探测触发点 | `backend/internal/service/account_usage_service.go#getOpenAIUsage`、`shouldProbeOpenAICodexSnapshot`、`probeOpenAICodexSnapshot`；调用方 `backend/internal/handler/admin/account_handler.go`（`GetUsage / GetUsageBatch`）；自动用卡 `backend/internal/service/openai_quota_auto_reset.go` |
| 账号级额度自动暂停与过期 / 重置语义 | `backend/internal/service/openai_gateway_scheduling.go#shouldAutoPauseOpenAIAccountByQuota`（约 521 行）、`resolveOpenAIQuotaAutoPauseThresholds`、`resolveOpenAIQuotaUtilization`、`openAICodexSnapshotStaleForPause`、`openAIQuotaWindowReset`、`openAICodexWindowResetAt`；候选过滤调用处约 405 行；`openai_gateway_service.go#openAICodexAutoPauseStaleAfter`（2h）；阈值模型 `ops_settings_models.go#OpsOpenAIAccountQuotaAutoPauseSettings` |
| 分组作用域调度上下文先例 | `backend/internal/service/openai_gateway_scheduling.go#withOpenAIQuotaAutoPauseContext`、`withOpenAIGroupPrivacyRequirement`、`SelectAccountWithLoadAwareness`（约 1108 行）、`SelectAccountForModelWithExclusions`（约 257 行） |
| 公共请求资格检查 | `backend/internal/service/billing_cache_service.go#CheckBillingEligibility`、`checkRPM`；约 20 个 handler 调用点，含 `backend/internal/handler/openai_gateway_handler.go`（HTTP 约 599、1235 行，WS 握手约 2523 行）、兜底分组 `backend/internal/handler/gateway_handler.go`（约 1006 行） |
| 用户与分组 RPM 键 | `backend/internal/repository/user_rpm_cache.go` |
| 路由与额度评分 | `backend/internal/service/openai_account_scheduler.go`，重点 `buildOpenAIAccountLoadPlan`、`openAIQuotaHeadroomFactor`、`buildOpenAISelectionOrder` |
| 粘性与账号复核 | `backend/internal/service/openai_gateway_scheduling.go`、`backend/internal/service/scheduler_snapshot_service.go` |
| 账号并发 | `backend/internal/service/concurrency_service.go#AcquireAccountSlot`、`GetAccountsLoadBatch`；`backend/internal/repository/concurrency_cache.go` |
| 负载分母与配置上限 | `backend/internal/service/account.go#EffectiveLoadFactor`、`Concurrency` |
| 容量观测汇总 | `backend/internal/service/group_capacity_service.go` |
| 缓存模式参考 | `backend/internal/service/setting_gateway_runtime.go#GetOpenAIQuotaAutoPauseSettings` |
| WebSocket 后续轮次 | `backend/internal/handler/openai_gateway_handler.go` 中 `OpenAIWSIngressHooks` 的 `BeforeRequest`（约 2794 行）与 `BeforeTurn`（约 2851 行，含 `ProfitControlVetoLatest` 复核与槽位重抢）；钩子定义 `backend/internal/service/openai_ws_forwarder.go`（约 264 行）；调用处 `backend/internal/service/openai_ws_forwarder_ingress.go`（约 592、1462–1471 行）、`openai_ws_v2_passthrough_adapter.go`（约 1057 行） |
| 调度配置 | `backend/internal/config/config.go#GatewayOpenAIWSSchedulerScoreWeights`，包括 `priority`、`load`、`quota_headroom` |
| 前端配置入口 | `frontend/src/views/admin/GroupsView.vue`、`frontend/src/api/admin/groups.ts` |

现有回归线索包括 `gateway_group_isolation_test.go`、`billing_cache_service_rpm_test.go`、`openai_account_scheduler_test.go`、`openai_account_scheduler_canonical_quota_test.go`、`openai_ws_turn_pricing_test.go`、`openai_ws_account_sticky_test.go`、`openai_quota_auto_reset_test.go`、`ops_settings_advanced_test.go`（自动暂停阈值）及并发服务测试。先读断言和构建标签，确需执行时选窄范围，不以整仓构建代替方案论证。行号为 2026-09-21 工作区快照，仅用于定位，以函数名为准。

## 九 当前缺少的生产信息

请指出哪些缺失信息是设计阻塞，哪些可以先采用明确假设继续；不要为了补信息访问真实凭据或生成上游请求。

| 信息 | 需要的最小内容 |
| --- | --- |
| 目标账号类型 | 平台、套餐、是否只剩周额度、周额度能否取得绝对量 |
| 规模与共享关系 | 账号数、真实配额主体数、普通与高用量分组绑定重叠情况 |
| 模型能力 | 是否同质、是否有只能由少量账号服务的模型 |
| 并发配置与实际负载 | 各账号上限、负载因子、峰值占用、排队和超时情况 |
| 用量特征 | 普通组与高用量组近七天消耗、峰值消耗速度、长请求占比 |
| 部署形态 | 实例数、共享缓存、后台任务与通知机制 |
| 服务目标 | 普通组容许等待、资源不足失败率、是否要求硬性并发保留 |
| 路由经营约束 | 优先级是否必须严格、是否允许改变粘性或上游成本偏好 |

## 十 希望收到的评审结果

请用中文，优先用清晰表格，并给出以下内容：

1. **先给结论**：最少改代码且稳妥的首期方案是什么；成立前提是什么。
2. **核对关键假设**：按已证实、尚待验证、不能成立分类；有源码权限时附路径和函数。
3. **定义资源指标**：明确单位、范围、时间窗和去重方式；说明额度与并发是否分别控制。
4. **给出调用流程**：后台计算、缓存发布、请求准入与原路由的关系；说明哪里只是配置，哪里会改主干。
5. **公平比较方案**：包含现有四种候选及有价值的替代方案；分别评价保障、效率、侵入程度、性能、运维和回退。先列前置门槛（哪些方案在当前源码状态下不可实施、缺什么），再评分；若评分，公开权重和主观性，门槛不得折算成扣分。
6. **提出最小实施范围**：模块及调用点、必要的数据变更、原有能力的复用边界，以及明确不在首期做的内容。
7. **给出独立工作量**：按 AI 辅助方式拆分人日、验证时间、估算条件和扩大范围的触发点。
8. **列出验证与决策项**：需要哪些反例测试、哪些生产数据，以及哪些问题必须由业务负责人确认。

独立分析后再参考以下旧材料。它们是待评估的讨论稿，评分、示例阈值和早期工期不是约束：

- `docs/features/shared-compute-pool-three-options-comparison-cn.html`（2026-09-21 修订：增加前置门槛表、方案四、准入覆盖矩阵，演示改为按账号重置事件恢复）
- `docs/features/shared-compute-pool-protection-report-cn.md`
- `docs/features/shared-compute-pool-protection-report-cn.html`
