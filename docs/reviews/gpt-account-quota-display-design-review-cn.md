# 《GPT 账号额度展示》设计审核报告

被审文档：`docs/features/gpt-account-quota-display-design-cn.md`（日期 2026-09-23，状态：待审核，未实施）
审核日期：2026-09-23 初稿；2026-09-24 修订（变更见 §0.1）
代码基线：分支 `feature/hy/10208_sub_admin_assign_subscription`（`main@d120d499e`）
审核方式：对照设计稿引用的现有实现做源码核对，并按本仓库后台任务、账号类型、路由挂载和前端组件惯例评估方案重量。未执行迁移、未启动服务、未做真实采集。

本报告是审核意见，不是已采纳的设计，也不是已实现行为。采纳前不得按本文修改业务代码或迁移。

---

## 0. 给审阅者的背景（自包含摘要）

被审设计要做的事：给所有登录用户一个只读页面，共享展示 GPT / OpenAI 上游账号额度。桌面左列「迅游」对应账号原名 `c-` 前缀，右列「速宝」对应 `d-` 前缀；手机上下排列。管理员勾选参与展示的账号，并配置北京时间白天的自动采集。用户不能刷新、测试或重置额度。

设计稿的关键选型：

- 用户读取只返回已保存快照，缺数据、过期、失败、反复刷新都不得打上游。
- 数据源抽取 `OpenAIQuotaService` 的 wham usage 查询，不附带 reset-credit 明细，也不复用会发模型探测的 `getOpenAIUsage`。
- 展示默认关闭，初始不勾选账号。
- 定时与手动采集共用持久化任务、槽位唯一键、租约和执行代次。

**审核总评**：读写隔离、默认关闭、避开模型探测入口，这三条约定和现有代码对齐，应保留。实施前必须先改设计，共有 **3 个阻断级问题**、**1 个 P1 问题**、**4 项过重设计**和 **11 条需补进设计的遗漏**。其中最大的一项是 P0-1：设计没有考虑网关已经写入账号的被动额度快照。这一项决定第 5–8 节要不要整体改写，应最先定。60 分钟档是否在 18:00 收尾仍是产品选择，不阻塞结构决定。

### 0.1 修订记录（2026-09-24）

| 变更 | 说明 |
| --- | --- |
| 新增 P0-1 | 设计遗漏网关写入 `accounts.extra` 的 Codex 被动快照；主动采集应降为兜底 |
| 原 P0-1 降为 P1-1 | token provider 的禁用路径在账号首个网关请求时同样会触发；改为采集前预检，不另造 token 路径 |
| 原 P0-2 修订 | 撤回「管理端账号页与本页允许不一致」；主动兜底结果写调度中立键 |
| 原 P0-3 补充 | 窗口判定改为与自动重置一致的区间规则，不按秒数精确匹配 |
| 过重设计 | §4.1 补充仓库内对口模板 `ollama_cloud_usage.go`；新增 §4.3 三重防重、§4.4 用户 DTO 暴露运维状态；§4.2 扩到页面轮询间隔和双开关 |
| 遗漏 | 新增 §5.6–§5.11：渠道监控额度模式重叠、组织键命名、菜单开关、管理员限流豁免、前端组件复用、可见人群 |
| 章节号 | 原 §3 过重设计改为 §4，原 §4 遗漏改为 §5，原 §5 结论改为 §6 |

---

## 1. 已核实为准确的引用

| # | 设计稿论断 | 核实位置 | 结论 |
| --- | --- | --- | --- |
| 1 | `QueryUsage` 请求 `GET /backend-api/wham/usage`，并额外查询 reset-credit 明细 | `backend/internal/service/openai_quota_service.go:27-29`、`:146-213`（成功解析 usage 后调用 `queryResetCreditDetails`） | 属实 |
| 2 | 单次上游超时现为 20 秒 | 同文件 `openaiQuotaUpstreamTimeout = 20 * time.Second`；usage 与 reset-credit 两次 GET 共用这一个预算（`:157`、`:196`） | 属实 |
| 3 | `getOpenAIUsage` 可能发 Responses 模型探测；窗口缺失时补出 0 使用率 | `backend/internal/service/account_usage_service.go:709-769`（普通账号走 `probeOpenAICodexSnapshot`；`:755-764` 在窗口为 nil 时写入 `Utilization: 0`） | 属实 |
| 4 | Spark 影子没有自己的凭据，配额查询会解析到母账号 | `openai_quota_service.go:423-431`（`IsShadow` 后 `resolveCredentialAccount`） | 属实 |
| 5 | 渠道监控配额模式复用账号用量服务，不适合当本功能数据源 | `backend/internal/service/channel_monitor_quota_fetcher.go:232-236`（海外平台走 `GetUsageForAccount`） | 属实；但设计缺否决其他方案的论证，见 §5.6 |
| 6 | 子管理员对未知管理路由默认拒绝 | `backend/internal/service/admin_permission.go:151-174`（白名单精确匹配，未命中返回 false） | 属实 |
| 7 | 用户已认证路由带 JWT、面板限流和审计 | `backend/internal/server/routes/user.go:20-26` | 属实（同组还有后台模式拦截，见 §5.3） |
| 8 | 上游未用窗口用 nil 表示无数据；使用率在本仓库按 0–100 解释 | `OpenAIRateLimitWindow` 为指针；`resolveOpenAIQuotaUtilization` 返回前执行 `usedPercent / 100`（`openai_gateway_scheduling.go:670-683`） | 属实 |

抽取「只查 usage、不查 reset-credit」的窄方法，以及用户读路径禁止调用 `getOpenAIUsage`，都是必要约束。`QueryUsage` 本身不写库，也没有缓存或 singleflight。

---

## 2. 阻断级问题（P0，实施前必须修改设计）

### P0-1　数据源遗漏网关被动快照，主动采集应降为兜底

设计第 3.2 节把「数据采样时间」定义为「上游 usage 请求成功并解析有效结果的时间」，第 6 节据此设计整套定时采集。设计没有提到：网关转发 OpenAI OAuth 响应时，已经把上游 `x-codex-*` 限额响应头写进账号 `extra`。

```go
// backend/internal/service/openai_gateway_forward.go:1309-1312
if account.UsesOpenAICodexProtocol() && !account.IsShadow() {
    if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
        s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
    }
```

- **写入字段**：`codex_5h_used_percent`、`codex_5h_reset_at`、`codex_5h_window_minutes`，`codex_7d_*` 的同组字段，以及 `codex_usage_updated_at`。见 `openai_gateway_usage.go:1106-1167`，时间戳在 `:1136`。
- **写入频率**：同账号最快 30 秒落库一次（`openai_gateway_service.go:66` 的 `openAICodexSnapshotPersistMinInterval`）；遇到 429 时同步落库（`ratelimit_service.go:1186`）。
- **写入路径**：Chat Completions（`openai_gateway_chat_completions.go:530`）、Messages（`openai_gateway_messages.go:540`）、passthrough（`openai_gateway_passthrough.go:516`）同样写入。
- **不影响调度**：这组键都在调度中立前缀内（`backend/internal/repository/account_repo.go:54-70`），读取它不会改变调度。

09:30—18:00 正是业务流量时段。有流量的账号，被动快照比每 30 分钟一次的主动采集更新，而且不额外请求上游。照设计现稿，用户页反而显示更旧的数据，并且和读同一组 extra 的管理端账号页对不上。

**设计必须写成**：

1. **用户页首选被动快照。**
   - 直接读 extra 的原始 `codex_5h_*` / `codex_7d_*`，用 `codex_usage_updated_at` 作采样时间。
   - 只采用同时写有对应 `*_window_minutes`、且时长落在正确区间的窗口：5 小时窗口 ≤ 360 分钟，7 天窗口 > 360 分钟。这样可以避开 `Normalize` 在缺时长时按「primary=7d」旧映射归类的分支（`openai_gateway_service.go:197-200`）。
   - 禁止经过 `buildCodexUsageProgressFromExtra`（见 P0-2）。
2. **主动 wham/usage 只做兜底。**
   - 已选账号的被动快照超过阈值（建议 30 分钟）没有更新时，在计划时点补采。这类账号通常闲置、被暂停或正在限流。
   - 管理员手动刷新也走主动查询。
3. **两份来源按时间取较新的一份。** 采样时间如实标注来源（响应头 / usage 查询）。

改完后，定时任务只处理少数闲置账号，§4.1 的精简方案就够用。设计第 3.2 节「数据采样时间」「最近尝试时间」的定义、第 7 节的快照表都要随之改写。

### P0-2　展示采集不得写入调度与自动用卡读取的 extra 键

网关暂停和自动重置读的是账号 `extra` 里的 `codex_5h_*` / `codex_7d_*`。`resolveOpenAIQuotaUtilization` 用这份 extra 决定是否暂停（`openai_gateway_scheduling.go:521-537`、`:670-683`）。

- `QueryUsage` 自己不写库，写回窗口的是另外两条路径：
  - 自动重置：`persistFreshUsage`（`openai_quota_auto_reset.go:584-592`）；
  - 重置后回写：`CachePostResetSnapshot`（`openai_quota_service.go:232-241`）。
- 这两条路径都会顺带缓存 reset-credit。网关被动写入成功后还会 `notifyOpenAIAutoReset`（`openai_gateway_usage.go:1194-1196`）。

设计只禁止「调用额度重置接口」，没有禁止上述写路径。展示采集一旦把主动查询的新值写进这组键，就会带来两个变化：

- 自动暂停的陈旧兜底会失效。快照超过 2 小时就放行一次请求让账号自愈，见 `openai_gateway_service.go:67-70`。
- 自动用卡的触发时机会改变。

这和「不改调度、不重置额度」冲突。

**设计必须写成**：

- 主动兜底结果只写新的调度中立键，例如 `gpt_quota_display_snapshot`，或者写独立表。
  - 写 extra 时必须把前缀加进 `schedulerNeutralExtraKeyPrefixes`，参照已在列表中的 `ollama_cloud_usage`（`account_repo.go:54-64`）。
  - 否则每次写快照都会触发调度 outbox（`account_repo.go:2874-2885`）。
- 不得写 `codex_5h_*`、`codex_7d_*`、`codex_usage_updated_at`。
- 不得调用 `notifyOpenAIAutoReset`，也不得调用 `CacheResetCreditsSnapshot`。
- 按 P0-1 读 extra 原始键后，本页与管理端账号页口径一致。初稿「两边允许不一致」的说法撤回。

同时点名禁止复用 `buildCodexUsageProgressFromExtra`。该函数在重置时间已过时把使用率改成 0（`account_usage_service.go:1537-1540`），界面会变成剩余 100%。设计要求显示「已到重置时间，待更新」，这条现成函数会破坏该口径。

### P0-3　按秒数扫整包 usage，会把带 Spark 窗口的成功响应判成冲突

设计第 3.2 节：用 `limit_window_seconds` 识别 18000 与 604800；同长度窗口冲突则判无效，不生成满额结果。设计没有限定 JSON 路径。

`/wham/usage` 的主额度在 `rate_limit`，Spark 在 `additional_rate_limits`。仓库测试夹具里 `codex_bengalfox` 用的就是这两个秒数：

```go
// backend/internal/service/openai_quota_spark_window_test.go:117-126
PrimaryWindow: &OpenAIRateLimitWindow{
    UsedPercent:        0.42,
    LimitWindowSeconds: 18000,
    ResetAfterSeconds:  3600,
},
SecondaryWindow: &OpenAIRateLimitWindow{
    UsedPercent:        0.15,
    LimitWindowSeconds: 604800,
```

母账号同一次响应可以同时带主窗口和 Spark 窗口。若实现扫整包，两个 5 小时窗口会命中「同长度冲突」，成功响应变成 `no_supported_windows`，页面一直留着旧快照。

另外，设计按 18000 / 604800 精确匹配，比仓库其他地方的口径更严：自动重置用 `LimitWindowSeconds <= 6*60*60`（`openai_quota_auto_reset.go:648`），`Normalize` 用 ≤ 360 分钟。上游时长稍有变化，就会出现账号页显示 5h、本页显示「未提供」。

**设计必须写成**：

- 只读 `rate_limit.primary_window` 与 `rate_limit.secondary_window`，忽略 `additional_rate_limits`。
- 两个窗口按与自动重置一致的规则判定：时长大于 0 且 ≤ 6 小时为 5 小时窗口，大于 6 小时为 7 天窗口。
- 两个窗口落入同一类，或时长 ≤ 0 时，判为无效并显示「未提供」。
- 不使用 `OpenAICodexUsageSnapshot.Normalize`：它在缺时长时有旧映射，见 `openai_gateway_service.go:149-220`。

---

## 3. P1 问题（实施前应写进设计）

### P1-1　展示采集可能沿 token 路径禁用账号（原 P0-1，降级）

现有 provider 在 access token 已过期且没有 refresh token 时，会永久摘除账号：

```go
// backend/internal/service/openai_token_provider.go:159-166
needsRefresh := !account.IsOpenAIPersonalAccessToken() && (expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew)
if needsRefresh && strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
    if expiresAt != nil && !time.Now().Before(*expiresAt) {
        const reason = "openai access_token expired and refresh_token is missing"
        p.disableAccountMissingRefreshToken(account, reason)
```

`disableAccountMissingRefreshToken`（同文件 `:279-307`）调用 `BlockAccountScheduling` 和 `accountRepo.SetError`。Agent Identity 也不是只读：`buildCodexQuotaHeaders` 发请求前会调用 `ensureAgentIdentityTaskForAccount`，失败响应还会 `recoverAgentIdentityTask`（`openai_quota_service.go:476-493`、`:532-537`）。

**降级理由**：同一判断在该账号的首个网关请求里也会执行。对参与调度的账号，展示采集只是提前触发，不是新增风险。真正受影响的是只展示、不参与调度或长期闲置的账号。

**设计应写成**：不另造 token 路径。窄方法在调用 token provider 之前做同条件预检：账号不是 PAT、`expires_at` 已过期、且没有 refresh token，就记 `token_unavailable` 并跳过。首版资格排除 Agent Identity（§5.1），这样就不会触发 task 恢复。

---

## 4. 过重设计（实施前应收）

### 4.1　持久化任务、租约和 7 天明细超过本仓库惯例

设计第 6–7 节要求很重：

- 配置版本 + 计划时刻唯一键、数据库领取、有期限租约、执行代次；
- 快照与任务同一事务，任务明细保留 7 天；
- 用多实例测试证明旧 worker 不能覆盖新快照；
- 快照行和任务行各有一套 generation。

仓库里已有几乎对口的模板 `backend/internal/service/ollama_cloud_usage.go`（Ollama Cloud 官方用量）：

- 配置存为 settings JSON；账号快照写 extra 中立键 `ollama_cloud_usage_snapshot`（`:30-33`）。
- 单主运行用 `tryAcquireSingletonLeaderLock`（`:732`）。它优先用 Redis，出错时回落到 PG advisory lock，两者都没有时不设门控（`leader_lock.go:40-68`）。
- 手动刷新 30 秒冷却，并返回 `retry_after_seconds`（`:50`、`:822-836`）。
- 同时最多刷新 4 个账号，同账号请求用 singleflight 合并（`:55`、`:800-804`）。
- 失败保留旧数据（`persistFailure`，`:954`）；按 Retry-After 退避（`nextOllamaCloudUsageDelay`，`:1211`）。

同类的 OpenAI 自动重置也只用 leader lock：锁键 `jobs:openai-auto-reset-credit`，拿不到锁就跳过本轮（`openai_quota_auto_reset.go:29`、`:246`）。

部署现状：手册允许多实例（`docs/ARCHITECTURE_AND_OPS_HANDBOOK.md:195-202`），但随仓库发布的 compose 只有一个固定 `container_name: sub2api` 的应用容器（`deploy/docker-compose.yml:18-20`）。

本功能按 P0-1 改成兜底后，每轮只补采少数闲置账号，上游调用是只读 GET。计划时刻去重不需要任务表，用 leader lock 加单行条件更新即可：`UPDATE … SET last_slot_at = $slot WHERE last_slot_at IS NULL OR last_slot_at < $slot`，受影响 0 行就跳过。不要照搬 `UserConcurrencyPresetRunner`「执行成功后才标记日期」的写法，手册已把它列为多实例会重复触发（`ARCHITECTURE_AND_OPS_HANDBOOK.md:868`）。防止旧结果覆盖新快照，按 `sampled_at` 条件更新即可。

**建议收成**：

- **保留**：单例配置，含勾选账号与别名，可以沿用 `channel_monitor_v2_config` 的 `version` 条件更新（`repository/channel_monitor_v2_repo.go:68-86`）；主动兜底快照写 extra 中立键（P0-2）。
- **去掉**：任务表和子项表、租约、执行代次、7 天明细及其清理任务、`GET /api/v1/admin/gpt-quota/tasks/:id`，以及 202 异步返回。
- **手动刷新**：单个账号同步返回结果。全部刷新异步触发后，管理页重读各账号快照的最近尝试时间和状态，就能看到进度，不需要任务资源。

### 4.2　可配置项超出已确认范围

第 1 节确认的是每天北京时间 09:30—18:00。第 4 节又把 `timezone` 做成可配置字段，并增加「每天或周一至周五」。`Asia/Shanghai` 无夏令时。任意 IANA 时区在夏令时切换日会让 09:30 跳过或出现两次，设计没有处理。

另外两处也可以收：

- **页面读取间隔**：15 / 30 分钟只影响前端轮询，做成前端常量即可，不必入库、也不必给管理员配置。
- **双开关**：展示总开关加自动采集开关，让设计第 9 节多出「展示开、自动关」等状态组合。采纳 P0-1 后自动采集只是兜底，可以考虑去掉自动采集开关。

**建议收成**：

- 时区直接用全局 `cfg.Timezone`，默认 `Asia/Shanghai`（`backend/internal/config/config.go:2381`）；现有 cron 任务都用 `cron.WithLocation(loc)` 加载它（例如 `user_concurrency_preset_runner.go:33`）。
- 日期固定每天。
- 60 分钟档最后一轮是 17:30 还是补一次 18:00，保留为产品选择。

### 4.3　手动刷新三重防重

设计同时要求三层防重：同账号只能有一个执行中的采集、60 秒冷却、刷新请求按请求标识去重（第 6.2、8 节）。仓库的管理端幂等 helper 会强制要求前端带 `Idempotency-Key`（`handler/admin/idempotency_helper.go:88-90`）。

对一次只读 GET，「冷却 + singleflight」已经够用，这也是 Ollama 的做法。请求标识去重可以删掉。

### 4.4　用户 DTO 暴露运维状态

单卡片白名单里有 `last_attempt_at` 和 `last_attempt_status`。`fetch_failed`、`rate_limited` 这类类别对普通用户没有可执行意义，却暴露了共享上游账号的运维状况。

**建议**：用户侧只保留 `sampled_at` 与 `stale`；尝试时间和状态只放管理端。

---

## 5. 需补进设计的遗漏

### 5.1　账号资格还没闭合

`prepareUpstreamCall` 只要求 OpenAI 且 `Type == OAuth`（`openai_quota_service.go:416-420`）。下面两类都满足：

- 个人访问令牌：`IsOpenAIPersonalAccessToken` 仍是 OAuth，只看 `auth_mode`（`account.go:1340-1346`）。它不走 refresh token。
- Agent Identity：`IsOpenAIAgentIdentity` 同样是 OAuth（`openai_agent_identity.go:57-62`）。

`IsShadow()` 的定义是 `parent_account_id != nil`（`account.go:3268-3269`），不限于 `quota_dimension=spark`。影子会解析到母账号凭据，结果是卡片名字是影子、额度是母账号的。网关被动写入同样跳过影子（`openai_gateway_forward.go:1309`），所以影子也拿不到 P0-1 的被动快照。

账号删除是软删除：`accounts.deleted_at`，查询条件如 `account_repo.go` 中的 `deleted_at IS NULL`。外键不会因软删除而级联。

**建议写死首版资格**：

- 平台 OpenAI、类型 OAuth，`deleted_at` 为空。
- 排除影子、PAT、Agent Identity 和 Setup Token。
- 已勾选账号在暂停、错误、不可调度、限流中仍然展示，避免额度最差时卡片消失。
- 同一 `chatgpt_account_id` 的多条本地账号如果都被勾选，会各打一次上游。首版至少在管理页标出重复，避免无提示地双倍请求。

### 5.2　前缀、展示名和排序还不能直接写成测试

「去首尾空白、大小写不敏感、`c-` / `d-` 前缀」会把 `c-backup@mail`、`c-01 备注` 都归进迅游。默认展示名「已识别的编号，如 `c-01`」没有提取规则。`c-1`、`c-01`、`c-01-team`、`c-2a` 的显示文本和自然排序都未定义。

仓库里没有按 `c-` / `d-` 前缀解析账号名的现成代码。后端账号列表默认按名称的数据库文本序排序（`account_repo.go:1138-1139`），不是自然序。前端只有 `DataTable.vue:496-499` 用 `Intl.Collator(..., { numeric: true })` 做表格列排序。

管理员别名没有禁止写入邮箱、token、chatgpt account id。账号原名经常带邮箱，这是本页的主要泄露面。

**建议补上**：

- 编号提取的具体规则，并附排序样例。
- 排序在服务端完成，前端不做。
- 校验别名不得包含凭据和邮箱。

### 5.3　用户路由必须挂进现有已认证组，并承认后台模式

`RegisterUserRoutes` 的已认证组除 JWT、面板限流、审计外，还有 `BackendModeUserGuard`（`user.go:20-24`）。后台模式开启时，非 `admin` 角色直接 403（`backend_mode_guard.go:12-26`）。

设计写的是「所有登录用户看到同一列表」。后台模式下普通用户读不到用户自助接口。这页应跟随该拦截，而不是单独放行。

前端简易模式会隐藏一批菜单（`frontend/src/components/layout/AppSidebar.vue` 的 `hideInSimpleMode`）。渠道状态在简易模式中仍显示。新菜单要明确是否同样保留。

子管理员默认拒绝未知管理路由这一点设计是对的。新路由不要放进 `subAdminCommonRouteRules`。

页面上的 15/30 分钟是客户端轮询间隔。服务端不按这个间隔拒绝读取，但读取仍不得打上游。接口挂进上述路由组后，沿用面板按用户限流。

### 5.4　过期、停采和长批次的组合没有规则

- **过期判定的时间戳**：采纳 P0-1 后，过期要按两份来源中较新的采样时间判断，不再只看主动采集。
- **自动采集关闭后**：要显示「自动更新已关闭」，但响应里仍有 `stale`。关闭之后是否还按下一计划时点算过期，没有规则。
- **时段外手动刷新**：18:00 之后手动刷新成功，下一轮过期锚点是当晚已过的时点还是次日 09:30，没有规则。
- **批次跨时点**：账号变多、单次 20 秒时，一个批次可能跨过 30 分钟。设计没有账号上限，也没有写「上一批未完成则跳过本槽位」。按 §4.1 改用 leader lock 后，上一批持锁期间新槽位自然拿不到锁，但设计要写明这时是跳过还是补跑。
- **重置时间单位**：`reset_at` 在 `OpenAIRateLimitWindow` 里是 `int64`，现有测试按 Unix 秒使用。设计只说「合法」，没有排除毫秒时间戳，也没有把重置时间限制在采样时间之后的有限天数内。

### 5.5　审计额外字段有白名单

管理路由挂上现有 `AuditLogMiddleware` 后，PUT/POST 会留下方法、路径和操作者。`SetAuditExtra` 只接受固定键（`backend/internal/server/middleware/audit_log.go:51-61`）。

- 若要在审计里记录刷新账号数或配置版本，需要扩展白名单。
- 不能把账号别名、上游错误正文放进审计 extra。

### 5.6　与渠道监控额度模式重叠，设计未写否决理由

`/monitor` 已经能向普通用户展示关联账号的额度：

- 监控项 `check_mode` 为 `quota` / `quota_probe` 时绑定 `account_id` 抓取额度（`channel_monitor_service.go:647-655`）。
- 打开 `channel_monitor_show_quota` 后（默认 `"false"`，`setting_parse.go:193`），用户列表返回 `latest_quota`（`channel_monitor_user_handler.go:129-131`、`:170`）。
- 前端用 `MonitorQuotaView` 渲染（`frontend/src/components/user/monitor/MonitorCard.vue:59`）。

不能直接复用的理由确实存在，但设计只写了「可借鉴」：

- 用户接口要求 V1 模式（`channel_monitor_user_handler.go:35-41`），V2 模式下不可用；
- OpenAI 账号走 `GetUsageForAccount` → `getOpenAIUsage`，会发模型探测（`channel_monitor_quota_fetcher.go:232-236`）；
- 没有双列归类，也没有采集时段。

**建议**：设计补一节「备选方案与否决理由」，避免后续评审反复提出合并。

### 5.7　响应键与「组织」措辞和现有代码冲突

- 设计第 8 节把用户响应的分组键写作 `xunyou/subao`。代码里速宝的键一直是 `wsdashi`（`backend/internal/service/organization_usage_service.go:11-15`、`frontend/src/i18n/locales/zh/misc.ts:128`），`subao` 在仓库中不存在。
- 设计第 3.1 节、第 11 节称前缀与「迅游/速宝组织授权」无绑定。但系统没有组织实体，也没有组织授权；组织是由邮箱域名推导出来的（`backend/internal/repository/organization_usage_repo.go:278-284`）。

**建议**：键名改为 `xunyou` / `wsdashi`；措辞改为「与按邮箱域名推导的组织归属无关」。

### 5.8　展示关闭时的菜单与公开开关

设计只规定展示关闭时接口返回空分组，没有规定用户菜单是否隐藏。按现稿，关闭后用户仍能看到菜单，点进去是空页面。

现有做法是公开设置加前端 feature flag：

- 侧栏按 `featureFlag` 过滤菜单项（`AppSidebar.vue:945-948`）；`/monitor` 就是这么做的（`:984`）。
- `frontend/src/utils/featureFlags.ts` 文件头有新增 flag 的接入清单，其中包括后端 public settings 注入的 drift 测试。

**建议**：新增 opt-in 公开开关，例如 `gpt_quota_display_enabled`。关闭时隐藏菜单；有人直接访问路由时显示「未开启」。

### 5.9　「复用现有限流」对管理员无效

设计第 8 节写管理操作「复用现有审计与限流」。但面板限流默认豁免管理员：设置默认 `ExemptAdmin: true`（`backend/internal/service/setting_panel_rate_limit.go:51`），中间件命中后直接放行（`backend/internal/server/middleware/panel_rate_limit.go:81-86`）。

所以管理员刷新实际只受冷却保护，设计不应把限流列为防护手段。

### 5.10　前端复用评估不准

- `MonitorQuotaView` 不适合直接用：它只在快照成功时渲染已用百分比条，缺失窗口直接不显示，也没有采样时间（`frontend/src/components/common/MonitorQuotaView.vue:10-24`）。
- 应直接复用 `frontend/src/components/account/UsageProgressBar.vue`：
  - 已有 `remainingCapacity` 参数（`:80`）；
  - 重置时间已过且使用率大于 0 时显示 `usage.resetPending`（`:198-206`），正好是设计要的「已到重置时间，待更新」。
- 该组件按整数显示百分比（`:175-182`），和设计要求的「1 位小数」冲突。要么扩展参数，要么接受整数、与账号页保持一致。
- `frontend/src/composables/useAutoRefresh.ts` 只在 `shouldPause` 为真时冻结倒计时，恢复可见后不会补拉（`:48-59`）。设计要的「恢复可见时按间隔补一次」需要扩展它，或参照 `frontend/src/views/admin/BackupView.vue:853` 的 `visibilitychange` 处理实现。

### 5.11　「所有登录用户」的实际范围随注册策略变化

需求已确认不按组织过滤。设计仍应写明实际范围：

- 包括组织归为 `other` 的用户；
- 包括开放注册或第三方 OAuth 注册进来的用户。注册由 `SettingKeyRegistrationEnabled` 与邮箱后缀白名单控制（`backend/internal/service/auth_service.go:1199`）。

上线前应按当时的注册配置确认，这个暴露面可以接受。

---

## 6. 建议的审核结论

通过前先改设计，再进入实现：

1. **数据源**：被动快照优先，主动 wham/usage 只做兜底。读 extra 原始字段，并校验窗口时长（P0-1）。
2. **采集边界**：
   - 主动查询只解析 `rate_limit` 的两个窗口，按 ≤ 6 小时规则判定（P0-3）。
   - 不写调度相关的 extra 键，不触发自动重置，结果写调度中立键（P0-2）。
   - 采集前做 token 预检，不禁用账号（P1-1）。
3. **调度与存储**：用现有 leader lock 加单行 `last_slot_at` 条件更新；配置单例带 `version`。去掉任务表、租约、代次、7 天明细和任务查询接口（§4.1）。
4. **账号资格**：收成可刷新的普通 OAuth，排除影子、PAT、Agent Identity、Setup Token 和软删除账号。已选账号不论暂停或错误都展示（§5.1）。
5. **配置项**：时区用全局 `cfg.Timezone`，每天执行；页面轮询间隔用前端常量；用户 DTO 不带尝试状态（§4.2、§4.4）。
6. **其余遗漏**：
   - 展示名提取、排序、别名脱敏（§5.2）；
   - 后台模式和简易模式菜单（§5.3）；
   - 公开开关（§5.8）；
   - `wsdashi` 键名（§5.7）；
   - 渠道监控方案的否决理由（§5.6）。

60 分钟档是否在 18:00 收尾，仍由产品决定。

在上述结论写入 `docs/features/gpt-account-quota-display-design-cn.md` 之前，本文意见不得当成实现合同。
