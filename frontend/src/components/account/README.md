# Account Components

本目录维护管理端账号创建、编辑、认证、配额展示和凭据构建组件。账号字段变更必须同时检查 UI 显隐、初始化、提交 payload 和后端账号语义。

## 主要文件

| 文件 | 职责 |
|---|---|
| `CreateAccountModal.vue` | 创建 Anthropic/OpenAI/Gemini/Antigravity/Grok 等账号，组织 OAuth/API Key/Setup Token 流程 |
| `EditAccountModal.vue` | 编辑凭据、模型映射、endpoint capability、pool mode、订阅档位和平台扩展 |
| `credentialsBuilder.ts` | 纯函数形式构建、清理和规范化账号 credentials |
| `AccountUsageCell.vue` / `UsageProgressBar.vue` | 账号额度、剩余额度和未知状态展示 |
| `OAuthAuthorizationFlow.vue` | OAuth 授权输入和状态复用 |
| `UpstreamBillingRateCell.vue` | OpenAI API Key 账号的上游计费倍率快照、stale 状态和手动 probe |
| `GrokBaseUrlPresets.vue` / `HeaderOverrideEditor.vue` | Grok 上游地址 preset 与自定义请求头编辑 |

## API Key 创建契约

`CreateAccountModal.vue` 的 API Key 路径统一提交 `credentials.base_url` 和 `credentials.api_key`：

| 平台 | 默认 Base URL | API Key 占位 | 额外规则 |
|---|---|---|---|
| OpenAI | `https://api.openai.com` | 由 compatible provider preset 决定 | preset 可同时调整 base URL、key 占位和 endpoint capability；提交前仍应用本地 capability 构建逻辑 |
| Gemini | `https://generativelanguage.googleapis.com` | `AIza...` | 同时提交 `tier_id` |
| Grok | `https://api.x.ai/v1` | `xai-...` | 支持 Grok API Key 账号，不显示空的通用 hint |
| Anthropic | `https://api.anthropic.com` | `sk-ant-...` | 可配置 header override 等既有扩展 |

OpenAI-compatible provider preset 是本地功能，不能在上游合并时被整块替换。Grok API Key 是 0.1.153 新增路径，两者必须同时保留；对应回归位于 `__tests__/credentialsBuilder.spec.ts` 和 `__tests__/CreateAccountModal.grok.spec.ts`。

## 编辑与凭据规则

### OpenAI `plan_type`

- 仅 OpenAI OAuth、非 Spark 影子账号允许手动覆盖。
- `credentialsBuilder.ts#readPlanType` 只接受字符串，脏数据不回填。
- 下拉预设包含 Plus、Pro、Free，同时保留后端返回的未知 canonical 值。
- 选择空值表示恢复自动识别，提交时必须删除 `credentials.plan_type`，不能发送 stale value。
- 该字段影响 `Account.IsOpenAIChatGPTSubscription`，空、`free`、`abnormal` 不视为订阅账号。

### Pool mode retry

- `pool_mode_retry_count` 只在 pool mode 开启时提交，关闭时与 status code 列表一起删除。
- 前端通过 `normalizePoolModeRetryCount` 收口数值；后端 `Account.GetPoolModeRetryCount` 读取并限制范围。
- 0 表示不做同账号重试；配置值现已在 Anthropic、Gemini 和通用转发 failover 路径生效，不能退回硬编码次数。

### Grok Base URL

- Grok API Key 默认使用官方 credit-backed API。
- Grok OAuth 默认使用 CLI subscription proxy；旧的官方 API base URL 会在运行时归一到 CLI proxy。
- 自定义 OAuth base URL 必须通过 `xai.ValidateTrustedBaseURL`。未显式允许不安全覆盖时，普通第三方 URL回落到默认 CLI proxy。

### 管理端账号复制

- 账号操作菜单只为 API Key、upstream、Bedrock、service account 等静态凭据账号显示复制入口；OAuth/cookie 等旋转凭据和 credential shadow 不可复制。
- 前端 `adminAPI.accounts.duplicate` 为同一账号在内存和 sessionStorage 中复用 `Idempotency-Key`, 成功后才清理；超时/网络失败时保留 key 供重试恢复同一个副本。
- 后端返回的新账号默认不可调度，前端只刷新列表和显示结果，不自动启用。

### 上游计费倍率与账号外链

- upstream billing probe 只适用于 `platform=openai && type=apikey`; 单个/批量 probe 结果更新 `account.extra.upstream_billing_probe`，UI 根据采集时间显示 stale 状态。
- 账号名称只有在上游 URL 可安全解析为 HTTP(S) 站点时才渲染外链；缺失、非法或凭据型值保持纯文本，不能拼接为可点击 URL。
- probe 设置、账号快照和调度成本倍率属于同一合同；修改 `UpstreamBillingRateCell.vue` 时同步 API client、账号类型和对应 spec。

### Antigravity refresh token

- 批量 refresh-token 导入必须把管理员原始输入继续传入 OAuth 组合逻辑；解析结果不能替代原始 refresh token，否则手工 token 会在授权流程中丢失。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/account
cmd.exe /c pnpm --dir frontend run typecheck
```

修改 credentials 时还应运行后端账号与网关聚焦测试，确认 UI payload 和服务端读取契约一致。
