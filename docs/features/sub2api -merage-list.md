# sub2api -merage-list

本文件用于记录本项目与远程仓库 `Wei-Shaw/sub2api` 的合并历史。

维护规则:

- 每次合并远程仓库 `https://github.com/Wei-Shaw/sub2api.git` 后, 必须在本文件追加一条记录。
- 只能追加新记录, 不可以直接覆盖或删除已有记录。
- 记录应包含合并分支、上游提交、合并提交、冲突文件、处理方式和验证结果。

## 2026-05-26

| Item | Value |
|---|---|
| Integration branch | `codex/upstream-sync` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `upstream/main` |
| Base before merge | `4dfcbf79` |
| Upstream head merged | `9ef14487` |
| Merge commit | `0a7baaf9` |
| Files changed | 166 |
| Conflict files | `backend/cmd/server/wire_gen.go`, `frontend/src/views/admin/UsersView.vue` |

### Summary

Merged Wei-Shaw/sub2api updates through `v0.1.131` into the dedicated integration branch.

Major upstream changes included:

| Area | Notes |
|---|---|
| User platform quota | Added user x platform quota schema, repository, service, admin APIs, settings defaults, frontend modal/cell, and tests. |
| Gateway / upstream handling | Included upstream HTTP profile changes, response failed SSE handling, and OpenAI/WS continuation fixes. |
| Content moderation | Added configurable content moderation risk threshold support. |
| Docs / assets | Updated README files and partner logos. |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/cmd/server/wire_gen.go` | Kept local user concurrency preset wiring and also used upstream `admin.NewUserHandler(adminService, concurrencyService, serviceUserPlatformQuotaRepository, billingCache)` signature. |
| `frontend/src/views/admin/UsersView.vue` | Kept local `showConcurrencyPresetsModal` state and added upstream `showPlatformQuotaModal` state so both user concurrency presets and platform quota modal remain available. |

### Verification

| Command | Result |
|---|---|
| `go test ./cmd/server -run Wire` | Passed |
| `pnpm --dir frontend test:run src/views/admin/__tests__/UsersView.spec.ts src/components/admin/user/__tests__/UserPlatformQuotaModal.spec.ts src/components/admin/user/__tests__/UserConcurrencyPresetsDialog.spec.ts` | Passed, 16 tests |

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 0a7baaf9
git show --cc 0a7baaf9 -- backend/cmd/server/wire_gen.go frontend/src/views/admin/UsersView.vue
git diff --stat 4dfcbf79..0a7baaf9
git diff --name-only 4dfcbf79..0a7baaf9
git diff 4dfcbf79..0a7baaf9 -- backend/cmd/server/wire_gen.go frontend/src/views/admin/UsersView.vue
```

## 2026-05-29

| Item | Value |
|---|---|
| Integration branch | `codex/upstream-sync` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `upstream/main` |
| Base before merge | `b3ff58955c7e` |
| Merge base | `9ef144874adc` |
| Upstream head merged | `7321e4dea807` |
| Merge commit | `1687bd6a6b75` |
| Conflict files | `backend/internal/handler/endpoint.go`, `backend/internal/handler/gateway_handler.go`, `backend/internal/handler/openai_gateway_handler.go`, `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`, `backend/internal/server/routes/gateway.go`, `backend/internal/service/openai_gateway_chat_completions.go`, `frontend/src/components/account/CreateAccountModal.vue`, `frontend/src/components/keys/UseKeyModal.vue` |

### Summary

Merged Wei-Shaw/sub2api updates through upstream `v0.1.133`.

Major upstream changes included:

| Area | Notes |
|---|---|
| OpenAI gateway | Added embeddings gateway support, endpoint capability gating, usage context preservation, WS usage fixes, and Codex/Claude Code client allowlist support. |
| Gateway compatibility | Added count_tokens filtering, Anthropic/Responses token detail passthrough, context_management sanitization, and model-not-found cooldown behavior. |
| Account and ops | Added account created-at list field, configurable pool retry status codes, quota threshold auto-pause, ops business-limit classification, and already-up-to-date update handling. |
| Groups and pricing | Added group custom `/v1/models` list config and updated model pricing metadata. |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/handler/endpoint.go` | Combined local alias normalization for `/chat/completions`, `/responses`, and `/backend-api/codex/responses` with upstream `/v1/embeddings` normalization. |
| `backend/internal/server/routes/gateway.go` | Kept local request archive/intercept middleware on image generation aliases and added upstream `/embeddings` OpenAI-only route. |
| `backend/internal/handler/gateway_handler.go`, `backend/internal/handler/openai_gateway_handler.go`, `backend/internal/handler/concurrency_error_response.go` | Used upstream shared `concurrencyErrorResponse` helper, then preserved local `ConcurrencyCacheError` service-unavailable message and `server_error` classification. |
| `backend/internal/service/openai_gateway_chat_completions.go` | Combined upstream terminal `event.usage` handling with local detail/cache usage extraction from JSON for both `usage` and `response.usage`. |
| `backend/internal/pkg/apicompat/chatcompletions_responses_test.go` | Kept local Responses-to-Chat request conversion tests and upstream reasoning/token-detail passthrough tests. |
| `frontend/src/components/account/CreateAccountModal.vue` | Kept local OpenAI-compatible provider preset reset/application and upstream endpoint capability plus Claude Code allowlist fields. |
| `frontend/src/components/keys/UseKeyModal.vue` | Kept local `xunyou` Codex provider template and adopted upstream `model_reasoning_effort = "xhigh"`. |

### Verification

| Command | Result |
|---|---|
| `pnpm --dir frontend exec vue-tsc --noEmit` | Passed |
| `go test ./internal/handler -run "Test(ConcurrencyErrorResponse\|GatewayHandleConcurrencyError\|OpenAIHandleConcurrencyError\|OpenAIRecoverResponsesPanic\|OpenAIEnsureResponsesDependencies\|OpenAIResponses_MissingDependencies)"` | Passed |
| `go test ./internal/pkg/apicompat -run "TestResponsesToChatCompletions\|TestResponsesToChatCompletionsRequest\|TestResponsesEventToChatChunks"` | Passed |
| `go test ./internal/server/routes -run Test` | Passed |
| `go test ./internal/handler ./internal/pkg/apicompat ./internal/server/routes ./internal/service` | Failed before conflict follow-up; exposed concurrency cache error classification mismatch, then fixed and covered with focused tests. |

## 2026-06-07

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0706_合并1.134版本` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `upstream/main` |
| Base before merge | `dff279f267eb` |
| Merge base | `7321e4dea807` |
| Upstream head merged | `635ad81cdcad` |
| Upstream version | `0.1.134` |
| Merge commit | `8d34a467` |
| Conflict files | `backend/cmd/server/wire.go`, `backend/cmd/server/wire_gen.go`, `backend/cmd/server/wire_gen_test.go`, `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`, `backend/internal/service/openai_gateway_service.go`, `frontend/src/components/account/CreateAccountModal.vue` |

### Summary

Merged Wei-Shaw/sub2api updates through upstream `v0.1.134`.

Major upstream changes included:

| Area | Notes |
|---|---|
| OpenAI / Codex gateway | Added a redesigned Responses <-> Chat Completions bridge, stricter stream lifecycle validation, Responses sticky account binding, OpenAI WS HTTP bridge for large initial payloads, and Codex / Claude Code behavior alignment. |
| Gateway reliability | Added multi-instance background-job leader lock, scheduler sticky escape, stream field validation, upstream `response.failed` passthrough, missing terminal event failover, image rate-limit cooldown failover, and real image upstream error passthrough. |
| Usage, ops, and audit | Added failed request recording and user/admin error views, TTFT sample weighting, deleted API key audit lookup, deleted user lookup support, and usage search/cache improvements. |
| Billing and quota | Added image token billing, user x platform quota flusher, shorter quota sentinel TTL, and quota window normalization fixes. |
| Security and auth | Added Linux DO pending-flow fixes, admin moderation auto-ban exemption, API key name XSS escaping, and unauthorized key lookup 404 behavior to reduce key-oracle leakage. |
| Build and docs | Bumped Go patch version to `1.26.4`, updated CI/deploy Docker images, README files, and partner assets. |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/cmd/server/wire.go` | Kept local `promptMetrics *promptmetrics.Extension` cleanup and added upstream `quotaFlusher *service.UserPlatformQuotaUsageFlusher` cleanup. |
| `backend/cmd/server/wire_gen.go` | Regenerated with `go generate ./cmd/server` after resolving provider and cleanup signatures. |
| `backend/cmd/server/wire_gen_test.go` | Updated `provideCleanup` tests to pass both `nil // promptMetrics` and `nil // quotaFlusher`. |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | Adopted upstream stream lifecycle bridge and removed duplicate request conversion helpers already kept in `responses_to_chatcompletions_request.go`. |
| `backend/internal/pkg/apicompat/responses_to_chatcompletions_request.go` | Kept local `ResponsesToChatCompletionsRequestWithOptions`, but routed it through the upstream `responsesInputToChatMessages` pipeline and `responsesToolsToChatTools`. |
| `backend/internal/pkg/apicompat/types.go` | Removed duplicate old `ResponsesStreamEvent.MarshalJSON` / stream item definitions and used upstream `responses_stream_event_wire.go`. |
| `backend/internal/service/openai_gateway_service.go` | Kept local OpenAI official endpoint `thinking` filtering, changed it to patch request-view bodies, and only materialized maps with `ensureReqBody()` when needed to avoid nil map writes. |
| `frontend/src/components/account/CreateAccountModal.vue` | Kept local `openAICompatibleProvider` preset behavior and added upstream `syncPreviewCredentials` flow. |
| `backend/internal/service/openai_gateway_responses_compat_test.go` | Updated local fallback stream test to assert dynamic Responses item IDs are consistent across SSE events instead of the old hard-coded `item_msg_0` contract. |

### Verification

| Command | Result |
|---|---|
| `go generate ./cmd/server` | Passed |
| `pnpm --dir frontend run typecheck` | Passed |
| `go test ./cmd/server -run Wire` | Passed |
| `go test ./internal/pkg/apicompat -run "TestResponsesToChatCompletions\|TestResponsesToChatCompletionsRequest\|TestChatCompletionsToResponses\|Test.*ResponsesStream"` | Passed |
| `go test ./internal/service -run "TestOpenAIGatewayService_Forward_FailoverReparsesCachedBodyForNextAccount\|TestGetOpenAIRequestBodyMap_IgnoresLegacyContextCache"` | Passed |
| `go test ./internal/service -run TestOpenAIGatewayService_ResponsesStreamFallsBackToRawChatCompletionsForOpenAICompatibleAPIKey -v` | Passed after updating the local test assertion to the new dynamic item-id stream contract |
| `go test ./internal/service -run "Test(OpenAI\|Responses\|ChatCompletions\|Codex\|GetOpenAIRequestBodyMap\|Sanitize\|Gateway\|UserConcurrencyPreset).*"` | Passed |

### Useful Diff Commands

```bash
git show --stat --summary --find-renames <merge-commit>
git show --cc <merge-commit> -- backend/cmd/server/wire.go backend/cmd/server/wire_gen.go backend/cmd/server/wire_gen_test.go backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go backend/internal/service/openai_gateway_service.go frontend/src/components/account/CreateAccountModal.vue
git diff --stat dff279f267eb..<merge-commit>
git diff --name-only dff279f267eb..<merge-commit>
```

### Post-merge Review (2026-06-07)

合并提交 `8d34a467` 完成后做了一次完整三方审查(merge-base 三分类 + `git merge-tree` 自动合并树对比 + 全量测试)。

发现并修复的遗漏:

| Item | Value |
|---|---|
| 问题 | 上游为 `service.UserRepository` 接口新增 `GetByIDIncludeDeleted` 方法后, `backend/internal/handler/admin/user_concurrency_preset_handler_test.go` 中的 `handlerPresetUserRepoStub` 未补该方法, 导致 `go test ./internal/handler/admin/` 编译失败(合并时只修了 service 侧同类 stub, 验证命令未覆盖该包) |
| 修复 | 为 `handlerPresetUserRepoStub` 补 `GetByIDIncludeDeleted`, 委托 `GetByID`, 与 service 侧 `userConcurrencyPresetUserRepoStub` 的修法一致 |
| 修复提交 | 见本备注所在提交 |

已确认无问题的事项:

- 6 个冲突文件的解决方案均同时保留本地与上游修改(wire 注入链/cleanup 顺序、apicompat 桥接取舍、thinking 过滤全路径、前端双边功能)。
- 35 个双边修改的自动合并文件语义正确(设置项前后端贯通、路由/配置/i18n 无缺漏)。
- 合并期间的额外编辑均为必要适配(上游接口/签名变更引起的本地测试调整、llm-wiki 文档同步)。

遗留小瑕疵(不影响正确性, 可后续清理):

- `backend/internal/pkg/apicompat/responses_to_chatcompletions_request.go` 中旧的 `convertResponsesToolsToChat` / `convertResponsesInputToChatMessages` 已无调用方, 为死代码。
- 上游 `responsesToolsToChatTools` 较本地旧实现少 `strings.TrimSpace` 防御, 边界场景极低风险。

补充验证:

| Command | Result |
|---|---|
| `go build ./...` | Passed |
| `go test ./...`(全量) | Passed(修复 stub 后; 首轮 6 个包因 Windows 文件锁误报, `-p 1` 串行重跑全部通过) |
| `pnpm --dir frontend run typecheck` | Passed |

经验教训: 合并后应执行全量 `go test ./...` 验证, 而非仅运行冲突相关包, 否则接口扩展引起的测试桩编译错误会漏检。

## 2026-06-09

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0609_合并1.135版本` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch/tag | `v0.1.135` |
| Base before merge | `125296de` |
| Merge base | `635ad81c` |
| Upstream head merged | `8c782bcc` |
| Upstream version | `0.1.135` |
| Merge commit | `d187587c` |
| Files changed | 96 |
| Conflict files | `backend/cmd/server/wire.go`, `backend/cmd/server/wire_gen.go`, `backend/cmd/server/wire_gen_test.go`, `backend/internal/service/wire.go` |

### Summary

Merged Wei-Shaw/sub2api updates through upstream `v0.1.135`(基于 `v0.1.134` 的 26 个增量提交)。

Major upstream changes included:

| Area | Notes |
|---|---|
| 代理有效期与失败回退 | 新增 `ProxyExpiryService`(`ProvideProxyExpiryService`, 每分钟 `SweepExpiredProxies`)、`ResolveProxyFallbackTarget` / `RevertProxyFallback`; `proxies` 新增 `expires_at` / `fallback_mode` / `backup_proxy_id` / `expiry_warn_days`, `accounts` 新增 `proxy_fallback_origin_id`(手动回切来源); 前端 `ProxiesView.vue` 有效期/回退模式 UI 与 `AccountsView.vue` 回切入口。 |
| OpenAI transport failover | 新增 `handleOpenAIUpstreamTransportError`(`openai_upstream_transport_error.go`); `/responses` 传输层错误转 failover, 持久网络/代理故障临时摘除账号。 |
| API Key exclusive group | `APIKeyAuth` 在用户不再被授权独占分组时拒绝访问并补 middleware 单测。 |
| usage cache token split | `UsageLogStats` 与聚合拆分 `cache_creation_tokens` / `cache_read_tokens`, 前端 i18n 增加缓存创建/命中/命中率文案。 |
| ops 告警 | 新增 `account_temp_unscheduled_count` 告警指标(`OpsAlertRulesCard.vue` + evaluator)。 |
| 其它修复 | 5h reset stale 同步 `SessionWindowEnd`; 非流式响应强制 `application/json`; 切组后剥离失配 `previous_response_id`; `Select.vue` 下拉高度修复; 新增 `skills/sub2api-admin` 管理 skill。 |

### Conflict Resolution Notes

四个冲突文件均为本地与上游各自新增独立 provider 而相邻冲突, 一律**双边保留**(本地 `TokenAnalysisService` + 上游 `ProxyExpiryService`):

| File | Resolution |
|---|---|
| `backend/internal/service/wire.go` | 同时保留 `ProvideTokenAnalysisService`(启动自动索引)与 `ProvideProxyExpiryService`(启动代理过期服务), ProviderSet 两者均注册。 |
| `backend/cmd/server/wire.go` | `provideCleanup` 参数与 cleanup steps 同时包含 `tokenAnalysis` / `TokenAnalysisAutoIndex` 与 `proxyExpiry` / `ProxyExpiryService`。 |
| `backend/cmd/server/wire_gen.go` | 解决源 `wire.go` 后用 `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server` 重新生成(`go generate ./cmd/server` 的 `main.go` 指令缺 `-mod=mod`, 会因 `google/subcommands` 缺 go.sum 条目失败)。 |
| `backend/cmd/server/wire_gen_test.go` | `provideCleanup` 最小依赖测试同时构造 `tokenAnalysisSvc` 与 `proxyExpirySvc` 并按签名顺序传入。 |

ent schema(`proxy.go` / `account.go`)、生成的 ent 代码、i18n 文案均自动合并成功; 按 llm-wiki 规则另跑 `go generate ./ent` 重新生成 ent(输出与自动合并一致)。`-mod=mod` 对 `go.sum` 引入的 wire/entc CLI 工具传递依赖已还原, 保持合并聚焦。

### Verification

| Command | Result |
|---|---|
| `go build ./...` | Passed |
| `go generate ./ent` / wire 重生成 | Passed |
| `go test ./cmd/server -run Wire` | Passed |
| `go test -tags=unit ./...` | Passed(`internal/pkg/ip` 首轮 Windows `.test.exe` 文件锁误报, 单独重跑通过) |
| `go test -tags=integration ./...` | 编译通过; 运行期无 Docker, testcontainers 优雅跳过(`CI` 未设置) |
| `pnpm --dir frontend run typecheck` | Passed |
| `pnpm --dir frontend exec vitest run` | 733 passed / 4 failed; 4 个失败为 `UsageTable.spec.ts` / `UsageView.spec.ts` 图片用量行用例, 经合并前 main(`125296de`)对比验证为**预存问题, 非本次合并引入** |

### Known Risks / 待办

- **`proxies.backup_proxy_id` 唯一约束不一致(上游 v0.1.135 自带, 非本次合并引入)**: `backend/ent/schema/proxy.go` 的 `backup_proxy` edge 用 `.Unique()`(无反向 `.From()` 边), 生成的 `ent/migrate/schema.go:1111` 把列标记为 `Unique: true`; 但 SQL migration `149_proxy_expiry_fallback.sql` 是普通外键 + 普通索引(非唯一)。本项目**不使用 Ent auto-migrate**(建表仅走 SQL migration), 真实库/集成测试拿到的是非唯一列, 多个代理可共用同一备用代理、回退链 `ResolveProxyFallbackTarget` 正常, 故**当前运行无影响**。按团队决定**仅记录不改代码**, 后续可向上游反馈。详见 `docs/features/sub2api-v0.1.135-merge-review-cn.md` P2。
- 前端 4 个图片用量用例为 main 预存红(`billing_mode=null` 历史行未走 `image_count`/模型名兜底, 被显示成 "Token"), 与本次合并无关, 团队决定暂不修复。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames d187587c
git show --cc d187587c -- backend/cmd/server/wire.go backend/cmd/server/wire_gen.go backend/cmd/server/wire_gen_test.go backend/internal/service/wire.go
git diff --stat 635ad81c..8c782bcc
git diff --name-only 635ad81c..8c782bcc
```
