# 支付组件

- `PaymentProviderDialog.vue` 用已加载的支付方式合同展示和确认，余额充值/订阅入口还受站点计费模式与服务端支付校验约束；隐藏 UI 不是支付授权。
- 变更方式或回调状态时同步 `PaymentProviderDialog.spec.ts`、支付 API 类型和用户购买视图。
