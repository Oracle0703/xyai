# GPT 账号额度共享展示

状态：2026-09-28 已实施，待上线验收。完整设计与实施记录：`docs/features/gpt-account-quota-display-design-cn.md`（第 13 节为最终接口与实施澄清）。审核稿 `docs/reviews/gpt-account-quota-display-design-review-cn.md` 仅作历史参考。

- 范围：只展示 GPT/OpenAI 上游账号额度，不是用户订阅余额。通过现有模式门禁的登录用户看到同一列表；账号原名 TrimSpace 后按大小写不敏感的 `c-`/`d-` 前缀归入迅游（`xunyou`）/速宝（`wsdashi`），桌面左右双列，移动端单列迅游在前。与组织、分组授权无关。
- 资格：OpenAI OAuth 普通主账号、未软删除；排除 shadow、PAT、Agent Identity，Setup Token 因类型不是 OAuth 天然排除。管理员勾选后才展示，暂停或错误账号仍展示已有快照。已保存但后来失去资格的条目在管理页标原因，用户侧和采集排除，但不挡保存。
- 读写隔离（硬约束）：用户读取、管理页读取、状态接口都只查数据库快照，任何读取路径不得调用上游。主动采集只走 `OpenAIQuotaService.QueryUsageReadOnly`：只请求 wham/usage，不查 reset-credit，不写 `accounts.extra` 或任何调度/自动用卡键，拒绝 shadow 与 Agent Identity，调用 token provider 前拦截"token 已过期且无 refresh token"以免禁用账号。
- 数据：迁移 `backend/migrations/243_gpt_quota_display.sql` 建三张表：单例 `gpt_quota_display_config`（enabled、interval、version、last_slot_at）、`gpt_quota_display_entries`、每账号一行的 `gpt_quota_display_snapshots`。缺失窗口为 SQL NULL；失败只更新尝试状态，不覆盖成功额度和 `sampled_at`。
- 归一化：只解析 `rate_limit.primary_window/secondary_window`，时长 (0, 6h] 为 5 小时、大于 6 小时为 7 天；同类别两个窗口、非法数值或缺失/null 的 `used_percent` 该类为"未提供"，未知时长不归类。`reset_*` 只接受正值且不晚于采样后 8 天。
- 排程：每天固定 Asia/Shanghai（不随全局 timezone 配置）09:30—18:00，30/60 分钟，60 分钟含 18:00 收尾。leader lock + `last_slot_at` 条件更新去重（先读条目、原子占用本实例批次，再领取槽位；任一步失败都不消耗槽位并释放占用），只补最近一个到期槽位，跨槽批次直接跳过下一槽位；开启展示或改间隔时保存事务把 `last_slot_at` 抬到当前时刻，不隐式补跑已过槽位。并发 3，关闭展示后批次停止派发；60 秒冷却与 429 退避（Retry-After，缺省 5 分钟、上限 24 小时）用快照行条件 upsert 跨实例生效，进程内 singleflight 合并。Stop 取消在途请求。
- 过期：最近一个已过 5 分钟宽限的计划时点之后仍无成功采样才 stale（允许 60 秒冷却提前量）；夜间不变过期；无采样显示"暂无数据"。
- 接口：用户 `GET /api/v1/gpt-quota`（卡片白名单 id/display_name/five_hour/seven_day/sampled_at/stale）、`GET /api/v1/gpt-quota/status`；完整管理员 `GET /api/v1/admin/gpt-quota`、`GET .../candidates`（数据库分页，迅游/速宝前缀按编号自然排序在前）、`PUT .../config`（expected_version，409 冲突，同事务）、`POST .../refresh`（`{entry_id}` 同步或 `{all:true}` 异步，关闭时 409）。管理路由不在子管理员白名单。
- 前端：`/gpt-quota`（`views/user/GPTQuotaView.vue`，15 分钟只读轮询、隐藏暂停、可见补读、卸载取消请求）、`/admin/gpt-quota`（`views/admin/GPTQuotaDisplayView.vue`）。菜单开关不走 public settings，侧栏经 `composables/useGPTQuotaVisibility.ts` 调状态接口，opt-in，缓存 5 分钟。
- 验证：service 单测覆盖读路径零上游调用、DTO 白名单、失败保留、429 退避、冷却、跨槽跳过、停机取消、子管理员拒绝；PostgreSQL 集成测试 `gpt_quota_display_repo_integration_test.go`（`go test -tags integration -run TestGPTQuotaDisplayRepositoryRoundTrip ./internal/repository/`；无 Docker 时可设 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 指向任意空 PostgreSQL）。
