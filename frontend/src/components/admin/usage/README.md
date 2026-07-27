# 管理端用量组件

本目录维护管理端用量筛选、统计卡片、明细表、排行、导出进度和清理确认。子管理员只拥有只读 usage 能力, `allowCleanup=false` 时不得渲染清理入口。

## 主要合同

| 文件 | 约束 |
| --- | --- |
| `UsageFilters.vue` | 用户/API Key/账号/分组/模型/request type/计费筛选；子管理员使用 compact search API |
| `UsageTable.vue` | 用量明细、IP 地理信息、延迟健康与图片/视频计费展示 |
| `UsageStatsCards.vue` | input/output/cache creation/cache read 等互斥统计口径 |
| `UserTokenRanking.vue` | 按当前筛选条件展示用户 Token 排行并支持下钻 |
| `UsageCleanupDialog.vue` | 仅完整管理员可用的清理确认 |

`UsageFilters.vue` 对外暴露 `setUserKeyword(email)` 和 `getUserSearchRevision()`。路由 `user_id` 的异步邮箱 lookup 必须先记录 revision, 只在 user ID 与 revision 都未变化时回填；用户新输入、清空或程序化设置都不能被迟到响应覆盖。

`live` 与 `cyber` 是独立 request type, 不映射为 legacy stream。可选 `session_id` 只用于客户端会话关联；管理端后端 list 还支持 `request_id` 精确筛选。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/admin/usage src/views/admin/__tests__/UsageView.spec.ts
cmd.exe /c pnpm --dir frontend run typecheck
```
