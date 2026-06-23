# Codex Prompt 风险过滤器设计(网关侧前置审查)

> 2026-06-22。问题来源:团队共享的上游 OpenAI 账号(ChatGPT/Codex OAuth)陆续收到 OpenAI "Cyber Abuse" 警告邮件。本稿已并入 2026-06-22 设计审核结论(见文末「修订记录」),为最终实现口径。

## 背景

团队日常做游戏加速器、VPN 协议、UDP 转发、WireGuard、NAT 穿透、节点调度、网络路径优化等**合法**网络工程。这类工作的自然语言描述(绕过、隐藏、穿透、反检测……)会命中 OpenAI 滥用分类器,导致共享账号被判 "Cyber Abuse" 并发警告,长此以往面临封号风险。

目标:在请求送达 OpenAI **之前**,于网关侧做一道**本地 Prompt 风险前置审查**——检测 / 审计 / 告警 / 拦截 / 给改写建议,同时把**误杀**压到最低,绝不影响团队的正常工作。

### 三个已确认决策

| 决策点 | 结论 | 理由 |
| --- | --- | --- |
| 流量范围 | **经本网关** | 团队成员用 Sub2API 签发的 key,所有 Codex/Chat 流量都中转到共享上游账号。网关侧过滤看得全、绕不过,直接保护正在被警告的那批账号。 |
| 实现方式 | **扩展现有内容审核(Content Moderation / 风控)系统** | 不另起平行模块。复用其日志、邮件、自动封禁、指标、管理页与总开关。 |
| 高风险动作 | **拦截 + 返回改写建议**(用户自行改写后重发;**首版先 `observe` 校准、再切拦截**) | 非交互式 API 调用里没有"弹窗让用户确认"这种东西;绝不静默改写后转发。安全/网络团队通过**按分组豁免**(封顶为只记录 observe,不硬拦)。 |

## 关键发现(本次"查漏补缺"的核心)

Sub2API **已经具备约 80%** 的能力。原始需求文档是按"从零造轮子"写的,真正要做的是在既有能力上做一层**分级薄扩展 + 补逻辑漏洞**。两套现成系统相关:

- **`request_intercept` 中间件**([request_intercept.go](../../backend/internal/server/middleware/request_intercept.go)、[request_intercept_rules.go](../../backend/internal/service/request_intercept_rules.go)):关键词/正则命中 → 返回伪造回复。已支持**全协议 + 流式响应**、归一化(全角转半角、大小写折叠)、作用域。但它**身份无关**(无法按分组豁免)、无日志/告警/指标,且只能伪造 200 回复。
- **`ContentModerationService`**([content_moderation.go](../../backend/internal/service/content_moderation.go)):`off/observe/pre_block` 三模式、扁平 `BlockedKeywords` 列表、OpenAI moderation API、哈希缓存,以及**自动封禁、邮件事件、`ContentModerationLog`、留存清理、管理端指标**,还有**风控管理页**([RiskControlView.vue](../../frontend/src/views/admin/RiskControlView.vue))。由总开关 `risk_control_enabled` 控制。它**身份感知**(每次检查都带 UserID/GroupID/APIKeyID/Provider)。**这是正确的落点。**

方案:在 `ContentModerationService.Check` 内新增一道**分级(低/中/高)Prompt 风险前置阶段**,由**独立设置**驱动(不依赖旧内容审核配置),复用其日志/封禁/指标/看板管线。

## 与既有「请求拦截」(request_intercept) 的关系:冲突 / 合并 / 老 bug

团队此前已有一个**关键词拦截**功能(管理页「请求拦截」/ [RequestInterceptView.vue](../../frontend/src/views/admin/RequestInterceptView.vue)):命中关键词 → 返回一段**伪造的成功回复**(例:发 "hi" → "你好,我是迅游AI,有什么可以帮助你?")。需要厘清它与本设计的关系。

### 会冲突吗?——不会,但有重叠

- **执行次序**:`request_intercept` 是网关**中间件**,在 handler **之前**跑(中间件链末位,[gateway.go:53](../../backend/internal/server/routes/gateway.go#L53));prompt-risk 在 `ContentModerationService.Check` 里、handler **内部**跑。所以 intercept **先执行**——一旦命中就 `c.Abort()` 返回伪造回复,handler(及 prompt-risk)根本不运行。两者**顺序串联、intercept 命中即短路**,无运行期冲突。
- **唯一副作用**:某条 prompt 若同时命中 intercept 规则与 prompt-risk 高危词,会被 intercept 用伪造回复短路、**不进风控日志**;但它也没上游,账号无风险,影响轻微(日志里标注即可)。
- **重叠点**:两者都在做"从请求里抽取用户文本 + 关键词匹配",却各有**一套独立的抽取器与匹配器**(intercept 用 `extractRequestInterceptText` + `requestInterceptKeywordMatches`;moderation 用 `ExtractContentModerationInput`)。这正是老功能 bug 会被新功能"原样继承"的根源。

### 能合并吗?——合并「检测层」,不合并「动作层」(推荐)

两者本质是**对同一检测基质做不同动作**:

| | request_intercept | prompt-risk(本设计) |
| --- | --- | --- |
| 所处层 | 中间件(handler 前) | service(handler 内) |
| 命中动作 | 伪造 200 回复(假装成功) | 拦截 4xx + 改写建议 / 记录通知 / 放行 |
| 身份 | 中间件其实**拿得到** API Key(apiKeyAuth 在前),但目前没用 | 全程身份感知(用户/分组/key) |
| 复用基建 | 自带全协议 + 流式响应写出器 | moderation 的日志/邮件/封禁/指标/看板/留存 |
| 受众 | 给终端用户固定话术(问候/合规) | 保护共享上游账号不被滥用分类器误判 |

- **不建议整体合并**:动作模型(伪造回复 vs 拦截)与所处层不同;intercept 已是**线上可用功能**,大改有回归风险;prompt-risk 要的是 moderation 那套**日志/封禁/看板**,搬进中间件并不划算。
- **建议合并检测层**:抽出一个共享的**用户文本抽取器 + 关键词匹配器(带词边界)**,intercept 与 prompt-risk 都调用它。**一次修好、两边受益**,也避免新功能重蹈老功能覆辙。**本期必做**(见影响范围)。

### 老功能的「大 bug」:为什么很多情况下不触发

根因不是没生效,而是**匹配过于脆弱**——从单测 fixture 即可坐实:所有用例发的都是**干净的裸问候**(`{"input":"你好"}` 或最后一条 user 恰好是 `"hi"`),从没测过 "hi there"、"你好啊" 或夹在长消息里的问候。

1. **`exact` 模式 = 整条最新用户文本必须严格等于关键词。** 线上问候规则用的就是 `match: exact`(见 [request_intercept_test.go:24](../../backend/internal/server/middleware/request_intercept_test.go#L24))。于是用户只要多打了字、换了说法,或客户端在用户消息里**裹了别的内容**,归一化后就 `!= "hi"` → 不触发。真实问候千变万化("hello"、"hi 在吗"、"你好啊"),`exact` 几乎只认裸词。
2. **Codex 等编码客户端会包裹用户消息。** 团队主力是 Codex(`/v1/responses`、`/backend-api/codex/responses`),它把 `<environment_context>`、`<user_instructions>` 等作为 input 项注入,"最新一条 user"抽出来的常常**不是**用户敲的那句话,或那句话被别的内容包着,`exact` 必然落空。
3. **`contains` 又会过度命中,救不了场。** 想换 `contains`?"hi" 是 "this/which/architecture/ship" 的子串,英文几乎条条命中——短词在 `contains` 下灾难性误触发。**exact 漏、contains 滥,短词两头不讨好**——因为引擎缺**词边界**匹配(恰好就是本设计为 prompt-risk 引入的那条)。
4. **默认只扫"最新一条用户消息"(`latest_user`)。** 关键词出现在历史轮次、system、工具输出里时默认扫不到;`full_context` 模式存在但非默认,且它对 Responses 数组 input 的处理是把整段 JSON `.String()` 拼进去,较 hacky。

> 直观结论:老功能"能在演示里对裸 hi 生效、在真实 Codex 流量里基本不触发",根因是 **exact/contains 二选一 + 缺词边界 + 抽取器对编码客户端不鲁棒**。

### 本设计如何顺带修掉它

prompt-risk 的匹配器本就要做**词边界(Latin)+ 子串(CJK)+ 正则**三模式与更鲁棒的抽取。把这层抽成共享 helper 后,给 request_intercept:

- 新增 `word`(词边界)匹配模式,并作为**问候类短词规则的新默认**:"hi" 用 `word` 时,命中 "hi"、"hi there"、"oh, hi!" 里独立的 hi,但**不**命中 "this/architecture";中文问候继续走 `contains`。
- 抽取器统一到一套,修正 Codex/编码客户端"最新用户消息被包裹"的取值(取真正的用户 turn 文本)。
- 两项都落在共享 helper 里,prompt-risk 与 request_intercept **同时**拿到修复。

> 排障提示:request_intercept 有**两道**启用闸——静态 `gateway.request_intercept.enabled`(config.yaml)与管理端开关(DB,默认 true)。线上若"完全无反应",先确认**静态那道**为 true(它一关,管理页规则全失效)。本 bug 指的是"已生效但很多情况不命中",属匹配层问题,与启用闸无关。

## 逻辑查漏补缺(原需求 vs 本方案)

| 原文档的漏洞 | 本方案如何解决 |
| --- | --- |
| "要求用户确认"——非交互式 API 调用里根本没有弹窗 | 改为**拦截 + 改写建议**,用户改写后重发。安全团队走**按分组豁免**(封顶生效等级,从不拦截,但仍记日志)。 |
| "自动生成更安全的改写"易被理解成静默改写 | **仅给建议**——写在拦截消息里。绝不"改写后转发"(破坏原意、难调试、还可能把风险文本照样送上游)。 |
| 对 `scan`/`payload`/`proxy`/`bypass` 做子串匹配 | 这些恰恰是团队的正常词(port scan vs "scan the doc"、HTTP payload、reverse proxy、bypass cache)。裸 `strings.Contains` 会**亲手制造**我们要避免的误杀。Latin 词改用**词边界**匹配模式(中文无词边界,保持子串);这些双用途词默认只观察、不硬拦。 |
| 单个关键词 → 高风险太武断 | **升级评分(0~1)**:中=0.4、高=1.0、阈值=1.0 ⇒ 任一高直接拦;**≥3 个不同的中风险词(0.4×3=1.2)升级为高**(两个中=0.8 **不**升级,避免误杀 `proxy`+`payload`、`vpn`+`tunnel` 这类常见双用途词对)。按命中词去重,重复刷词不能堆分。分数归一化到 0~1 以适配日志列 `highest_score DECIMAL(8,6)`(否则 1000 会写入溢出)。 |
| 中风险要"通知用户" | 成功响应里没有干净的内联通道(注入文字会污染正文)。中风险 = **放行 + 记 `prompt_risk_observe`**(可被管理端结构化筛选,且**不**落入 pass 桶);**不**通知终端用户。管理员邮件通知留待 v1.1(见非目标),v1 仅看板可见。 |
| 客户端实现可被绕过 | **服务端网关侧实现**——绕不过,直接护住共享账号。 |
| 未指明 fail-open / fail-closed | 配置加载失败时**一律 fail-open**(与现有 `Check` 一致;本功能默认 off/observe,保护对象是共享账号可用性,fail-closed 会因一处配置错误拦死整组流量,故 v1 不提供该选项)。四类错误:setting 缺失→默认配置、JSON 损坏→放行+日志、regex 编译失败→**保存时**校验拒绝、repo 不可用→放行+暴露运行态。 |
| 未指明作用域(哪些供应商/账号) | provider 级(`input.Provider`)只是粗近似——它来自分组 platform,**不等于**最终命中的共享 OAuth 账号。改用 **group 级 opt-in**(只在共享 OAuth 池所在分组启用),精度更高;`GroupID` 每次检查都带。 |
| 记录风险 prompt 涉及隐私/留存 | 复用 `ContentModerationLog`:`InputExcerpt` 已限 240 rune + 密钥脱敏 + 既有留存清理;命中词写入 `category_scores`(JSONB)便于审计。**无需 DB migration**——前提是分数归一化到 0~1 适配现有 `DECIMAL(8,6)` 列。 |

## 实现设计

一个纯函数评估器 + 在 `Check` 里插入一小段。**无需改任何 handler**——11 个网关 handler 调用点本就是 `if decision != nil && decision.Blocked { …按协议返回带 decision.Message 的错误… }`,所以返回一个 `Blocked` 决策,就会在**全协议 + 流式**下自动渲染拒绝 + 改写建议。

### 数据模型 — 新文件 `backend/internal/service/prompt_risk.go`

纯函数、表驱动、无 I/O(对标 [token_analysis_risk.go](../../backend/internal/service/token_analysis_risk.go)):

```go
const (
    PromptRiskLevelLow, PromptRiskLevelMedium, PromptRiskLevelHigh = "low", "medium", "high"
    PromptRiskModeOff, PromptRiskModeObserve, PromptRiskModeBlock  = "off", "observe", "block"
    PromptRiskActionAllow, PromptRiskActionLogNotify, PromptRiskActionBlock = "allow", "log_notify", "block"
    PromptRiskMatchContains, PromptRiskMatchRegex, PromptRiskMatchWord = "contains", "regex", "word"
)

type PromptRiskKeywordSet struct { Level, MatchMode string; Keywords []string; Score float64 } // 分数 0~1
type PromptRiskExemption  struct { GroupIDs, UserIDs, APIKeyIDs []int64; MaxLevel string }     // 封顶等级,如 "medium";封顶后只记不拦
type PromptRiskSubject    struct { UserID int64; GroupID *int64; APIKeyID int64; Provider string }

type PromptRiskConfig struct {
    Enabled    bool                         // 功能总开关(默认 false)
    Mode       string                       // off / observe / block;首版默认 observe(只记不拦)
    AllGroups  bool                         // 是否对所有分组生效
    GroupIDs   []int64                      // group 级 opt-in:仅这些分组生效(空 + !AllGroups ⇒ 不生效)
    InputScope string                       // newest(默认,仅最新一轮意图)/ full(全历史 user turn,显式 opt-in)
    BlockStatus       int                   // 403
    EscalateThreshold float64               // 1.0
    BlockMessage, RewriteSuggestion string  // 改写建议拼接到拦截消息
    KeywordSets []PromptRiskKeywordSet
    Exemptions  []PromptRiskExemption
}
// 注:v1 不提供 FailOpen(一律 fail-open)、AutoBanOnHigh / AdminEmail*(副作用留待 v1.1);
// 避免暴露后端不执行的死开关。详见"非目标 / v1.1"。

type PromptRiskReason   struct { Level, Keyword, Source string; Score float64 }
type PromptRiskDecision struct { Level, Action string; Score float64; Reasons []PromptRiskReason }

func DefaultPromptRiskConfig() PromptRiskConfig                              // 默认 Mode=observe、InputScope=newest
func EvaluatePromptRisk(cfg *PromptRiskConfig, text string, subj PromptRiskSubject) PromptRiskDecision
func (c *PromptRiskConfig) includesGroup(groupID *int64) bool               // AllGroups || groupID ∈ GroupIDs
func (c *PromptRiskConfig) validateRaw() error                              // 保存时先校验原始枚举(非法即拒,不静默纠正)
func promptRiskKeywordMatches(mode, loweredText, keyword string) bool       // "word" 走词边界;导出供 request_intercept 共用
```

**`EvaluatePromptRisk` 判定逻辑**:文本统一转小写一次;对每个关键词集逐词匹配(Latin 走词边界,用预编译 `(^|[^\p{L}\p{N}])kw([^\p{L}\p{N}]|$)`;中文走子串;`regex` 走正则);累加分数(0~1,按命中词去重)。任一高命中 **或** `score ≥ EscalateThreshold(1.0,即 ≥3 个不同中风险词)` → block;否则任一中 → log_notify;否则 allow。随后两道收敛:① 套用匹配到的豁免 `MaxLevel` 封顶(如安全分组封到 `medium`,高降级为 log_notify);② 按全局 `Mode` 收敛——`observe` 把 block 一律降为 log_notify(只记不拦)。

### 默认关键词集(为保护团队而调校)

> 默认词表按"双用途 → 观察、明确恶意 → 拦截"重排;且**首版 `Mode=observe`,以下 block 仅记录不拦,校准后再切**。

- **低(放行)**:空。"不命中即放行";正常词(udp、wireguard、"nat traversal"、网络优化)只要**不命中**即可——靠词边界 + 作用域,而非白名单。
- **中(`log_notify`,0.4 分)——双用途、需观察:** Latin `word`:`vpn, proxy, tunnel, socks5, openvpn, shadowsocks, v2ray, trojan, bypass, payload, spoof, obfuscate, stealth, evade, evasion, enumerate, exploit, nmap`;一条 `regex`:`\bport\s+scan\b`;中文 `contains`:`代理, 翻墙, 梯子, 隧道, 绕过, 隐藏, 规避, 反检测, 反审计, 反追踪, 伪装, 渗透, 端口扫描, 枚举`。
- **高(`block`,1.0 分)——明确攻击工具 / 恶意动作:** Latin `word`:`sqlmap, metasploit, masscan, hydra, nuclei, gobuster, ffuf, nikto, bruteforce, backdoor`;中文 `contains`:`爆破, 提权, 后门, 木马, 免杀, 撞库, 漏洞利用, 凭据窃取`。
- 注:① `bypass/绕过/payload/proxy/nmap/端口扫描` 等**双用途**词全部下放观察层——它们正是团队合法网络工程的高频词(审计举的"游戏加速器绕过 DPI""nmap 扫自有节点""reverse proxy payload"都不应硬拦)。② 真正升高危应靠**组合**(攻击动词 + 未授权/第三方目标信号),留作后续增强;v1 高危只放语义明确的攻击工具名与恶意动作。③ 刻意不收裸 `scan`。

### 在 `Check` 中的插入点 — [content_moderation.go:752](../../backend/internal/service/content_moderation.go#L752)

在 **`isRiskControlEnabled` 之后、`loadConfig` 之前**插入风险阶段,并**独立加载**自己的配置——这样旧 `content_moderation_config` JSON 即便损坏(`loadConfig` 失败 fail-open),Prompt 风险仍照常运行。总开关 `isRiskControlEnabled`(763 行)仍是总闸。示意:

```go
if !s.isRiskControlEnabled(ctx) { return allow, nil }   // 既有总闸

// 新增:Prompt 风险阶段(独立加载,不依赖旧 content_moderation_config)
if prCfg := s.loadPromptRiskConfig(ctx); prCfg.Enabled && prCfg.Mode != PromptRiskModeOff &&
    prCfg.includesGroup(input.GroupID) {                                 // group 级 opt-in
    content := extractPromptRiskInput(input.Protocol, input.Body)        // 专用抽取:剥环境上下文、取本轮 user turn
    if !content.IsEmpty() {
        content.Normalize()
        d := EvaluatePromptRisk(&prCfg, content.Text, promptRiskSubject(input))  // 已套豁免 + Mode 收敛
        switch d.Action {
        case PromptRiskActionBlock:
            s.recordPreBlockSyncMetric(0, ContentModerationActionPromptRiskBlock)   // 已纳入 blocked 计数
            log := s.buildPromptRiskLog(input, ContentModerationActionPromptRiskBlock, true, "prompt_risk_"+d.Level, d.Score, content.ExcerptText(), d.Reasons)
            s.enqueueRecord(input, nil, log, content.Hash(), false, false)           // v1 不喂自动封禁/邮件副作用(留待 v1.1)
            msg := strings.TrimSpace(prCfg.BlockMessage + "\n" + prCfg.RewriteSuggestion)
            return &ContentModerationDecision{Blocked: true, Flagged: true, Message: msg,
                StatusCode: orDefault(prCfg.BlockStatus, 403), Action: ContentModerationActionPromptRiskBlock,
                HighestCategory: "prompt_risk_" + d.Level, HighestScore: d.Score}, nil
        case PromptRiskActionLogNotify:
            log := s.buildPromptRiskLog(input, ContentModerationActionPromptRiskObserve, false, "prompt_risk_medium", d.Score, content.ExcerptText(), d.Reasons)
            s.enqueueRecord(input, nil, log, content.Hash(), false, false)           // observe:从不喂封禁
            // 继续走既有内容审核流程
        }
    }
}

cfg, err := s.loadConfig(ctx)                            // 既有内容审核;失败 fail-open
if err != nil { return allow, nil }
// …既有内容审核逻辑原样继续…
```

- **专用抽取器 `extractPromptRiskInput(protocol, body, scope)`**:不直接复用只看末项的 `ExtractContentModerationInput`(对 Codex 多 item/工具循环会漏扫);剥离 `<environment_context>`/`<user_instructions>`。默认 `newest`——定位最新 user item 并纳入**同轮**相邻 user item(覆盖 Codex 把环境上下文与真实问题拆成多 item)、跳过尾部工具调用/输出,**不回溯历史轮次**(避免多轮里上一轮高危在"谢谢/继续"等普通后续上重复触发);`full` 为显式 opt-in(扫全部 user turn)。与 request_intercept **共享**词边界匹配器与 wrapper 剥离。
- **`buildPromptRiskLog`**:不依赖旧 `cfg`(Mode 固定 `prompt_risk`、空 thresholds),命中词 `Reasons` 以 `level:source:keyword → score` 扁平化写入 `category_scores`(JSONB),保留等级/匹配模式便于审计筛选(复用现有列,无需 migration;完整结构化快照留待 v1.1)。
- **副作用**:`enqueueRecord` 末参 `applySideEffects` 传 `false`——prompt-risk **不走**既有自动封禁/邮件副作用(封禁针对用户账号,合法队友不应被封)。若团队日后要"高危计封禁",经独立开关 + 专属阈值实现(v1.1),且必须把 prompt-risk 行**排除出** `CountFlaggedByUserSince`(已实现),避免污染内容审核的封禁计数。
- **词边界正则缓存**:Prompt 风险配置加载时一次性编译 `word`/`regex` 匹配器;进程内按配置哈希缓存、约 60s TTL(与 `setting_service.go` 一致的套路),避免每请求查库或每请求 `regexp.Compile`。**不要**复用 `requestInterceptKeywordMatches` 那种每次 `regexp.MatchString`。

## 影响范围

### 后端

| 文件 | 改动 |
| --- | --- |
| [domain_constants.go](../../backend/internal/service/domain_constants.go)(约 140 行) | 在 `SettingKeyContentModerationConfig` 旁加 `SettingKeyPromptRiskConfig = "prompt_risk_config"`;加 action 常量 `ContentModerationActionPromptRiskBlock = "prompt_risk_block"` / `...PromptRiskObserve = "prompt_risk_observe"` |
| `backend/internal/service/prompt_risk.go`(新增) | 上述结构体(分数 `float64` 0~1、全局 `Mode`、group 级 `includesGroup`、拆分的副作用开关)、`DefaultPromptRiskConfig`(默认 `Mode=observe`)、`EvaluatePromptRisk`(豁免 + Mode 收敛)、**导出**的词边界匹配器(共享给 request_intercept)、编译匹配器缓存 |
| `backend/internal/service/prompt_risk_input.go`(新增) | `extractPromptRiskInput(protocol, body, scope)`:剥离 wrapper、按 `scope` 取最新一轮(`newest` 默认,含同轮多 item、跳过尾部工具输出)或全历史(`full`)user turn |
| [content_moderation.go](../../backend/internal/service/content_moderation.go) | 风险阶段插在 **`loadConfig` 之前**;`loadPromptRiskConfig`(带缓存)+ `GetPromptRiskConfig`/`UpdatePromptRiskConfig`(`validateRaw` 拒非法枚举原值 → `normalize` → `Validate` 校验 regex 可编译 + 分组存在)/`TestPromptRisk`;`buildPromptRiskLog`(不依赖旧 cfg、reasons→`category_scores`);`recordPreBlockSyncMetric` 把 `prompt_risk_block` 计入 blocked;`promptRiskSubject(input)` 映射(无需改 input) |
| [content_moderation_repo.go](../../backend/internal/repository/content_moderation_repo.go) | `buildContentModerationLogWhere` 的 `blocked` 加入 `prompt_risk_block`、新增 `observe` 结果桶(`action = 'prompt_risk_observe'`);`CountFlaggedByUserSince` 加 `AND action NOT LIKE 'prompt_risk_%'`(prompt-risk 不进封禁计数) |
| [admin/content_moderation_handler.go](../../backend/internal/handler/admin/content_moderation_handler.go) | 加 `GetPromptRisk`/`UpdatePromptRisk`/`TestPromptRisk`(测试 handler 对标 `RequestInterceptHandler.Test`,对粘贴的 prompt 返回 `{matched, decision}`) |
| [routes/admin.go](../../backend/internal/server/routes/admin.go) `registerContentModerationRoutes`(137 行) | 在既有 `risk := admin.Group("/risk-control")` 加 `GET /prompt-risk`、`PUT /prompt-risk`、`POST /prompt-risk/test` |

> **不**改 `ContentModerationCheckInput`、[content_moderation_helper.go](../../backend/internal/handler/content_moderation_helper.go)、各网关 handler、`wire.go`——`ContentModerationService` 已构造注入,决策完全由 input 上已有数据生成。

### 合并检测层(本期必做):顺带修掉 request_intercept 的 bug

本期共享**词边界匹配器 + wrapper 剥离**(动作层仍分离),一次修掉老功能"很多情况不触发"的 bug。**完整文本抽取器统一留待 v1.1**:prompt-risk 与 request_intercept 均收敛到 newest 口径后,同一请求的命中文本已基本一致;而 request_intercept 抽取器位于中间件热路径,贸然重构的回归风险大于收益。

| 文件 | 改动 |
| --- | --- |
| `backend/internal/service/prompt_risk.go` | 词边界匹配器导出为可复用函数,供 prompt-risk 与 intercept 共用 |
| [request_intercept_rules.go](../../backend/internal/service/request_intercept_rules.go) | `MatchMode` 增加 `word`(词边界)模式,走共享匹配器;`normalizeRequestInterceptMatchMode` 识别它;新建短词问候规则默认用 `word` |
| [request_intercept.go](../../backend/internal/server/middleware/request_intercept.go) | 抽取器修正 Codex/编码客户端"最新用户消息被包裹"的取值(取真正的用户 turn 文本) |
| [RequestInterceptView.vue](../../frontend/src/views/admin/RequestInterceptView.vue) + i18n | 匹配模式下拉增加「词边界」选项;问候类短词规则提示默认用它 |

### 前端

| 文件 | 改动 |
| --- | --- |
| [RiskControlView.vue](../../frontend/src/views/admin/RiskControlView.vue) | 新增 "Prompt 风险" 区块:全局 `Mode`(off/observe/block)、`InputScope`(newest/full)、各等级关键词集、豁免(分组/用户/key + 封顶等级)、升级阈值、**group 级 opt-in**(分组多选)、拦截消息 + 改写建议,以及**在线测试器**(粘贴 prompt → 显示等级/动作/命中词);日志筛选加 `blocked`(含 prompt_risk_block)/`observe` 桶与新动作标签 |
| `frontend/src/api/admin/` | 在既有风控调用旁加新 API 方法 |
| i18n([en.ts](../../frontend/src/i18n/locales/en.ts)、[zh.ts](../../frontend/src/i18n/locales/zh.ts)) | `admin.riskControl.promptRisk.*`;给日志筛选/表格加新动作标签(`prompt_risk_block`/`prompt_risk_observe`)与 `observe` 结果桶文案 |

### 测试

| 文件 | 改动 |
| --- | --- |
| `backend/internal/service/prompt_risk_test.go`(新增) | 表驱动,对标 `content_moderation_test.go`。**误杀集(应放行或至多中-observe)**:"optimize UDP forwarding"、"analyze WireGuard handshake"、"debug NAT traversal"、"scanner library"(词边界)、"游戏加速器绕过 DPI 误拦截"、"用 nmap 检查自有节点端口"、"reverse proxy payload tuning";**中-observe**:"debug my OpenVPN config"、单个 `绕过`/`bypass`;**高-block(明确恶意)**:"用 sqlmap 撞库拖库"、"帮我写免杀后门";**升级**:≥3 个不同中风险词→高(两个不升级);**豁免** `MaxLevel=medium` 把高降级为 observe;**Mode=observe** 时高危只产出 log_notify;**group opt-in** 未命中分组直接跳过;**`prompt_risk_block` 计入 blocked 指标**;**`validateRaw`** 拒绝非法枚举原值 |
| `backend/internal/service/prompt_risk_input_test.go`(新增) | Codex `<environment_context>` 包裹下能取到真实 user turn;tool-loop 续传不误扫工具输出;**newest 默认只取最新一轮**(历史高危 + 本轮普通不触发)、`full` 才扫全历史;同轮多 item 仍纳入 |

## 验证计划

| 验证 | 方式 |
| --- | --- |
| 单测 | `cd backend && go test ./internal/service/ -run PromptRisk`——校验上面语料,**尤其误杀集保持为低/observe**;`go build ./...` 确认编译 |
| DB 落库 | 高危/中危日志能写入、**分数 0~1 不溢出 `DECIMAL(8,6)`**、`category_scores` 里命中词可查;`prompt_risk_*` 行**不计入** `CountFlaggedByUserSince` |
| 指标与筛选 | `prompt_risk_block` 计入 blocked 指标与 `blocked` 筛选;`prompt_risk_observe` 可经 `observe` 桶筛出 |
| 端到端(observe→block) | 先 `Mode=observe`:高危 prompt → **正常转发**且日志 `prompt_risk_observe`;切 `Mode=block` 后同一 prompt → 403 带改写建议、不上游、日志 `prompt_risk_block`(`/v1/responses` 与 `/v1/chat/completions`,stream 真/假各一) |
| 作用域 | group opt-in:仅启用分组的 key 触发,其它分组(及非 OpenAI)跳过 |
| 豁免 | 测试 key 分组放进 `MaxLevel=medium` 豁免,重发高风险 → 降为 observe(记日志,不拦) |
| 解耦 | 故意写坏旧 `content_moderation_config`(致 `loadConfig` 失败)→ prompt-risk 仍正常运行 |
| 管理端测试器 | 各 prompt 粘进在线测试器,确认等级/动作/命中词 |
| 回归 | 既有内容审核(`pre_block` + OpenAI API)与 `request_intercept`(含新增 `word` 模式)行为符合预期;风险阶段独立于 `request_intercept` |

## v1 已知局限与 v1.1 计划

- **账号池隔离(v1 不覆盖的已知残留风险 / v1.1 必做)**:v1 用"拦截 + 改写建议"闭环 + 豁免=封顶 observe 来缓解,但**被豁免/被放行的合法高风险流量仍走同一批共享 OAuth 账号**,即仍可能触发 OpenAI 滥用分类器——这是 v1 的已知残留风险。**v1.1 必做**:安全/网络团队的合法高风险工作**路由到独立上游账号池**,并监测上游 `policy_violation` 错误对受影响账号**自动降权/摘除**,形成真正的账号级闭环。
- **账号级精确作用域(v1.1)**:v1 以 group 级 opt-in 近似;若同一分组混用 OAuth/API Key 等多种账号类型,需把检查下沉到账号选择后,或用分组绑定的账号池元数据判断。

## 非目标(后续单列)

- 绕开网关、直连 ChatGPT Pro Web 的用法(客户端侧防护是本仓库之外的独立工作)。
- LLM 动态改写(额外成本/时延;先用静态 `RewriteSuggestion`)。
- 面向管理员(而非终端用户)的违规邮件事件标准化——v1 仅风控日志看板(无邮件;开关随死配置一并移除,留待 v1.1)。

## 修订记录

- **2026-06-22**:经 Codex 设计审核([codex-prompt-risk-filter-design-audit-cn.md](codex-prompt-risk-filter-design-audit-cn.md))核验,确认其「关键问题清单」代码级论断全部属实,已将结论**直接折叠进本设计正文**(最终口径,非附加回应):分数归一化 0~1(修 `DECIMAL(8,6)` 溢出)、删除 `EmailOnHigh`/`NotifyOnMedium` 改为拆分副作用开关且 v1 不喂自动封禁、默认词表按"双用途→observe / 明确恶意→block"重排并首版 `Mode=observe`、新 action 纳入 metric/筛选/前端、风险阶段上移到 `loadConfig` 之前并独立加载、作用域改 group 级 opt-in、新增专用抽取器、request_intercept `word` 模式合并升为本期必做;账号池隔离列为 v1 已知局限 / v1.1。
- **2026-06-22(实现审核)**:经 Codex 实现审核([codex-prompt-risk-filter-implementation-audit-cn.md](codex-prompt-risk-filter-implementation-audit-cn.md))核验落地代码,采纳并修复:`prompt_risk_block` 计入 blocked 指标(P0-1);抽取改为 `input_scope=newest` 默认、`full` 显式 opt-in,避免多轮里历史高危在"谢谢/继续"等普通后续上重复触发(P0-2);删除热路径不执行的死配置 `FailOpen` 与 `AutoBan/AdminEmail*`(P1-1/P1-2,明确**只支持 fail-open**);保存改 `validateRaw` 先拒非法枚举原值再 normalize(P1-3);`pass` 桶排除 `prompt_risk_observe`(P1-4);`category_scores` 键改 `level:source:keyword` 保留等级(P2-1)。`request_intercept` 完整抽取器统一(P1-5)、结构化 reasons 快照、管理员邮件均评估后**列入 v1.1**(理由见正文)。
