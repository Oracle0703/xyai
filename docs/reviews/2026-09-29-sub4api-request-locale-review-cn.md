# OpenAI 请求时区与语言迁移审核

- 审核日期：2026-09-29
- 审核分支：`feature/hy/10212_sub4api_request_locale`
- 当前 HEAD：`bdf31e87223058200c614a807e24169e12bb31ea`（与本地 `main` 相同）
- 审核方式：只读代码审查。不改业务代码，不提交，不创建 PR，不启动服务。
- 对照语义：参考 MACOS-DO/sub4api，只迁移 OpenAI 账号级请求时区，并核对现有 Accept-Language 透传。不迁移 Codex 打票、降智检测或 BPS。

## 结论

未发现会误改普通文本、破坏请求或绕过环境上下文解析的 P0 阻断缺陷。

部署前有两项需要明确决策：

1. 缺省 `Asia/Singapore` 会立即改写全部现有 OpenAI 账号的 Responses HTTP、透传和 WebSocket 流量。这符合当前实现语义，但是兼容性影响，不是可选开关。
2. 批量更新可以绕过时区白名单。热路径仍会忽略非法值，不会把脏时区送上游。

| 严重度 | 数量 | 说明 |
| --- | ---: | --- |
| P0 | 0 | 无请求破坏或解析绕过 |
| P1 | 1 | 确认的默认时区兼容性影响 |
| P2 | 2 | 1 个确认的写入合同缺口，1 个潜在覆盖缺口 |
| P3 | 3 | 文案、文档和测试缺口 |

## 工作区基线

`git diff main...HEAD` 为空。本轮功能都在未提交工作区。

已跟踪修改 14 个文件，另有 5 个未跟踪文件：

- `backend/internal/service/openai_request_locale.go`
- `backend/internal/service/openai_request_locale_test.go`
- `backend/internal/service/openai_request_timezone.go`
- `backend/internal/service/openai_request_timezones.txt`
- `frontend/src/components/account/OpenAIRequestTimezoneField.vue`

`git diff --check` 对已跟踪改动无空白错误。新增文件为 LF。时区目录 463 行，无空行、无重复名。没有数据库迁移，也没有打票、降智检测或 BPS 相关改动。

## 已核对且符合当前语义

- `rewriteOpenAIRequestEnvironment` 要求 trim 后的整段文本是一个完整的 `environment_context`。普通句子、带前缀的示例、代码围栏、引用块和历史说明文字保持原样。
- 重复 `timezone`、自关闭、嵌套标签、错误闭合和两个根节点会被拒绝，原文不改。
- 只处理 user 角色的字符串 `content` 或 `input_text`。assistant、developer、tool、`function_call_output` 不改。`current_date` 不改。
- `content_item_kinds` 不是门闩。标成 `user.text` 的完整环境上下文也会改时区，日期仍保留。这是测试锁定的行为。
- Responses `Forward()` 在透传分支之前改写。透传使用改写后的 body。WebSocket 的 `parseClientPayload` 覆盖首轮和后续轮，包括原生 WS 与 HTTP bridge。
- 创建、编辑、Codex session 导入、复制和 `UpdateAccountExtra` 对 OpenAI 账号写入并校验该字段。非 OpenAI 平台在这些路径不能写入非空时区。空字符串允许保存，读取时回落默认值。
- `Accept-Language` 没有新增强制改写。HTTP 与透传仍走既有白名单。WebSocket 握手复制非空值。API Key 的 `header_overrides` 在复制之后覆盖，OAuth 覆写仍是 no-op。
- 管理端创建和编辑 OpenAI 账号会显示时区选择器，提交 `extra.openai_request_timezone`。列表接口注册在 `/:id` 之前，不会被当成账号 ID。

## 问题

### P1. 缺省时区会改写全部现有 OpenAI 流量

- 类型：确认的兼容性影响。实现与“默认 `Asia/Singapore`”一致，但上线前必须验收。
- 位置：
  - `backend/internal/service/openai_request_timezone.go:58-64`
  - `backend/internal/service/openai_request_locale.go:99-103`
  - `backend/internal/service/openai_gateway_forward.go:23`
  - `backend/internal/service/openai_ws_forwarder_ingress.go:266`
- 问题：`extra` 缺失、空字符串或非法值时，`OpenAIRequestTimezone()` 都返回 `Asia/Singapore`。所有 `platform=openai` 账号都会进入改写。
- 影响：旧账号无需管理员保存，就会把结构化 `environment_context` 的 `timezone`，以及顶层 Web Search 的 `user_location.timezone`，改成新加坡。`current_date`、国家和城市保持原值，模型时间和搜索“今天”可能与客户端时区、城市不一致。OpenAI 兼容供应商只要平台仍是 `openai`，走 Responses 时同样被改写。
- 建议：若实验就是全量默认新加坡，用真实 Codex 请求验收后再部署。若只想验证迁移、避免影响存量流量，缺省应保留客户端时区，只有 `extra.openai_request_timezone` 显式合法时才改写。

### P2. 批量更新可以绕过时区白名单

- 类型：确认问题。
- 位置：
  - `backend/internal/handler/admin/account_handler.go:2325-2365`
  - `backend/internal/service/admin_account.go:966-1129`
- 问题：创建、编辑、复制、`UpdateAccountExtra` 会调用 `ValidateOpenAIRequestTimezoneExtra`。`POST /api/v1/admin/accounts/bulk-update` 会合并任意 `extra`，没有做同样校验。
- 影响：管理接口可以把非法时区写入 OpenAI 账号，或把该字段写入非 OpenAI 账号。热路径仍会忽略非法值和非 OpenAI 平台，不会把脏值送上游。复制这种账号时，`DuplicateAccount` 会因校验失败而拒绝复制。
- 建议：批量更新在写库前按目标账号平台调用同一校验。混合平台或非法值整批拒绝。

### P2. Chat Completions、Messages、Alpha Search 不改写

- 类型：潜在覆盖缺口。
- 位置：
  - `backend/internal/handler/openai_chat_completions.go:250`
  - `backend/internal/service/openai_gateway_chat_completions.go:87-95`
  - `backend/internal/handler/openai_gateway_handler.go:1356`
  - `backend/internal/service/openai_alpha_search.go:290-315`
- 问题：Responses HTTP、透传和 WebSocket 已覆盖。Chat Completions、Anthropic Messages 桥和 Alpha Search 不调用 `normalizeOpenAIRequestLocale`。Alpha Search 会自行组装 `web_search.user_location`。
- 影响：Codex Responses / WS 覆盖符合本轮语义。走 `/v1/chat/completions` 的 Web Search，或 Alpha Search 的 `settings.user_location`，会保留客户端时区。
- 建议：若本轮只验收 Codex Responses，把这三条标成范围外。若“HTTP”包含全部 OpenAI HTTP，需要在这些入口补同一函数。

### P3. 加载失败文案与编辑态实际行为不一致

- 类型：确认的文案问题。
- 位置：
  - `frontend/src/components/account/OpenAIRequestTimezoneField.vue:39-48`
  - `frontend/src/i18n/locales/zh/admin/accounts.ts:736-738`
- 问题：列表加载失败时只置 `loadFailed`，不改 `modelValue`。编辑已有时区时，提交仍是原值；提示却写“已保留默认值”。加载完成前，非默认值在选择器里会显示占位符，值本身不会被清掉。
- 影响：管理员可能以为失败后会落成新加坡，实际保存的是原时区。
- 建议：失败文案改为“列表未加载，已保留当前值”。

### P3. 组件说明少写了 Web Search

- 类型：建议性文档缺口。
- 位置：`frontend/src/components/account/README.md:23`
- 问题：README 只写环境上下文。代码还会改顶层 `web_search` / `web_search_*` 的 `user_location.timezone`。`llm-wiki/wiki/backend.md` 写的是完整语义。
- 建议：README 与组件 hint 补上 Web Search 时区。

### P3. 前端和 WebSocket 缺少端到端断言

- 类型：建议性改进。
- 位置：`frontend/src/components/account/__tests__/` 没有 `openai_request_timezone` 断言。WebSocket 只在 `parseClientPayload` 内调用，没有后续轮集成测试。
- 问题：创建、编辑、复制的 UI 提交和 WS 后续轮目前靠代码阅读确认。
- 建议：补一条创建/编辑 payload 断言，以及一条 WS 第二轮 `response.create` 仍被改写的测试。

## 仍需真实上游验收的边界

- 用真实 Codex HTTP、透传和 WebSocket 多轮报文确认 `environment_context` 是否总是单独、完整的 user `input_text`。若和用户问题写在同一段，或含无法闭合的字段，本实现会保持客户端时区，功能静默不生效。
- 确认默认新加坡是否就是本次实验要观察的效果。一旦部署，存量 OpenAI 账号会立即改变环境时区和 Web Search 时区。
- 核对 Web Search：城市、国家保留，时区变为新加坡后，搜索结果的本地日期是否可接受。
- 请求归档和用量 hash 记录的是进入 `Forward()` 之前的客户端 body，上游看到的是改写后的 body。排障时两边时区会不同。

## 验证

本审核会话复跑：

```text
cd backend
go test ./internal/service -run "Test(OpenAIRequest|RewriteOpenAIRequest|NormalizeOpenAIRequest|ForwardOpenAIRequest)" -count=1
```

结果：`ok github.com/Wei-Shaw/sub2api/internal/service`，退出码 0。

以下命令在本审核会话没有历史日志，也没有重跑：

```text
cd backend
go test ./internal/service -count=1

pnpm --dir frontend run check:i18n
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
pnpm --dir frontend exec vitest run src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
```

行号对应当时工作区源码。后续修改后需重新核对。
