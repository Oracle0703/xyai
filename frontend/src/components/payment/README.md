# 支付组件

## 0.2.6 合并增量

- `AmountInput.vue` 遇到不合法金额文本时恢复输入框的上一个合法值，保持 DOM、内部文本与父级金额一致。验证为 `__tests__/AmountInput.spec.ts`；支付配置请求的并发等待由 `stores/payment.ts` 负责。

- `PaymentProviderDialog.vue` 用已加载的支付方式合同展示和确认，余额充值/订阅入口还受站点计费模式与服务端支付校验约束；隐藏 UI 不是支付授权。
- 变更方式或回调状态时同步 `PaymentProviderDialog.spec.ts`、支付 API 类型和用户购买视图。
