# 管理端订阅批量动作

- `SubscriptionSelfResetPolicyDialog.vue` 供完整管理员编辑次数和分组。普通写携带加载时的配置版本，冲突后用服务器快照重建表单。结果未知先用原版本确认，再回读。紧急关闭保留库内次数与勾选。关闭回包丢失后，只读到 `off` 不足以证明在途保存已经失效，可显式重发关闭。策略损坏时的明确拒绝允许管理员回到修复表单。

- `BulkSubscriptionActionDialog.vue` 只在完整管理员选中可操作订阅后挂载；`bulkSubscriptionOperation.ts` 规范化 extend/reset/revoke/restore 的输入与结果。
- 每次打开确认生成独立 `Idempotency-Key`；部分失败保留错误项，成功项从选择中移除。不要复用按筛选全量重置日限的操作键或范围快照。
- 修改动作集合、确认/重试语义时同步本目录 `__tests__`、`frontend/src/api/admin/subscriptions.ts` 与 `SubscriptionsView.spec.ts`。
