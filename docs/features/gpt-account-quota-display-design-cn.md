# GPT 账号额度展示设计

更新：2026-11-04。状态：**已实施（2026-09-28）并在 main 中，待上线验收。展示默认关闭；真实账号的主动采集要等管理员上线后显式开启并勾选账号。真实 OpenAI 账号采集尚未验证（§14）。**

本文是 GPT 账号额度展示功能的唯一文档，包含需求、规则、接口、数据模型、代码位置、易错点和验证方式。原先"第 1–12 节设计合同 + 第 13 节实施记录"的两层结构已合并为一套现行口径，内容以源码为准。设计审核的结论都已写入正文；被否决的建议在对应位置写成"已否决"。审核原文已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/reviews/gpt-account-quota-display-design-review-cn.md` 查看）。

阅读顺序建议：
- 改上游采集或数据口径：先读 §4、§5 和 §12；
- 改排程、冷却或多实例行为：读 §6；
- 改接口：读 §8；
- 定位代码：读 §11；
- 跑测试或验收：读 §13、§14。

## 1. 目标与范围

| 项目 | 规则 |
| --- | --- |
| 展示对象 | 只展示 GPT / OpenAI 上游账号额度；不是本站用户订阅余额，不展示 Claude、Gemini 等其他平台 |
| 可见范围 | 通过现有模式门禁的登录用户看到同一列表，不按组织、订阅或 API 分组过滤；不开放匿名访问 |
| 实际人群 | 包括组织归为 `other` 的用户，以及开放注册或第三方 OAuth 注册进来的用户（由 `registration_enabled` 与邮箱后缀白名单决定）。上线前按当时的注册配置确认这一暴露面可以接受 |
| 双列 | 桌面左"迅游"（键 `xunyou`，账号原名 `c-` 前缀），右"速宝"（键 `wsdashi`，`d-` 前缀）；手机单列，迅游在前 |
| 展示选择 | 管理员勾选；未勾选及其他前缀不展示；初始选择为空，不自动选中所有 `c-`/`d-` 账号 |
| 用户操作 | 只读；无刷新、测试、调度或额度重置按钮 |
| 管理员操作 | 手动刷新单个或全部展示账号；配置采集间隔 |
| 采集时段 | 每天 Asia/Shanghai 09:30—18:00，30 或 60 分钟一轮；管理员手动刷新不受时段限制 |
| 页面轮询 | 前端常量 15 分钟，不入库、不可配置 |

前缀只用于展示分类，与按邮箱域名推导的组织归属、组织负责人的组织授权（`organization_access_grants`，migration 262）、分组和订阅都没有绑定。

参考项目：<https://github.com/Yajun312890225/sub_monitor/tree/4131f060d4b9c5b9b4fba9480f8f93635126b0b5>。借鉴额度窗口展示；不照搬其用户实时采集、独立用户 Key、60 秒刷新和按阈值开关调度。

## 2. 账号资格与名称

### 2.1 资格

- 候选：平台 OpenAI、类型 OAuth、`deleted_at` 为空的普通主账号。
- 排除：任意 shadow（`parent_account_id` 非空，不限于 spark）、PAT（`IsOpenAIPersonalAccessToken`）、Agent Identity（`IsOpenAIAgentIdentity`）、Setup Token（类型不是 OAuth，天然排除）。
  - 依据：PAT 与 Agent Identity 的类型仍是 OAuth，只按类型过滤挡不住；shadow 会解析到母账号凭据，结果是卡片名是影子、额度是母账号的；Agent Identity 请求前会创建或恢复 agent task，不是只读。
- 已勾选账号在暂停、错误、不可调度、限流时**仍展示**已有快照，避免额度最差时卡片消失。采集不会因这些状态把条目从配置中删除。
- 已保存但后来失去资格（删除、改为 PAT/shadow、改名为其他前缀）的条目：管理页标出原因，可保留或移除，不挡保存；用户读取和采集都排除，不静默替换。
- 同一 `chatgpt_account_id` 的多条本地账号都被勾选时，保存响应带 warning `duplicate_chatgpt_account`（会各打一次上游）。

### 2.2 归类、展示名与排序

- 归类始终用账号原名：TrimSpace 后大小写不敏感地匹配 `c-` / `d-` 前缀。改名后下一次用户读取即生效；删除或失去资格立即从后续读取排除，不等采集。
- 编号：前缀后的连续数字。默认展示名为"前缀 + 原样编号"（`c-01`）；无编号时为"前缀 + `#条目ID`"，避免多卡同名。原名常带邮箱，所以默认不公开原名。
- 管理员别名：先去首尾空白再校验；只允许有限长度的中英文、数字、空格、下划线、短横线；拒绝邮箱格式、换行、含 `sk-`、UUID 片段或 20 位以上连续字母数字的内容（防 token、API key、ChatGPT account ID）。
- 排序由服务端完成，前端不重排：迅游在速宝前（用户页、管理页已选列表、候选列表一致）；同组内有编号的按数值升序（`c-2` 在 `c-10` 前，`c-002` 在 `c-10` 前），无编号的排在后面，再按规范化名称、账号 ID。

## 3. 用户界面

- 用户页 `/gpt-quota`（`GPTQuotaView.vue`）：顶部说明"共享上游账号额度，非个人订阅余额"、采集计划与下次计划时间；左右两列（`GPTQuotaColumn.vue`），空列保留标题并显示"暂无展示账号"，不自动挪用另一列。
- 卡片：展示名、5 小时与 7 天剩余比例、各窗口重置时间、采样时间、过期状态。
  - `GPTQuotaMeter.vue` 的 `variant="ring"` 用 SVG 圆环显示剩余：12 点固定起点，剩余下降时末端逆时针回退；100% 为完整圆环，0% 无残点；中心百分比保留 1 位小数。管理页默认用紧凑条形。
  - 颜色（`utils/gptQuota.ts` 的 `gptQuotaLevel`）：剩余 > 30% 绿，≤ 30% 黄，≤ 10% 红。
  - 5 小时窗口为 null 时整行隐藏，有值即显示；7 天窗口缺失显示灰色空环/"未提供"。
  - 账号状态取已提供窗口的最小剩余；真实用尽照常显示；重置时间到点只显示"已到重置时间，待更新"，不假定恢复额度。
  - 列头汇总取有采样卡片的 7 天剩余平均值。
- 轮询：进入时读一次，之后每 15 分钟只读轮询；标签页隐藏时暂停，恢复可见且距上次读取已满间隔时补一次；卸载时清理定时器并取消请求；读取失败保留当前画面并提示，不高频重试。重置倒计时在本地走，不发请求。
- 菜单：公开开关只存于 `gpt_quota_display_config`，不进 public settings。侧栏经 `useGPTQuotaVisibility.ts` 调 `GET /gpt-quota/status`，按 opt-in 处理（未加载或失败时隐藏），结果缓存 5 分钟，侧栏随路由重挂载时过期重读；用户页读取和管理员保存后同步该状态。关闭时直接访问显示"未开启"。
  - 已否决：照 `/monitor` 用 public settings + 前端 feature flag。原因：状态接口已能控制菜单，且避免把开关复制到第二处。
- 管理页 `/admin/gpt-quota`（`GPTQuotaDisplayView.vue`）：开关、间隔、已选条目（含失去资格原因、警告、快照、最近尝试状态、`retry_after`）、本实例最近批次计数、单条/全部刷新。
  - 选择账号用弹窗 `GPTQuotaAccountPickerDialog.vue`（2026-09-29 由右栏改为弹窗）：打开时默认勾选当前本地选择（含未保存新增）；勾选草稿跨分页/搜索保留，取消或关闭即丢弃；不合格候选不能勾选，已选但失去资格的可以取消；弹窗顶部"已选"区列出全部勾选项，已删除或改类型的账号也能在此取消（候选接口不返回它们）。候选加载失败时清空当前条目并显示错误与重试，不展示与页码/搜索不一致的旧结果。弹窗"保存"只更新本地选择，点"保存配置"才提交；本次移除后又勾回的已保存账号恢复服务端别名。
  - 批次进度轮询只刷新条目状态和计数，**不更新编辑基线的 `version`**，避免本地未保存修改绕过乐观锁。

## 4. 上游采集

### 4.1 只读上游方法

主动采集只走 `OpenAIQuotaService.QueryUsageReadOnly`：

- 只请求 `GET /backend-api/wham/usage`；不查 reset-credit 明细，不发模型推理或测试对话。原 `QueryUsage` 行为保持不变（它额外查 reset-credit）。
- 拒绝 shadow 与 Agent Identity，不创建或恢复 agent task。
- **token 预检**：在调用 token provider 之前，若账号不是 PAT、access token 已过期且没有 refresh token，记 `token_unavailable` 并跳过。依据：token provider 在这种情况下会调用 `disableAccountMissingRefreshToken`（`BlockAccountScheduling` + `SetError`）把账号永久摘除；只展示、不参与调度或长期闲置的账号会被展示采集"禁用"。
- 不写 `accounts.extra`；不调用 `SetError`、`BlockAccountScheduling`、`UpdateExtra`、`notifyOpenAIAutoReset`、`CacheResetCreditsSnapshot`。
- 调用方取消或整体超时优先记为 `cancelled` / `timeout`。单次上游超时沿用 20 秒。

"手动刷新"指立即发起一次额度采集，不是 OAuth token 刷新、额度窗口重置或消费 reset credit。

### 4.2 窗口归一化

- 只解析 `rate_limit.primary_window` 与 `rate_limit.secondary_window`，**忽略 `additional_rate_limits`**。依据：Spark 窗口在 `additional_rate_limits`，时长同样是 18000/604800 秒；扫整包会让母账号的成功响应出现两个 5 小时窗口而判冲突（审核 P0-3 用仓库夹具 `openai_quota_spark_window_test.go` 复现）。
- 时长 (0, 6h] 归 5 小时，> 6h 归 7 天，与自动重置的 `LimitWindowSeconds <= 6*60*60` 口径一致；不按 18000/604800 精确匹配，不假设 primary 永远是 5 小时。
- 同一类别出现两个窗口、使用率非有限数或为负、时长 ≤ 0、`used_percent` 缺失或为 null：该类别"未提供"（null 不会被解码成 0% 已用）。未知时长不强行归类。
- 剩余比例 = `max(0, min(100, 100 - used_percent))`，四舍五入到 1 位小数；不推算 token 数或金额。零使用率是有效值，与缺失严格区分。
- 重置时间：`reset_at` 与 `reset_after_seconds` 都是 int64，缺失即 0，所以只接受正值；优先 `reset_at`，只有 `reset_after_seconds` 时用本次采样时间换算并记 `reset_time_source`。晚于采样时间 8 天以上（如毫秒时间戳）视为未提供。
- 不使用 `OpenAICodexUsageSnapshot.Normalize`（缺时长时按旧"primary=7d"映射归类），也不使用 `buildCodexUsageProgressFromExtra`（重置时间已过时把使用率改成 0，会显示成剩余 100%）。
- 一次成功只拿到一个有效窗口时，新快照只保留这一个，另一个显示"未提供"，不混入上次的窗口冒充同一采样时间。两个都不可用记 `no_supported_windows`，保留旧快照。

## 5. 数据源与读写隔离

- **硬约束**：用户读取、管理页读取、状态接口都只查数据库快照，任何读取路径不得调用上游。没有快照、快照过期、用户反复刷新、加查询参数、管理员浏览列表，都不能触发采集。
- 快照存独立表 `gpt_quota_display_snapshots`，与 `accounts.extra` 完全隔离。展示采集不得写 `codex_5h_*`、`codex_7d_*`、`codex_usage_updated_at` 或任何会触发调度 outbox 的键。依据：网关暂停（`resolveOpenAIQuotaUtilization`）和自动用卡读的就是这组键；写入新值会让"快照超过 2 小时放行一次请求自愈"的陈旧兜底失效，并改变自动用卡的触发时机。
- 用户读路径：鉴权与模式门禁 → 读配置 → 按最新账号元数据批量过滤、归类 → 批量读快照 → 返回白名单字段。账号元数据批量加载，不逐卡查询；首版不加跨实例展示缓存。

已否决：
- 已否决：以网关写入 `accounts.extra` 的 Codex 被动快照（`x-codex-*` 响应头）为首选数据源、主动采集只做兜底（审核 P0-1）。原因：选择独立表 + 主动只读采集，使展示链路与调度键彻底隔离；采集频率每天最多 18 轮、并发 3，上游压力可控。代价：有流量的账号，本页数据可能比管理端账号页（读被动快照）更旧，两页数字不必一致。迁移注释"快照与 accounts.extra 完全隔离"即此决策。
- 已否决：复用 `/monitor` 渠道监控额度模式。原因：用户接口要求 V1 模式；OpenAI 路径走 `GetUsageForAccount` → `getOpenAIUsage` 会发 Responses 模型探测，且会给缺失窗口补 0 使用率；没有双列归类与采集时段。
- 已否决：复用 `account_usage_service.getOpenAIUsage`。原因同上：可能读旧 Extra、发模型探测，探测失败仍返回旧用量，"返回成功"不代表拿到了最新上游额度。
- 已否决：复用 `MonitorQuotaView.vue`。原因：只在成功时渲染已用百分比，缺失窗口不显示，没有采样时间；本功能新建 `GPTQuotaMeter.vue`。

## 6. 排程、并发与过期

### 6.1 排程

- 时区固定 Asia/Shanghai（缺 tzdata 时回落 UTC+8），**不跟随全局 `timezone` 配置**。依据：部署为 UTC 时采集时段会整体偏移；Asia/Shanghai 无夏令时，不需处理 09:30 跳过或重复。
  - 已否决：时区与"每天/工作日"做成可配置。原因：超出确认范围；任意 IANA 时区在夏令时切换日需额外处理。
- 以计划起点为锚点算固定时刻，不用"上次完成后等 N 分钟"：
  - 30 分钟：09:30、10:00……17:30、18:00，共 18 次；
  - 60 分钟：09:30、10:30……17:30，加 18:00 收尾，共 10 次。
- 每 30 秒检查一次。顺序：读取已选条目 → 检查并设置的方式原子占用本实例批次 → 条件领取槽位（`last_slot_at` 只进不退）→ 执行批次。读取失败、手动批次执行中、领取失败都**不消耗槽位**（领取失败时释放占用并恢复上一批次统计），在槽位有效期内下次检查重试。
- 槽位只在该时点到下一时点之间有效（18:00 末槽宽限 5 分钟），因此重启或短暂停机只补最近一个到期槽位，不追补历史。
- 开启展示或修改间隔时，保存事务把 `last_slot_at` 抬到保存时刻，当前已过的槽位不会被隐式补跑；开关开启或新选账号不隐式请求上游，管理员可手动刷新。
- 批次并发 3，单批上限 25 分钟，leader lock（`LeaderLockCache`）TTL 30 分钟；每次派发前重读开关，关闭展示后不再派发。批次结束时若已跨过下一槽位，直接领取该槽位并记 skipped 日志，不补跑、不排队。
- 定时批次占用后，同实例的手动全量刷新返回 409，避免"槽位已领取但批次未执行"。
- 已否决：持久化任务表/子项表、租约、执行代次、7 天任务明细、`GET /admin/gpt-quota/tasks/:id`、请求标识去重。原因：仓库同类后台任务（`ollama_cloud_usage.go`、OpenAI 自动重置）只用 leader lock；计划去重用单行 `last_slot_at` 条件更新即可；对只读 GET，"冷却 + singleflight"已足够。
- 已否决：独立的"自动采集开关"。原因：与展示开关组合出多余状态；原设计的"服务采集暂停"状态因此没有实现。

### 6.2 冷却、退避与尝试状态

- 60 秒冷却与 429 退避（尊重 Retry-After，缺省 5 分钟，上限 24 小时）由快照行的条件 upsert `ClaimAttempt` 实现，**跨实例生效**；进程内再用 singleflight 合并同账号请求。冷却内返回 `skipped_cooldown`，退避内返回 `skipped_backoff`，都不访问上游。
- `ClaimAttempt` 写 `running`；`FinishAttempt` 与 `PublishSnapshot` 只在 `last_attempt_at` 仍等于本次尝试时才改状态，晚返回的旧尝试不覆盖新状态；快照另按 `sampled_at` 条件发布，旧结果不覆盖新快照。
- 失败只更新尝试状态，不覆盖成功额度和 `sampled_at`。失败类别：`token_unavailable`、`account_unavailable`、`not_supported`、`unauthorized`、`forbidden`、`rate_limited`、`timeout`、`upstream_error`、`request_failed`、`parse_failed`、`no_supported_windows`、`cancelled`、`internal_error`。
- 单账号失败不阻断其他账号；普通失败等下一计划或管理员操作。

### 6.3 过期判定

- 取最近一个已过 5 分钟宽限的计划时点 S；`sampled_at` 早于 `S - 60 秒`（冷却提前量）即 stale。
- 夜间不因时间流逝变成过期；从未成功采集时 `sampled_at` 为 null，前端显示"暂无数据"（缺失不解释为 0% 使用率）。

### 6.4 停机

`Stop` 取消服务根 context，在途上游请求随之取消；尝试结果用独立短超时 context 记为 `cancelled`。手动全量批次和单条同步刷新由服务 WaitGroup 跟踪。单条刷新后端上限 45 秒，前端该请求单独用 60 秒超时。已发出的网络请求只能尽力取消。

## 7. 数据模型（`backend/migrations/243_gpt_quota_display.sql`）

| 表 | 主要字段 | 约束 |
| --- | --- | --- |
| `gpt_quota_display_config` | `enabled`（默认 false）、`interval_minutes`（30/60）、`start_time` 09:30、`end_time` 18:00、`version`、`last_slot_at`、`updated_by`、`updated_at` | 单例（`id BOOLEAN = TRUE`）；乐观版本检查 |
| `gpt_quota_display_entries` | `id`、`account_id`（唯一，FK accounts，ON DELETE CASCADE）、`display_name`、`selected`、`updated_at` | 软删除不级联，读取时按 `accounts.deleted_at` 排除；部分索引 `WHERE selected` |
| `gpt_quota_display_snapshots` | `account_id`（PK）、`five_hour` / `seven_day`（JSONB，缺失为 SQL NULL）、`sampled_at`、`last_attempt_at`、`last_attempt_status`（默认 `never`）、`retry_after`、`source`（默认 `active`）、`updated_at` | 每账号一行；失败不覆盖成功额度 |

快照只存展示所需数值，不存完整上游 JSON、邮箱、ChatGPT account ID、reset credit 明细或 token。配置撤销或账号删除后，即使历史快照未清理，也不得出现在用户响应中。迁移号 243 是因为 main 已有 239—242；migration 受 checksum 校验，不得修改。

## 8. 接口合同

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| `GET /api/v1/gpt-quota` | 登录用户（JWT、`BackendModeUserGuard`、面板限流、审计） | 返回 `enabled`、`server_time`、`poll_interval_seconds=900`、`schedule{start,end,interval_minutes,timezone}`、`next_scheduled_at`、`in_schedule_window`、`groups.xunyou` / `groups.wsdashi`。关闭时分组为空，不泄漏旧快照 |
| `GET /api/v1/gpt-quota/status` | 登录用户 | 只返回 `{enabled}`，供侧栏决定菜单 |
| `GET /api/v1/admin/gpt-quota` | 完整管理员 | 配置、计划、全部已选条目（含失去资格原因、warnings、快照、最近尝试状态、`retry_after`）、本实例最近批次计数；不受公开开关影响 |
| `GET /api/v1/admin/gpt-quota/candidates` | 完整管理员 | `search`、`page`、`page_size`；数据库侧 COUNT + LIMIT/OFFSET；列出未删除的 OpenAI OAuth 账号及资格原因，`c-`/`d-` 账号按编号自然排序在前；可添加的还要有前缀；搜索词里的 `%`、`_` 按字面匹配 |
| `PUT /api/v1/admin/gpt-quota/config` | 完整管理员 | `{enabled, interval_minutes, expected_version, entries[{account_id, display_name}]}`；配置与选择同一事务保存；版本不符 409 `GPT_QUOTA_CONFIG_CONFLICT`；响应可带 `warnings`（`duplicate_chatgpt_account`） |
| `POST /api/v1/admin/gpt-quota/refresh` | 完整管理员 | 必须且只能二选一：`{entry_id}` 同步返回状态和条目；`{all: true}` 返回 202 异步批次，已有批次时 409 `GPT_QUOTA_BATCH_RUNNING`；展示关闭时 409 `GPT_QUOTA_DISPLAY_DISABLED`；畸形请求体 400 |

- 用户卡片白名单：`id`、`display_name`、`five_hour`、`seven_day`、`sampled_at`、`stale`；窗口字段为 `remaining_percent`、`reset_at`、`reset_time_source`。不返回账号 ID、原名、邮箱、尝试状态。依据：`rate_limited` 之类状态对普通用户没有可执行意义，却暴露共享上游账号的运维状况。
- 分组由服务端完成，不接受用户按任意 `account_id` 过滤。用户错误只返回稳定类别，不含上游原文、URL、邮箱或凭据。
- 四个管理路由不在 `subAdminCommonRouteRules` 或任何权限白名单内；子管理员访问得到 `ADMIN_PERMISSION_DENIED`。
- 用户路由跟随后台模式：后台模式开启时非 admin 角色 403，这页同样不可用。
- 管理员默认豁免面板限流（`ExemptAdmin: true`），所以刷新防重依赖 60 秒冷却 + singleflight，**不依赖限流**。
- 审计：沿用管理审计中间件，PUT 配置的脱敏请求体（含别名）进入操作日志；别名已按 §2.2 校验为非敏感内容。本功能不写审计 extra（`SetAuditExtra` 只接受固定键）。

## 9. 失败与关闭行为

| 场景 | 结果 |
| --- | --- |
| 从未成功采集 | 暂无数据 |
| 上游超时/401/403/429/解析失败 | 保留旧成功快照及其时间；记失败类别；不改账号调度、订阅或网关额度 |
| 本站读接口失败 | 前端保留画面并标记读取失败，按正常周期重试 |
| 展示关闭 | 用户读取返回关闭状态与空分组；自动采集停止；手动刷新 409 |
| 账号删除/不再选中/不再符合规则 | 后续用户读取立即排除；采集派发前再次校验 |
| 数据库不可用 | 返回服务错误；不因无快照而临时调用上游 |

关闭展示不删除配置和快照，重新开启后仍显示真实采样时间。批次计数只保存在执行实例内存中，多实例下管理页以各条目的最近尝试状态为准。

## 10. 已确认决策

| # | 决策 | 状态 |
| --- | --- | --- |
| D1 | 所有登录用户共享同一列表；左迅游、右速宝；不改回按组织授权隔离 | 已确认 |
| D2 | 资格限 OpenAI OAuth 普通主账号，管理员勾选；排除 shadow、PAT、Agent Identity、Setup Token | 已确认 |
| D3 | 每天 Asia/Shanghai 09:30—18:00，默认 30 分钟；60 分钟档包含 18:00 收尾 | 已确认 |
| D4 | 只读 wham/usage 窄方法，不复用会发模型探测的用量入口，不附带 reset-credit 查询 | 已确认 |
| D5 | 默认关闭、初始选择为空，显式发布 | 已确认 |
| D6 | 独立持久化快照 + leader lock + 槽位条件更新 + 账号级 singleflight/冷却 + `sampled_at` 条件写入；不建通用任务表 | 已确认 |

## 11. 代码地图

| 层次 | 位置 |
| --- | --- |
| 迁移 | `backend/migrations/243_gpt_quota_display.sql` |
| 服务 | `backend/internal/service/gpt_quota_display.go`（`GPTQuotaDisplayService`、`ErrGPTQuotaDisplayDisabled`、`ErrGPTQuotaBatchRunning`、`gptQuotaDisplayTimezone`）；只读上游 `OpenAIQuotaService.QueryUsageReadOnly` 与 `OpenAIQuotaReadOnlyError`（`openai_quota_service.go`） |
| 仓储 | `backend/internal/repository/gpt_quota_display_repo.go`（`ClaimAttempt`、`FinishAttempt`、`PublishSnapshot`） |
| Handler 与路由 | `backend/internal/handler/gpt_quota_display_handler.go`；`backend/internal/server/routes/user.go`、`admin.go` |
| Wire 与生命周期 | `ProvideGPTQuotaDisplayService`（`backend/internal/service/wire.go`）构造时 `Start()`；`provideCleanup` 调 `Stop()` |
| 前端 | `frontend/src/api/gptQuotaDisplay.ts`、`views/user/GPTQuotaView.vue`、`components/user/GPTQuotaColumn.vue`、`components/user/GPTQuotaMeter.vue`、`utils/gptQuota.ts`、`views/admin/GPTQuotaDisplayView.vue`、`components/admin/gpt-quota/GPTQuotaAccountPickerDialog.vue`、`composables/useGPTQuotaVisibility.ts`、`i18n/locales/{zh,en}/gptQuota.ts` |

## 12. 关键约束与易错点

1. **读取路径零上游调用。** 新增任何读取接口或缓存失效逻辑都不能触发采集；单测用 spy/fake 断言调用数为零。
2. **不写 `accounts.extra` 的调度键。** 依据：写入会改变网关自动暂停的陈旧兜底和自动用卡时机（§5）。若将来改为写 extra，必须用新的调度中立前缀并加入 `schedulerNeutralExtraKeyPrefixes`，否则每次写快照都触发调度 outbox。
3. **采集前必须做 token 预检。** 依据：过期且无 refresh token 时 token provider 会禁用账号（§4.1）。
4. **只解析 `rate_limit` 两个窗口，按 ≤6h 区间归类。** 依据：Spark 窗口同秒数会让成功响应判冲突（§4.2）。
5. **不复用 `Normalize`、`buildCodexUsageProgressFromExtra`、`getOpenAIUsage`。** 依据：旧映射、过期改 0、模型探测（§4.2、§5）。
6. **重置时间只接受正值且不晚于采样后 8 天。** 依据：int64 缺失为 0；毫秒时间戳会被当成遥远未来。
7. **不要把面板限流当作刷新防护。** 管理员默认豁免；防重靠冷却与 singleflight。
8. **管理页轮询不得更新编辑基线 `version`。** 依据：否则本地未保存修改会绕过乐观锁覆盖别人的保存。
9. **槽位领取必须在批次占用之后，失败要释放占用。** 依据：先领槽位再占批次时，手动批次在途会导致"槽位已消耗但批次未执行"。
10. **时区不要改成跟随全局 `timezone`。** 依据：UTC 部署时采集时段偏移。
11. **管理路由不得加入子管理员白名单**；`gpt_quota_display_test.go` 有拒绝测试。
12. **本页与管理端账号页数字可以不一致。** 前者是主动采集快照，后者读网关被动快照（§5 已否决项）；不要为"对齐"而改读 extra。
13. **5 小时窗口为 null 时隐藏整行。** 改前端时注意"未提供"（7 天缺失）与"隐藏"（5 小时缺失）是两种处理。

## 13. 验证

### 13.1 命令

```bash
cd backend
go test -tags=unit -count=1 ./internal/service -run 'GPTQuota'
go test -tags=unit -count=1 ./internal/handler -run 'GPTQuota'
# PostgreSQL 集成：无 Docker 时把 DSN 指向新建的空库（可用本机嵌入式 PG）
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN=... go test -tags integration -count=1 -run 'TestGPTQuotaDisplayRepository|TestMigrationsRunner' ./internal/repository/
cd ../frontend && pnpm run typecheck && pnpm exec vitest run
```

### 13.2 覆盖

- service 单测（`gpt_quota_display_test.go`）：读路径零上游调用、DTO 白名单、失败保留、429 退避、冷却、跨槽跳过、停机取消、子管理员拒绝、排程边界（09:29/09:30/18:00/18:01、30/60 两档、跨日、配置变更、重启最近槽位）、数据口径（0%、100%、>100%、null、未知时长、窗口位置交换、非法数值、缺失重置时间）。
- 仓储集成（`gpt_quota_display_repo_integration_test.go`）：往返读写、候选分页/排序/转义、16 路并发领取槽位与采集尝试各只有一个成功。

### 13.3 最近结果

| 时间 | 项目 | 结果 |
| --- | --- | --- |
| 2026-11-04（本机，HEAD `26f6363af`） | `go test -tags=unit ./internal/service -run GPTQuota`、`./internal/handler -run GPTQuota` | 通过 |
| 2026-09-28 | 构建与静态检查 | `go build ./...`、`go vet` 通过；golangci-lint v2.14（go1.27 构建）`--new-from-rev=HEAD` 改动包 0 issues |
| 2026-09-28 | 后端测试 | `internal/service` 全量通过（GPT 额度用例另跑 `-race`）；handler、server、repository、cmd/server、migrations 通过 |
| 2026-09-28 | 前端 | `vue-tsc --noEmit`、改动文件 eslint、vitest 全量通过 |
| 2026-09-28 | PostgreSQL 16/18 | 嵌入式 PG 16 与 CI 同版本 PG 18：`TestGPTQuotaDisplayRepository*` 与 `TestMigrationsRunner*`（从零执行全部迁移含 243）通过；另做过一次端到端冒烟（真实 handler+service+repository+PG，fake 上游），覆盖默认关闭、保存/409/非法别名、读路径零上游、单条刷新与冷却、全量批次计数、删除即时排除、关闭后 409 |
| 2026-09-28 | 真实链路 API | 真实服务二进制（AUTO_SETUP 从零迁移）+ PG 18 + Redis 8 + 真实 JWT：匿名 401、普通用户与子管理员访问管理接口 403、候选分页/搜索/资格、保存校验与 409、用户卡片白名单与脱敏、单条/全量刷新与计数、软删除即时排除、关闭后 409、配置写入审计。测试账号均为"token 已过期且无 refresh token"，采集停在预检，未访问 OpenAI；账号状态与 `accounts.extra` 未被修改 |
| 2026-09-28 | 浏览器 | Playwright 驱动 Edge 23 项通过：管理员配置（失去资格条目可移除、保存、双会话版本冲突、单条刷新状态）、侧栏随开关显隐、桌面双列与手机单列、剩余比例/未提供/暂无数据/已到重置时间、无邮箱、子管理员无菜单且直接访问被重定向。截图与脚本未入库 |

## 14. 状态与待办

- 已完成：§1—§9 全部规则与接口；2026-09-29 账号选择改为弹窗；用户页圆环展示。
- 待办：
  - 上线后用隔离的真实 OpenAI 账号验证一次采集（唯一未执行项）；
  - CI（PostgreSQL 18）再跑一次集成测试；
  - 上线前按当时注册配置确认"所有登录用户"的暴露面（§1）。
