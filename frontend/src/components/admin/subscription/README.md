# 管理端订阅批量动作

- `BulkSubscriptionActionDialog.vue` 只在完整管理员选中可操作订阅后挂载；`bulkSubscriptionOperation.ts` 规范化 extend/reset/revoke/restore 的输入与结果。
- 每次打开确认生成独立 `Idempotency-Key`；部分失败保留错误项，成功项从选择中移除。不要复用按筛选全量重置日限的操作键或范围快照。
- 修改动作集合、确认/重试语义时同步本目录 `__tests__`、`frontend/src/api/admin/subscriptions.ts` 与 `SubscriptionsView.spec.ts`。
