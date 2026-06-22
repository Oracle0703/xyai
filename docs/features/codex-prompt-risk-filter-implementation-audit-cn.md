# Codex Prompt 风险过滤器实现审核报告

> 审核日期: 2026-06-22  
> 被审实现: Claude 根据 `docs/features/codex-prompt-risk-filter-design-cn.md` 落地的当前工作区改动  
> 审核目标: 核验实现是否偏离设计最终口径, 是否存在逻辑未闭环、死代码/无效代码、测试缺口。

## 总体结论

| 维度 | 结论 |
| --- | --- |
| 主体方向 | 基本按设计新增了独立 `prompt_risk_config`、Prompt 风险评估器、专用抽取器、管理端配置页和 `prompt_risk_*` 日志 action。 |
| 最大阻断问题 | `prompt_risk_block` 仍未计入 pre-block blocked 指标, 与设计里“新 action 纳入 metric”的必改项直接冲突。 |
| 最大行为偏差 | Prompt 风险抽取器默认扫描所有 user turn, 不是设计要求的 newest-turn 默认; 多轮会话里历史高危输入会在后续普通请求里反复触发。 |
| 最大无效代码 | `FailOpen=false`、`AutoBanOnHigh`、`AdminEmailOnHigh`、`AdminEmailOnMedium` 都有配置/UI 字段, 但热路径没有实际行为。 |
| 是否建议直接合入 | 不建议。至少先修 P0/P1 闭环问题并补测试后再进入实现验收。 |

## 整改落地(2026-06-22 Claude 复核 + 修复)

核验后逐条采纳;判断为"删除死配置/延后"的两项附理由。全部改动已过 `go build`/`go vet`/后端单测与前端 `vue-tsc`。

| ID | 处置 | 落地位置 |
| --- | --- | --- |
| P0-1 | ✅ 已修 | `recordPreBlockSyncMetric` blocked case 加入 `prompt_risk_block`;新增 `TestRecordPreBlockSyncMetric_PromptRiskBlockCountsBlocked` |
| P0-2 | ✅ 已修 | 新增 `InputScope`(`newest` 默认 / `full` 显式 opt-in);newest 定位最新 user item 并纳入同轮相邻 item、跳过尾部工具输出、不回溯历史;新增历史高危不复触发等 3 个抽取单测 |
| P1-1 | ✅ 已删 | 删除死字段 `FailOpen`(后端结构体/默认值/前端/TS/i18n),文档明确**只支持 fail-open**——本功能默认 off/observe 且保护账号可用性,fail-closed 会因一处配置错误拦死整组流量 |
| P1-2 | ✅ 已删 | 删除死字段 `AutoBanOnHigh`/`AdminEmailOnHigh`/`AdminEmailOnMedium`(自动封禁合法队友本就违背目标),副作用列入 v1.1 |
| P1-3 | ✅ 已修 | `UpdatePromptRiskConfig` 先 `validateRaw`(拒非法枚举原值)再 `normalize` 再 `Validate`;新增 6 例非法枚举拒绝 + 空值放行单测 |
| P1-4 | ✅ 已修 | `pass` 桶加 `AND l.action <> 'prompt_risk_observe'`;新增 `TestBuildContentModerationLogWhere_PassExcludesObserve` |
| P1-5 | ⏭ v1.1 | 已共享词边界匹配器 + wrapper 剥离(老 bug 已修);**完整文本抽取器统一延后**——两功能收敛到 newest 后命中文本已基本一致,而 request_intercept 抽取器在中间件热路径,贸然重构回归风险大于收益 |
| P2-1 | ✅ 已改进 | `category_scores` 键由 `source:keyword` 改为 `level:source:keyword`,在现有 `map[string]float64` 列内保留等级/匹配模式(**无 migration**);完整结构化快照列入 v1.1 |
| P2-2 | ✅ 已补 | 补齐 P0-1 指标 / P0-2 scope / P1-3 validateRaw / P1-4 pass⊥observe 单测;前端 `vue-tsc` 通过。前端组件级单测仓库暂无 harness,沿用 typecheck |

## 关键问题清单

| ID | 严重级别 | 问题 | 影响 |
| --- | --- | --- | --- |
| P0-1 | 阻断 | `prompt_risk_block` 调用了指标记录, 但指标函数未把它算作 blocked | 管理端运行指标仍把 Prompt 风险拦截算成 allowed, 设计硬闭环失效 |
| P0-2 | 阻断 | Prompt 抽取默认扫描所有 user turn, 与设计“newest-turn 默认”不一致 | 历史高危/中危输入会污染后续请求, 造成重复 observe 或误拦 |
| P1-1 | 高 | `FailOpen=false` 配置字段没有任何热路径效果 | 管理员以为可 fail-closed, 实际 JSON 损坏/仓储错误仍一律放行 |
| P1-2 | 高 | `AutoBanOnHigh` / `AdminEmailOnHigh` / `AdminEmailOnMedium` 是可保存的无效开关 | UI 暴露“可配置能力”, 但后端明确不执行, 形成误导和死配置 |
| P1-3 | 高 | 配置保存先 normalize 再 Validate, 非法 mode/match/max_level 会被静默改默认 | “保存时校验拒绝”没有闭环, 管理端错误配置得不到反馈 |
| P1-4 | 高 | `prompt_risk_observe` 同时落入 `observe` 和 `pass` 结果桶 | 管理端“未命中/pass”统计被观察日志污染, 运营口径不清 |
| P1-5 | 高 | request_intercept 没有真正共享 prompt-risk 抽取层 | 只复用了 wrapper 剥离和 word 匹配, 两套抽取逻辑仍会分叉 |
| P2-1 | 中 | `category_scores` 只能存 `map[string]float64`, 不能完整保存 `Reasons` 结构 | 命中词可查, 但 level/source/score 的结构化审计信息不完整 |
| P2-2 | 中 | 测试覆盖没有覆盖指标、fail-open=false、pass/observe 互斥、前端面板 | 定向单测通过但关键运营闭环仍可能坏 |

## 详细问题与证据

### P0-1: `prompt_risk_block` 仍未计入 blocked 指标

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/content_moderation.go:1460-1462` | Prompt 风险高危分支调用 `s.recordPreBlockSyncMetric(0, ContentModerationActionPromptRiskBlock)`。 |
| `backend/internal/service/content_moderation.go:1082-1098` | `recordPreBlockSyncMetric` 的 blocked case 仍只包含 `block/hash_block/keyword_block`, 不包含 `prompt_risk_block`;默认分支会加到 `preBlockAllowed`。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:160`、`:194`、`:233` | 设计明确要求 `prompt_risk_block` “已纳入 blocked 计数”并列为验证项。 |

**影响**

真实拦截会返回 4xx, 但运行态指标 `PreBlockAllowed` 增加、`PreBlockBlocked` 不增加。管理端会误判 Prompt 风险没有拦截效果或 allowed 偏高。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 代码 | 在 `recordPreBlockSyncMetric` blocked case 加入 `ContentModerationActionPromptRiskBlock`。 |
| 测试 | 增加 service 单测直接断言调用该 action 后 `preBlockBlocked=1` 且 `preBlockAllowed=0`。 |

### P0-2: Prompt 抽取默认扫描所有 user turn, 不是 newest-turn 默认

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/prompt_risk_input.go:10-13` | 注释写明扫描本请求里“所有 user turn”。 |
| `backend/internal/service/prompt_risk_input.go:46-59` | Chat/Anthropic messages 遍历所有 `role=user`。 |
| `backend/internal/service/prompt_risk_input.go:62-75` | Responses input 数组遍历所有 item。 |
| `backend/internal/service/prompt_risk_input_test.go:43-56` | 测试显式要求 Chat Completions 收集所有 user turns。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:180`、`:193` | 设计要求“默认 newest-turn、可选 full-context”。 |

**影响**

多轮 Chat/Codex 请求通常会带历史消息。用户前一轮问过高危语句后, 下一轮只说“谢谢/继续解释”也会被历史 user turn 触发 observe/block。即使 `Mode=observe`, 日志也会重复污染;切到 block 后会误拦后续普通请求。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 代码 | 给 PromptRiskConfig 增加 `input_scope` 或内部固定 `newest_user` 默认;只取最新 user item, 跳过 assistant/tool/system。 |
| 可选 | full-context 作为显式配置项, 默认关闭。 |
| 测试 | 增加“历史高危 + 最新普通 user”不触发 block/observe 的测试;再增加 full-context 开启后才触发的测试。 |

### P1-1: `FailOpen=false` 没有实际语义

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/prompt_risk.go:72` | 配置结构定义了 `FailOpen bool`。 |
| `backend/internal/service/content_moderation.go:1498-1524` | `loadPromptRiskConfig` 发生 setting repo 错误、空配置、JSON 损坏时都返回默认配置 `Enabled=false`, 没有读取或执行已保存的 `fail_open=false`。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:84` | 设计要求 JSON 损坏时默认放行, 但 `fail_open=false` 时改为拦截。 |

**影响**

管理员在 UI 关闭 fail-open 后, 配置损坏或仓储异常仍会 fail-open。该字段实际是死配置, 同时也给安全运营造成错误预期。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 最小方案 | v1 删除 `FailOpen` 字段和 UI, 明确只支持 fail-open。 |
| 完整方案 | 保存一个可独立读取的 last-known-good 配置;加载失败且 last-known-good.FailOpen=false 时返回明确的 fail-closed 决策和 `prompt_risk_error` 日志。 |

### P1-2: 副作用开关是无效代码/无效 UI

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/prompt_risk.go:77-79` | 定义 `AutoBanOnHigh`、`AdminEmailOnHigh`、`AdminEmailOnMedium`。 |
| `backend/internal/service/content_moderation.go:1463-1465` | 高危分支注释说明这些字段只是预留, `enqueueRecord(... applySideEffects=false)`。 |
| `backend/internal/service/content_moderation.go:1491-1493` | 中危 observe 同样只落库, 没有管理员邮件路径。 |
| `frontend/src/views/admin/PromptRiskPanel.vue:75-92` | UI 暴露三个可切换开关并保存。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:250` | 文档又写“管理员邮件事件标准化”为后续项。 |

**影响**

用户可在管理端开启这些开关, 保存也会成功, 但运行时没有任何封禁或邮件效果。这属于典型无效配置, 后续排障时很难解释。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| v1 推荐 | 删除这三个字段和 UI, 或禁用显示为“后续功能, 不可编辑”。 |
| 若保留 | 后端必须实现对应行为, 并补管理员收件人、邮件模板、幂等、失败记录和测试。 |

### P1-3: 保存配置会静默纠正非法值, 不是拒绝非法配置

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/content_moderation.go:1553-1559` | `UpdatePromptRiskConfig` 先 `cfg.normalize()` 再 `cfg.Validate()`。 |
| `backend/internal/service/prompt_risk.go:236-255` | normalize 会把非法 mode/match_mode/max_level 改为默认 observe/contains/medium。 |
| `backend/internal/service/prompt_risk.go:267-279` | Validate 再检查时已经看不到原始非法值。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:194` | 设计要求 `UpdatePromptRiskConfig` 校验配置, 包括 regex 可编译。 |

**影响**

例如请求体传 `mode:"blok"` 或 `match_mode:"words"` 会被保存为 `observe` 或 `contains`, 管理员不会收到错误。配置错误被静默吞掉, 可能导致预期拦截没有生效。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 代码 | 分离 `validateRawPromptRiskConfig` 与 `normalizePromptRiskConfig`。先校验枚举原值, 再填默认。 |
| 测试 | 增加非法 mode、match_mode、max_level 保存失败测试。 |

### P1-4: `observe` 日志同时出现在 `pass` 桶

| 证据 | 说明 |
| --- | --- |
| `backend/internal/repository/content_moderation_repo.go:252-255` | `observe` 筛选是 `action = 'prompt_risk_observe'`; `pass` 仍是 `flagged = FALSE AND error = ''`。 |
| `backend/internal/service/content_moderation.go:1491-1493` | observe 日志写入 `flagged=false` 且默认 `error=''`。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:82`、`:233` | 设计目标是中风险结构化筛出, 避免落入 pass 造成不可运营。 |

**影响**

同一条 `prompt_risk_observe` 会同时在 observe 和 pass 结果中出现。管理端“未命中/pass”不再代表安全通过, 运营统计会被观察事件污染。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 代码 | 将 pass 条件改为 `flagged = FALSE AND error = '' AND action <> 'prompt_risk_observe'`。 |
| 测试 | 增加 pass where 条件不包含 observe 的仓储单测。 |

### P1-5: request_intercept 没有真正共享抽取层

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/request_intercept_rules.go:456-468` | word 匹配复用了 `promptRiskKeywordMatches`。 |
| `backend/internal/service/request_intercept_rules.go:409-418` | 只复用了 `stripPromptRiskWrappers`。 |
| `backend/internal/server/middleware/request_intercept.go:193-207`、`:240-268` | request_intercept 仍使用自己的 `extractRequestInterceptText` / `extractResponsesInputText`。 |
| `backend/internal/service/prompt_risk_input.go:14-44` | prompt-risk 是另一套 `extractPromptRiskInput`。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:64-70`、`:207-210` | 设计要求共享“用户文本抽取器 + 关键词匹配器”。 |

**影响**

两套功能仍可能对同一个请求得到不同文本。尤其 prompt-risk 当前扫所有 user turn, request_intercept 取最后 user/fallback;后续排障仍会出现“一个命中一个不命中”的分叉。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 代码 | 抽出公共 `PromptTextExtractor` 或 `gateway_text_extractor.go`, 参数化 scope(newest/full) 和协议, 两边共同调用。 |
| 测试 | 同一组 Codex Responses/Chat/Gemini fixture 同时断言 request_intercept 与 prompt-risk 抽取结果一致。 |

### P2-1: `Reasons` 没有完整结构化落库

| 证据 | 说明 |
| --- | --- |
| `backend/internal/service/content_moderation.go:1603-1633` | `buildPromptRiskLog` 将 reasons 转为 `map[source:keyword]score`。 |
| `backend/internal/service/content_moderation.go:389` | `ContentModerationLog.CategoryScores` 类型是 `map[string]float64`。 |
| `docs/features/codex-prompt-risk-filter-design-cn.md:181` | 设计写“命中词 Reasons 序列化进 category_scores(JSONB)”。 |

**影响**

当前能看到命中词和单项分数, 但丢失原始 `PromptRiskReason` 的结构边界, 例如 level/source/keyword 只能从 key 里拆, 不适合后续结构化查询或 UI 展示。

**整改建议**

| 动作 | 建议 |
| --- | --- |
| 最小方案 | 明确文档改为 `category_scores` 仅存扁平命中分数, UI 不承诺完整 reasons。 |
| 更优方案 | 增加 `metadata`/`reason_snapshot` JSONB, 或扩展日志模型支持 `map[string]any` 存完整 reasons。 |

### P2-2: 测试覆盖缺口

| 已验证 | 结果 |
| --- | --- |
| `go test ./internal/service -run PromptRisk` | 通过 |
| `go test ./internal/repository -run ContentModeration` | 通过 |
| `cmd.exe /c pnpm --dir frontend run typecheck` | 通过 |
| `go test ./...` | 未通过, Windows Go build cache 报 `Access is denied`, 未形成业务结论 |

| 缺失测试 | 建议补齐 |
| --- | --- |
| 指标闭环 | `prompt_risk_block` 增加 blocked 而不是 allowed。 |
| 抽取 scope | 默认 newest-turn;历史高危 + 最新普通不触发。 |
| fail-open=false | 配置损坏时按设计 fail-closed 或删除该字段。 |
| pass/observe 互斥 | `prompt_risk_observe` 不出现在 pass 桶。 |
| 前端面板 | PromptRiskPanel 加载、保存、测试器、无效开关展示策略。 |
| request_intercept 共享抽取 | 同 fixture 两边抽取结果一致。 |

## 正向确认

| 项 | 结论 |
| --- | --- |
| DB 分数溢出 | 已改为 `float64` 和 0~1 默认分数, 当前 `highest_score DECIMAL(8,6)` 不会因默认词表溢出。 |
| 独立配置 | `prompt_risk_config` 已独立于旧 `content_moderation_config` 加载, 插入点在 `loadConfig` 之前。 |
| 默认 observe-first | `DefaultPromptRiskConfig` 默认 `Enabled=false`、`Mode=observe`, 符合灰度策略。 |
| blocked 日志筛选 | 仓储 `blocked` 已加入 `prompt_risk_block`。 |
| 封禁计数污染 | `CountFlaggedByUserSince` 已排除 `prompt_risk_%`。 |
| 前端类型 | `pnpm --dir frontend run typecheck` 通过。 |

## 建议整改顺序

| 顺序 | 整改项 | 原因 |
| --- | --- | --- |
| 1 | 修 `recordPreBlockSyncMetric` 的 `prompt_risk_block` 指标归类并补测 | 当前直接违反设计必改项, 影响运营看板。 |
| 2 | 修 prompt-risk 抽取默认 newest-turn, full-context 显式配置 | 当前可能导致真实多轮误拦。 |
| 3 | 删除或实现 `FailOpen=false`、`AutoBanOnHigh`、`AdminEmail*` | 消除死配置/无效 UI。 |
| 4 | 调整配置校验顺序, 非法枚举保存时拒绝 | 防止错误配置被静默吞掉。 |
| 5 | 修 pass/observe 互斥筛选 | 保持管理端统计口径干净。 |
| 6 | 真正抽出共享文本抽取 helper 给 prompt-risk 与 request_intercept 共用 | 消除后续分叉维护风险。 |
| 7 | 补前端与 E2E/运营闭环测试 | 防止“单测通过但线上不可运营”。 |

