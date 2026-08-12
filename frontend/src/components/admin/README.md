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

## 依赖接口

- 使用记录账号/分组筛选走 `/admin/usage/search-accounts` 和 `/admin/usage/search-groups`。
- `usage/UsageFilters.vue` 与 `usage/UsageTable.vue` 把 `live` 作为独立 request type 展示和筛选, 不映射为 legacy stream；可选 `session_id` 只用于客户端会话关联。
- 订阅分组筛选走 `/admin/subscriptions/search-groups`; 不得调用 `/admin/groups/all`。
- 订阅组织筛选使用内部值 `xunyou` / `wsdashi`, 页面显示“迅游”/“速宝”。管理列表始终按日剩余比例升序为主序, 表格选择的 `sort_by` / `sort_order` 仅作为同剩余比例时的次序。
- 一键重置日限只使用最近一次成功列表请求对应的 `status,user_id,group_id,platform,organization`; 初次列表尚未成功时按钮禁用, 后续筛选加载失败时保留原成功快照。打开确认框后再改 UI 不能改变本次请求; 同一确认框失败重试复用幂等键, 重新打开确认框生成新键。成功后按 `reset_count` 区分正数和零匹配提示, 失败保留确认框与当前筛选。
- 新增管理权限时同步后端 catalog/路由白名单、前端路由 `adminPermission`、侧边栏、i18n 和测试。
