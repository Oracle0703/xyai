# Codex Prompt 风险过滤器设计（网关侧前置审查）

更新：2026-11-04。状态：**已实现并在 main（`26f6363af`）中；默认关闭（`enabled=false`、`mode=observe`）。LLM 语义复核（judge）已实现，默认关闭；HTTP 回环防护已补强（§6.3）。生产是否已开启以生产 settings 为准，本文不记录。**

本文是 Codex Prompt 风险过滤器（含 LLM 语义复核 judge）的唯一文档，包含需求、规则、配置合同、代码位置、易错点和验证方式。设计审核、实现审核和 judge 上线前审查的结论都已写入正文；审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/reviews/codex-prompt-risk-filter-design-audit-cn.md`、`…/codex-prompt-risk-filter-implementation-audit-cn.md`、`…/prompt-risk-judge-release-review-cn.md` 查看）。

阅读顺序建议：
- 调词表、阈值或豁免：先读 §4 和 §11；
- 改接入点、缓存或失败语义：读 §5、§11；
- 改或启用 judge：读 §6、§11；
- 改 request_intercept 匹配：读 §3；
- 定位代码：读 §10；跑测试：读 §13。

## 1. 背景与目标

团队共享的上游 OpenAI 账号（ChatGPT/Codex OAuth）陆续收到 OpenAI "Cyber Abuse" 警告邮件。团队日常做游戏加速器、VPN 协议、UDP 转发、WireGuard、NAT 穿透、节点调度、网络路径优化等**合法**网络工程，这类描述（绕过、隐藏、穿透、反检测……）容易命中 OpenAI 滥用分类器。

目标：请求送达上游**之前**，在网关侧做一道本地 Prompt 风险前置审查——检测、审计、拦截并给改写建议，同时把**误杀压到最低**，不影响团队正常工作。

外部依据：OpenAI Codex Cyber Safety 文档把渗透测试、漏洞研究、高规模扫描、恶意软件分析等列为"双用途"能力。因此本地策略不能把双用途词等同违规，硬拦只留给语义明确的攻击工具和恶意动作。

## 2. 已确认决策

| 决策点 | 结论 | 依据 |
| --- | --- | --- |
| 流量范围 | 经本网关的流量 | 团队用 Sub2API 签发的 key，Codex/Chat 流量都中转到共享上游账号；网关侧看得全、绕不过 |
| 落点 | 扩展现有内容审核（`ContentModerationService`），不另起模块 | 复用日志、指标、管理页、留存清理与总开关 `risk_control_enabled`；它每次检查都带 UserID/GroupID/APIKeyID |
| 高风险动作 | 拦截 + 返回改写建议；**首版 `observe` 校准，再切 `block`** | 非交互 API 没有"弹窗确认"；绝不静默改写后转发（破坏原意、难调试、风险文本照样送上游） |
| 作用域 | group 级 opt-in（`group_ids` 或 `all_groups`） | `Provider` 来自分组 platform，不等于最终命中的共享 OAuth 账号；只在共享 OAuth 池所在分组启用更准 |
| 失败语义 | 只支持 fail-open | 功能默认 off/observe，保护对象是共享账号可用性；fail-closed 会因一处配置错误拦死整组流量 |
| 副作用 | prompt-risk 不触发自动封禁、不发邮件 | 封禁针对用户账号，合法队友不应被封 |
| 中风险通知 | 只记 `prompt_risk_observe` 日志，看板可见，不通知终端用户 | 成功响应没有干净的内联通道，注入文字会污染正文 |

已否决：
- 已否决：单个关键词直接判高危、两个中风险词即升级。原因：`vpn`+`tunnel`、`proxy`+`payload` 等常见双用途词对会被误拦（设计审核 P0-2 复现的场景）。
- 已否决：高危分数用 `1000`。原因：日志列 `content_moderation_logs.highest_score` 是 `DECIMAL(8,6)`，整数部分只有 2 位，高危日志会落库失败（设计审核 P0-1）。
- 已否决：`FailOpen=false`（fail-closed）开关。原因：热路径从未执行它，是死配置；见 §2 失败语义。
- 已否决：`AutoBanOnHigh`、`AdminEmailOnHigh`、`AdminEmailOnMedium`、`EmailOnHigh`、`NotifyOnMedium` 等副作用开关。原因：后端不执行或语义混用（`EmailOnHigh` 曾被当作 `applySideEffects` 总开关，会同时触发封禁），v1 删除；需要时按 §12 另起独立开关。
- 已否决：LLM 动态改写。原因：成本和时延；用静态 `rewrite_suggestion`。

## 3. 与请求拦截（request_intercept）的关系

`request_intercept`（管理页「请求拦截」）命中关键词后返回一段伪造的成功回复（例：发 "hi" 回"你好，我是迅游AI……"）。

- **执行顺序**：request_intercept 是网关中间件，在 handler 之前；prompt-risk 在 handler 内的内容审核里。intercept 命中即 `c.Abort()` 短路，prompt-risk 不运行，也不进风控日志；但该请求也没去上游，账号无风险。
- **合并检测层、不合并动作层**：两者动作模型（伪造 200 vs 拦截 4xx）和所处层不同，intercept 是线上功能，整体合并回归风险大。共享的只有两项：
  - 词边界匹配器：request_intercept 新增 `word` 匹配模式，调用 `promptRiskKeywordMatches(PromptRiskMatchWord, …)`；
  - 包裹剥离：`requestInterceptTextForRule` 调用 `stripPromptRiskWrappers`。
- **老功能"很多情况不触发"的根因**（已由上面两项修掉）：
  1. `exact` 要求整条最新用户文本等于关键词，"hi there"、"你好啊"都不命中；
  2. Codex 把 `<environment_context>` 等注入 user item，取到的文本不是用户敲的那句；
  3. 改 `contains` 又会过度命中（"hi" 是 this/which/architecture 的子串）；
  4. 默认只扫 `latest_user`。
  问候类短词规则应改用 `word`；中文问候继续 `contains`。
- **排障提示**：request_intercept 有两道启用闸——静态 `gateway.request_intercept.enabled`（config.yaml）和管理端开关（DB，默认 true）。"完全无反应"先查静态那道。
- **未统一的部分**：request_intercept 中间件仍用自己的 `extractRequestInterceptText` / `extractResponsesInputText`，与 `extractPromptRiskInput` 是两套抽取器，同一请求两边取到的文本可能不同（实现审核 P1-5，列入 §12）。

## 4. 判定规则

### 4.1 分级与评分（`EvaluatePromptRisk`）

1. 文本 TrimSpace 后转小写一次；空文本直接 `low/allow`。
2. 逐个关键词集、逐词匹配（§4.2 的三种模式）。每词按 `level|keyword` 去重：同一词重复出现不重复计分，刷词不能堆分。
3. 每次命中累加该集合的 `score`（未填或 ≤0 时：high=1.0，medium=0.4，low=0），并记一条 `PromptRiskReason{level, keyword, source=匹配模式, score}`。
4. 判定：
   - 任一 high 命中，**或**累计分 ≥ `escalate_threshold`（默认 1.0）→ `high/block`；
   - 否则任一 medium 命中 → `medium/log_notify`；
   - 否则 `low/allow`。
   默认参数下，3 个不同中风险词（0.4×3=1.2）升级为高；2 个（0.8）不升级。
5. 豁免封顶（§4.3）。
6. Mode 收敛：`mode=observe` 时 block 一律降为 `log_notify`（只记不拦）。

`Score` 是累计分，经 `clampPromptRiskScore` 截到 `[0, 99.999999]` 以适配 `DECIMAL(8,6)`；它**不是** 0~1 的概率值（例如 3 个中风险词为 1.2）。单词分数建议保持 0~1。

### 4.2 匹配模式与默认词表

| 模式 | 规则 |
| --- | --- |
| `word` | 词边界：`(^|[^\p{L}\p{N}])` + 转义关键词 + `([^\p{L}\p{N}]|$)`；用于 Latin 词，"scanner" 不命中 `scan` |
| `contains` | 子串；用于中文（无词边界） |
| `regex` | 关键词本身是正则，对小写文本匹配 |

正则由 `promptRiskCompiledRegex` 按模式串缓存在进程内 `sync.Map`（编译失败也缓存错误），避免每请求 `regexp.Compile`。

默认词表（`DefaultPromptRiskConfig`），原则"双用途 → 观察、明确恶意 → 拦截"：

- **low**：空。正常词（udp、wireguard、nat traversal、网络优化）靠"不命中"放行，不靠白名单。
- **medium（0.4）**：
  - `word`：vpn, proxy, tunnel, socks5, openvpn, shadowsocks, v2ray, trojan, bypass, payload, spoof, obfuscate, stealth, evade, evasion, enumerate, exploit, nmap；
  - `regex`：`\bport\s+scan\b`；
  - `contains`：代理, 翻墙, 梯子, 隧道, 绕过, 隐藏, 规避, 反检测, 反审计, 反追踪, 伪装, 渗透, 端口扫描, 枚举。
- **high（1.0）**：
  - `word`：sqlmap, metasploit, masscan, hydra, nuclei, gobuster, ffuf, nikto, bruteforce, backdoor；
  - `contains`：爆破, 提权, 后门, 木马, 免杀, 撞库, 漏洞利用, 凭据窃取。

说明：`bypass/绕过/payload/proxy/nmap/端口扫描` 全部在观察层——"游戏加速器绕过 DPI 误拦截""用 nmap 检查自有节点端口""reverse proxy payload tuning"都不应硬拦。刻意不收裸 `scan`。真正升高危应靠组合（攻击动词 + 未授权/第三方目标），目前由 judge（§6）做语义复核，规则侧组合留作后续。

### 4.3 豁免与 Mode

- 豁免 `PromptRiskExemption{group_ids, user_ids, api_key_ids, max_level}`：主体的分组、用户或 API Key 任一命中即生效；多条命中取**最严格**（等级最低）的 `max_level`。
- 封顶后同步动作：封到 `medium` → `log_notify`（只记不拦），封到 `low` → `allow`（不记日志）。安全/网络团队通常封到 `medium`，仍留审计。
- 豁免只免本地拦截，**不隔离上游账号**：被豁免的高风险流量仍走同一批共享 OAuth 账号（§12 已知残留风险）。

### 4.4 输入抽取（`extractPromptRiskInput`）

不复用内容审核的 `ExtractContentModerationInput`：后者只看 Responses `input` 最后一项，工具循环末项是 `function_call_output` 时整条跳过，会漏扫本轮真实用户请求。

| `input_scope` | 行为 |
| --- | --- |
| `newest`（默认） | `newestUserRange`：从末尾找最后一个 user item（跳过尾部工具调用/输出、reasoning、模型 item），再并入其前面**相邻**的 user item（Codex 把 environment_context 与真实问题拆成多个 item 的同一轮）；遇到第一个非 user item 即停，不回溯历史轮次 |
| `full`（显式 opt-in） | 扫请求体内全部 user turn；仍排除 assistant/system/工具输出 |

- 协议：Chat/Anthropic 读 `messages`，Responses 读 `input`，Gemini 读 `contents`，Images 读 `prompt`；未知协议三者都试。
- Responses 中 `function_call`、`function_call_output`、`reasoning`、`computer_call(_output)`、`web_search_call`、`file_search_call`、`item_reference` 不算 user item；带非 user `role` 的 item 也不算。
- 抽取后统一剥离 `<environment_context>`、`<user_instructions>`、`<system-reminder>` 包裹（`stripPromptRiskWrappers`），文本收集使用 `moderationTextCollector{filterReminders: true}`。

## 5. 运行时接入

### 5.1 位置与顺序

网关 handler 经 `securityaudit` 协调器（`Coordinator.Check` → `LegacyModerationAdapter.Check`）调用 `ContentModerationService.Check`。`Check` 内顺序：

1. `loadRuntimeSnapshot` 取运行时快照；失败则整条放行（`content_moderation.skip_config_load_failed`）。
2. `risk_control_enabled` 为 false → 放行（总闸）。
3. **`evaluatePromptRiskStage`**：返回非 nil 决策即短路返回（block）；observe 只异步落日志后继续。
4. 网络安全策略用户白名单（`cyber_policy_user_allowlist`，`riskControlLogOnly`）的 defer 在此之后才注册。
5. 既有内容审核（`content_moderation_config` 的 mode/分组/模型范围、关键词、OpenAI/TypeSafe 引擎）。

`evaluatePromptRiskStage` 内部：judge 回环头校验（§6.3）→ `enabled` 且 `mode≠off` → `includesGroup`（`all_groups` 或 `group_ids` 含本请求分组；无分组请求不生效）→ 抽取（§4.4）→ `EvaluatePromptRisk` → 按动作处理（§5.3）。

不需要改 `ContentModerationCheckInput` 以外的 handler 逻辑：协调器 `prioritize` 把 `Blocked` 决策转成带 `Message`、`StatusCode` 的拒绝（错误码 `content_policy_violation`），全协议与流式都适用。

### 5.2 配置加载、缓存与失败语义

- 配置存 settings 表 `prompt_risk_config`（JSON，`SettingKeyPromptRiskConfig`），与旧 `content_moderation_config` 分开。
- 热路径读 `contentModerationRuntimeSnapshot`：`refreshRuntimeSnapshot` 用一次 `GetMultiple` 读 `risk_control_enabled`、`content_moderation_config`、`prompt_risk_config`、`cyber_policy_user_allowlist`；TTL `contentModerationRuntimeCacheTTL`（1 秒），过期后先用旧快照、后台异步刷新（stale-while-refresh），刷新失败按 TTL 退避。两份配置原文的 sha256 不变时复用已解析对象。
- `UpdatePromptRiskConfig` 保存成功后 `replaceRuntimePromptRiskConfig` 立即替换快照里的 prompt-risk 部分，不等 TTL。
- 失败语义（一律 fail-open）：

| 情况 | 结果 |
| --- | --- |
| `prompt_risk_config` 不存在 | 默认配置（`enabled=false`） |
| `prompt_risk_config` JSON 损坏 | 热路径 `parsePromptRiskConfig` 退回默认配置（功能关闭）并打 `prompt_risk_invalid_json`；管理端 GET 返回 400 `INVALID_PROMPT_RISK_CONFIG` 以便修复 |
| regex/word 关键词编译失败 | 保存时 `Validate` 拒绝，运行时不应出现 |
| settings 仓储不可用 / 快照首次加载失败 | 整个 `Check` 放行 |
| 快照刷新失败但已有旧快照 | 继续用旧快照 |

- **与旧配置的耦合（当前实现）**：`refreshRuntimeSnapshot` 先解析 `content_moderation_config`，它损坏会让整次刷新失败——冷启动时整个 `Check`（含 prompt-risk）放行，已有快照时沿用旧快照。早期设计"旧配置损坏时 prompt-risk 仍正常运行"在冷启动下不成立。旧配置只能经 `UpdateConfig` 校验后写入，正常不会损坏；手工改库时注意。

### 5.3 动作

| 评估动作 | 处理 |
| --- | --- |
| `block` | （judge 见 §6）`recordPreBlockSyncMetric(0, prompt_risk_block)` 计入 blocked；`buildPromptRiskLog` 写 `action=prompt_risk_block`、`flagged=true`；返回 `Blocked` 决策：`Message = block_message + "\n" + rewrite_suggestion`，`StatusCode = block_status`（默认 403），`HighestCategory = prompt_risk_<level>` |
| `log_notify` | 写 `action=prompt_risk_observe`、`flagged=false` 日志，继续走既有内容审核 |
| `allow` | 不记日志 |

两类日志都用 `enqueueRecord(…, recordHash=false, applySideEffects=false)` 异步落库：不记命中哈希、不自动封禁、不发邮件。

### 5.4 日志、指标与筛选

- 复用 `content_moderation_logs`，**无 migration**：`mode='prompt_risk'`；`highest_category=prompt_risk_<level>`；`highest_score` 为截断后的累计分；`input_excerpt` 沿用 240 rune 截断与密钥脱敏，留存走既有清理。
- 命中证据写 `category_scores`（JSONB，`map[string]float64`），键为 `level:source:keyword`，值为单词分数。judge 结果另写 `judge:risk:<none|low|high>=1`、`judge:latency_ms`、`judge:downgraded=1`、`judge:error=1`；judge 理由追加到 `input_excerpt`，judge 错误写入 `error`。
- 指标：`recordPreBlockSyncMetric` 的 blocked 分支包含 `prompt_risk_block`。
- 管理端结果筛选（`buildContentModerationLogWhere`）：

| result | 条件 |
| --- | --- |
| `blocked` | `action IN ('block','keyword_block','hash_block','prompt_risk_block')` |
| `observe` | `action = 'prompt_risk_observe'` |
| `pass` | `flagged = FALSE AND error = '' AND action <> 'prompt_risk_observe'` |
| `error` | `error <> ''` |

- 封禁计数：`CountFlaggedByUserSince` 带 `AND action NOT LIKE 'prompt_risk_%'`，prompt-risk 行不进内容审核的自动封禁计数。

## 6. LLM 语义复核（judge）

judge 是本过滤器的一部分：关键词评估会真正拦截时，调一次外部 chat 模型做语义精判，降低双用途词误杀。代码在 `prompt_risk_judge.go`，配置是 `prompt_risk_config.judge`。

### 6.1 触发与融合

- 触发条件同时满足：`judge.enabled`；评估动作是 `block`（即已经过豁免封顶和 Mode 收敛——`observe` 模式下永远不触发）；命中等级在 `trigger_levels`（默认 `["high"]`）；当前 context 没有 judge 在途标记。
- 融合：judge 返回 `none`/`low` **或调用失败** → 降级为 `prompt_risk_observe` 放行（日志带 `judge:downgraded=1`）；返回 `high` → 保持拦截。
- 结论：judge 一旦启用，端点配错或不可用时，所有本应拦截的高危请求都会降为观察放行。这是有意的 fail-open，启用后要监控 `judge:error` 日志。

### 6.2 请求与配置

| 字段 | 规则 |
| --- | --- |
| `base_url` | OpenAI 兼容网关根地址，请求 `POST {base_url}/v1/chat/completions`；启用时必填 |
| `model` | 启用时必填；建议便宜的 chat 模型 |
| `api_key` | 启用时必填；GET 只返回 `api_key_configured` 与 `api_key_masked`，明文不回传；PUT 传空表示沿用已存值 |
| `timeout_ms` | 默认 4000，范围 500–15000（`validateRaw` 拒绝越界非零值，`normalize` 再 clamp） |
| `prompt_template` | system 提示词；空则用内置 `defaultPromptRiskJudgePrompt`（判 none/low/high，"仅凭出现 vpn/proxy/代理/exploit/nmap 等词不能判风险……无法判断时倾向 low"，只输出 JSON） |
| `trigger_levels` | 默认 `["high"]`；非法等级在 `validateRaw` 拒绝 |

- 请求体：`temperature=0`、`stream=false`、`response_format=json_object`；user 消息是"被关键词规则命中的词 + 用户原文"（`buildPromptRiskJudgeUserMessage`）。
- 解析：`parsePromptRiskJudgeContent` 截取首个 `{` 到末个 `}`，容忍代码块包裹；`risk` 不是 none/low/high 判为错误。
- 出站保护：`validatePromptRiskJudgeBaseURL` 走 `urlvalidator`，`security.url_allowlist.enabled=true` 时要求 HTTPS 且主机在 `upstream_hosts`；进程内并发上限 8（`acquirePromptRiskJudgeSlot`，满了不排队，直接按失败处理即 fail-open）；响应体上限 64 KiB；非 2xx、超时、解码失败都 fail-open。

### 6.3 防回环

judge 若指向本网关，复核请求体含原文关键词，会再次进入 prompt-risk，可能自触发或递归。两层防护：

1. **同进程**：`withPromptRiskJudgeInFlight` 在出站请求 context 上打标记，`ctxHasPromptRiskJudgeInFlight` 为真时不再调 judge。它**不能跨 HTTP 入站请求**（新入站请求的 context 是新建的），只防同进程直接递归。
2. **跨 HTTP**：出站请求带头 `X-Sub2API-Prompt-Risk-Judge`，值为 `HMAC-SHA256(key=judge api_key, "sub2api-prompt-risk-judge-v1" + 0x00 + 请求体)` 的 hex。入站时 handler 把该头复制进 `ContentModerationCheckInput.PromptRiskJudgeHeader`，`evaluatePromptRiskStage` 在 `judge.enabled` 时用 `IsPromptRiskJudgeRequestHeader` 常数时间比较，通过则跳过 prompt-risk 阶段。外部无法用固定头值伪造（需要 judge key 和精确请求体）；judge 关闭时该头被忽略。

跳过的只是 prompt-risk 阶段，复核请求**仍会经过既有内容审核**。管理端黄色提示仍建议：judge 用专属 API Key，把它的 `api_key_id` 加入豁免（封顶 `low`），并确保该 Key 所属分组不在内容审核拦截范围内。

### 6.4 在线测试器口径

`POST /prompt-risk/test` 只跑关键词规则：响应带 `scope: "keyword_rules_only"`、`judge_evaluated: false`，前端显示 `testerRuleOnlyHint`。测试器用空主体评估，**不套豁免**；但套用当前 `mode`（`observe` 下高危显示为 `log_notify`）。真实请求启用 judge 后结论可能不同，灰度时以日志为准。

### 6.5 启用建议

| 场景 | 建议 |
| --- | --- |
| 只合入代码，judge 关闭 | 可上线 |
| 启用关键词 `block`，judge 关闭 | 先 `observe` 收集真实语料，误杀集稳定后再切，并只对共享 OAuth 池分组 opt-in |
| 启用 judge 指向本网关 | 必须用专属 Key + 豁免；小流量灰度，观察 `judge:error` 与延迟 |
| 启用 judge 指向外部独立端点 | 风险较低；同样观察错误率，并确认端点在 URL 白名单内 |

## 7. 配置合同（`prompt_risk_config`）

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `enabled` | false | 功能开关；仍受总闸 `risk_control_enabled` 约束 |
| `mode` | `observe` | `off` / `observe` / `block` |
| `all_groups` | false | true 时所有分组生效 |
| `group_ids` | `[]` | group 级 opt-in；空且 `all_groups=false` 时不生效 |
| `input_scope` | `newest` | `newest` / `full`（§4.4） |
| `block_status` | 403 | 400–599 |
| `escalate_threshold` | 1.0 | 累计分达到即升级为 high |
| `block_message` | 内置中文提示 | 空则回填默认 |
| `rewrite_suggestion` | 内置改写建议 | 拼在拦截消息后；可置空 |
| `keyword_sets[]` | §4.2 | `{level, match_mode, keywords[], score}` |
| `exemptions[]` | `[]` | `{group_ids, user_ids, api_key_ids, max_level}` |
| `judge` | 关闭 | §6.2 |

保存流程（`UpdatePromptRiskConfig`）：judge `api_key` 为空时合并已存值 → `validateRaw`（非空但非法的 `mode`、`input_scope`、`level`、`match_mode`、`max_level`、judge 字段直接拒绝，空值视为默认）→ `normalize` 填默认 → `Validate`（`word`/`regex` 可编译、judge 启用时三项必填）→ `validatePromptRiskGroups`（`group_ids` 与豁免分组必须存在）→ 落库 → 替换运行时快照 → 返回掩码后的配置。错误码 `INVALID_PROMPT_RISK_CONFIG`、`INVALID_PROMPT_RISK_GROUP`、`PROMPT_RISK_UNAVAILABLE`（均 400）。

## 8. 管理接口

挂在 `registerContentModerationRoutes` 的 `/api/v1/admin/risk-control` 组下：

| 方法与路径 | Handler | 行为 |
| --- | --- | --- |
| `GET /prompt-risk` | `GetPromptRisk` | 读配置（judge key 掩码）；JSON 损坏返回 400 |
| `PUT /prompt-risk` | `UpdatePromptRisk` | 全量替换，见 §7 |
| `POST /prompt-risk/test` | `TestPromptRisk` | 请求 `{prompt}`；响应 `{matched, decision, scope, judge_evaluated}`，见 §6.4 |

## 9. 前端

- `frontend/src/views/admin/PromptRiskPanel.vue`（嵌在 `RiskControlView.vue` 风控页）：模式、输入范围、分组 opt-in、关键词集、豁免、升级阈值、拦截消息与改写建议、judge 配置（含回环提示与"为 judge 专属 Key 加一条豁免"按钮）、在线测试器。
- `RiskControlView.vue` 日志列表：`blocked` / `observe` 结果筛选与 `prompt_risk_block` / `prompt_risk_observe` 动作标签。
- `frontend/src/views/admin/RequestInterceptView.vue`：匹配模式含「词边界」。
- API：`frontend/src/api/admin/riskControl.ts`；文案：`frontend/src/i18n/locales/{zh,en}/admin/channels.ts` 的 `admin.riskControl.promptRisk.*`。

## 10. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/internal/service/prompt_risk.go` | 常量与默认词表、`PromptRiskConfig` / `PromptRiskJudgeConfig` / `PromptRiskKeywordSet` / `PromptRiskExemption` / `PromptRiskDecision`、`DefaultPromptRiskConfig`、`EvaluatePromptRisk`、`includesGroup`、`normalize`、`validateRaw`、`Validate`、`matchedExemptionMaxLevel`、`capPromptRiskLevel`、`promptRiskKeywordMatches`、`promptRiskPattern`、`promptRiskCompiledRegex`、`clampPromptRiskScore` |
| `backend/internal/service/prompt_risk_input.go` | `extractPromptRiskInput`、`newestUserRange`、`isResponsesUserItem`、`collectNewest*` / `collectAll*`、`stripPromptRiskWrappers` |
| `backend/internal/service/prompt_risk_judge.go` | `runPromptRiskJudge`、`buildPromptRiskJudgePayload`、`buildPromptRiskJudgeUserMessage`、`parsePromptRiskJudgeContent`、`PromptRiskJudgeHeaderName`、`IsPromptRiskJudgeRequestHeader`、`withPromptRiskJudgeInFlight`、`validatePromptRiskJudgeBaseURL`、`acquirePromptRiskJudgeSlot` |
| `backend/internal/service/content_moderation.go` | `Check`、`evaluatePromptRiskStage`、`attachPromptRiskJudgeFields`、`promptRiskSubject`、`loadPromptRiskConfig`、`parsePromptRiskConfig`、`GetPromptRiskConfig`、`UpdatePromptRiskConfig`、`validatePromptRiskGroups`、`TestPromptRisk`、`buildPromptRiskLog`、`recordPreBlockSyncMetric`、`refreshRuntimeSnapshot`、`replaceRuntimePromptRiskConfig`；action 常量 `ContentModerationActionPromptRiskBlock` / `ContentModerationActionPromptRiskObserve` |
| `backend/internal/service/domain_constants.go` | `SettingKeyPromptRiskConfig = "prompt_risk_config"` |
| `backend/internal/repository/content_moderation_repo.go` | `buildContentModerationLogWhere`（结果桶）、`CountFlaggedByUserSince`（排除 `prompt_risk_%`） |
| `backend/internal/handler/admin/content_moderation_handler.go` | `GetPromptRisk`、`UpdatePromptRisk`、`TestPromptRisk` |
| `backend/internal/server/routes/admin.go` | `registerContentModerationRoutes` |
| `backend/internal/handler/content_moderation_helper.go`、`security_audit_helper.go` | 构造审核输入，复制 `X-Sub2API-Prompt-Risk-Judge` 头 |
| `backend/internal/securityaudit/coordinator.go`、`coordinator_legacy.go` | `Coordinator.Check`、`prioritize`、`LegacyModerationAdapter` |
| `backend/internal/service/request_intercept_rules.go` | `RequestInterceptMatchWord`、`requestInterceptKeywordMatches`、`requestInterceptTextForRule` |
| `backend/internal/server/middleware/request_intercept.go` | 中间件与独立抽取器 `extractRequestInterceptText` |

## 11. 关键约束与易错点

1. **双用途词不得放进 high。** 依据：设计审核用"游戏加速器绕过 DPI 误拦截""debug VPN tunnel route flapping""reverse proxy payload size tuning""用 nmap 检查自有节点端口"复现了误拦。改词表后 `TestEvaluatePromptRisk_Classification` 的误杀集必须仍为 low 或 observe。
2. **升级阈值保持"2 个中风险不升级"。** 依据：`proxy`+`payload`、`vpn`+`tunnel` 是常见组合。改分数或阈值时同步 `TestEvaluatePromptRisk_TwoMediumDoesNotEscalate`。
3. **`highest_score` 列是 `DECIMAL(8,6)`。** 单词分数不要用大整数；累计分由 `clampPromptRiskScore` 截到 99.999999。依据：曾设计的 1000 分会让高危日志落库失败，看板丢证据。
4. **新增 prompt-risk action 必须同步四处**：`recordPreBlockSyncMetric`、`buildContentModerationLogWhere`、前端结果标签与 i18n、对应单测。依据：实现审核发现 `prompt_risk_block` 曾被计为 allowed（P0-1），`prompt_risk_observe` 曾同时出现在 `observe` 和 `pass` 桶（P1-4）。
5. **默认抽取必须是 `newest`。** 依据：实现审核复现——扫全部 user turn 时，上一轮问过"用 sqlmap 撞库"，下一轮只说"谢谢"也会被拦（`TestExtractPromptRiskInput_HistoricalHighRiskDoesNotReFire`）。
6. **工具循环续传会重复评估同一轮。** `newest` 跳过尾部工具输出后取到的仍是本轮 user 文本，所以一个工具循环里的每次续传都会重复记 observe 或重复拦截。统计 observe 数量时按请求计，不等于按用户意图计。
7. **保存时先 `validateRaw` 再 `normalize`。** 依据：先 normalize 会把 `mode:"blok"`、`match_mode:"words"` 静默改成默认值，管理员收不到错误（实现审核 P1-3）。新增枚举字段要同时加进 `validateRaw`。
8. **不要暴露后端不执行的开关。** 依据：`FailOpen`、`AutoBanOnHigh`、`AdminEmail*` 曾能在 UI 保存但热路径无行为（实现审核 P1-1/P1-2）。新增开关必须先有执行路径和测试。
9. **prompt-risk 不得喂自动封禁。** `enqueueRecord` 的 `applySideEffects` 保持 false；若将来要"高危计封禁"，用独立开关和独立阈值，并保留 `CountFlaggedByUserSince` 对 `prompt_risk_%` 的排除。
10. **网络安全策略白名单不豁免 prompt-risk。** `cyber_policy_user_allowlist` 的 log-only 处理在 prompt-risk 阶段之后才注册，白名单用户仍会被 prompt-risk 拦截。需要豁免某人时用 prompt-risk 自己的 `exemptions`。
11. **observe 模式下 judge 不会被调用。** judge 只在最终动作是 `block` 时触发；想先观察 judge 效果，需要在 `block` 模式下小范围分组灰度。
12. **judge 失败即放行。** 端点错误、超时、并发满（>8）、响应超 64 KiB 都会把高危降为 observe。启用后看 `judge:error` 计数。
13. **judge 降级的 observe 日志在 `error` 筛选中也会出现。** 失败时 `error` 字段写 `judge: …`，该行同时属于 `observe` 和 `error` 桶；正常降级（judge 判 none/low）只在 `observe`。
14. **回环防护依赖 judge key 与请求体完全一致。** 中间代理若改写请求体，HMAC 校验会失败，复核请求会再次进入 prompt-risk；所以专属 Key 豁免仍要配。
15. **测试器不套豁免、不调 judge。** 依据：judge 上线前审查指出测试器结论与真实请求不一致会误导灰度（P2），现以 `scope=keyword_rules_only` 明示。
16. **request_intercept 与 prompt-risk 的抽取器仍是两套。** 改其中一边的抽取逻辑时，检查另一边对同一 Codex 请求的取值；只有匹配器和包裹剥离是共享的。
17. **冷启动时旧内容审核配置损坏会连带关闭 prompt-risk**（§5.2）。排查"prompt-risk 不生效"时先看 `content_moderation.skip_config_load_failed` 日志。

## 12. 已知局限、后续与非目标

- **账号池隔离（已知残留风险，最重要的后续项）**：豁免与 judge 降级放行的合法高风险流量仍走同一批共享 OAuth 账号，仍可能触发 OpenAI 滥用分类器。后续应把安全/网络团队的合法高风险工作路由到独立上游账号池，并按上游 `policy_violation` 错误对账号自动降权或摘除。
- **账号级精确作用域**：目前以 group 级 opt-in 近似；同一分组混用 OAuth 与 API Key 账号时，需要把检查下沉到账号选择之后，或用分组绑定的账号池元数据判断。
- **抽取器统一**：把 request_intercept 与 prompt-risk 的文本抽取收敛成一个参数化 helper，并用同一组 Codex/Chat/Gemini fixture 断言两边结果一致。暂缓原因：两边都已收敛到"最新一轮"口径，命中文本基本一致；request_intercept 抽取器在中间件热路径，重构回归风险大于收益。
- **结构化 reasons**：`category_scores` 只能存 `map[string]float64`，完整 `PromptRiskReason` 结构要新增 `metadata` 类 JSONB 列（需 migration）。
- **管理员通知**：高危/中危的管理员邮件，需要先定收件人、模板、幂等与失败记录。
- **规则侧组合评分**：意图 + 授权上下文 + 行为动词 + 目标范围；目前由 judge 承担。
- 非目标：绕开网关、直连 ChatGPT Web 的用法（客户端侧防护）；LLM 动态改写。

## 13. 验证

### 13.1 命令

```bash
cd backend
# CI 单测门禁使用 unit 标签
go test -tags=unit -count=1 ./internal/service -run 'PromptRisk|ExtractPromptRiskInput|RecordPreBlockSyncMetric_PromptRisk|ContentModerationRuntimeSnapshot|RiskControlAllowlistSurvives'
go test -tags=unit -count=1 ./internal/repository -run 'ContentModerationLogWhere'
go test -tags=unit -count=1 ./internal/handler -run 'PromptRiskJudgeHeader'
cd ../frontend && pnpm run typecheck
```

### 13.2 测试清单

| 测试 | 覆盖 |
| --- | --- |
| `prompt_risk_test.go` | 误杀集/中/高分类、两个中风险不升级、observe 收敛、豁免封顶、去重、分数不溢出、`includesGroup`、`Validate`、`validateRaw` 拒非法枚举与放行空值、词边界、`prompt_risk_block` 计入 blocked |
| `prompt_risk_input_test.go` | Codex 包裹剥离、工具输出不算用户意图、newest 只取最新一轮、full 扫全部 user turn、历史高危不复触发、同轮多 item |
| `prompt_risk_judge_test.go` | payload/解析、normalize、`validateRaw`、成功/非 2xx/超时/坏 JSON/超大响应/并发满均 fail-open、URL 白名单、融合降级与保持拦截、豁免短路、context 标记、回环头发送与入站跳过、judge 关闭时忽略回环头、key 掩码与沿用 |
| `content_moderation_runtime_cache_test.go`、`content_moderation_merge_contract_test.go` | 快照缓存、保存后立即生效、刷新失败保留旧快照与退避、白名单跨更新保留 |
| `repository/content_moderation_repo_test.go` | `TestBuildContentModerationLogWhere_PassExcludesObserve` |
| `handler/content_moderation_helper_test.go`、`security_audit_helper_test.go` | 回环头复制进审核输入 |

尚无自动化覆盖：真实 HTTP 回环端到端（judge 指向本网关）、冷启动时旧配置损坏的行为、前端面板组件测试（仓库无对应 harness，沿用 typecheck）。

### 13.3 最近结果

- 2026-11-04（本机，HEAD `26f6363af`）：§13.1 三条 Go 命令全部 `ok`。前端 typecheck 本次未跑。
- 2026-06-26（judge 上线前审查，分支 `feature/hy/0621_敏感词过滤`）：定向 Go 单测、`vue-tsc --noEmit`、前端 build 通过；`./internal/service` 全包在本机 180 秒超时，未作为通过证据。

### 13.4 上线验收（人工）

1. `mode=observe`：高危 prompt 正常转发，日志 `prompt_risk_observe`；`/v1/responses` 与 `/v1/chat/completions`，stream 真/假各一。
2. 切 `mode=block`：同一 prompt 返回 403 与改写建议、不到上游、日志 `prompt_risk_block`、blocked 指标增加。
3. 未 opt-in 分组的 key 不触发；豁免分组（`max_level=medium`）高危降为 observe。
4. 日志 `category_scores` 可见 `level:source:keyword`；`prompt_risk_*` 不进封禁计数。
5. 启用 judge 时：none/low 降级、high 保持拦截、端点不可达时降级并记 `judge:error`；指向本网关时确认复核请求未被 prompt-risk 再次评估。

## 14. 状态与待办

- 已完成：关键词分级评估、newest/full 抽取、group opt-in、豁免、observe/block、日志/指标/筛选、管理端配置与测试器、request_intercept 词边界与包裹剥离共享、judge 语义复核与跨 HTTP 回环防护、运行时快照缓存。
- 待办：
  - 生产 observe 期语料校准后再决定切 `block`（需管理员操作）；
  - 账号池隔离（§12，最重要）；
  - 抽取器统一、结构化 reasons、管理员通知（§12）；
  - 补真实 HTTP 回环端到端验证。
