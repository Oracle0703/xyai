# GPT 账号额度展示设计

状态：**已实施（2026-09-28），待上线验收**。设计修订日期：2026-09-24；实施记录见第 13 节。

本文记录已确认需求、技术方案和实施结果。第 1—12 节是设计合同，第 13 节记录实施时的落地细节与澄清；两者冲突时以第 13 节和源码为准。展示默认关闭，真实账号的主动采集要等管理员上线后显式开启并勾选账号。

## 1. 目标与已确认范围

| 项目 | 约定 |
| --- | --- |
| 展示对象 | 只展示 GPT / OpenAI 上游账号额度，不展示 Claude、Gemini 等其他平台；不是本站用户订阅余额 |
| 可见范围 | 所有登录用户看到相同列表，不按用户组织、订阅或 API 分组过滤；不开放匿名访问 |
| 双列布局 | 桌面左侧“迅游”，对应账号名称 `c-` 前缀；右侧“速宝”，对应 `d-` 前缀 |
| 名称匹配 | 先去掉首尾空白，再按大小写不敏感的前缀匹配；按编号自然排序，`c-2` 在 `c-10` 前面 |
| 展示选择 | 管理员勾选参与展示的账号；未勾选及其他前缀不在用户页展示 |
| 用户操作 | 仅展示，无手动刷新、测试、账号调度或额度重置按钮；页面自动读取已保存的数据 |
| 管理员操作 | 可手动刷新单个账号、全部展示账号；可配置自动采集时段与间隔 |
| 手机布局 | 单列上下排列，迅游在前、速宝在后；不挤压成两列 |
| 实施门禁 | 本轮只写设计；设计审核通过后才能进入实现 |

默认方案：用户页面每 15 分钟只读轮询，作为前端常量；后台每天使用全局服务时区 Asia/Shanghai，在 09:30—18:00 每 30 分钟采集一次，可选择 60 分钟间隔；管理员手动刷新不受时段限制。日期固定为每天，时区、工作日和页面轮询间隔不做本功能配置。

## 2. 参考项目与现有能力核实

参考项目：<https://github.com/Yajun312890225/sub_monitor/tree/4131f060d4b9c5b9b4fba9480f8f93635126b0b5>，本次阅读固定提交 `4131f060d4b9c5b9b4fba9480f8f93635126b0b5`。

| 来源 | 已核实事实 | 设计结论 |
| --- | --- | --- |
| 参考项目 `main.go`、`web.go` | 用户查询会调用 Sub2API 管理用量接口；后台还支持按阈值开关账号调度 | 借鉴额度窗口展示，不照搬用户实时采集、独立用户 Key 和自动调度体系 |
| 参考项目 `web/index.html` | 提供刷新、测试按钮和 60 秒自动刷新 | 与本需求不同，用户侧只保留快照轮询 |
| `backend/internal/service/account_usage_service.go` | `getOpenAIUsage` 可能读取旧 Extra、发起 Responses 模型探测，探测失败仍返回既有用量；还可能为本地统计补出 0 使用率窗口 | 不能把该方法返回成功视为成功获取最新上游额度，也不能使用补出的 0 表示剩余 100% |
| `backend/internal/service/openai_quota_service.go` | `QueryUsage` 使用账号凭据、代理及现有 token provider 请求 `GET /backend-api/wham/usage`；返回 `rate_limit`、窗口时长、使用率、重置时间和 `fetched_at`；还额外查询 reset-credit 明细 | 优先复用其认证、代理和请求封装，抽取只查询 usage 的窄方法；保留原 `QueryUsage` 行为不变，新采集不请求 reset-credit 明细 |
| `backend/internal/service/channel_monitor_quota_fetcher.go` | 已有额度归一化、缓存及同账号请求合并模式 | 可借鉴，不依赖现有渠道监控模式开关或其主动探测调度 |
| `frontend/src/components/common/MonitorQuotaView.vue` | 已有窗口额度展示组件 | 实施时评估可复用展示片段；不足时新增独立卡片，不改其他渠道监控语义 |

不把参考项目“约 10 分钟缓存”的说明视为当前仓库合同。当前不同平台缓存语义不同；本功能通过独立采集计划和持久化快照控制展示更新。

## 3. 用户界面

建议新增登录后菜单“GPT 账号额度”，页面路径暂定 `/gpt-quota`。该入口独立于 `/monitor`，不切换或改变现有渠道监控。

| 页面区域 | 内容 |
| --- | --- |
| 顶部 | 标题、说明“以下为共享上游账号额度，非个人订阅余额”；后台采集计划、下次计划采集时间 |
| 左列 | 迅游标题、账号数，以及全部符合条件的 `c-` 账号卡片 |
| 右列 | 速宝标题、账号数，以及全部符合条件的 `d-` 账号卡片 |
| 空列 | 保留列标题，显示“暂无展示账号”；不自动移动另一列归属 |
| 两列均为空 | 显示“暂无展示账号”，不因空数据触发采集 |
| 卡片 | 脱敏展示名、5 小时剩余比例、7 天剩余比例、每个窗口的额度重置时间、数据采样时间、过期状态 |

### 3.1 展示名称与归类

- 归类始终使用后台账号原始名称，展示别名不改变归属。
- 建议默认展示已识别的编号，如 `c-01`；原名附带邮箱、备注时不直接公开完整名称。管理员可覆盖为非敏感别名。
- 归类规则为：名称 TrimSpace 后以不区分大小写的 c- 或 d- 开头，前缀后的编号取连续数字；没有连续数字的名称仍可归类，但编号排序键置于有编号条目之后，再按规范化名称和账号 ID 排序。示例：c-2 在 c-10 前。服务端完成排序，前端不重新排序。
- 默认展示名只保留前缀和编号（如 c-01）；管理员别名只能使用有限长度的中英文、数字、空格、下划线和短横线，不得包含邮箱格式、token、API key、ChatGPT account ID 或换行。用户响应只返回展示条目 ID，不返回完整账号 DTO。
- 账号从 `c-` 改为 `d-` 后，在下一次用户读取时进入右列；改为其他前缀、删除或失去平台/类型资格时立即从后续读取结果中排除，不等下次采集。
- 前缀仅用于展示分类，与按邮箱域名推导的组织归属、分组和订阅无绑定关系。

### 3.2 额度和时间语义

| 字段/状态 | 规则 |
| --- | --- |
| 剩余比例 | 对合法数值使用 `max(0, min(100, 100 - used_percent))`；展示 1 位小数；不推算 Token 数量或金额 |
| 窗口识别 | 只读取 `rate_limit.primary_window` 与 `secondary_window`；时长大于 0 且不超过 6 小时归入 5 小时，大于 6 小时归入 7 天；不假设 primary 永远是 5 小时 |
| 缺失窗口 | 显示“未提供”；零使用率是有效值，与缺失严格区分 |
| 异常窗口 | 非有限数、负使用率、两个窗口落入同一类别或时长非法时判为无效，不生成满额结果；未知长度本期不强行归类 |
| 额度重置时间 | 优先使用合法的上游 `reset_at`；仅提供合法 `reset_after_seconds` 时，以本次采样时间换算并记录来源 |
| 倒计时到期 | 显示“已到重置时间，待更新”；不能自行把剩余比例改为 100% |
| 数据采样时间 | 上游 usage 请求成功并解析有效结果的时间；数据库读取、页面轮询、读取缓存都不能更新它 |
| 最近尝试时间 | 最近一次真实采集尝试的时间，与成功采样时间独立 |
| 数据过期 | 超过计划应更新时点加宽限时间仍无成功采样时标记；宽限建议 5 分钟，不简单按自然时间 TTL 让整晚变成故障 |
| 非采集时段 | 显示“当前不在自动更新时段”，保留旧数据及真实时间；可同时显示此前采集失败状态 |

若一次成功响应只提供一个有效窗口，新快照仅保留本次窗口，另一个显示“未提供”，不混入上次窗口冒充同一采样时间。若两种目标窗口均不可用，本次记为 `no_supported_windows`，保留旧快照并显示更新异常；首次则显示暂无数据。

用户页面进入时读取一次，之后按配置间隔读取；隐藏标签页暂停轮询，恢复可见时仅在距离上次读取已达到间隔时补一次。组件卸载清理定时器和请求，失败保留当前画面并提示读取失败，避免高频重试。重置倒计时可在本地更新，不产生网络请求。

## 4. 管理端

建议新增 admin/gpt-quota，作为完整管理员配置入口；普通用户及现有子管理员默认无配置、采集权限，新路由不加入子管理员通用权限白名单。用户入口跟随现有 JWT、面板限流、审计和 BackendModeUserGuard；因此“所有登录用户”是指通过现有系统模式门禁的用户。

| 配置/操作 | 默认或约束 |
| --- | --- |
| 公开展示开关 | 默认关闭；管理员选好账号后开启；关闭时隐藏用户菜单，直接访问返回“未开启” |
| 候选账号 | 平台 OpenAI、类型 OAuth、未软删除的普通主账号；排除任意 shadow、PAT、Agent Identity、Setup Token；暂停、错误、不可调度账号仍可展示旧快照 |
| 初始选择 | 空；不自动选择所有 c- / d- 账号；同一 chatgpt_account_id 重复选择时告警 |
| 时间与时区 | 固定使用全局服务时区 Asia/Shanghai；每天 09:30—18:00 |
| 后台间隔 | 30 / 60 分钟；60 分钟默认 09:30、10:30…17:30、18:00，包含收尾采集 |
| 页面读取间隔 | 固定前端 15 分钟，仅影响本站只读接口 |
| 手动刷新 | 单个同步返回结果；全部刷新异步分批执行，管理页读取各快照状态；允许时段外刷新 |
| 执行反馈 | 管理端显示成功/失败/跳过数量和脱敏失败类别；使用冷却和 singleflight 防重复 |
| 配置保存 | 校验账号资格、前缀、别名和配置版本；冲突返回 409，要求重新读取 |

首版明确排除任意 shadow、PAT、Agent Identity、Setup Token；已保存但后来失去资格的条目在管理页标记原因，并从用户响应和后续采集中排除，不静默替换。采集不得因为账号暂停、错误、不可调度或限流而把条目从展示配置中删除。

“手动刷新”指立即发起一次额度采集，不是 OAuth token 刷新、额度窗口重置或消费 reset credit。采集前对非 PAT 账号执行 token 可用性预检；access token 已过期且无 refresh token 时记录 token_unavailable 并跳过，不调用会禁用账号的路径。采集专用窄方法不得调用 SetError、BlockAccountScheduling、UpdateExtra、notifyOpenAIAutoReset 或 CacheResetCreditsSnapshot，也不得发起 reset-credit、模型推理或测试对话。

## 5. 数据流与服务边界

| 路径 | 执行过程 |
| --- | --- |
| 用户读取 | 登录鉴权与现有模式门禁 → 读取展示配置 → 按最新账号元数据过滤/归类 → 批量读取独立持久化快照 → 返回白名单字段 |
| 定时采集 | leader lock 单主 → 单行计划槽位条件更新去重 → 校验展示账号资格 → 调用展示专用只读 usage → 归一化 → 条件写入独立快照 |
| 管理员单账号采集 | 完整管理员鉴权 → 校验展示条目 → 账号级冷却/singleflight → 同步调用采集 → 返回脱敏结果 |
| 管理员全部采集 | 完整管理员鉴权 → 异步分批执行选中账号 → 更新各账号快照与最近尝试状态；不建立通用任务队列 |

硬性约束：用户读取、配置读取和管理端快照读取都不调用上游。没有快照、快照过期、用户反复刷新或增加查询参数，都不能触发采集。管理员浏览列表本身也不触发采集。

主动采集只解析 rate_limit.primary_window 与 rate_limit.secondary_window，忽略 additional_rate_limits。窗口时长大于 0 且不超过 6 小时归入 5 小时窗口，大于 6 小时归入 7 天窗口；两个窗口落入同一类别或时长非法时，该类别显示“未提供”。不使用会把过期窗口改成 0 使用率的 Normalize。

快照必须存储在独立展示表中。被动读取 accounts.extra 只能作为只读兼容数据源，不能把主动结果写入 accounts.extra，不能写 codex_5h_*、codex_7d_*、codex_usage_updated_at，也不能写任何会触发调度 outbox 的展示键。这样展示采集不会改变网关暂停、自动用卡或自动重置。

本功能不复用 /monitor 的用户额度链：现有渠道监控受 V1/V2 和监控项绑定约束，OpenAI quota 路径会进入 getOpenAIUsage 的模型探测，且无法满足本功能的共享双列与白天采集合同。

## 6. 排程与并发

### 6.1 排程规则

以本地计划起点为锚点计算固定时刻，不使用“上次任务完成后等待 N 分钟”。

| 模式 | 当日触发时刻 |
| --- | --- |
| 默认 30 分钟 | 09:30、10:00、10:30……17:30、18:00，共 18 次 |
| 60 分钟 | 09:30、10:30……17:30、18:00，共 10 次；默认包含 18:00 收尾 |
| 起点终点不可整除 | 只运行不晚于终点的计划时点，不额外插入终点任务 |
| 非运行日或结束后 | 不自动采集；展示下一个运行日的首个计划时点 |
| 运行中修改配置 | 以新版本计算后续时点；旧采集完成后仍需按 sampled_at 条件写入 |
| 重启/短暂停机 | 在时段内最多补当前最近一个已到期时点，不追补历史槽位 |

开关开启或新选账号后不隐式请求上游，管理员可点击手动刷新；否则等待下一计划时点。页面显示的“下次计划采集”是计划时间，不保证上游成功或全批次同时完成。

### 6.2 执行控制

- 定时任务使用现有 LeaderLockCache/leader lock 单主执行；计划槽位用配置单例中的 last_slot_at 条件更新去重，不能只依靠进程内 mutex。
- 同账号手动和定时采集使用 singleflight 合并，并设置 60 秒冷却；定时采集撞上在途请求时合并，撞上冷却窗口时跳过并复用最近结果。
- 建议初始并发 3、单请求超时沿用 20 秒。上一批跨过下一计划时点时，下一槽位直接跳过并记录 skipped，不排队追补，避免账号增加后任务堆积。
- 单账号失败不阻断其他账号。普通失败等下一计划或管理员操作；429 尊重 Retry-After，缺失时至少退避 5 分钟。
- 快照按 sampled_at 条件更新；旧请求晚返回时不得覆盖更新的快照。服务关闭取消在途请求，重启时只补当前最近计划槽位，不追补历史时点。

## 7. 持久化模型（提案）

沿用 PostgreSQL 持久化配置、展示条目和当前快照，不仅存进程内缓存。实施迁移为 `backend/migrations/243_gpt_quota_display.sql`。

| 对象 | 主要字段 | 约束 |
| --- | --- | --- |
| 展示配置 | enabled、interval_minutes、start_time、end_time、version、updated_by、updated_at、last_slot_at | 单例；时区使用全局 cfg.Timezone（本功能要求为 Asia/Shanghai）；乐观版本检查；默认关闭 |
| 展示条目 | id、account_id、display_name、selected、updated_at | account_id 唯一且引用账号；归类取当前原名，不维护第二份可漂移的组织映射 |
| 当前快照 | account_id、five_hour、seven_day、sampled_at、last_attempt_at、last_attempt_status、retry_after、source、updated_at | 每账号一行；窗口可空；失败只更新尝试状态，不覆盖成功额度和采样时间；独立于 accounts.extra |

成功快照在短事务中按 sampled_at 条件发布。快照只存展示所需数值，不存完整上游 JSON、邮箱、ChatGPT account ID、reset credit 明细或 token。配置撤销/账号删除后，即便历史快照尚未清理，也不得出现在用户响应中。

首版用户读接口直接批量读数据库，避免引入额外跨实例展示缓存失效协议；账号查询必须批量加载所需元数据，不按卡片逐个查询。若后续确有读压再加入短 TTL 共享缓存，仍不得发生读穿透上游。

## 8. API 提案

下表是设计阶段的接口提案；实施后的最终路径、请求体与错误码见第 13.2 节。

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| GET /api/v1/gpt-quota | 已登录用户 | 返回 enabled、server_time、poll_interval_seconds、schedule、next_scheduled_at、xunyou/wsdashi 两组卡片；只读 |
| GET /api/v1/admin/gpt-quota | 完整管理员 | 读取配置、候选账号、快照摘要；候选账号支持分页和搜索 |
| PUT /api/v1/admin/gpt-quota/config | 完整管理员 | 保存配置、账号选择与别名；携带 expected_version，冲突返回 409 |
| POST /api/v1/admin/gpt-quota/refresh | 完整管理员 | 指定单个展示条目时同步返回结果；全量刷新异步分批执行，管理页读取各快照状态，不提供通用任务资源 |

用户单卡片响应白名单：id、display_name、five_hour、seven_day、sampled_at、stale。管理端额外返回 last_attempt_at、last_attempt_status 和脱敏错误类别。窗口字段为 remaining_percent、reset_at、reset_time_source。分组由服务端完成，不接收用户过滤任意 account_id。

用户错误状态只返回稳定类别，不返回上游原文、URL、邮箱或凭据。管理操作复用现有审计；管理员默认豁免面板限流，因此刷新保护依赖 60 秒冷却和 singleflight，不把面板限流当作防重复机制。审计 extra 只能使用已批准白名单字段，不写别名、账号邮箱或上游错误正文。

## 9. 失败与关闭行为

| 场景 | 结果 |
| --- | --- |
| 从未成功采集 | 暂无数据，缺失值不解释为 0% 使用率 |
| 上游超时/401/403/429/解析失败 | 保留旧成功快照及其时间；记录本次失败类别；不改账号调度、订阅或网关额度 |
| 本站读接口失败 | 前端保留现有画面，标记读取失败；按正常周期重试 |
| 展示关闭 | 用户读取返回关闭状态与空分组，不泄漏旧快照；自动采集停止，不接受新手动采集 |
| 服务采集暂停 | 继续展示旧快照，明确“自动更新暂不可用”；管理员仍可手动刷新 |
| 账号删除/不再选中/不再符合平台与前缀规则 | 后续用户读取立即排除；任务发布前再次校验 |
| 数据库不可用 | 返回服务错误；不因本地无快照而临时调用上游 |

关闭展示不自动删除配置和快照，重新开启后仍需显示真实采样时间及状态。已在途上游请求尽力取消，不能承诺撤回已经发送的网络请求。

## 10. 实施范围与验证门禁

| 层次 | 预计改动（审核通过后） |
| --- | --- |
| 后端 | 独立展示服务、仓储、用户/管理 handler、路由与后台生命周期；OpenAI quota 服务抽取只读 usage 方法 |
| 数据库 | 追加迁移，不修改历史迁移；需要 Ent schema 时按仓库规则生成 Ent/Wire |
| 前端 | 用户双列页、管理配置页、额度卡片、API client、菜单/路由和中英文文案 |
| 文档 | 对应 Wiki 与组件 README；实施完成后另记验证证据，不能把设计标成已交付 |

| 验收项 | 必须覆盖的场景 |
| --- | --- |
| 账号范围 | OpenAI OAuth 普通主账号；PAT/Agent Identity/Setup Token/shadow/软删除排除；未选中排除；名称大小写、空白、编号提取、别名脱敏、自然排序、改名与删除 |
| 权限 | 通过现有模式门禁的登录用户得到相同列表；匿名不能访问；普通用户/子管理员不能保存配置或刷新；不可通过构造 ID 越权 |
| 读写隔离 | 页面打开、定时轮询、F5、快照缺失/过期、数据库失败均不会调用上游；使用 spy/fake 验证调用数为零 |
| 上游查询 | 只访问 usage；不发模型推理、reset-credit 查询/消费或调度变更；token 预检失败不禁用账号；原有 QueryUsage 行为有回归测试 |
| 数据口径 | 0% 使用率、100%、大于 100%、null、未知窗口长度、窗口位置交换、非法数值、缺失重置时间、倒计时结束 |
| 失败保留 | 采集失败不更新时间、不清空成功额度；仅一个窗口有效时不混入旧窗口 |
| 排程 | 09:29、09:30、18:00、18:01，30/60 分钟两档、跨日、配置变更、重启最近槽位、跨槽位跳过 |
| 并发 | 两个服务实例 leader lock/槽位去重；手动/定时碰撞；singleflight、冷却、429 退避；旧请求按 sampled_at 条件写回拒绝 |
| 前端 | 双列/单列、空列、长名称、暂无数据、失败/过期/非采集时段；15/30 分钟轮询、隐藏暂停、卸载清理 |
| 持久化 | PostgreSQL 迁移、leader lock/槽位条件更新、失败事务回滚、重启后快照回读；不能只用 mock 代替多实例验证 |

实现阶段运行专项 Go 测试、必要的 PostgreSQL 集成测试、前端 Vitest/typecheck/lint 和 `git diff --check`。浏览器验收使用隔离测试账号/模拟上游；真实账号的主动采集另按实施授权范围执行。本轮仅校验设计文档路径、内容与格式，不运行构建、服务、迁移或真实采集。

## 11. 本轮综合审核决策

已吸收 Claude 与 Grok 审核中的共同阻断项：主动采集不得改变账号状态；展示快照与调度数据隔离；只解析 rate_limit 两个窗口；普通 OAuth 账号资格收敛；用户路由服从现有模式门禁；固定上海时区和每天排程；不建设通用任务队列。

本轮明确采用以下折中：调度使用现有 leader lock 和单行槽位条件更新，账号请求使用 singleflight/冷却，快照使用独立表并按 sampled_at 条件更新。这样保留多实例安全边界，同时避免引入任务表、租约、执行代次和任务明细。

60 分钟间隔默认包含 18:00 收尾；这只改变排程计算，不改变数据模型和权限边界。

渠道监控 /monitor 明确作为否决的复用方案：它受 V1/V2 和监控项绑定约束，OpenAI 路径会进入模型探测，且无法满足本功能的共享双列与白天采集合同。

## 12. 设计审核清单

| 项目 | 当前推荐结论 | 审核关注点 |
| --- | --- | --- |
| 整体需求 | 所有登录用户共享列表；左迅游、右速宝 | 已确认，不改回组织授权隔离 |
| 账号资格 | OpenAI OAuth 主账号，管理员勾选；shadow 排除 | 是否确有需要展示的特殊认证账号，避免仅按名称误纳入 |
| 默认采集 | 每天北京时间 09:30—18:00，30 分钟一轮 | 60 分钟档默认包含 18:00 收尾 |
| 只读数据源 | 抽取现有 wham usage 查询，不复用会发模型探测的用量入口 | 保持原调用者和 token/代理合同；不附带 reset-credit 明细请求 |
| 展示发布 | 默认关闭、初始选择为空，显式发布 | 避免上线即公开账号名称或触发真实采集 |
| 一致性 | 独立持久化快照、leader lock、槽位条件更新、账号级 singleflight、sampled_at 条件写入 | 不建立通用任务表/租约/代次；实现前验证多实例与崩溃恢复 |
| 审核结果 | **已通过并实施（2026-09-28）** | 已吸收 Claude/Grok 共同意见；实施细节与验证见第 13 节 |

## 13. 实施记录（2026-09-28）

### 13.1 代码位置

| 层次 | 路径 |
| --- | --- |
| 迁移 | `backend/migrations/243_gpt_quota_display.sql`（main 已有 239—242，不重号） |
| 服务 | `backend/internal/service/gpt_quota_display.go`；只读上游方法 `OpenAIQuotaService.QueryUsageReadOnly`（`openai_quota_service.go`） |
| 仓储 | `backend/internal/repository/gpt_quota_display_repo.go` |
| Handler 与路由 | `backend/internal/handler/gpt_quota_display_handler.go`；`backend/internal/server/routes/user.go`、`admin.go` |
| Wire | `ProvideGPTQuotaDisplayService` 构造时 `Start()`；`provideCleanup` 调用 `Stop()` |
| 前端 | `frontend/src/api/gptQuotaDisplay.ts`、`views/user/GPTQuotaView.vue`、`components/user/GPTQuotaColumn.vue`、`views/admin/GPTQuotaDisplayView.vue`、`composables/useGPTQuotaVisibility.ts`、`i18n/locales/{zh,en}/gptQuota.ts` |

### 13.2 最终接口

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| GET /api/v1/gpt-quota | 登录用户（JWT、BackendModeUserGuard、面板限流、审计） | 返回 enabled、server_time、poll_interval_seconds=900、schedule{start,end,interval_minutes,timezone}、next_scheduled_at、in_schedule_window、groups.xunyou / groups.wsdashi。卡片只含 id、display_name、five_hour、seven_day、sampled_at、stale；关闭时分组为空 |
| GET /api/v1/gpt-quota/status | 登录用户 | 只返回 {enabled}，供侧栏决定是否显示菜单 |
| GET /api/v1/admin/gpt-quota | 完整管理员 | config、计划、全部已选条目（含失去资格条目的 reason、warnings、快照、最近尝试状态、retry_after）和本实例最近批次计数；不受公开开关影响 |
| GET /api/v1/admin/gpt-quota/candidates | 完整管理员 | search、page、page_size；数据库侧 COUNT + LIMIT/OFFSET 分页，列出未删除的 OpenAI OAuth 账号及资格原因（c-/d- 前缀账号按编号自然排序在前），可添加的账号还要有前缀；搜索词中的 `%`、`_` 按字面匹配 |
| PUT /api/v1/admin/gpt-quota/config | 完整管理员 | {enabled, interval_minutes, expected_version, entries[{account_id, display_name}]}；配置和选择在同一事务保存；版本不符返回 409 `GPT_QUOTA_CONFIG_CONFLICT`；响应可带 warnings（`duplicate_chatgpt_account`） |
| POST /api/v1/admin/gpt-quota/refresh | 完整管理员 | 必须且只能二选一：{entry_id} 同步返回 status 和条目；{all: true} 返回 202 异步批次，已有批次时 409 `GPT_QUOTA_BATCH_RUNNING`；展示关闭时 409 `GPT_QUOTA_DISPLAY_DISABLED`；畸形请求体 400 |

四个管理路由都不在子管理员白名单内。

### 13.3 实施澄清

- **菜单开关**：公开开关只存于 `gpt_quota_display_config`，不复制到 public settings。侧栏调用 `/gpt-quota/status`，按 opt-in 处理（未加载或读取失败时隐藏），结果缓存 5 分钟，侧栏随路由重新挂载时过期重读；用户页读取和管理员保存后会同步该状态。
- **冷却与退避**：60 秒冷却和 429 退避（尊重 Retry-After，缺失时 5 分钟，最长 24 小时）由快照行的条件 upsert（`ClaimAttempt`）实现，跨实例生效；进程内再用 singleflight 合并同账号请求。冷却内返回 `skipped_cooldown`，退避内返回 `skipped_backoff`，都不访问上游。
- **尝试状态**：`ClaimAttempt` 写 `running`；`FinishAttempt` 和 `PublishSnapshot` 只在 `last_attempt_at` 仍等于本次尝试时改状态，晚返回的旧尝试不覆盖新状态；快照另按 `sampled_at` 条件发布。失败类别为 `token_unavailable`、`account_unavailable`、`not_supported`、`unauthorized`、`forbidden`、`rate_limited`、`timeout`、`upstream_error`、`request_failed`、`parse_failed`、`no_supported_windows`、`cancelled`、`internal_error`。
- **只读上游**：`QueryUsageReadOnly` 只请求 wham/usage；拒绝 shadow 与 Agent Identity，不创建或恢复 agent task；在调用 token provider 前拦截"access token 已过期且无 refresh token"；不写 `accounts.extra`，不查 reset-credit。窗口缺少或为 null 的 `used_percent` 按缺失窗口处理，不会被解码成 0% 已用。调用方取消或整体超时优先记为 `cancelled` / `timeout`。
- **重置时间**：上游 `reset_at`、`reset_after_seconds` 是 int64，缺失即 0，所以只接受正值，0 按"重置时间未提供"展示；晚于采样时间 8 天以上（如毫秒时间戳）也视为未提供。剩余比例四舍五入到 1 位小数。
- **默认展示名**：前缀加原样编号（`c-01`）；无编号时为前缀加 `#条目ID`，避免多张卡片同名。别名先去首尾空白再校验，另拒绝含 `sk-`、UUID 片段或 20 位以上连续字母数字的内容。
- **排序**：迅游固定在速宝之前（用户页、管理页已选列表、候选列表一致）；同组内有编号的按数值升序（`c-002` 在 `c-10` 前），无编号的排在后面，再按规范化名称和账号 ID。
- **保存**：管理页的批次进度轮询只刷新条目状态和计数，不更新编辑基线的 `version`，避免本地未保存修改绕过乐观锁。新增条目必须通过资格与前缀校验。已保存但后来失去资格（删除、改为 PAT/shadow、改名）的条目可以保留或移除，不挡保存；用户侧和采集仍排除这些条目。
- **过期**：取最近一个已过 5 分钟宽限的计划时点 S，`sampled_at` 早于 S 减 60 秒（冷却提前量）即 stale；夜间不因时间流逝变成过期。从未成功采集时 `sampled_at` 为 null，前端显示"暂无数据"。
- **时区**：排程、过期判断与"下次计划采集"固定使用 Asia/Shanghai（缺少 tzdata 时回落 UTC+8），不再跟随全局 `timezone` 配置，避免部署为 UTC 时采集时段偏移。
- **排程**：每 30 秒检查一次。顺序为：读取已选条目 → 以检查并设置的方式原子占用本实例批次 → 条件领取槽位 → 执行批次。读取失败、手动批次执行中或领取失败都不消耗槽位（领取失败时释放占用并恢复上一批次统计），在槽位有效期内下次检查重试；定时批次占用后，同实例的手动全量刷新返回 409，避免"槽位已领取但批次未执行"。槽位只在该时点到下一时点之间有效（18:00 末槽宽限 5 分钟），所以重启只补最近一个到期槽位。开启展示或修改间隔时，保存事务把 `last_slot_at` 抬到保存时刻（只进不退），当前已过的槽位不会被隐式补跑，等待下一计划时点。批次并发 3，单批上限 25 分钟，leader lock TTL 30 分钟；每次派发前重读开关，关闭展示后不再派发新请求；批次结束时若已跨过下一槽位，直接领取该槽位并记录 skipped 日志，不补跑。
- **停机**：`Stop` 取消服务根 context，在途上游请求随之取消；尝试结果用独立短超时 context 记为 `cancelled`。手动全量批次和单条同步刷新都由服务 WaitGroup 跟踪。单条刷新后端上限 45 秒，前端该请求单独使用 60 秒超时。
- **审计**：沿用现有管理审计中间件，PUT 配置的脱敏请求体会进入操作日志，其中包含别名；别名已按 §3.1 校验为非敏感内容。本功能不写审计 extra。
- **未单独实现**：第 9 节"服务采集暂停"没有独立开关（设计已去掉自动采集开关），不单独展示。批次计数只保存在执行实例内存中，多实例下管理页以各条目的最近尝试状态为准。

### 13.4 验证

| 项目 | 结果 |
| --- | --- |
| 后端构建与静态检查 | `go build ./...`、`go vet`（service/handler/repository/server/cmd/server）通过；golangci-lint v2.14（go1.27 构建）`--new-from-rev=HEAD` 对改动包 0 issues，含 integration tag |
| 后端测试 | `internal/service` 全量通过（GPT 额度用例另跑 `-race`）；handler、server、repository、cmd/server、migrations 通过 |
| 前端 | `vue-tsc --noEmit` 与改动文件 eslint 通过；vitest 全量通过 |
| PostgreSQL | 本机无 Docker，改用临时 embedded PostgreSQL 16 并设置 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN`：`TestGPTQuotaDisplayRepositoryRoundTrip` 与 `TestMigrationsRunner*` 通过（从零执行全部迁移含 243）。另做过一次一次性端到端冒烟（真实 handler + service + repository + PostgreSQL，fake 上游），覆盖默认关闭、保存/409/非法别名、读路径零上游、单条刷新与冷却、全量批次计数、删除账号即时排除、关闭后刷新 409，已通过后删除。CI 使用 PostgreSQL 18 镜像，仍需在 CI 再跑一次集成测试 |
| PostgreSQL 18 | 用 CI 同版本（embedded PostgreSQL 18）执行 `TestGPTQuotaDisplayRepository*`（含候选分页/排序/转义、16 路并发领取槽位与采集尝试各只有一个成功）与 `TestMigrationsRunner*`，全部通过 |
| 真实链路 API 验收 | 真实服务二进制（AUTO_SETUP 从零迁移）+ PostgreSQL 18 + Redis 8，真实 JWT 登录：匿名 401、普通用户与子管理员访问管理接口 403（子管理员为 `ADMIN_PERMISSION_DENIED`）、候选分页/搜索/资格、保存校验与 409、用户卡片白名单与脱敏、单条/全量刷新与计数、软删除即时排除、关闭后 409、配置写入审计。测试账号均为"token 已过期且无 refresh token"，采集在预检处停止，未访问 OpenAI；账号状态与 `accounts.extra` 未被修改 |
| 浏览器验收 | Playwright 驱动 Edge：管理员配置页（失去资格条目可移除、保存、双会话版本冲突提示、单条刷新状态）、普通用户侧栏随开关显隐、桌面左右双列与手机单列、剩余比例/未提供/暂无数据/已到重置时间、无邮箱、子管理员无菜单且直接访问被重定向，23 项全部通过。截图与脚本保存在本机 `E:	mp\e2e`，不入库 |
| 未执行 | 真实 OpenAI 账号采集（需上线后用隔离账号验证） |
