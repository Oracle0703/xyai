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

## 2026-06-10

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0609_合并1.135版本` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `e30ccd8d` |
| Upstream head merged | `c32e29ba` |
| Merge commit | `97266dbd` |
| Files changed | 56 |
| Conflict files | `README_CN.md`, `backend/internal/handler/gateway_handler_error_fallback_test.go`, `backend/internal/service/openai_gateway_chat_completions_test.go` |

### Summary

Merged Wei-Shaw/sub2api `main` into the current integration branch after the previous `v0.1.135` merge.

Major upstream changes included:

| Area | Notes |
|---|---|
| Admin users filter | `GET /api/v1/admin/users` adds `api_key_group_id` to filter users by the exact group bound to their non-soft-deleted API keys; frontend `/admin/users` adds an API Key group filter, including disabled groups for investigation. |
| Account group scheduler indexes | Added `backend/migrations/150_account_group_scheduler_indexes_notx.sql` with concurrent indexes on `account_groups` for group/account priority scheduler lookups. |
| Gateway error writes | Added `MarkResponseCommitted` coverage and `gatewayForwardErrorAlreadyCommunicated` handling to prevent non-stream upstream JSON error passthrough from being polluted by an extra fallback SSE error frame. |
| OpenAI compatibility | Chat Completions -> Responses API key path now propagates `prompt_cache_key` into the Responses body and derives stable session headers from API key/cache key context. |
| Bedrock compatibility | `ApplyBedrockCCCompat` centralizes body cleanup and `anthropic-beta` filtering while preserving supported Bedrock beta tokens such as `context-management-2025-06-27`. |
| Idempotency | Stored idempotency responses now truncate on UTF-8-safe boundaries. |
| Misc | Added `claude-fable-5`, sponsor/README updates, Bedrock CC reload fix, gateway debug log loop optimization, and precomputed model body replacement fix. |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `README_CN.md` | Upstream deleted the Chinese README while this branch still keeps and modifies it; preserved the local branch version to avoid dropping project-local documentation. |
| `backend/internal/handler/gateway_handler_error_fallback_test.go` | Both sides added adjacent regression tests. Kept the local `ConcurrencyCacheError` -> 503 test and the upstream `gatewayForwardErrorAlreadyCommunicated` double-write prevention tests. |
| `backend/internal/service/openai_gateway_chat_completions_test.go` | Both sides added adjacent regression tests. Kept local large request compaction tests and appended upstream `prompt_cache_key` propagation coverage as a separate test. |

### Verification

| Command | Result |
|---|---|
| `go test -p 1 ./internal/handler ./internal/service -count=1` with repo-local `GOCACHE/GOTMPDIR` | Passed before merge commit after conflict resolution. The first attempt without local `GOCACHE` failed with Windows `go-build` access denied cache lock. |
| `go test -p 1 ./internal/handler -run 'TestGateway(HandleConcurrencyError|ForwardErrorAlreadyCommunicated|EnsureForwardErrorResponse)' -count=1` with repo-local `GOCACHE/GOTMPDIR` | Passed. |
| `go test -p 1 ./internal/service -run 'TestForwardAsChatCompletions(CompactsLargeToolOutputForEnabledAPIKey|WarnModeDoesNotMutateLargeRequest|CompactionUsesAPIKeyFromContext|_APIKeyPropagatesPromptCacheKeyInResponsesBody)' -count=1` with repo-local `GOCACHE/GOTMPDIR` | Passed. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed. Direct PowerShell `pnpm` was blocked by local execution policy, so `cmd.exe /c` was used. |
| `go test -p 1 ./internal/handler ./internal/service -count=1` after wiki update | Timed out in this Codex tool run before emitting a code failure; narrower final regression runs above passed. |

### Wiki Updates

Updated `llm-wiki/wiki/backend.md`, `frontend.md`, `data-and-domain.md`, and `security-and-reliability.md` for the stable knowledge introduced by this upstream merge: API key group filtering, scheduler indexes, UTF-8-safe idempotency truncation, OpenAI prompt cache key propagation, gateway double-write prevention, and Bedrock CC compatibility filtering.

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 97266dbd
git show --cc 97266dbd -- README_CN.md backend/internal/handler/gateway_handler_error_fallback_test.go backend/internal/service/openai_gateway_chat_completions_test.go
git diff --stat e30ccd8d..97266dbd
git log --oneline e30ccd8d..c32e29ba
```

## 2026-06-17

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0617_合并上游1.137版本` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` (tag `v0.1.137`) |
| Base before merge | `dd1700fc` |
| Upstream head merged | `4a5665da` |
| Merge commit | `0e86a67c` |
| Upstream commits | 80 |
| Files changed | 176 (`+11023 / -661`) |
| Conflict files | `backend/internal/service/domain_constants.go`, `backend/internal/service/openai_gateway_responses_chat_fallback.go`, `backend/internal/service/openai_gateway_chat_completions_test.go` |

### Summary

将 Wei-Shaw/sub2api `main`（`v0.1.137`，本地上次合并基线 `dd1700fc` 之后的 80 个上游提交）合入专用集成分支。VERSION `0.1.136 -> 0.1.137`。

主要上游内容:

| Area | Notes |
|---|---|
| cyber 内容审计 | 新增 `openai_cyber_policy.go` / `openai_cyber_session_block.go`：cyber 策略命中后可按会话级自动屏蔽（默认关，`cyber_session_block_enabled` / `cyber_session_block_ttl_seconds` 两个设置键，默认 TTL 3600s）；usage 请求类型新增 `cyber` 维度（前端 badge/label/export）。 |
| thinking 协议过滤 | 新增 `thinking_protocol.go`：按 Anthropic-compatible 上游协议对 thinking-block 做协议感知过滤；区分 `mappedModel` 与 `originalModel`；MiniMax M 系列 `thinking.type=enabled` 改写为 adaptive。 |
| 国产 LLM 计费/推理 | GLM / Kimi / MiniMax / DeepSeek V4 Pro·Flash / doubao-embedding-vision / kimi-for-coding 兜底定价；`thinking-enabled` 自动填充 `reasoning_effort` 默认值；DeepSeek `reasoning_effort` `max` 归一化为 `xhigh`。 |
| OpenAI 配额/探测 | 新增 `openai_quota_service.go`：查询并重置 OpenAI 账号 rate-limit credits；`/responses` 能力探测增加工具调用校验。 |
| 调度 outbox | scheduler outbox dedup 修复（claim 时释放、非法 dedup index 恢复、snapshot coalesce、消费后清理 + 10s grace）；迁移重排 151/152 -> 152/153。 |
| 账号/监控 | `accounts` autopause/expiry 部分索引（`151_account_autopause_expiry_index_notx.sql`）；渠道监控正负随机抖动 `jitter_seconds`（Ent schema + `151_channel_monitor_jitter.sql`）；账号列表展示 account id；refresh candidates SQL 修复（不再排除健康账号）。 |
| 网关健壮性 | Anthropic 429 窗口重置；上游 zstd 响应解压；非 JSON 200 响应 failover；SSE `event:error` body 保留以反映真实上游错误；haiku 探测流式拦截；OpenAI images server error failover。 |
| 安全/认证 | sub2api 注入修复；ACL 拒绝信息包含 client IP；OAuth 注册应用优惠码；antigravity system-role message 处理；OpenAI cyber policy passthrough。 |
| 杂项 | `form-data` 升级到 `>=4.0.6`（pnpm override）；token refresh 重试退避降幅；用户等待队列计数移出热路径；Dockerfile 复制 docs/legal。 |

### Conflict Resolution Notes

三处冲突均为语义融合，未做无脑 ours/theirs，全部保留本地能力并吸收上游新增逻辑:

| File | Resolution |
|---|---|
| `backend/internal/service/domain_constants.go` | 设置键常量块的加性冲突：保留本地 `SettingKeyRequestInterceptRules`（本地请求拦截能力）+ 吸收上游 `SettingKeyCyberSessionBlockEnabled` / `SettingKeyCyberSessionBlockTTLSeconds`，二者并存。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 语义冲突：本地已将 `billingModel` / `upstreamModel` 计算上移到转换调用之前（因 `ResponsesToChatCompletionsRequestWithOptions` 需要 `upstreamModel`）。上游在原位置重复声明这两个变量并新增 `ApplyThinkingEnabledFallback` 调用。解决方式：保留本地上移声明，去掉上游重复 `:=` 声明，仅吸收上游 `reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)` 一行（复用已算出的 `billingModel`）。 |
| `backend/internal/service/openai_gateway_chat_completions_test.go` | 测试辅助/用例的加性冲突：保留本地 `largeOpenAIChatCompletionsBody` / `openAIChatCompletionsOAuthTestAccount`（被本文件多处大请求与 OAuth 用例引用）+ 吸收上游新增 `TestBuildChatStreamErrorSSE`，并补齐 HEAD 函数自身的闭合花括号，三者并存。`gjson` import 与 `buildChatStreamErrorSSE` 实现均已存在。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| token 分析（索引/API/前端页面/邮箱搜索/项目归因） | 未被覆盖，保留。 |
| request archive / intercept 管理（含 `request_intercept_rules` 设置键） | 保留，冲突中显式保住。 |
| 生图工具 `/image-gen`、图片生成网关 | 未受影响。 |
| 用户并发方案（migration/repo/service/admin API/runner） | 未受影响。 |
| Redis 7+ 要求、并发错误分类 | 未引入 Redis 3 workaround。 |
| OpenAI Responses/Chat 兼容、大请求 role=tool 压缩、空响应兜底 | 保留；fallback 文件冲突中保住本地 model 上移逻辑。 |

### Verification

`GOCACHE` 指向仓库内 `backend/.gocache`（已被 `.gitignore` 忽略），`GOTMPDIR` 指向仓库外 `e:\tmp`，规避 Windows `.test.exe` 文件锁。

| Command | Result |
|---|---|
| `go build ./internal/service/...` | Passed |
| `go vet -tags=unit ./internal/service/`（含测试编译） | Passed |
| `go test -tags=unit -p 1 -count=1 ./internal/service/` | Passed (88.8s) |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat/` | Passed |
| `go test -tags=unit -p 1 -count=1 ./cmd/server ./internal/handler ./internal/handler/admin` | Passed |
| `pnpm --dir frontend run typecheck`（`vue-tsc --noEmit`） | Passed |
| `pnpm --dir frontend run test:run`（vitest） | 750 passed / 4 failed（124 files，2 failed） |

### Known Risks / 待办

- **前端 4 个图片用量 tooltip 用例为 main 预存红，非本次合并引入**: 失败用例为 `src/views/user/__tests__/UsageView.spec.ts`（image billing metadata tooltip）与 `src/components/admin/usage/__tests__/UsageTable.spec.ts`（historical image rows without 2K fallback），断言 `Image count` / `Billing size` / `Per-image price` 等文案。本次合并对 `UsageTable.vue` / `UsageView.vue` / `usageRequestType.ts` 的改动**仅为加性 `cyber` 请求类型 label/badge/validity-set**，与图片计费 tooltip 渲染正交；两个失败 spec 文件本次合并未改动。与既有记录 `known-red-image-usage-vitest` 一致，团队决定暂不修复。
- **`backend/migrations/` 出现两个 `151_` 前缀**（`151_account_autopause_expiry_index_notx.sql` 与 `151_channel_monitor_jitter.sql`）: 均来自上游（本地合并前仅到 `150`），是上游各自分支独立编号所致。迁移 runner（`backend/internal/repository/migrations_runner.go`）按**完整文件名** `sort.Strings` 排序并以 `WHERE filename = $1` 去重，不依赖数字前缀唯一，故两文件独立执行、互不覆盖，**当前运行无影响**；仅记录，待后续上游或本地决定是否重排。
- **`proxies.backup_proxy_id` 唯一约束不一致**: 上游 v0.1.135 自带的历史问题，详见 `docs/features/sub2api-v0.1.135-merge-review-cn.md` P2，本次合并未触及，状态不变。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 0e86a67c
git show --cc 0e86a67c -- backend/internal/service/domain_constants.go backend/internal/service/openai_gateway_responses_chat_fallback.go backend/internal/service/openai_gateway_chat_completions_test.go
git diff --stat dd1700fc..0e86a67c
git log --oneline dd1700fc..4a5665da
```

## 2026-06-22

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `5965ef7f` |
| Upstream head merged | `85a3b122` |
| Merge base | `4a5665da` |
| Merge commit | `5e115ac6` |
| Upstream commits | 38 |
| Files changed from merge base | 58 |
| Conflict files | `backend/internal/service/openai_gateway_chat_completions_raw_test.go` |

### Summary

将 Wei-Shaw/sub2api `main`（上游 `85a3b122`，本地上次上游基线 `4a5665da` 之后的 38 个提交）合入当前敏感词过滤分支。VERSION 更新到 `0.1.138`。

主要上游内容:

| Area | Notes |
|---|---|
| OpenAI / GLM 兼容 | `glm-*` OpenAI Chat Completions raw 路径把 `reasoning.effort` / `reasoning_effort` 归一化为 GLM 上游接受的 `high` / `max`。 |
| OpenAI images | `response.incomplete` 被显式识别; 非 content filter 类 incomplete 视为 502 可 failover。上游 completed 但无图时记录诊断摘要并优先同账号快速重试。 |
| Vertex Anthropic | service account 路径按 Vertex 支持范围过滤 `anthropic-beta`, 并以最终 beta 决定 body sanitize; BetaPolicy block 仍生效。 |
| OpenAI 调度 | 新增 `gateway.openai_ws.scheduler_score_weights.reset` 与 `gateway.scheduling.prefer_soonest_reset`, 支持 use-it-or-lose-it 优先使用最早重置窗口账号; 默认关闭。 |
| 认证安全 | OAuth/合成邮箱用户绑定真实邮箱时复用注册邮箱后缀白名单策略。 |
| 订阅/返利 | 订阅支付履约也会触发邀请返利; 通过 `SUBSCRIPTION_ASSIGNED` 与返利审计占位保证重试幂等。 |
| 前端 | 自定义页面标题随公开/管理员自定义菜单、站点名和语言切换重新解析; 管理端用量卡片展示 cache creation/read token 明细。 |
| 杂项 | sponsor 资源更新, CI Node/Go action 更新, promo 过期时间可清空, usage cache tooltip 修复, ccswitch 默认模型与 Claude Code CLI 检测更新。 |

### Conflict Resolution Notes

冲突只出现在测试文件, 但处理时仍按语义合并, 未整文件取一边:

| File | Resolution |
|---|---|
| `backend/internal/service/openai_gateway_chat_completions_raw_test.go` | 本分支新增 `TestForwardAsRawChatCompletions_StripsTopLevelThinkingAndKeepsReasoningEffort`, 用于锁定 Chat Completions raw 上游请求删除 top-level `thinking` 且保留 `reasoning_effort`; 上游新增 `TestForwardAsRawChatCompletions_NormalizesGLMReasoningEffortForUpstream`, 用于锁定 GLM `xhigh -> max` 归一化。两者是独立回归覆盖, 不互斥, 因此拆成两个独立测试函数全部保留。 |
| `backend/internal/service/prompt_risk_test.go` | 合并后 `internal/service` 包测试编译暴露本分支 helper `ptrInt64` 与上游既有 `payment_config_plans_validation_test.go` 中同名 helper 冲突。仅将本分支测试 helper 改名为 `promptRiskPtrInt64` 并更新三处调用, 不改变业务逻辑。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| prompt risk / 敏感词过滤实现与测试 | 保留; 仅测试 helper 为避免上游同名 helper 改名。 |
| content moderation 与 cyber policy 审计口径 | 保留; 本次合并未覆盖此前 `prompt_risk_%` 与 `cyber_policy` 的过滤/展示处理。 |
| token 分析、request archive/intercept、生图工具、用户并发方案等本地扩展 | 无冲突覆盖; 合并后仍在工作树中。 |
| 上游 v0.1.138 新增配置 | 已同步 `deploy/config.example.yaml`, `llm-wiki/wiki/ops.md` 与相关后端基线。 |

### Verification

| Command | Result |
|---|---|
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)" backend frontend docs llm-wiki` | Passed, no conflict markers. |
| `git diff --check` | Passed. |
| `git diff --cached --check` | Passed before merge commit. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `go test -tags=unit -p 1 -count=1 ./internal/service -run 'TestForwardAsRawChatCompletions_(StripsTopLevelThinkingAndKeepsReasoningEffort|NormalizesGLMReasoningEffortForUpstream)|TestFilterBySoonestReset|TestExtractImagesUpstreamError|TestImagesOAuthNonStreaming_CompletedNoImageTriggersSameAccountRetry|TestEvaluatePromptRisk'` with repo-local `GOCACHE/GOTMPDIR` | Passed. First attempt with `GOTMPDIR=E:\tmp` failed before tests with `Access is denied`; rerun with `.gotmp-test` succeeded. |

### Wiki Updates

更新 `llm-wiki/wiki/backend.md`, `frontend.md`, `data-and-domain.md`, `ops.md`, `security-and-reliability.md`, 记录本次上游合并带来的稳定知识: GLM reasoning effort 归一化、OpenAI images incomplete/无图软失败、OpenAI 最早重置调度、邮箱绑定白名单、Vertex beta 过滤、订阅返利幂等、前端标题和缓存 token 明细。

### Known Risks / 待办

- Git 在合并提交后提示 loose objects 较多: `There are too many unreachable loose objects; run 'git prune' to remove them.` 这是仓库维护提示, 非本次代码冲突, 未主动执行 destructive/cleanup 操作。
- 全量 `go test ./...` 未执行; 本次选择冲突与上游新增服务逻辑的定向单测 + 前端 typecheck。若要发布, 建议在 CI 或干净环境跑完整后端/前端矩阵。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 5e115ac6
git show --cc 5e115ac6 -- backend/internal/service/openai_gateway_chat_completions_raw_test.go backend/internal/service/prompt_risk_test.go
git diff --stat 4a5665da..85a3b122
git log --oneline 4a5665da..85a3b122
```

## 2026-06-27

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `57dbdc9f2cfb` |
| Upstream head merged | `c275422251e72` |
| Merge base | `85a3b122545a` |
| Merge commit | `pending` |
| Upstream version | `0.1.139` |
| Upstream commits | 72 |
| Files changed from merge base | 236 (`+13272 / -1004`) |
| Conflict files | `README_CN.md`, `backend/cmd/server/wire_gen.go`, `backend/internal/server/routes/gateway.go`, `backend/internal/server/routes/gateway_test.go`, `backend/internal/service/openai_gateway_chat_completions.go`, `frontend/src/components/account/AccountUsageCell.vue`, `frontend/src/components/account/CreateAccountModal.vue`, `frontend/src/components/charts/GroupDistributionChart.vue`, `frontend/src/components/charts/ModelDistributionChart.vue`, `frontend/src/views/admin/DashboardView.vue`, `frontend/src/views/auth/EmailVerifyView.vue` |

### Summary

将 Wei-Shaw/sub2api `main`（上游 `c27542225`, 本地上次合并基线 `85a3b122` 之后的 72 个提交）合入当前敏感词过滤分支。VERSION 更新到 `0.1.139`。

主要上游内容:

| Area | Notes |
|---|---|
| Grok/xAI 订阅支持 | 新增 `PlatformGrok`, Grok OAuth 授权/刷新/账号创建、xAI Responses 转发、Grok token provider/refresher、xAI quota header 解析与管理端主动 probe。 |
| Codex 客户端限制加固 | `codex_cli_only` 增加全局黑/白名单、最低/最高 Codex 版本、engine fingerprint signals、全局/账号级 app-server 放行与检测测试。 |
| OpenAI 账号认证 | 新增 Codex personal access token(`at-*`)导入与 whoami 校验, PAT 账号清理 OAuth-only credential 字段。 |
| 网关可靠性 | 上游 `response.failed` verbose sanitize、OpenAI chat 传输错误 failover、模型不支持时返回 404 `model_not_found`、GLM/Codex tool args duplicate 修复、Responses passthrough duplicate function args 修复。 |
| 支付与计费 | 修复余额扣费持续透支、订阅订单充值倍率、订单币种符号、subscription validity unit 单复数、支付 provider supported_types 空数组展示。 |
| 管理端与前端 | 管理端用量 cache creation/read token 明细、Grok quota probe cell、Grok platform UI、Codex fingerprint settings、Dashboard/图表 `toFiniteNumber` 防 NaN。 |
| 文档与资产 | 更新 sponsors/README、sub2api-admin JWT fallback、GPT-5.5 Codex instructions。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `README_CN.md` | 以上游版本为骨架, 补回本分支官方域名提示、在线体验、PinCC/PackyCode/APIKEY.FUN/YLSCode/RunAPI 赞助商和 Windows 本机手动重启记录。 |
| `backend/cmd/server/wire_gen.go` | 解决 provider 注入后, 用 repo-local `GOCACHE`/`GOTMPDIR` 执行 `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server` 重新生成。 |
| `backend/internal/server/routes/gateway.go` | 保留本分支 `RequestArchive`/`RequestIntercept` 中间件, 同时吸收上游 Grok 路由限制; 根级 `/responses` 与 `/chat/completions` 继续带 archive/intercept, Grok 不支持路径返回 404。 |
| `backend/internal/server/routes/gateway_test.go` | 合并测试 helper 为 `withGatewayRoutesTestConfig` / `withGatewayRoutesTestPlatform`, 同时保留 archive/intercept 路由测试与 Grok route 测试。 |
| `backend/internal/service/openai_gateway_chat_completions.go` | 先基于原始 body 执行上游新增 `codex_cli_only` 检测, 再保留本分支默认 `reasoning_effort` 注入, 最后进入 Grok raw/APIKey raw 分流。 |
| `frontend/src/components/account/AccountUsageCell.vue` | `openAIUsageRefreshKey` 变化时采用上游 `requestAutoLoad()`, 同时保留本地用量展示逻辑。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 同时保留本地 OpenAI-compatible provider presets、Antigravity project/warmup 逻辑与上游 Grok OAuth/Codex app-server 字段; 移除不存在的 `codexCLIOnlyAllowClaudeCodeEnabled`。 |
| `frontend/src/components/charts/GroupDistributionChart.vue`, `ModelDistributionChart.vue`, `DashboardView.vue` | 采用上游 `toFiniteNumber` 兜底, 避免 null/字符串/NaN 进入图表排序和格式化。 |
| `frontend/src/views/auth/EmailVerifyView.vue` | 采用上游 payload 细粒度 undefined 判断, 修复合并后的调用换行语法。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 合并未覆盖 `prompt_risk.go`, `prompt_risk_judge.go` 与对应安全说明。 |
| RequestArchive / RequestIntercept | 保留; 冲突路由中显式保留 archive/intercept 中间件。 |
| token 分析、图片生成、用户并发方案 | 保留; 上游合并未删除本地核心入口。 |
| 默认 OpenAI `reasoning_effort` 注入 | 保留; 在 `codex_cli_only` 检测之后、raw 分流之前执行。 |

### Verification

| Command | Result |
|---|---|
| `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server` with repo-local `.gocache-wire/.gotmp-wire` | Passed |
| `gofmt -w backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go backend/internal/service/openai_gateway_chat_completions.go backend/cmd/server/wire_gen.go` | Passed |
| `rg -n "^(<<<<<<<|>>>>>>>)"` on conflict files | Passed, no conflict markers |
| `git diff --check -- <conflict files>` | Passed |
| `rg -n "^(<<<<<<<|>>>>>>>)" backend frontend docs llm-wiki README.md README_CN.md README_JA.md skills deploy` | Passed, no conflict markers (rg exit 1 with no matches) |
| `git diff --check` | Passed |
| `git diff --cached --check` | Passed |
| `go test -tags=unit -p 1 -count=1 ./cmd/server ./internal/server/routes ./internal/handler ./internal/handler/admin` with repo-local `GOCACHE/GOTMPDIR` | Passed |
| `go test -tags=unit -p 1 -count=1 ./internal/service` with repo-local `GOCACHE/GOTMPDIR` | Passed |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/xai ./internal/pkg/openai ./internal/pkg/apicompat` with repo-local `GOCACHE/GOTMPDIR` | Passed |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `security-and-reliability.md`, `data-and-domain.md`, 记录本次上游合并带来的稳定知识: Grok/xAI OAuth 与 quota, Codex client restriction signals/app-server/PAT, model_not_found 404, payment/billing 修复, frontend Grok/Codex UI 与 Windows Go cache 验证方式。

### Useful Diff Commands

```bash
git diff --stat 85a3b122545a6c914704f716a612aea00c3d7ecd..c275422251e72750bebe53e41fcf59db7f83fe6b
git log --oneline 85a3b122545a6c914704f716a612aea00c3d7ecd..c275422251e72750bebe53e41fcf59db7f83fe6b
git diff --name-only --diff-filter=U
```

## 2026-06-30

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `revert-114-feature/atomic-scheduling` |
| Base before merge | `33e9233d5640` |
| Upstream head merged | `30326cf2671a` |
| Merge commit | `1db79dfc5afcf` |
| Upstream commit date | `2026-01-01` |
| Conflict files | `backend/cmd/server/wire_gen.go`, `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/internal/handler/gateway_handler.go`, `backend/internal/handler/gateway_helper.go`, `backend/internal/handler/gemini_v1beta_handler.go`, `backend/internal/handler/openai_gateway_handler.go`, `backend/internal/pkg/antigravity/request_transformer.go`, `backend/internal/pkg/antigravity/request_transformer_test.go`, `backend/internal/pkg/claude/constants.go`, `backend/internal/repository/concurrency_cache.go`, `backend/internal/repository/concurrency_cache_integration_test.go`, `backend/internal/service/antigravity_gateway_service.go`, `backend/internal/service/concurrency_service.go`, `backend/internal/service/gateway_multiplatform_test.go`, `backend/internal/service/gateway_service.go`, `backend/internal/service/gemini_messages_compat_service.go`, `backend/internal/service/gemini_messages_compat_service_test.go`, `backend/internal/service/gemini_oauth_service.go`, `backend/internal/service/gemini_token_provider.go`, `backend/internal/service/openai_gateway_service.go`, `backend/internal/service/wire.go`, `deploy/config.example.yaml`, `frontend/package-lock.json`, `frontend/src/components/account/AccountStatusIndicator.vue` |

### Summary

将 Wei-Shaw/sub2api 元旦分支 `revert-114-feature/atomic-scheduling` 合入当前敏感词过滤分支。该分支只有一个提交 `30326cf26`, 内容是撤销 `8d252303` 的 "feat(gateway): 实现负载感知的账号调度优化 (#114)"。

合并前核对历史发现: 当前分支已包含后续主线提交 `c5c12d4c8`(同内容 revert) 和 `7568dc850`(Reapply #114), 之后负载感知调度、快照调度、OpenAI/Gemini/Grok 网关选择逻辑已继续演进。因此本次冲突处理以保留当前分支实现为准, 只补齐该元旦分支在当前分支历史中的 merge 拓扑, 不回退当前调度代码。

### Conflict Resolution Notes

| File / Area | Resolution |
|---|---|
| `backend/internal/config/config.go`, `deploy/config.example.yaml` | 保留当前 `gateway.scheduling` 配置, 包括 `load_batch_enabled`, wait timeout, snapshot/outbox 和后续调度参数; 不按元旦 revert 删除。 |
| `backend/internal/service/gateway_service.go`, `openai_gateway_service.go`, gateway handlers | 保留当前 `SelectAccountWithLoadAwareness`、sticky session、wait plan、scheduler snapshot 与后续网关能力; 不回退到旧随机/优先级选择路径。 |
| `backend/internal/repository/concurrency_cache.go`, `concurrency_service.go`, `wire.go` | 保留当前 account load batch、fresh load、wait count TTL 和 slot cleanup 支撑; 不删除当前并发缓存接口。 |
| `backend/internal/pkg/antigravity/*`, `gemini_*` | 保留当前 Antigravity/Gemini thinking signature、custom tool、tier_id 与 quota 相关演进; 不按元旦 revert 删除后来稳定功能。 |
| `frontend/package-lock.json` | 维持当前分支删除状态; 本项目使用 pnpm, 不恢复 npm lockfile。 |

### Verification

| Command | Result |
|---|---|
| `git show --stat --summary --find-renames 1db79dfc5` | Passed, merge commit relative to first parent has no file changes. |
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `Select-String -Path backend/**/*.go,deploy/config.example.yaml,frontend/src/**/*.vue -Pattern '<<<<<<<|>>>>>>>'` | Passed, no conflict markers. |

### Wiki Updates

更新 `llm-wiki/wiki/ops.md`, 记录 `revert-114-feature/atomic-scheduling` 是 2026-01-01 的旧 revert 分支, 当前分支已包含后续主线同内容 revert 和 reapply, 后续合并时不应据此回退当前负载感知调度实现。

## 2026-06-30 main sync

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `7bd0a8502684` |
| Merge base | `c275422251e72` |
| Upstream head merged | `930326116ed6` |
| Merge commit | `d49b486893830` |
| Upstream version | `0.1.140` |
| Upstream commits | 53 |
| Files changed | 136 (`+4870 / -461`) |
| Conflict files | `backend/internal/server/routes/gateway.go`, `backend/internal/server/routes/gateway_test.go` |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入当前敏感词过滤分支。上游 head 为 `930326116ed6`, VERSION 更新到 `0.1.140`。

主要上游内容:

| Area | Notes |
|---|---|
| Grok/xAI 兼容 | Grok group 支持 Messages/Chat/Responses CLI 兼容入口; Grok Responses payload 删除/过滤 xAI 不支持字段和工具; 账号测试走 xAI Responses 并写 quota 快照。 |
| OpenAI 网关 | 新增 `/v1/messages/count_tokens` 到 OpenAI `/v1/responses/input_tokens` 的桥接; context-window 错误不再误触发账号 runtime block; Codex image bridge tool_choice auto 与 GPT-5.5 instructions 修复。 |
| 调度与 quota | 新增 `gateway.openai_ws.scheduler_score_weights.quota_headroom`, 可按 OpenAI/Codex 7d 剩余额度健康度给账号加分; 默认关闭。 |
| 支付与退款 | 订阅订单按套餐 price 直付, 不再被余额充值倍率反算; 新增 `REFUND_PENDING` 与 provider refund query/finalize 流; 匿名 out_trade_no 查单收敛为最小状态字段。 |
| 数据与审计 | 新增 ops system logs `api_key_id` 字段/索引; content moderation 日志记录 `matched_keyword`; user platform quota CHECK 约束加入 `grok`。 |
| 前端 | 新增统一 API URL builder, 修复自定义 API base 下 fetch/WebSocket/setup 直连; API Key 列设置; DataTable 排序可访问性; 管理端退款 pending 查询与风险控制命中关键词展示。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/server/routes/gateway.go` | 保留本分支 `RequestArchive` / `RequestIntercept` 中间件链, 同时采用上游 Grok OpenAI-compatible CLI 入口判断: Grok 可走 `/v1/messages`, `/v1/chat/completions`, 根级 `/chat/completions` 与 Responses WebSocket/HTTP; `/v1/messages/count_tokens` 仅 OpenAI group 走新桥接, Grok 仍返回不支持。 |
| `backend/internal/server/routes/gateway_test.go` | 保留本分支 request archive/intercept 路由回归测试和 helper, 吸收上游 Grok CLI compatibility 与 OpenAI count_tokens 路由测试; test router 继续使用 option 形式支持自定义 config/platform。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 合并未覆盖本地 prompt risk 配置、judge 与审计边界。 |
| RequestArchive / RequestIntercept | 保留; 冲突路由中显式保留所有相关中间件, 并有路由测试覆盖。 |
| token 分析、图片生成、用户并发方案 | 保留; 本次上游合并没有删除本地核心入口。 |
| 默认 OpenAI reasoning_effort 注入与 DeepSeek cache-hit usage 兼容 | 保留; 上游改动自动合并, 未覆盖本地相关逻辑。 |

### Verification

| Command | Result |
|---|---|
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)$" backend frontend deploy llm-wiki docs README.md README_CN.md README_JA.md AGENTS.md Makefile Dockerfile` | Passed, no conflict markers. |
| `git diff --check` | Passed. |
| `git diff --cached --check` | Passed before merge commit. |
| `go test -tags=unit -p 1 -count=1 ./internal/server/routes ./internal/handler ./internal/handler/admin ./internal/service` | Timed out at 240s after `internal/server/routes`, `internal/handler`, `internal/handler/admin` passed; `internal/service` was rerun separately and passed. |
| `go test -tags=unit -p 1 -count=1 ./internal/service` with repo-local `GOCACHE/GOTMPDIR` | Passed (`90.165s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/repository ./internal/config` with repo-local `GOCACHE/GOTMPDIR` | Passed. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录本次上游合并带来的稳定知识: v0.1.140、OpenAI count_tokens bridge、Grok CLI 兼容、quota headroom 调度、ops/risk-control migrations、退款 pending/finalize、前端 API base builder 与相关 UI 约束。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames d49b486893830
git show --cc d49b486893830 -- backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go
git diff --stat c275422251e72750bebe53e41fcf59db7f83fe6b..930326116ed6bbc68c64e9536f8ed5778f078aaf
git log --oneline c275422251e72750bebe53e41fcf59db7f83fe6b..930326116ed6bbc68c64e9536f8ed5778f078aaf
```

## 2026-07-01 main sync

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `f96bab3d1be9` |
| Merge base | `930326116ed6` |
| Upstream head merged | `db0414233ce3` |
| Merge commit | `9c2717951d59` |
| Upstream version | `0.1.141` |
| Upstream commits | 9 |
| Files changed | 49 (`+3022 / -1622`) |
| Conflict files | 无冲突 |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入当前敏感词过滤分支。上游 head 为 `db0414233ce3`, VERSION 更新到 `0.1.141`。

主要上游内容:

| Area | Notes |
|---|---|
| 用户用量统计 | 用户侧 `/usage`、stats、trend、models、snapshot-v2 共用过滤口径, 支持 API Key、分组、请求模型、request_type、billing_type、billing_mode 和日期范围; 用户使用量页对齐管理端 token/cache 统计与图表能力。 |
| Anthropic OAuth 转发 | 新增 `anthropicfp` dateline 归一化, 默认开启 `enable_client_dateline_normalization`, 仅清理 Anthropic OAuth/SetupToken 请求体中 system/system-reminder 的 dateline 隐写指纹。 |
| Codex/OpenAI 兼容 | Codex OAuth reasoning item 保留 `encrypted_content`/summary/content, 剥离 `rs_*` id 并补空 summary; 请求带 reasoning 时自动 include `reasoning.encrypted_content`; `gpt-5.5`/`gpt-5.5-pro` 模型名保持原样。 |
| Claude/Sonnet | 默认模型列表加入 `claude-sonnet-5`; `context-1m-2025-08-07` beta 默认只对 Sonnet 5 直连/Vertex/Bedrock ID 变体放行, 其他模型过滤。 |
| 前端 | 用户侧 UsageView 重构为筛选驱动的统计/图表/日志视图, 列显隐持久化为 `user-usage-hidden-columns`, group/model 图表支持用户侧隐藏 account cost 与禁用下钻。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| 无 | 本次 `git merge --no-ff upstream/main` 自动合并完成, 未产生冲突文件。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 本次上游增量未覆盖本地 prompt risk 配置、judge 与审计边界。 |
| RequestArchive / RequestIntercept | 保留; 本次上游增量未触碰路由中间件冲突面。 |
| token 分析、图片生成、用户并发方案 | 保留; 本次合并没有删除本地核心入口。 |
| 默认 OpenAI reasoning_effort 注入与 DeepSeek cache-hit usage 兼容 | 保留; 上游 Codex reasoning 兼容与本地 usage parser 兼容并存。 |

### Verification

| Command | Result |
|---|---|
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)$" backend frontend deploy llm-wiki docs README.md README_CN.md README_JA.md AGENTS.md Makefile Dockerfile` | Passed, no conflict markers; `rg` exit 1 because no matches. |
| `git diff --check` | Passed. |
| `git diff --cached --check` | Passed. |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/anthropicfp ./internal/service ./internal/handler ./internal/repository` with repo-local `GOCACHE/GOTMPDIR` | `anthropicfp`, `service`, `handler` passed; `repository` hit Windows file-lock on `repository.test.exe`, then rerun separately with fresh cache/tmp. |
| `go test -tags=unit -p 1 -count=1 ./internal/repository` with fresh repo-local `GOCACHE/GOTMPDIR` | Passed (`3.596s`). |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录本次上游合并带来的稳定知识: v0.1.141、用户侧用量统计过滤/snapshot-v2、Anthropic dateline 归一化、Codex encrypted reasoning 续轮、`gpt-5.5-pro` 模型保真、Sonnet 5 与 1M context beta 默认策略、前端 UsageView 新约束。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 9c2717951d59
git diff --stat 930326116ed6bbc68c64e9536f8ed5778f078aaf..db0414233ce324903adc72e858374086da158b4b
git log --oneline 930326116ed6bbc68c64e9536f8ed5778f078aaf..db0414233ce324903adc72e858374086da158b4b
```
## 2026-07-03 main sync

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `43c2c369d604` |
| Merge base | `db0414233ce3` |
| Upstream head merged | `a5638a4e5408` |
| Merge commit | `91d67e816` |
| Upstream version | `0.1.143` |
| Upstream commits | 83 |
| Files changed | 470 (`+16053 / -42421`) |
| Conflict files | `backend/internal/config/config.go`; `backend/internal/handler/endpoint.go`; `backend/internal/server/routes/gateway.go`; `backend/internal/server/routes/gateway_test.go`; `deploy/config.example.yaml` |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入当前敏感词过滤分支。上游 head 为 `a5638a4e5408`, VERSION 更新到 `0.1.143`, 同步 v0.1.142 / v0.1.143 变更。

主要上游内容:

| Area | Notes |
|---|---|
| OpenAI/Codex | 新增 `/responses/compact` 默认模型 `gateway.openai_compact_model`, compact 路径保留子路径并可账号级 model mapping; compact 跳过 Codex image bridge 注入。 |
| Grok/xAI media | Grok group 接入 images generations/edits 与 videos generations/status, `grok-imagine` 图片别名归一, 旧 Grok group 回填 `allow_image_generation`。 |
| Spark shadow | OpenAI OAuth 母账号可创建 `quota_dimension=spark` 影子账号, 凭据透传母账号但独立调度/分组/spark 配额窗口; 导出备份排除 shadow。 |
| Group billing | 订阅分组新增高峰时段倍率, 按服务器全局时区判定, 仅叠加 token 倍率, 图片按次倍率不受影响。 |
| Usage/Admin UI | 用户/管理用量表新增 IP geolocation 单查/批量获取和 24h localStorage cache; 管理端 group column settings; 用户模型统计按 requested model 聚合。 |
| Subscription/Payment | 订阅支持撤销与恢复; OpenAI subscription expiration / plan type 持久化; reset credit expiration 展示; refund pending 继续沿用终态查询语义。 |
| OpenAI WS/OAuth | WS ingress 增加 http_bridge 模式和账号选择, setup-token 账号支持 ws mode 编辑; count_tokens 对 OpenAI OAuth scope error 做兼容处理。 |
| Anthropic/Ollama/Claude | Ollama Anthropic Bearer API Key 认证兼容; Claude Code stream keepalive stall 修复; Claude OAuth 删除 expires_in 依赖。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/config/config.go` | 同时保留本分支 `openai_default_reasoning_effort`、`request_archive`、`request_intercept` 默认值, 并吸收上游 `openai_compact_model` 默认值。 |
| `backend/internal/handler/endpoint.go` | 同时保留根级 `/responses` 与 `/backend-api/codex/responses` 归一化, 并新增 videos endpoint 归一化。 |
| `backend/internal/server/routes/gateway.go` | 根级 images/videos 路由使用上游 `imagesHandler` / Grok video handler, 同时显式保留本分支 `RequestArchive` / `RequestIntercept` 中间件链。 |
| `backend/internal/server/routes/gateway_test.go` | 保留本分支 request archive/intercept 路由回归测试, 吸收上游 Grok images/videos 与非 Grok videos gate 测试, 并适配本地 option-style test router。 |
| `deploy/config.example.yaml` | 同时保留本分支 reasoning effort / request archive 示例配置, 并追加上游 `openai_compact_model` 示例配置。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 本次上游合并未覆盖本地 prompt risk 配置、judge fail-open 与审计边界。 |
| RequestArchive / RequestIntercept | 保留; 冲突路由中根级 images/videos 也显式带上 archive/intercept 中间件, 并保留路由测试。 |
| token 分析、图片生成、用户并发方案 | 保留; 本地归档/分析/并发入口未被删除, Grok media 新增能力与现有图片 gate 并存。 |
| 默认 OpenAI reasoning_effort 注入与 DeepSeek cache-hit usage 兼容 | 保留; config 冲突中保留默认 effort 注入配置, usage parser 相关逻辑未被覆盖。 |

### Verification

| Command | Result |
|---|---|
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)$" backend/internal/config/config.go backend/internal/handler/endpoint.go backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go deploy/config.example.yaml` | Passed, no conflict markers; `rg` exit 1 because no matches. |
| `gofmt -w backend/internal/config/config.go backend/internal/handler/endpoint.go backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go` | Passed. |
| `git diff --cached --check` | Passed before merge commit. |
| `go test -tags=unit -p 1 -count=1 ./internal/config` with repo-local `GOCACHE/GOTMPDIR/GOPATH/GOMODCACHE` | Passed (`ok .../internal/config 1.243s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/handler` with repo-local `GOCACHE/GOTMPDIR/GOPATH/GOMODCACHE` | Passed (`ok .../internal/handler 55.178s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/server/routes` with repo-local `GOCACHE/GOTMPDIR/GOPATH/GOMODCACHE` | Passed (`ok .../internal/server/routes 3.439s`). |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录本次上游合并带来的稳定知识: v0.1.143、`/responses/compact` 模型配置、Grok images/videos media、OpenAI Spark shadow、高峰时段倍率、IP geolocation、订阅撤销/恢复、OpenAI WS http_bridge ingress。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 91d67e816
git show --cc 91d67e816 -- backend/internal/config/config.go backend/internal/handler/endpoint.go backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go deploy/config.example.yaml
git diff --stat db0414233ce324903adc72e858374086da158b4b..a5638a4e5408b14f05a63d7d3b118d6359489b32
git log --oneline db0414233ce324903adc72e858374086da158b4b..a5638a4e5408b14f05a63d7d3b118d6359489b32
```

## 2026-07-08 main sync

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `62e6a2e99` |
| Merge base | `a5638a4e5` |
| Upstream head merged | `6f43986c3` |
| Merge commit | `未提交（按用户要求不提交）` |
| Upstream version | `0.1.146` |
| Upstream commits | 180 |
| Files changed | 656 (`+118779 / -100144`) |
| Conflict files | `backend/cmd/server/wire_gen.go`; `backend/go.mod`; `backend/internal/handler/admin/setting_handler.go`; `backend/internal/handler/endpoint.go`; `backend/internal/service/concurrency_service_test.go`; `backend/internal/service/openai_gateway_responses_chat_fallback.go`; `backend/internal/service/openai_gateway_service.go`; `backend/internal/service/setting_service.go`; `frontend/src/components/layout/AppSidebar.vue`; `frontend/src/i18n/locales/en.ts`; `frontend/src/i18n/locales/zh.ts` |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入当前敏感词过滤分支。上游 head 为 `6f43986c3`, VERSION 更新到 `0.1.146`。

主要上游内容:

| Area | Notes |
|---|---|
| Batch image | 新增 batch image 任务/事件/下载/结算/清理/worker runtime 体系, 路由为 `/v1/images/batches*`, 并新增用户侧 `/batch-image` 指引页。 |
| Backend split | `setting_service.go`, `setting_handler.go`, `admin_service.go`, `gateway_service.go`, `openai_gateway_service.go`, `openai_ws_forwarder.go`, `usage_log_repo.go` 等大文件拆分为领域文件。 |
| Gateway/OpenAI | 新增 shared CC pipeline 拆分、compact body-signal 路由修复、OpenAI official payload sanitizer 继续删除顶层 `thinking` 并保留 `reasoning`。 |
| Scheduling/Admin | 账号 scheduler score 改为前端列可见时 opt-in 请求, 避免默认列表触发高成本计算。 |
| Frontend/i18n | `locales/en.ts`、`zh.ts` 拆为域模块并增加 key collision 测试; 侧栏同时保留本地 `/image-gen` 与上游 `/batch-image`。 |
| Dependencies | `github.com/aws/aws-sdk-go-v2/service/s3` 升级到 `v1.97.3`。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/cmd/server/wire_gen.go` | 保留本地 `tokenAnalysisService`、`userConcurrencyPresetRunner`、`extension`, 同时加入上游 `batchImageCleanupService`、`batchImageWorkerRuntime`。 |
| `backend/go.mod` | 采用上游 AWS S3 SDK `v1.97.3` 并保留本地依赖约束。 |
| `backend/internal/handler/admin/setting_handler.go` | 接受上游 settings handler 拆分结构, 并补回 RequestArchive settings DTO/handler 到 runtime 文件。 |
| `backend/internal/handler/endpoint.go` | 保留根级 Responses alias, 同时采用上游 `/responses/compact` 独立归一化。 |
| `backend/internal/service/concurrency_service_test.go` | 同时保留本地 cache error 测试与上游 API key slot 测试。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` / `openai_gateway_service.go` | 接受上游 OpenAI gateway 拆分, 补回 compatible cache usage parser、request body sanitizer 和 Responses fallback usage 写回。 |
| `backend/internal/service/setting_service.go` | 接受上游 service 拆分, 补回 RequestArchive runtime cache/singleflight 和独立 `setting_request_archive.go`。 |
| `frontend/src/components/layout/AppSidebar.vue` | 同时保留本地 `/image-gen` 菜单和上游 `/batch-image` 菜单。 |
| `frontend/src/i18n/locales/en.ts` / `zh.ts` | 接受上游 i18n 域模块拆分, 删除旧单文件, 并把本地 `nav.imageGeneration` 迁入 `common.ts`。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 本次同步未删除本地风险控制入口。 |
| RequestArchive / RequestIntercept | 保留; setting runtime、service cache 与路由中间件语义继续存在。 |
| Token Analysis | 保留; 用户排行按用户聚合、隐藏 key 的局部改动继续存在。 |
| 图片生成 | 保留; 用户侧 `/image-gen` 与上游 `/batch-image` 并存。 |
| 用户并发方案 | 保留; 并发 preset runner 与 cache-error 语义测试保留。 |
| OpenAI compatible cache usage | 保留并修复; Chat/Responses fallback 继续识别 `cache_read_input_tokens` / `cached_tokens` / `prompt_cache_hit_tokens`。 |

### Verification

| Command | Result |
|---|---|
| `go test -tags=unit -p 1 -count=1 -run TestOpenAIGatewayService_ResponsesCompatPreservesCompatibleCacheUsage ./internal/service` | Passed (`ok .../internal/service 2.152s`). |
| `go test -tags=unit -p 1 -count=1 -run 'Test(ParseSSEUsage_CompatibleCacheFieldsFallback|ExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback|OpenAIGatewayService_APIKeyPassthrough_StripsTopLevelThinkingAndKeepsReasoning)$' ./internal/service` | Passed (`ok .../internal/service 2.228s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/service` with repo-local `GOCACHE/GOTMPDIR/GOPATH/GOMODCACHE` | Passed (`ok .../internal/service 95.102s`). |
| `go test -tags=unit -p 1 -count=1 ./cmd/server ./internal/handler ./internal/server/routes` with repo-local Go cache | Passed (`cmd/server 1.828s`, `handler 26.818s`, `routes 3.358s`). |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)$" backend frontend deploy docs llm-wiki README.md README_CN.md README_JA.md AGENTS.md Dockerfile .github` | Passed, no conflict markers; `rg` exit 1 because no matches. |
| `git diff --check` | Passed. |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录本次上游合并带来的稳定知识: v0.1.146、batch image、i18n 拆分、大文件拆分、Responses/Chat fallback compatible cache usage、OpenAI passthrough sanitizer 和 Windows 验证命令。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames <merge-commit-after-user-approval>
git show --cc <merge-commit-after-user-approval> -- backend/cmd/server/wire_gen.go backend/internal/handler/endpoint.go backend/internal/service/openai_gateway_response_handling.go backend/internal/service/openai_gateway_passthrough.go frontend/src/components/layout/AppSidebar.vue
git diff --stat a5638a4e5408..6f43986c3
git log --oneline a5638a4e5408..6f43986c3
```