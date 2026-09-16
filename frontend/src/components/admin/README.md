# 管理端组件权限约定

## 子管理员角色

管理端用户弹窗支持 `admin`、`sub_admin`、`user`。`sub_admin` 的权限清单必须通过 `GET /api/v1/admin/permissions/catalog` 获取, 不在组件中维护第二份目录。

## 页面能力

| 权限 | 页面 | 子管理员能力 |
| --- | --- | --- |
| `admin.subscriptions` | `/admin/subscriptions` | 查看、筛选、单条重置配额/日限、按当前筛选一键重置全部分页结果的日限 |
| `admin.usage` | `/admin/usage` | 查询、统计、排行、错误详情、导出 |
| `admin.token_analysis` | `/admin/token-analysis` | 查看统计、项目、请求输入和索引状态 |

订阅分配/延期/撤销/恢复、使用记录清理、Token 立即索引只对完整管理员显示。隐藏按钮只是体验控制, 后端白名单必须独立拒绝。

## 分组 OpenAI 策略

- `group/ReasoningEffortPolicyFields.vue` 仅服务 OpenAI/Composite 分组，映射按 `exact|prefix|suffix + model` 范围分组，同一范围可增加多个 from/to pair。类型和模型均空表示全局；相同范围不可重复，同范围的 source 也不可重复。
- 推理上限启用时可选 `downgrade` 或 `deny`；没有上限时 over-limit 控件禁用，payload 仍保留规范化的默认 `downgrade`。
- `GroupsView.vue` 仅在 OpenAI/Composite 分组提交 `force_openai_fast` / `free_openai_fast`，切换其他 platform 时两字段必须归零。全局 OpenAI Fast Policy 仍可以对强制后的 priority 做 filter/block，前端不得把分组开关描述为绕过全局策略。
- `channel/PricingEntryCard.vue` 和 `channel/IntervalRow.vue` 把 cache-write 拆为 5m 与可选 1h 价；1h 空值表示后端沿用 5m，显式 0 表示免费，不可在表单归一化中混淆。

## 依赖接口

- 使用记录账号/分组筛选走 `/admin/usage/search-accounts` 和 `/admin/usage/search-groups`。
- `usage/UsageFilters.vue` 与 `usage/UsageTable.vue` 把 `live` 作为独立 request type 展示和筛选, 不映射为 legacy stream；可选 `session_id` 只用于客户端会话关联。
- 订阅分组筛选走 `/admin/subscriptions/search-groups`; 不得调用 `/admin/groups/all`。
- 订阅组织筛选使用内部值 `xunyou` / `wsdashi`, 页面显示“迅游”/“速宝”。管理列表始终按日剩余比例升序为主序, 表格选择的 `sort_by` / `sort_order` 仅作为同剩余比例时的次序。
- 一键重置日限只使用最近一次成功列表请求对应的 `status,user_id,group_id,platform,organization`; 每次列表请求开始立即失效快照, 请求失败后按钮继续禁用。打开确认框后再改 UI 不能改变本次请求; 同一确认框失败重试复用幂等键, 重新打开确认框生成新键。成功后按 `reset_count` 区分正数和零匹配提示, 失败保留确认框与当前筛选。
- 新增管理权限时同步后端 catalog/路由白名单、前端路由 `adminPermission`、侧边栏、i18n 和测试。

## 0.2.5 订阅批量操作

- `subscription/BulkSubscriptionActionDialog.vue` 只供完整管理员选中行操作；子管理员继续只显示单条重置与按筛选重置日限，批量选择、批量弹窗和命令入口均不得向子管理员开放。
- 批量 extend/reset/revoke/restore 按可操作状态过滤选中订阅、调用 `/admin/subscriptions/bulk-action`，部分失败保留失败项；`reset-daily-filtered` 仍是独立的已应用筛选快照与幂等操作，不要把两者合并。
