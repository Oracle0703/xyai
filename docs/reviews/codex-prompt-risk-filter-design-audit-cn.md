# Codex Prompt 风险过滤器设计审核报告

> 审核日期: 2026-06-22  
> 被审文档: `docs/features/codex-prompt-risk-filter-design-cn.md`  
> 结论级别: **不建议按当前设计直接进入实现**。该设计方向基本正确, 但存在若干会导致误拦、漏拦、日志/指标失真、数据库写入失败和账号风险闭环缺失的问题。

## 审核范围

| 范围 | 说明 |
| --- | --- |
| 设计目标 | 是否满足“保护共享 OpenAI/Codex OAuth 账号, 同时不影响合法网络工程”的目标 |
| 现有代码契约 | 核对 `ContentModerationService`, `request_intercept`, 路由、中间件、日志表和前端风控页 |
| 逻辑闭环 | 检查拦截、豁免、日志、告警、指标、验证计划是否能闭环 |
| 外部依据 | 核对 OpenAI 官方 Codex Cyber Safety 与 Moderation 文档当前口径 |

## 总体结论

| 维度 | 结论 |
| --- | --- |
| 落点选择 | 扩展现有 `ContentModerationService` 是合理方向, 能复用身份、日志和管理端能力。 |
| 最大问题 | 默认关键词和评分策略与需求目标冲突: 把团队正常网络工程常用词直接设为高危或“两个中风险升级拦截”。 |
| 最大实现风险 | 高危分数设计为 `1000`, 但数据库 `content_moderation_logs.highest_score` 是 `DECIMAL(8,6)`, 写入会溢出失败。 |
| 最大闭环缺口 | “分组豁免”只是放行, 不隔离上游账号; 合法高风险网络工程仍可能继续打到被警告的共享账号。 |
| 建议 | 先把本方案修订为“账号池隔离 + observe-first 校准 + 明确恶意意图硬拦 + 正确日志/指标/DB schema”的闭环方案, 再实现。 |

## 关键问题清单

| ID | 严重级别 | 问题 | 影响 |
| --- | --- | --- | --- |
| P0-1 | 阻断 | Prompt 风险分数 `1000` 无法写入现有日志表 `DECIMAL(8,6)` | 高危命中时日志异步落库失败, 风控看板和自动封禁证据缺失 |
| P0-2 | 阻断 | 默认关键词策略会拦截需求里明确要保护的合法网络工程表达 | 实现后会直接影响团队正常 Codex/Chat 工作 |
| P0-3 | 阻断 | 豁免只放行, 不隔离上游账号, 不能保护共享 OpenAI 账号 | 安全/网络团队一旦豁免, 高风险文本仍会送到被警告账号 |
| P1-1 | 高 | `prompt_risk_block` 不会被现有 blocked 指标和 blocked 日志筛选识别 | 管理页指标失真, 审计查询漏掉关键事件 |
| P1-2 | 高 | `EmailOnHigh` / 自动封禁 / 邮件副作用关系设计错误 | 默认不会自动封禁; 开启后也不一定发邮件, 行为与文档不一致 |
| P1-3 | 高 | `NotifyOnMedium` 没有真实通知路径, 且中风险日志会被归入 pass | 中风险“通知”无法兑现, 管理端难以筛出 |
| P1-4 | 高 | `OpenAIOnly` 按 group platform 判断, 不能精确定位“共享 ChatGPT/Codex OAuth 账号” | 会误伤 OpenAI API Key/上游兼容账号, 也无法按账号池控风险 |
| P1-5 | 高 | `FailOpen` 字段没有落地语义 | 配置异常时到底放行还是拦截没有可实现合同 |
| P1-6 | 高 | Prompt 风险依赖旧 `content_moderation_config` 加载成功 | 独立配置被旧风控配置污染, 旧配置损坏时新风险阶段也失效 |
| P2-1 | 中 | 复用 `ExtractContentModerationInput` 只扫最后用户输入, 与 Codex 多 item/工具循环不完全闭环 | 部分真实 Codex 请求可能漏扫或扫到环境上下文而非用户意图 |
| P2-2 | 中 | `request_intercept` 修复被列为可选, 但共享检测层又是设计前提 | 若不做, 两套抽取/匹配逻辑继续分叉, 后续维护风险高 |
| P2-3 | 中 | 测试计划没有覆盖日志落库、指标、筛选、豁免账号隔离和 observe-first 校准 | 容易出现“单测通过但线上不可运营”的实现 |

## 详细问题与证据

### P0-1: 高危分数 `1000` 会导致日志落库失败

**设计证据**

- 设计文档定义高风险命中分数为 `1000`, 并在日志里把 `d.Score` 写入 `HighestScore`: `docs/features/codex-prompt-risk-filter-design-cn.md:124`, `:130`, `:151-156`。
- 同一文档声称“零新增存储, 无需 DB migration”: `docs/features/codex-prompt-risk-filter-design-cn.md:86`。

**源码证据**

- `content_moderation_logs.highest_score` 当前是 `DECIMAL(8,6)`: `backend/migrations/135_content_moderation.sql:22-23`。这个类型最大整数部分只有 2 位, 无法存 `1000`。
- 日志写入会把 `log.HighestScore` 直接插入该列: `backend/internal/repository/content_moderation_repo.go:51-66`。

**影响**

高危命中后, 拦截响应可能已经返回给用户, 但异步日志落库会失败。结果是风控看板看不到这次高危命中, 自动封禁计数也可能缺证据, 与“复用日志、自动封禁、指标”的目标相反。

**建议**

| 方案 | 建议 |
| --- | --- |
| 最小修正 | Prompt risk 分数归一化到 `0~1`, 例如高危 `1.0`, 中危 `0.4`, 阈值 `0.8`。 |
| 更清晰修正 | 新增 `prompt_risk_score INT` / `metadata JSONB` 之类字段, 承认需要 migration。 |
| 不建议 | 继续使用 `1000` 并声称无需 DB migration。 |

### P0-2: 默认关键词策略与“误杀最低/不影响正常工作”冲突

**设计目标**

设计背景明确说团队合法工作会自然使用“绕过、隐藏、穿透、反检测”等描述, 目标是“把误杀压到最低, 绝不影响团队正常工作”: `docs/features/codex-prompt-risk-filter-design-cn.md:7-9`。

**冲突证据**

- 默认高风险中文关键词包含 `绕过`、`隐藏`、`反检测`、`伪装`、`端口扫描`: `docs/features/codex-prompt-risk-filter-design-cn.md:130`。
- 默认高风险英文关键词包含 `bypass`, `payload`, `spoof`, `enumerate`, `nmap`, `masscan`: `docs/features/codex-prompt-risk-filter-design-cn.md:130`。
- 默认中风险词包含 `vpn`, `proxy`, `tunnel`, `openvpn`, `shadowsocks`, `v2ray`: `docs/features/codex-prompt-risk-filter-design-cn.md:129`。
- 评分规则规定两个不同中风险词累计到阈值就拦截: `docs/features/codex-prompt-risk-filter-design-cn.md:81`, `:124`。

**举例**

| 合法网络工程输入 | 当前设计结果 | 为什么不合理 |
| --- | --- | --- |
| `帮我分析游戏加速器如何绕过运营商 DPI 误拦截` | 高危拦截 | `绕过` + `DPI` 正是背景里要保护的合法场景 |
| `debug VPN tunnel route flapping` | 两个中危词升级为高危 | VPN/tunnel 是正常网络排障词 |
| `reverse proxy payload size tuning` | 命中 `proxy` + `payload` | HTTP payload/reverse proxy 是普通工程语义 |
| `用 nmap 检查我们自有节点开放端口` | 高危拦截 | 授权资产扫描与恶意扫描没有区分 |

**外部依据**

OpenAI Codex Cyber Safety 页面把渗透测试、漏洞研究、高规模扫描、恶意软件分析、威胁情报等明确归为“双用途”能力, 并说明自动检测可能把类似安全工作流量触发缓解。因此本地策略需要区分授权、资产归属、意图和动作, 不能只用词表硬拦。参考: <https://developers.openai.com/codex/concepts/cyber-safety>。

**建议**

1. 默认上线先 `observe` 不拦截, 收集 3-7 天真实语料后再定阈值。
2. 把 `绕过/隐藏/反检测/bypass/proxy/tunnel/payload` 从默认高危拦截降为观察或上下文组合规则。
3. 高危硬拦只覆盖明确恶意组合, 例如“未授权目标 + 爆破/提权/免杀/后门/撞库/凭据窃取/恶意持久化”。
4. 测试语料应把“授权自有资产扫描”和“攻击第三方目标”成对验证。

### P0-3: 豁免放行不能保护共享上游账号

**设计证据**

- 文档说安全/网络团队通过“按分组豁免”放行: `docs/features/codex-prompt-risk-filter-design-cn.md:17`, `:78`, `:124`, `:215`。
- 豁免语义是把高风险降级为记录通知, 然后继续转发上游: `docs/features/codex-prompt-risk-filter-design-cn.md:124`。

**逻辑漏洞**

需求的真实目标是保护正在收到 OpenAI “Cyber Abuse” 警告的共享账号。若安全/网络团队被豁免后仍然使用同一批共享 OpenAI/Codex OAuth 账号, 那么高风险双用途 prompt 仍会送到被警告账号。这样只是避免了本地拦截, 没有降低上游账号风险。

**建议**

| 场景 | 推荐动作 |
| --- | --- |
| 普通团队成员 | 高危明确恶意组合拦截; 双用途词默认观察 |
| 安全/网络团队合法工作 | 不应只是豁免; 应路由到独立上游账号池或可信访问账号池 |
| 被 OpenAI 缓解/降级的账号 | 记录 `policy_violation/high-risk cyber` 上游错误, 自动摘除或降权 |

## 高优先级实现问题

### P1-1: `prompt_risk_block` 不会计入 blocked 指标和 blocked 日志筛选

**源码证据**

- `recordPreBlockSyncMetric` 只把 `block`、`hash_block`、`keyword_block` 计为 blocked, 其他 action 都计为 allowed: `backend/internal/service/content_moderation.go:1076-1092`。
- 日志筛选 `result=blocked` 只包含 `block`, `keyword_block`, `hash_block`: `backend/internal/repository/content_moderation_repo.go:246-255`。
- 设计新增 action 为 `prompt_risk_block` / `prompt_risk_observe`: `docs/features/codex-prompt-risk-filter-design-cn.md:151-159`, `:201`。

**影响**

高危 Prompt 风险拦截在运行指标里会被算成 allowed, 在管理端 blocked 筛选里也查不出来。这会直接破坏风控运营。

**建议**

新增 action 常量后同步修改:

- `recordPreBlockSyncMetric`
- `buildContentModerationLogWhere`
- `RiskControlView.vue` 的 `resultLabel` / `resultBadgeClass`
- i18n action 文案
- 仓储单测和前端展示单测

### P1-2: 邮件与自动封禁副作用的参数设计错误

**设计证据**

伪代码把 `prCfg.EmailOnHigh` 作为 `enqueueRecord(... applySideEffects)` 的最后一个参数: `docs/features/codex-prompt-risk-filter-design-cn.md:152`。

**源码证据**

- `enqueueRecord` 的最后一个参数叫 `applySideEffects`: `backend/internal/service/content_moderation.go:1123-1147`。
- `applySideEffects=true` 会同时执行自动封禁和邮件副作用: `backend/internal/service/content_moderation.go:1628-1630`。
- 是否真正发违规邮件又取决于旧内容审核配置 `cfg.EmailOnHit`: `backend/internal/service/content_moderation.go:1680-1702`。

**问题**

| 配置 | 实际结果 |
| --- | --- |
| `EmailOnHigh=false` | 高危命中不会触发自动封禁, 与“复用自动封禁”冲突 |
| `EmailOnHigh=true` 且 `cfg.EmailOnHit=false` | 会尝试自动封禁, 但不会发违规邮件 |
| `EmailOnHigh=true` 且达到封禁阈值 | 可能发送账号禁用邮件, 但这不是 `EmailOnHigh` 的直接语义 |

**建议**

把副作用拆成独立语义:

- `ApplyAutoBanOnHigh`
- `EmailOnHigh`
- `EmailOnMedium`
- `RecordHashOnHigh`

不要把邮件开关复用为总副作用开关。

### P1-3: 中风险 `NotifyOnMedium` 没有实现路径

**设计证据**

- 配置里有 `NotifyOnMedium`: `docs/features/codex-prompt-risk-filter-design-cn.md:111`。
- 文档说中风险“放行 + 记到风控看板, 可选邮件提醒”: `docs/features/codex-prompt-risk-filter-design-cn.md:82`。
- 伪代码中风险日志使用 `flagged=false`, `applySideEffects=false`: `docs/features/codex-prompt-risk-filter-design-cn.md:157-160`。

**源码影响**

`flagged=false` 且 `error=''` 在现有筛选中会被归为 pass: `backend/internal/repository/content_moderation_repo.go:251-252`。这既不是 hit, 也不会触发邮件或封禁。

**建议**

| 目标 | 修正 |
| --- | --- |
| 管理端可见 | 新增 result/action 筛选: `prompt_risk_observe` |
| 可选通知 | 明确是站内通知、邮件给管理员、还是邮件给用户 |
| 不污染封禁 | 中风险可 `flagged=false`, 但 action/result 必须可筛选 |

### P1-4: `OpenAIOnly` 作用域不等于“共享 Codex OAuth 账号”

**源码证据**

- `ContentModerationCheckInput` 只有 `Provider`, `Model`, `Protocol`, `Body`, 没有上游账号 ID / 账号类型 / channel ID: `backend/internal/service/content_moderation.go:290-303`。
- `Provider` 来自 API Key 分组的 platform, 或强制 platform 上下文: `backend/internal/handler/content_moderation_helper.go:82-119`。
- 账号类型在系统里是独立概念, 包括 OAuth、API Key、Upstream 等: `backend/internal/service/domain_constants.go:66-73`。
- OpenAI Responses handler 在账号选择前就执行内容审核: `backend/internal/handler/openai_gateway_handler.go:241-247`。

**问题**

设计目标是保护“共享上游 OpenAI 账号(ChatGPT/Codex OAuth)”。但 `input.Provider == PlatformOpenAI` 只能说明这是 OpenAI 平台分组, 不能说明最终会打到哪个账号、账号类型是否 OAuth、是否为正在被警告的共享池。

**建议**

| 方案 | 说明 |
| --- | --- |
| 组级近似 | 在分组上显式配置 `prompt_risk_profile`, 只给共享 OAuth 账号所在分组启用 |
| 账号级闭环 | 将 Prompt 风险检查移动到账号选择后, 或在选择前用分组绑定的上游账号池元数据判断 |
| 更优方案 | 高风险合法团队路由到隔离账号池, 普通团队继续共享池 |

### P1-5: `FailOpen` 没有可实现合同

**设计证据**

- 配置结构包含 `FailOpen`: `docs/features/codex-prompt-risk-filter-design-cn.md:107-113`。
- 文档说“另留 `fail_open=false` 开关”: `docs/features/codex-prompt-risk-filter-design-cn.md:84`。
- 伪代码没有处理 `loadPromptRiskConfig` 错误, 也没有说明编译正则失败时如何处理: `docs/features/codex-prompt-risk-filter-design-cn.md:141-164`。

**建议**

明确四类错误:

| 错误类型 | 默认建议 |
| --- | --- |
| setting 不存在 | 使用 `DefaultPromptRiskConfig()` |
| JSON 损坏 | fail-open 并写系统日志; 如果 `fail_open=false`, 返回 503/403 需明确 |
| regex 编译失败 | 保存配置时拒绝, 运行时不应出现 |
| setting repo 不可用 | fail-open, 并暴露 runtime status |

### P1-6: 独立 Prompt 风险配置被旧内容审核配置耦合

**源码证据**

`Check` 当前顺序是:

1. `risk_control_enabled` 为 false 直接放行: `backend/internal/service/content_moderation.go:763-770`
2. `loadConfig` 失败直接放行: `backend/internal/service/content_moderation.go:772-782`
3. 之后才检查 `cfg.Enabled`, `cfg.Mode`, group/model scope: `backend/internal/service/content_moderation.go:806-848`

设计建议把 Prompt 风险插在 `loadConfig` 成功之后、`cfg.Enabled` 之前: `docs/features/codex-prompt-risk-filter-design-cn.md:133-140`。

**问题**

这确实让 Prompt 风险独立于 `cfg.Enabled` / `cfg.Mode`, 但不独立于旧 `content_moderation_config` 的 JSON 解析。旧配置一旦损坏, Prompt 风险也失效。

**建议**

把 `buildLog` 需要的旧 `cfg` 降级为日志默认配置, 不要让 Prompt 风险依赖旧内容审核配置加载成功。也可以拆出 `buildPromptRiskLog`。

## 中优先级设计问题

### P2-1: 复用现有抽取器不等于“Codex 用户意图抽取”

**源码证据**

- Responses 抽取器只看 `input` 数组最后一个 item; 最后不是用户文本则跳过: `backend/internal/service/content_moderation_input.go:124-142`。
- 现有测试明确要求工具循环最后是 `function_call_output` 时跳过审核: `backend/internal/service/content_moderation_input_test.go:138-150`。

**影响**

这对普通内容审计是合理降噪策略, 但 Prompt 风险要保护上游账号, 只看最后 item 可能漏掉本轮真实用户请求。Codex 请求还常带 `<environment_context>`、`<user_instructions>` 等上下文, 设计没有说明如何剥离环境上下文、如何定位“用户真正要做的事”。

**建议**

新增专用抽取策略:

- 对 Codex/Responses 区分 `environment_context`、`user_instructions`、真实 user turn。
- 扫描本轮新增 user text, 不扫描 assistant/tool output。
- 保留“最新用户 turn”与“full context”两种模式, 默认用可解释的 newest-turn。

### P2-2: `request_intercept` 修复不应只是“可选”

设计提出共享检测层, 但又把 `request_intercept` 修复列为可选: `docs/features/codex-prompt-risk-filter-design-cn.md:184-193`。

现有代码确实存在 exact/contains/regex 三种模式, 没有 word 模式: `backend/internal/service/request_intercept_rules.go:17-20`, `:450-459`。测试也主要覆盖裸 `hi` 和最后一条用户消息: `backend/internal/server/middleware/request_intercept_test.go:18-27`, `:94-135`。

如果 Prompt 风险新增另一套抽取与匹配而不改 request_intercept, 两边会继续分叉。后续同一条请求在两个系统里命中结果不同, 运维排障会更困难。

**建议**

把共享抽取/匹配层作为本期必做基础设施, 但动作层保持分离。

### P2-3: 验证计划缺少运营闭环测试

现有验证计划主要覆盖纯函数、少量 E2E 和豁免: `docs/features/codex-prompt-risk-filter-design-cn.md:209-217`。

建议补充:

| 测试类别 | 必测内容 |
| --- | --- |
| DB 落库 | 高危/中危日志能写入, 分数不溢出, reasons 可查询 |
| 指标 | `prompt_risk_block` 计入 blocked, 中危计入 observe |
| 管理端筛选 | blocked / observe / pass / error 筛选均正确 |
| 账号作用域 | OpenAI OAuth 共享池、OpenAI API Key、非 OpenAI 分组分别验证 |
| 豁免闭环 | 豁免分组是否路由到隔离账号池, 而不是继续打共享池 |
| 误杀语料 | VPN、NAT、WireGuard、DPI、reverse proxy、payload、授权扫描等真实团队语料 |
| 恶意语料 | 未授权爆破、提权、后门、免杀、撞库等明确恶意组合 |

## 建议修订后的闭环方案

| 模块 | 修订建议 |
| --- | --- |
| 风险模型 | 从“关键词 = 风险”改为“意图 + 授权上下文 + 行为动词 + 目标范围”组合评分 |
| 默认动作 | 首版默认 observe-only; 高置信恶意组合才 pre-block |
| 账号保护 | 增加账号池隔离: 安全/网络团队合法高风险工作走独立 OpenAI/Codex OAuth 账号池 |
| 豁免语义 | 豁免不等于无条件走共享账号; 应等于“最高只记录 + 指定路由策略” |
| 数据结构 | 分数使用 `0~1` 或增加 migration; reasons 需要结构化保存 |
| 指标与查询 | 新 action 必须纳入 pre-block metrics、blocked/observe 筛选和前端标签 |
| 通知 | 明确中风险通知对象: 管理员看板、管理员邮件、用户邮件三者不要混用 |
| 提取器 | 建立共享 prompt extraction/matcher 包, request_intercept 和 prompt-risk 共用 |
| OpenAI 官方对齐 | 把 OpenAI Codex Cyber Safety 作为策略参考, 不把双用途安全工作简单等同违规 |

## 是否偏离需求

| 需求 | 当前设计状态 | 判断 |
| --- | --- | --- |
| 保护共享 OpenAI/Codex OAuth 账号 | 只在本地拦截, 豁免后仍可能发送共享账号 | 未闭环 |
| 不影响合法网络工程 | 默认高危词包含大量合法网络工程词 | 明显偏离 |
| 误杀最低 | 缺 observe-first 校准和真实语料基线 | 未满足 |
| 拦截 + 改写建议 | 方向正确 | 基本满足 |
| 复用现有风控日志/指标/封禁 | 落库分数、action、指标、筛选、副作用均需补齐 | 设计不完整 |
| 不静默改写 | 明确禁止静默改写 | 满足 |

## 最小修改建议清单

| 优先级 | 建议 |
| --- | --- |
| 必须 | 把 PromptRisk 分数改成 `0~1`, 或新增 migration 支持整数分和 reasons metadata |
| 必须 | 从默认高危拦截词中移除/降级 `bypass/绕过/隐藏/反检测/proxy/tunnel/payload` 等合法高频词 |
| 必须 | 新增 `prompt_risk_block` / `prompt_risk_observe` 的指标、日志筛选、前端标签和测试 |
| 必须 | 拆分自动封禁与邮件开关, 不复用 `EmailOnHigh` 作为 `applySideEffects` |
| 必须 | 明确豁免分组的上游账号池隔离策略 |
| 应该 | 增加 observe-first 灰度期和真实语料校准流程 |
| 应该 | 建立共享抽取/匹配 helper, 同步修 request_intercept |
| 应该 | 给 Prompt 风险配置错误定义 fail-open/fail-closed 具体行为 |
| 应该 | 测试覆盖 DB、指标、管理端筛选、账号类型作用域和 WebSocket |

## 本次审核执行记录

| 项目 | 结果 |
| --- | --- |
| 已读取项目规则 | `AGENTS.md`, `llm-wiki/wiki/README.md`, `backend.md`, `security-and-reliability.md`, `ops.md`, `ai-workflow.md` |
| 已核对源码 | `ContentModerationService`, `content_moderation_input`, `request_intercept`, `admin routes`, `RiskControlView`, `content_moderation_logs` migration/repository |
| 已核对外部依据 | OpenAI Codex Cyber Safety, OpenAI Moderation 官方文档 |
| 是否运行测试 | 未运行。任务是设计审核与文档产出, 未修改业务代码。 |
| 是否需要更新 llm-wiki | 不需要。本轮没有改变项目稳定架构/配置/数据库, 只是新增审核文档。 |
