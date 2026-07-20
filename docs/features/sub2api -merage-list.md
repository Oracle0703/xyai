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

## 2026-07-10 main sync

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `346aca880a13` |
| Merge base | `6f43986c376d` |
| Upstream head merged | `ddb1a210ce67` |
| Merge commits | `c0b00fb1227e` (to `301c99a26c53`); `a1745d11d6a1` (final to `ddb1a210ce67`) |
| Upstream version | `0.1.149` |
| Upstream commits | 93 |
| Files changed | 244 (`+13924 / -754`) |
| Conflict files | `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`; `backend/internal/pkg/apicompat/types.go`; `backend/internal/service/openai_gateway_chat_completions_raw_test.go`; `backend/internal/service/openai_gateway_responses_chat_fallback.go` |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入当前敏感词过滤分支。首次验证期间上游从 `301c99a26c53` 前进到 `ddb1a210ce67`, 因此继续完成第二个增量 merge; 最终 VERSION 为 `0.1.149`, Go 工具链为 `1.26.5`。

主要上游内容:

| Area | Notes |
|---|---|
| OpenAI/Codex | GPT-5.6 `max` effort、Codex 0.144.1、model manifest、Responses/Chat `parallel_tool_calls` 与 `response_format` 兼容映射。 |
| Compact / gateway | body-signal stream 响应恢复 SSE、SSE→JSON raw output preservation、keepalive 与并发写加固; `response.failed` 对齐错误透传/failover。 |
| Grok/xAI | 官方 Grok 4.5 支持, 图片/视频价格拆分, 视频按分辨率和秒数计费, quota/media flow 加固。 |
| Admin / usage | 用户角色创建编辑、用户 Token 排行、用量页延迟健康列、Group 已用额度、API Key 当前并发排序和 last used IP。 |
| Frontend / ops | 最近 3 个历史版本在线回退, site logo/doc URL sanitization, logout/payment/session 可靠性修复, public settings 请求去重与 feature-access fail-safe, i18n 缺失 key 补齐。 |
| Data / security | migrations 170-172 增加视频定价/usage metadata; Gemini API Key 鉴权、支付履约 lease、订阅窗口同步 CAS/回读、lenient JSON 上限、HTML escape 与渠道共享定价污染修复。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 上游新增的 `ResponsesToChatCompletionsRequest` 与本地已拆出的 options 版本重复; 保留本地独立转换文件, 将上游 `parallel_tool_calls` / `response_format` 映射补入 `ResponsesToChatCompletionsRequestWithOptions`, 避免重复符号并保留第三方上游过滤策略。 |
| `backend/internal/pkg/apicompat/types.go` | 同时保留本地 `PromptCacheKey` / `PreviousResponseID` 与上游 `ResponseFormat`, 并保留自动合入的 `ParallelToolCalls`。 |
| `backend/internal/service/openai_gateway_chat_completions_raw_test.go` | 两侧测试均保留: 本地 compatible cache usage streaming 回归和上游 mapped GPT-5.6 max effort 回归。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 保留本地 `ResponsesToChatCompletionsRequestWithOptions` 与兼容过滤, 同时采用上游 `upstreamModel/billingModel/originalModel` 候选 effort 提取顺序。 |
| `frontend/src/views/auth/__tests__/EmailVerifyView.spec.ts` | 全量测试发现本地历史 affiliate fixture 已带 `AFF123` 但期望遗漏 `aff_code`; 仅补测试期望, 不改业务逻辑。 |
| `301c99a26c53..ddb1a210ce67` latest increment | 无冲突; `frontend/src/router/index.ts` 自动合并后同时保留本地 `/image-gen`、Token Analysis、Request Intercept、Prompt Metrics 路由与上游 feature-access guard。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 路由、service、judge fail-open、i18n 和定向回归测试均存在。 |
| RequestArchive / RequestIntercept | 保留; `/v1`、Gemini、Responses/Codex、根级 Chat/images/videos 中间件链和路由回归均通过。 |
| Token Analysis | 保留; admin route、handler/service/repository 与归档索引入口未被覆盖。 |
| 图片生成 | 保留; 用户侧 `/image-gen` 与上游 `/batch-image` 并存, AppSidebar/Router 全量测试通过。 |
| 用户并发方案 | 保留; preset route、service/runner 和 cache invalidation 定向测试通过。 |
| OpenAI compatible cache usage | 保留; Responses→Chat fallback 继续识别 `cache_read_input_tokens` / `cached_tokens` / `prompt_cache_hit_tokens`, 与上游 GPT-5.6/response format 兼容共存。 |

### Verification

| Command | Result |
|---|---|
| `git diff --name-only --diff-filter=U` | Passed, no unresolved merge files. |
| `rg -n "^(<<<<<<< .+|=======$|>>>>>>> .+)$" backend frontend deploy docs llm-wiki README.md README_CN.md README_JA.md AGENTS.md Dockerfile .github` | Passed, no conflict markers; `rg` exit 1 because no matches. |
| `git diff --cached --check` | Passed before merge commit. |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat` with repo-local caches and fresh `GOTMPDIR` | Passed (`0.837s`). |
| Focused local-feature tests in `./internal/server/routes ./internal/service ./internal/handler/admin` | Passed (RequestArchive/Intercept, Prompt Risk, concurrency preset, request intercept admin, Token Analysis). |
| `go test -tags=unit -p 1 -count=1 ./...` on final `ddb1a210ce67` merge with repo-local caches and fresh `GOTMPDIR` | Passed in one complete run; `internal/service` passed in `109.696s`. |
| `go test -tags=integration -p 1 -count=1 ./...` on final `ddb1a210ce67` merge with repo-local caches and fresh `GOTMPDIR` | Passed; `internal/service` passed in `57.309s`. |
| `cmd.exe /c pnpm --dir frontend run lint:check` | Passed. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `cmd.exe /c pnpm --dir frontend exec vitest run` | Passed: 151 test files, 955 tests. |

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录 v0.1.149、Go 1.26.5、GPT-5.6/Codex/compact 兼容、Grok 视频按秒计费、支付/订阅并发恢复、角色/Token 排行/版本回退、feature-access、migrations 170-172 和安全加固。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames c0b00fb1227e
git show --cc c0b00fb1227e -- backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go backend/internal/pkg/apicompat/types.go backend/internal/service/openai_gateway_chat_completions_raw_test.go backend/internal/service/openai_gateway_responses_chat_fallback.go
git show --stat --summary --find-renames a1745d11d6a1
git diff --stat 6f43986c376d..ddb1a210ce67
git log --oneline 6f43986c376d..ddb1a210ce67
```

## 2026-07-10 main sync follow-up (v0.1.150)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/0621_敏感词过滤` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `4b1bb6cb0960` |
| Merge base | `ddb1a210ce67` |
| Upstream head merged | `9a2f11b4e217` |
| Merge commit | `61fec21ade8c` |
| Upstream version | `0.1.150` |
| Upstream commits | 13 |
| Files changed | 32 (`+703 / -172`) |
| Conflict files | `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`; `backend/internal/service/openai_gateway_chat_completions_raw.go` |

### Summary

在 v0.1.149 全量验证期间, 上游 `main` 从 `ddb1a210ce67` 继续前进到 `9a2f11b4e217`, 因此完成第三个增量 merge。最终 VERSION 为 `0.1.150`, Go 工具链保持 `1.26.5`。

本次增量上游内容:

| Area | Notes |
|---|---|
| GPT-5.6 billing | 增加 cache-write token 解析、官方 cache-write 价格、渠道显式价格保留和用量统计, 普通 input/cache read/cache write 改为互斥计费桶。 |
| OpenAI WS | WS passthrough reasoning effort 使用 `upstreamModel -> billingModel -> originalModel` 候选; Windows connection reset/abort 被识别为网络错误。 |
| Admin usage | `GetUserBreakdown` 使用 `ParseUsageRequestType`, 前端 `UserBreakdownParams.request_type` 改为 `UsageRequestType`, 修复请求类型筛选口径。 |
| Reliability | 稳定 expired lock reconciliation integration test, 并统一 `isOpenAIGPT56Model` 到 `openai_model_alias.go`。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/pkg/apicompat/chatcompletions_responses_test.go` | 同时保留本地 `PromptCacheKey` 转换回归和上游 cache-write usage round-trip 回归。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 采用上游 `openAIUsageFromGJSON` 统一解析 input/output/cache-write, 再调用本地 compatible cache helper 补 `prompt_cache_hit_tokens` 等第三方字段。 |
| `backend/internal/service/openai_gateway_response_handling.go` | 全量测试发现兼容 helper 会用 0 覆盖上游已解析的嵌套 `cache_write_tokens`; 改为复用统一 cache 字段解析且只在识别到兼容值时覆盖, 同时保留 DeepSeek `prompt_cache_hit_tokens`。补 compatible cache read 时改为原位更新 Responses/Chat details, 避免替换对象后丢失已有 cache-write 字段。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 路由、service、judge fail-open、i18n 与测试未被本次增量覆盖。 |
| RequestArchive / RequestIntercept | 保留; 网关中间件、运行时设置和管理路由继续存在。 |
| Token Analysis | 保留; admin route、handler/service/repository 与索引入口继续存在。 |
| 图片生成 | 保留; `/image-gen` 与上游 `/batch-image` 继续并存。 |
| 用户并发方案 | 保留; preset route/service/runner 与缓存失效逻辑继续存在。 |
| OpenAI compatible cache usage | 保留并与上游 cache-write 合并; `cache_read_input_tokens` / `cached_tokens` / `prompt_cache_hit_tokens` 与 `cache_write_tokens` / `cache_creation_input_tokens` 同时解析。 |

### Verification

| Command | Result |
|---|---|
| `git -c http.sslBackend=schannel -c http.curloptResolve=github.com:443:140.82.114.3 ls-remote upstream refs/heads/main` | Passed; final upstream `main` is `9a2f11b4e21763cb7003ea29921d9a672ab50b1f`. |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat` | Passed (`0.554s`). |
| Conflict-focused cache/GPT-5.6/WS tests in `./internal/service` | Passed (`2.986s`). |
| `go test -tags=unit -p 1 -count=1 -run '^TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes$' ./internal/service` | Passed after the merge-crossing cache-write fix (`4.072s`). |
| Cache-details preservation regression tests (`Responses` / `Chat`) plus compatible cache focused tests | Passed (`2.986s`); existing cache-write details remain while compatible cache-read is supplemented. |
| `go test -tags=unit -p 1 -count=1 ./...` with repo-local caches and fresh `GOTMPDIR` | Passed on final code in a complete attempt; `internal/service` passed in `95.832s`. |
| `go test -tags=integration -p 1 -count=1 ./...` with repo-local caches and fresh `GOTMPDIR` | Passed on final code in a complete attempt; `internal/service` passed in `56.085s`. |
| `cmd.exe /c pnpm --dir frontend run lint:check` | Passed. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `cmd.exe /c pnpm --dir frontend exec vitest run` | Passed: 151 test files, 955 tests. |
| Conflict-marker scan, unresolved-file scan and `git diff --check` | Passed. |

Windows 首轮 unit/integration 曾随机遇到 `.test.exe` `Access is denied` / file-in-use, 按 `llm-wiki/wiki/ops.md` 使用全新 `GOTMPDIR` 串行重跑后完整通过; 未为环境锁修改业务代码。

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录 v0.1.150、GPT-5.6 cache-write 计费、WS effort 候选与 Windows reset 分类、用户 breakdown request-type 口径, 以及 compatible cache usage 与上游 cache-write 的合并约束。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 61fec21ade8c
git show --cc 61fec21ade8c -- backend/internal/pkg/apicompat/chatcompletions_responses_test.go backend/internal/service/openai_gateway_chat_completions_raw.go
git diff --stat ddb1a210ce67..9a2f11b4e217
git log --oneline ddb1a210ce67..9a2f11b4e217
```

## 2026-07-13 main sync (v0.1.151)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10151_同步sub2api主线` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `89711be2324d` |
| Merge base | `6dd3274aafbc` |
| Upstream head merged | `42f3c22830b8` |
| Merge commit | `5655815f283e` |
| Upstream version | `0.1.151` |
| Upstream commits | 47 |
| Upstream files changed | 87 (`+7043 / -361`) |
| Merge result vs first parent | 88 files (`+7056 / -379`) |
| Conflict files | `backend/internal/handler/endpoint.go`; `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`; `backend/internal/pkg/apicompat/types.go`; `backend/internal/server/routes/gateway.go`; `backend/internal/service/openai_gateway_chat_completions_raw.go` |

### Summary

将 Wei-Shaw/sub2api 最新 `main` 合入新分支, VERSION 从 `0.1.150` 更新到 `0.1.151`。主要上游内容如下。

| Area | Notes |
|---|---|
| Responses/Chat tools | custom/freeform、namespace 和 tool_search 支持 Chat fallback 与 streaming/non-streaming 回程还原, 并拒绝工具名歧义和无效 tool_choice。 |
| Codex | 新增三路 alpha search, 修复 OAuth Messages identity、originator/User-Agent 配对和续链 `item_*` ID。 |
| Fast/Flex policy | 规则新增 `user_ids`, 设置页增加可搜索用户选择器及中英文 key coverage。 |
| Grok | Free OAuth prompt cache identity、cacheable Chat -> Responses bridge、quota exhausted 恢复与 usage snapshot 加固。 |
| Reliability | compact keepalive nil/lifecycle 守卫, `remote_compaction_v2` 原生 Responses 保留, ops capture writer 释放后 nil 安全。 |
| Usage compatibility | Responses/Anthropic streaming 与非流式路径补齐 `cache_creation_input_tokens`。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/handler/endpoint.go` | 同时保留本地裸 `/chat/completions` 归一化和上游 alpha search 归一化/直达 endpoint。 |
| `backend/internal/server/routes/gateway.go` | 保留 RequestArchive/RequestIntercept 中间件链, 同时加入 `/v1/alpha/search`、`/alpha/search` 与 Codex direct alpha search。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 先执行本地 official Chat sanitizer, 再执行上游 Grok Responses-only prompt cache key 清理; 上游 cache identity/endpoint tracking 保留。 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 删除与本地 options adapter 重复的上游入口, 保留上游 custom/namespace/tool_search 全套 helper 和回程生命周期。 |
| `backend/internal/pkg/apicompat/types.go` | 同时保留本地 streaming output DTO 与上游 `tool_search_call` 自定义 JSON 语义。 |
| Auto-merge follow-up | 将上游 tool conversion/declared-tool choice 接入 `ResponsesToChatCompletionsRequestWithOptions`; 修复 alpha route test helper 参数和 legacy tool-choice fixture。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; 管理路由、service 与前端 risk-control 入口存在。 |
| RequestArchive / RequestIntercept | 保留; `/v1`、Gemini、root Responses/Chat/images/videos、Codex direct 和新 bare alpha search 均保留中间件语义。 |
| Token Analysis | 保留; admin route、handler/service/repository、前端路由/API 未被删除。 |
| 图片生成 | 保留; `/image-gen` 与上游 `/batch-image` 在 router/sidebar 并存。 |
| 用户并发方案 | 保留; preset route/service/runner 和前端 dialog 未被删除。 |
| OpenAI compatible cache usage | 保留; raw/buffered/streaming normalization 与 cache-write additive/fill-missing 约束继续存在。 |

### Verification

| Command | Result |
|---|---|
| Pre-merge focused backend baseline | Passed: config/routes/handler/service. |
| Pre-merge frontend typecheck and Vitest | Passed: 151 files, 956 tests. |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat` | Passed (`0.771s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/handler ./internal/server/routes` | Passed (`33.188s`, `3.361s`). |
| `go test -tags=unit -p 1 -count=1 ./internal/service` | Passed (`97.563s`). |
| Focused frontend local/upstream feature set | Passed: 9 files, 46 tests. |
| Unresolved-file scan, conflict-marker scan and `git diff --cached --check` | Passed before merge commit. |

Windows 定向测试曾遇到 `.test.exe` `Access is denied` / file-in-use, 按 `llm-wiki/wiki/ops.md` 更换全新 repo-local `GOTMPDIR` 后通过; 未为环境锁修改业务代码。

### Wiki Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `security-and-reliability.md`, 记录 v0.1.151、alpha search、tool bridge、用户级 Fast/Flex、Grok prompt cache/quota recovery 与合并保留约束。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 5655815f283e
git show --cc 5655815f283e -- backend/internal/handler/endpoint.go backend/internal/server/routes/gateway.go backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go backend/internal/pkg/apicompat/types.go backend/internal/service/openai_gateway_chat_completions_raw.go
git diff --stat 6dd3274aafbc..42f3c22830b8
git log --oneline 6dd3274aafbc..42f3c22830b8
```

## 2026-07-13 main sync (v0.1.153, pinned)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10153_同步sub2api主线` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main`（合并时固定到目标提交） |
| Base before merge | `5e6e85568792`（0.1.151 分支基线） |
| Merge base | `42f3c22830b8` |
| Upstream head merged | `55ed0ab0da367183d97c15659e33ae9e83f6ff90` |
| Merge commit | `0d65f65a20df72aa1ec81966898e3be8699270a0` |
| Upstream version | `0.1.153` |
| Upstream commits | 58 |
| Upstream files changed | 157 (`+6184 / -318`) |
| Merge result vs first parent | 158 files (`+6182 / -319`) |
| Conflict files | `README_CN.md`; `backend/go.mod`; `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`; `backend/internal/server/routes/gateway.go`; `backend/internal/service/concurrency_service.go`; `frontend/src/components/account/CreateAccountModal.vue` |

### Boundary

本次按用户要求只同步到 `55ed0ab0d`。验证时远端 `upstream/main` 已前进到 `7d239d62e`; 后续 4 个提交 `5aeb03018`、`bb7341673`、`adb5106c1`、`7d239d62e` 均未合入。`git merge-base --is-ancestor 7d239d62e HEAD` 返回 1。

### Summary

| Area | Notes |
|---|---|
| OpenAI WS | 新增 completed turn 间空闲超时和 API Key 级分布式 ingress 连接 lease; 默认 300 秒 / 64 连接, 0 可关闭。 |
| Grok/xAI | 支持 API Key 账号、OAuth CLI proxy 与可信自定义 base URL、视频 edits/extensions、API Key 上游模型同步和 Grok CLI/OpenCode 配置；OAuth 模型同步仍显式不支持。 |
| Billing / data | alpha search 仅成功 2xx 按次计费, group 新增 `web_search_price_per_call`; 增加 API Key latest-IP 并发索引。 |
| API compatibility | 合并 Responses `additional_tools`; Read 参数 delta 实时原样透传; 补 Anthropic/Responses/Chat 的 max token/content filter stop reason 映射。 |
| Security / web | 删除泄露内部 AI 渠道配置的 payment channels endpoint; 嵌入静态资源设置一年 immutable 缓存。 |
| Frontend / deploy | DataTable 小列表关闭虚拟化并按 row key 缓存高度, 日期范围使用本地日期, OpenAI OAuth 支持 `plan_type`, pool retry 覆盖更多转发路径, 新增 Apple container。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `README_CN.md` | 同时保留本地 Windows 手动重启记录和上游 Apple container; 源码编译顺延为方式四。 |
| `backend/go.mod` | 保留上游直接依赖 `x/mod`, 同时保留本地直接使用的 `x/sys`、`x/text`; `go mod tidy` 后三者仍为 direct。 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 保留本地 `ResponsesToChatCompletionsRequestWithOptions` 唯一入口, 加入上游 `EffectiveResponsesTools`; options adapter 继续执行第三方 temperature/max token 过滤。 |
| `backend/internal/server/routes/gateway.go` | 保留 RequestArchive/RequestIntercept 中间件链, 加入 Grok videos edits/extensions 的 `/v1` 与根级别名; 非 Grok 仍本地 404 + business-limited。 |
| `backend/internal/service/concurrency_service.go` | 保留本地 `ConcurrencyCacheError` 与 503 语义, 加入上游 WS ingress lease/refresh/lost-close 生命周期。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 保留本地 OpenAI-compatible provider preset、动态 placeholder 和 credentials metadata, 同时加入 Grok API Key base URL、`xai-...` 与提交 fallback。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 敏感词过滤和 LLM judge | 保留; route/service/UI 与 fail-open 约束未被覆盖。 |
| RequestArchive / RequestIntercept | 保留; `/v1`、root、Codex、Gemini 与新增 Grok videos 路由维持中间件链。 |
| Token Analysis | 保留; admin route、handler/service/repository 与前端入口存在。 |
| 图片生成 | 保留; `/image-gen`、batch image 与 OpenAI/Grok image 路由并存。 |
| 用户并发方案 | 保留; preset/runner、普通 slot 与 `ConcurrencyCacheError` 均存在。 |
| OpenAI-compatible preset / options adapter | 保留并接入上游 Grok API Key、`additional_tools`。 |
| compatible cache usage | 保留; raw/buffered/streaming 和 billing cache 字段未被覆盖。 |

### Verification

| Command | Result |
|---|---|
| `git rev-list --parents -n 1 0d65f65a` | Passed; parents are `5e6e85568` and `55ed0ab0d`. |
| Target/later ancestor checks | Passed; target is included, `7d239d62e` is excluded. |
| Unresolved-file scan, exact conflict-marker scan, `git diff --check` | Passed. |
| `cd backend; go mod tidy -diff` | Passed; no module diff. |
| Focused conflict review/tests | Passed; backend apicompat/routes/concurrency and 5 frontend files / 53 tests. |
| `go test -tags=unit -p 1 -count=1 ./...` | Passed in a complete retry; `internal/service` `101.002s`. First attempt was interrupted by Windows security software holding `web.test.exe`, not by a test assertion. |
| `go test -tags=integration -p 1 -count=1 ./...` | Passed; `internal/service` `57.799s`. |
| `cmd.exe /c pnpm --dir frontend run lint:check` | Passed. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `cmd.exe /c pnpm --dir frontend exec vitest run` | Passed: 156 files, 997 tests. |
| `cmd.exe /c pnpm --dir frontend run build` | Passed: 926 modules, `15.56s`; only existing import/chunk warnings. |
| Frontend build followed by `go build -tags embed -trimpath ./cmd/server` | Passed; Windows artifact 145,468,416 bytes. |
| `golangci-lint run --new-from-rev HEAD^1 ./...` | Passed: 0 issues introduced by the merge. |
| Full `golangci-lint run ./...` | 29 existing issues; all affected lines already exist in 0.1.151 first parent, so this is recorded baseline debt rather than a merge regression. |

Windows unit 首轮遇到 `go: unlinkat ... web.test.exe: Access is denied`; 改用仓库外全新 `E:\tmp\xyai-unit-final-*` 作为 `GOTMPDIR` 后完整通过。Apple container shell test 未在无 bash 的 Windows 本机运行; 新增 macOS CI job 负责 `/bin/bash -n` 和 fixture test。

### Documentation Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`; 新增 account/keys 组件 README, 更新 common/DataTable README, 并新增 `docs/reviews/2026-07-13-upstream-55ed0ab-merge-review.md`。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames 0d65f65a20df
git show --cc 0d65f65a20df -- README_CN.md backend/go.mod backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go backend/internal/server/routes/gateway.go backend/internal/service/concurrency_service.go frontend/src/components/account/CreateAccountModal.vue
git diff --stat 42f3c22830b8..55ed0ab0da36
git log --oneline 55ed0ab0da36..upstream/main
```

## 2026-07-14 main sync (v0.1.155)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10155_同步sub2api主线` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main` |
| Base before merge | `20e6379f9243f00aaf84b562af40c7b80793d4fc` |
| Merge base | `55ed0ab0da367183d97c15659e33ae9e83f6ff90` |
| Upstream head merged | `7c717365ef728e53cdcf6d639a4dd68226db03b2` |
| Merge commit | `d294d493705a252a5038287a96643a43a38b330e` |
| Upstream version | `0.1.155` |
| Upstream commits | 71 |
| Upstream files changed | 238 (`+15089 / -880`) |
| Merge result vs first parent | 238 (`+15087 / -879`) |
| 双方同时修改文件 | 23 个, 均完成自动合并结果审查 |
| Conflict files | `backend/internal/repository/redis.go`; `backend/internal/server/router.go`; `backend/internal/service/content_moderation.go` |

### Summary

| Area | Notes |
|---|---|
| Observability | 增加 opt-in Admin UI Server-Timing, 汇总 SQL、Redis、外部 HTTP、cache 与总耗时; Ops system logs 增加 host 筛选。 |
| OpenAI compatibility | 增加 native Responses namespace 可逆摊平/回程恢复, Responses Lite 保留客户端 image tools, 图片非流式 keepalive 和图片终态修正, Codex manifest API Key failover。 |
| Billing / data | OpenAI 长上下文费率改为账号级 opt-in, 默认关闭并把应用结果写入 usage log; 增加 migrations 174-176 及 Ent 生成代码。 |
| Grok | 增加 Web SSO -> Build OAuth 批量导入、导入后 probe、Channel Monitor provider、滚动 24h Free 配额估算、OAuth media 官方 API 路由和 reasoning null 清理。 |
| Reliability | 修复 scheduler 全量重建并发合并/事件延迟、代理到期和账号自动暂停触发重建、HTTP/2 keep-alive PING 与 quota reset credit 检测。 |
| Frontend | 账号页增加 Grok SSO 和 OpenAI 长上下文计费开关, 监控页展示 Grok/Free 状态, Ops 日志增加 host 条件, 管理请求统一标记 `X-Admin-UI-Request`。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/repository/redis.go` | 保留本地 `InitRedis(...)(*redis.Client,error)`、启动时 Redis 7+/Memurai 校验和失败关闭客户端; 同时在 `server.enable_server_timing` 开启时注册上游 `serverTimingRedisHook`。 |
| `backend/internal/server/router.go` | 同时挂载上游 `ServerTiming` 和本地 Prompt Metrics `CaptureMiddleware`; 保留 Prompt Metrics 管理路由与合规确认门。 |
| `backend/internal/service/content_moderation.go` | 保留本地可选 `config.Config`、Prompt Risk judge semaphore 和 LLM 语义审核; 将共享 HTTP client 改为上游 `servertiming.InstrumentClient(nil)`。 |
| Auto-merge review | 对 23 个双方修改文件逐项检查相对本地 `main` 的删除行和合并结果, 确认 Wire/config/admin routes/OpenAI gateway/usage/frontend account locale 均为上游增量叠加, 未发现本地合同被静默删除。 |

### 本地能力保留确认

| 能力 | 状态 |
|---|---|
| Prompt Risk / 内容审核 / LLM judge | 保留; 配置注入、judge fail-open、管理路由、Prompt Metrics 和全量 service tests 均存在。 |
| RequestArchive / RequestIntercept | 保留; 网关路由、中间件链、运行时设置与管理端入口未被上游覆盖。 |
| Token Analysis / 图片生成 | 保留; admin service/repository、`/image-gen`、batch image 和 OpenAI/Grok images 路径并存。 |
| 用户并发与缓存错误 | 保留; preset/runner、普通 slot、`ConcurrencyCacheError` 与 503 语义仍存在。 |
| OpenAI-compatible options/cache usage | 保留; 本地 Responses→Chat options adapter、provider preset、DeepSeek `prompt_cache_hit_tokens` 和 cache write/read 互不覆盖。 |
| Redis 启动约束 | 保留; Redis 7+ / Memurai 检查与上游 Server-Timing hook 组合生效。 |

### Verification

| Command | Result |
|---|---|
| `git rev-list --parents -n 1 d294d4937` | Passed; parents are local `20e6379f9` and upstream `7c717365e`. |
| Unresolved-file scan, conflict-marker review and `git diff --check` | Passed; 0 unresolved files, 3 conflict files resolved semantically. |
| `go generate ./ent` and `go generate ./cmd/server` | Passed after aligning Wire directive to `go run -mod=mod`; Ent output stable, Wire output regenerated. |
| `go test -tags=unit -p 1 -count=1 ./...` | Passed. First full attempt hit Windows locks in `cmd/server`, `ent/schema`, `internal/service`; all three passed with fresh `GOTMPDIR`, `internal/service` in `100.293s`. |
| `go test -tags=integration -p 1 -count=1 ./...` | Passed. Packages interrupted by Windows `Access is denied` were rerun individually with fresh `GOTMPDIR`; all passed, `responseheaders` passed on retry 2. |
| Upstream frontend focused Vitest | Passed: 12 files, 129 tests. |
| `cmd.exe /c pnpm --dir frontend exec vitest run` | Passed: 163 files, 1055 tests. |
| `cmd.exe /c pnpm --dir frontend run typecheck` | Passed (`vue-tsc --noEmit`). |
| `cmd.exe /c pnpm --dir frontend run lint:check` | Passed when run alone; the parallel first attempt only raced a transient Vite timestamp file. |
| Full `golangci-lint run ./...` | Non-gating: reported 29 existing repository issues; user set this phase acceptance to passing test cases, so no unrelated lint-debt cleanup was included. |

### Documentation Updates

更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`, 记录 v0.1.155、Server-Timing、Responses namespace、Grok SSO/监控、账号级长上下文计费、migrations 174-176 和 Wire 生成命令。

### Useful Diff Commands

```bash
git show --stat --summary --find-renames d294d493705a
git show --cc d294d493705a -- backend/internal/repository/redis.go backend/internal/server/router.go backend/internal/service/content_moderation.go
git diff --stat 55ed0ab0da36..7c717365ef72
git log --oneline 55ed0ab0da36..7c717365ef72
```

## 2026-07-16 main sync (v0.1.156, pinned; awaiting review)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10157_同步sub2api主线` |
| Upstream remote | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git` |
| Upstream branch | `main`（按提交固定边界） |
| Base before merge / first parent | `4c456aad32c086bb32c650d0e8c659450cc6de3f`（本地 `main`、`HEAD`、`ORIG_HEAD`） |
| Merge base | `7c717365ef728e53cdcf6d639a4dd68226db03b2` |
| Release tag target | `v0.1.156` -> `12f991dde8a58e183d4bd16a87ef6fd0df714757` |
| Upstream head merged / second parent | `d515c3045ce838976ebedab87846aaaf893dbbf6` |
| Merge commit | **尚未创建；`MERGE_HEAD=d515c3045ce838976ebedab87846aaaf893dbbf6`，等待用户审核** |
| Upstream version | `0.1.156` |
| Upstream commits | 132 |
| Upstream files changed | 253 (`+31408 / -1710`) |
| 双方同时修改文件 | 29 个，已完成逐文件语义审查 |
| Conflict files | `backend/internal/config/config.go`; `backend/internal/config/config_test.go`; `backend/internal/service/content_moderation.go` |

### Boundary

本次从本地 `main@4c456aad32c0` 创建新分支，只合并上游 `main` 历史中的固定提交 `d515c3045ce8`。该边界包含 `v0.1.156@12f991dde8a5` 及其后的版本同步提交，不包含验证时已前进到 `eb2b8632ded614bf991d7d36abfa38b513ad8c2d` 的后续上游代码，也不包含本地 `feature/hy/10156_新增子管理员角色` 的提交。

### Summary

| Area | Notes |
|---|---|
| OpenAI reliability | 增加 native Responses 首次语义输出超时保护、attempt 级切号、响应头退避信息保留，以及 WS 首消息超时等配置边界。 |
| Grok / token refresh | 增加凭据失败 scope 隔离、OAuth reconcile、候选游标分页、provider QPS/并发 gate 与周期熔断。 |
| Accounts / scheduler | 增加静态凭据账号安全复制、幂等恢复、默认不可调度副本，以及 scheduler projection/batch 查询与生命周期加固。 |
| API compatibility | 增加 Chat Completions <-> Anthropic bridge、Responses Lite/custom tools 处理、Agent Identity 与更多 failover/cancellation 保护。 |
| Frontend / deploy | Codex 模板区分本地 `xunyou` 与 WS v2 `OpenAI` provider，DataTable identity cache 加固，embedded 静态缓存改为只信任 fingerprint 资源。 |
| Local extensions | 本地归档/拦截、Prompt Metrics/Risk、Token Analysis、组织用量、ImageGen/支付、并发 preset、compatible 适配、默认 reasoning effort 与 quota flusher 均保留或与上游组合。 |

### Conflict Resolution Notes

| File | Resolution |
|---|---|
| `backend/internal/config/config.go` | 保留本地 `gateway.openai_default_reasoning_effort` 的字段、默认值与校验，同时合入上游 first-output timeout、WS client-first-message timeout、token refresh 等配置；没有用任一侧整文件覆盖。 |
| `backend/internal/config/config_test.go` | 合并双方配置默认值、环境变量和非法值测试，保留本地 reasoning effort 回归并加入上游新配置边界。 |
| `backend/internal/service/content_moderation.go` | 采用上游 stale-while-refresh runtime snapshot 与预编译 keyword matcher，保留本地 Prompt Risk / LLM judge、semaphore 和 fail-open 语义；补 `risk_control_enabled` 持久化成功后的即时回调，使总开关与 config/prompt-risk snapshot 更新一致。 |

### 本地能力保留确认

| 能力 | 处理 | 代码锚点 | 测试锚点 |
|---|---|---|---|
| RequestArchive / RequestIntercept | **组合**：保留本地 middleware、运行时设置和管理端入口，并覆盖上游新增/调整后的 gateway 路由。 | `backend/internal/server/middleware/request_archive.go`; `backend/internal/server/middleware/request_intercept.go`; `backend/internal/server/routes/gateway.go` | `backend/internal/server/middleware/request_archive_test.go`; `backend/internal/server/middleware/request_intercept_test.go`; `backend/internal/server/routes/gateway_test.go` |
| Prompt Metrics | **保留**：采集 middleware、extension 和管理端页面未被替代。 | `backend/internal/service/promptmetrics/middleware.go`; `backend/internal/server/router.go`; `frontend/src/views/admin/PromptMetricsView.vue` | `backend/internal/service/promptmetrics/middleware_test.go` |
| Prompt Risk / LLM judge | **组合**：上游 runtime snapshot/matcher 与本地规则、judge、并发限制及 fail-open 合并为单条审核链。 | `backend/internal/service/content_moderation.go`; `backend/internal/service/prompt_risk.go`; `backend/internal/service/prompt_risk_judge.go` | `backend/internal/service/content_moderation_runtime_cache_test.go`; `backend/internal/service/prompt_risk_test.go`; `backend/internal/service/prompt_risk_judge_test.go` |
| Token Analysis | **保留**：handler/service/repository、归档索引和前端入口完整。 | `backend/internal/handler/admin/token_analysis_handler.go`; `backend/internal/service/token_analysis_service.go`; `backend/internal/repository/token_analysis_repo.go`; `frontend/src/views/admin/TokenAnalysisView.vue` | `backend/internal/handler/admin/token_analysis_handler_test.go`; `backend/internal/service/token_analysis_summary_test.go`; `backend/internal/repository/token_analysis_repo_test.go` |
| 组织用量 | **保留**：独立报表 API、服务、仓储与导出 UI 继续存在。 | `backend/internal/handler/admin/organization_usage_handler.go`; `backend/internal/service/organization_usage_service.go`; `backend/internal/repository/organization_usage_repo.go`; `frontend/src/views/admin/OrganizationUsageView.vue` | `backend/internal/handler/admin/organization_usage_handler_test.go`; `backend/internal/service/organization_usage_service_test.go`; `frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts` |
| ImageGen / 支付 | **组合**：保留本地 ImageGen 状态/UI 和完整支付域；与上游图片 intent、OpenAI/Grok image 路径组合。 | `frontend/src/views/ImageGenView.vue`; `backend/internal/service/image_generation_intent.go`; `backend/internal/server/routes/payment.go`; `backend/internal/service/payment_service.go` | `frontend/src/views/__tests__/ImageGenView.spec.ts`; `backend/internal/service/image_generation_intent_test.go`; `backend/internal/handler/admin/payment_handler_test.go` |
| 用户并发 presets / `ConcurrencyCacheError` | **保留**：preset runner、缓存错误类型和 503 映射未被上游调度改动覆盖。 | `backend/internal/service/user_concurrency_preset_service.go`; `backend/internal/service/user_concurrency_preset_runner.go`; `backend/internal/service/concurrency_service.go` | `backend/internal/service/user_concurrency_preset_service_test.go`; `backend/internal/service/concurrency_service_test.go`; `backend/internal/handler/concurrency_error_response_test.go` |
| OpenAI-compatible preset / options / cache usage | **组合**：本地 provider preset、Responses -> Chat options 和 cache read/write 补缺逻辑，与上游 bridge/failover 增量共同生效。 | `frontend/src/components/account/openaiCompatibleProviderPresets.ts`; `backend/internal/pkg/apicompat/responses_to_chatcompletions_request.go`; `backend/internal/service/openai_gateway_response_handling.go` | `frontend/src/components/account/__tests__/openaiCompatibleProviderPresets.spec.ts`; `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`; `backend/internal/service/openai_gateway_service_test.go` |
| 默认 reasoning effort | **组合**：本地默认注入能力与上游 first-output/WS 配置在同一 config 冲突中共同保留。 | `backend/internal/config/config.go`; `backend/internal/service/openai_gateway_chat_completions.go` | `backend/internal/config/config_test.go`; `backend/internal/service/openai_gateway_chat_completions_test.go` |
| user-platform quota flusher | **保留**：Redis 脏集快照批量落库、TimingWheel 注册和 shutdown 清理仍在 Wire 图中。 | `backend/internal/service/user_platform_quota_flusher.go`; `backend/internal/service/wire.go`; `backend/cmd/server/wire_gen.go` | `backend/internal/service/user_platform_quota_flusher_test.go`; `backend/internal/repository/user_platform_quota_upsert_test.go` |

本轮没有发现可由上游完整等价替代并安全删除的上述本地功能；因此没有把“上游互补”误写成“上游替代”。`docs/features/` 下 tracked 文件删除数为 0。

### Review Findings Fixed Before Commit

| Finding | Resolution | Regression evidence |
|---|---|---|
| Wire 缺少 `GrokOAuthTokenService -> *GrokOAuthService` binding，`go generate ./cmd/server` 失败 | 在 `backend/internal/service/wire.go` 增加接口绑定并重新生成 `wire_gen.go`。 | Wire 连续生成成功且输出稳定。 |
| `UseKeyModal.spec.ts` 把普通本地 Codex 模板误判为上游 `OpenAI` provider | 普通模板继续断言 `xunyou`，仅 WS v2 模板使用 `OpenAI`。 | 修复前 1/9 失败，修复后 9/9 通过；合并相关 19 个前端测试文件 131/131 通过。 |
| native Responses failover 未保留上游响应头 | 构造 failover error 时克隆 `ResponseHeaders`，只由统一安全过滤恢复允许的 `Retry-After`。 | `backend/internal/service/openai_gateway_forward_sanitize_test.go`; `backend/internal/handler/openai_gateway_credential_failover_test.go` 定向通过。 |
| `risk_control_enabled` 保存后 moderation snapshot 不即时生效 | SettingService 在持久化成功后触发回调，原子替换 snapshot 的 enabled 状态，覆盖 false -> true 与 true -> false。 | `TestContentModerationRuntimeSnapshotRiskControlSettingUpdateIsImmediate` 等 runtime cache 定向测试通过。 |
| passthrough/failover 可接受无上限 `Retry-After` | 统一校验原始值：拒绝 CR/LF 和超过 128 字节；数字只允许 `1..604800` 秒；HTTP date 必须在未来且不超过 7 天。 | service 与 handler 的数字、日期、注入和超界定向用例通过。 |

最终代码审查没有开放的 P0-P3；提交仍受下方完整验证门约束。

### Verification Status

| Check | Current result |
|---|---|
| 3 个文本冲突逐项语义审查、29 个双方修改交集审查 | 已完成；没有待解决文本冲突。 |
| `git ls-files -u` / conflict-marker / scoped `git diff --check` | 已完成定向检查；未发现 unmerged index、残留标记或 whitespace error。 |
| Wire generation | 修复 binding 后连续生成成功，第二次无漂移。 |
| 审查问题定向 RED -> GREEN | 已完成；Wire、UseKeyModal、native header、runtime switch、Retry-After 相关回归通过。 |
| 合并相关前端定向回归 | 19 files / 131 tests passed。 |
| Frontend full Vitest | 176 files / 1214 tests passed。 |
| Backend full unit | 完整重跑退出 0：50 个测试包通过、53 个无测试包、失败 0。首轮 `internal/repository` 的 Windows 启动锁已独立和全量双重重跑通过。 |
| Backend full integration | 完整运行到结尾：44 个测试包通过、57 个无测试包；仅 `internal/pkg/openai_compat` 的测试二进制被本机 360 安全软件持续拒绝执行。该包在 unit/integration tag 下文件集合完全相同，完整 unit 已通过；本条目不把环境拦截写成 integration 退出 0。 |
| Backend lint / module | `golangci-lint run ./...` 报 29 条本地 `main` 既有债务；`--new-from-rev main` 为 0 issues。`go mod tidy -diff` 只建议删除本地 `main` 已存在的 Wire CLI 校验和，模块文件未改。 |
| Frontend / embed | Vitest 176 files / 1214 tests；lint、typecheck、build 均退出 0；build 944 modules / 13.57s。随后 embed build 退出 0，产物 147,534,336 bytes。 |
| Wire / formatting | Wire 连续两次生成退出 0 且 SHA-256 稳定为 `233A9D6C...601E`；gofmt 与 worktree/cached/HEAD 三种 `diff --check` 均通过。 |
| Final staged snapshot | 274 files / `+32475 -1728`；无 unstaged/untracked/unmerged、精确冲突标记、whitespace error、CRLF/BOM、敏感模式或 `docs/features` 删除。 |

### Wiki Updates

已按实际源码边界更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`，覆盖 0.1.156 固定提交、账号复制、Agent Identity、first-output guard、token refresh/reconcile、moderation snapshot、failover `Retry-After` 与静态缓存边界。测试与环境限制记录在 merge review 和交付报告，不混入稳定架构结论。

### Commit Gate

当前仍处于 `git merge --no-commit --no-ff` 状态。验证已形成真实终态，并明确披露 360 integration 二进制拦截、全量 lint 既有债务和前端 warning；仍必须由用户审核本条目、merge review 与 diff 后，才能创建 merge commit。

### Useful Review Commands

```bash
git rev-parse HEAD
git rev-parse MERGE_HEAD
git diff --stat HEAD
git diff HEAD -- backend/internal/config/config.go backend/internal/config/config_test.go backend/internal/service/content_moderation.go
git diff --name-only 7c717365ef72..4c456aad32c0
git diff --name-only 7c717365ef72..d515c3045ce8
git log --oneline d515c3045ce8..eb2b8632ded6
```

## 2026-07-16 v0.1.156 merge commit 回填

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10157_同步sub2api主线` |
| Upstream branch / pinned commit | `Wei-Shaw/sub2api main@d515c3045ce838976ebedab87846aaaf893dbbf6` |
| Merge commit | `b5b54af2129bd5c7cc3d3b54e941deb8a35f31d9` |
| Parent order | `4c456aad32c086bb32c650d0e8c659450cc6de3f`（本地 `main`）; `d515c3045ce838976ebedab87846aaaf893dbbf6`（上游固定边界） |
| Conflict files | `backend/internal/config/config.go`; `backend/internal/config/config_test.go`; `backend/internal/service/content_moderation.go` |
| Conflict handling | 配置与测试保留本地默认 reasoning effort 并合入上游 timeout/token-refresh 边界；moderation 采用上游 runtime snapshot/matcher，同时保留本地 Prompt Risk/LLM judge 与 fail-open，并补总开关即时更新。 |
| Local features | 10 类 `docs/features` 本地能力均保留或与上游组合；tracked feature 文件删除数为 0。 |
| Verification | Unit 50 个测试包通过；integration 44 个测试包通过，唯一 `openai_compat.test.exe` 被本机 360 阻断且已如实披露；合并差异 lint 0 issues；前端 176 files / 1214 tests、lint、typecheck、build 与 embed build 通过。提交前 staged 硬门为 274 files / `+32475 -1728`，无 unstaged/untracked/unmerged、精确冲突标记、whitespace、CRLF/BOM、敏感模式或 `docs/features` 删除。 |
| Approval / delivery | 用户在知悉环境限制后明确回复“允许”；仅创建本地提交，未 push、未创建 PR、未部署。 |

## 2026-07-17 main sync (v0.1.159; awaiting review)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10157_同步sub2api主线` |
| Upstream remote / branch | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git`; `main` |
| Base before merge / first parent | `b5c9726262ca65bddc8ca4bc5a35ed26e1208cb3` |
| Merge base | `d515c3045ce838976ebedab87846aaaf893dbbf6` |
| Upstream head / second parent | `c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc` |
| Merge commit | **待用户审核，尚未创建；`MERGE_HEAD=c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc`** |
| Upstream version / delta | `0.1.159`; 114 commits、399 files、`+28829/-1666` |
| Conflict files | `backend/cmd/server/wire_gen.go`; `backend/internal/server/http.go`; `backend/internal/server/router.go`; `backend/internal/server/routes/gateway.go`; `backend/internal/server/routes/gateway_test.go`; `backend/internal/service/openai_gateway_forward.go`; `frontend/src/api/admin/index.ts`; `frontend/src/components/layout/AppSidebar.vue`; `frontend/src/i18n/locales/en/admin/index.ts`; `frontend/src/i18n/locales/zh/admin/index.ts` |
| Conflict handling | Wire 从双方 provider 源图重新生成；server/router/gateway 组合上游 Audit、StepUp、SessionBinding、async image、billing 与本地 Prompt Metrics、RequestArchive/Intercept；OpenAI failover 采用上游 helper 并保留本地 413/header/thinking 语义；前端 API、侧栏和 locale key 做并集。 |
| Local features | RequestArchive/Intercept、Prompt Metrics/Risk、Token Analysis、组织用量、用户并发 preset/`ConcurrencyCacheError`、OpenAI-compatible options/cache usage、quota flusher 均保留；`docs/features/` tracked 删除数为 0。只有两个本地并发 preset 测试桩适配上游新增 `BatchUpdateLimits` 接口。 |
| Upstream issues | 异步图片 URL 下载缺 SSRF 防护、进程内任务无重启恢复、批量限额 cache/参数上限风险，以及 locale compile 测试未声明直接依赖。按用户要求仅记录，不修改对应生产代码或依赖清单，等待 upstream 修复。 |
| Verification | Wire 连续两次生成稳定，SHA-256 `92D6F616...12A61`；backend unit、增量 lint、embed build 通过；integration 的 5 个 360 启动拦截包均经 fresh 目录或系统临时目录独立执行通过；frontend lint/typecheck/build 通过，Vitest 188 files / 1300 tests passed、1 个上游清单 suite 无法收集；`go mod tidy -diff` 只报告 12 行继承旧校验项。收尾 fetch 确认 `upstream/main=MERGE_HEAD=c2c19a7cb`。 |
| Documentation | 更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`、组件 README、merge review 与 delivery 文档。 |
| Approval / delivery | staged 门禁为 416 files / `+29294 -1677`，无 unmerged/unstaged/untracked/marker/features 删除/未解析敏感模式。保持 `git merge --no-commit --no-ff` 并等待用户审核；不 commit、不 push、不创建 PR、不部署。 |

## 2026-07-17 main sync (v0.1.160; committed)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10160_合并1.160版本` |
| Upstream remote / branch | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git`; `main` |
| Base before merge / first parent | `5d5f157854b9a88cc57da1600095bb404b78ed45`（本地 `main`） |
| Merge base | `c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc` |
| Upstream head / second parent | `57914967cbb127ff715719c3879d881c10d75274` |
| Merge commit | `8a022e9756d9dab36a3963b1e023d77d6fce0c75`（第一父 `5d5f157854b9a88cc57da1600095bb404b78ed45`；第二父 `57914967cbb127ff715719c3879d881c10d75274`） |
| Upstream version / delta | `0.1.160`; 20 commits、133 files、`+19766/-113`。`v0.1.160` tag target 的 `VERSION` 仍为 `0.1.159`, 因此固定采用后续 version-sync commit。 |
| Conflict files | `backend/cmd/server/wire.go`; `backend/cmd/server/wire_gen.go`; `frontend/src/components/layout/AppSidebar.vue`; `frontend/src/router/index.ts` |
| Conflict handling | Wire 同时保留本地 Prompt Metrics、Token Analysis、用户并发 preset、quota flusher 与上游 Prompt Audit；前端采用上游 Security Audit 分组, 同时保留本地 Request Intercept 和全部本地 admin 路由。 |
| Local features | RequestArchive/Intercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、图片生成/支付、用户并发、compatible preset/options/cache usage、默认 reasoning effort、quota flusher 均保留或组合；`docs/features/` tracked 删除数为 0。为保留 judge 防递归合同, 安全审计 request 继续向 legacy moderation 传递内部签名头。 |
| Upstream issues | Prompt Audit Wire ProviderSet 缺少 admin service binding；source-freeze patch 自带 whitespace；migration 182 将完整 prompt 持久化到 event, 改变 migration 181 的早期隐私说明。按用户要求只记录, 不修改上游实现。 |
| Verification | Backend targeted、完整 unit、完整 integration 通过；全量 lint 28 条本地 main 既有债务, 增量 lint 0 issues；frontend lint/typecheck、194 files / 1330 tests、979-module build 通过；embed build 通过（149,248,512 bytes）；`go mod tidy -diff` 仅提示既有 Wire CLI 传递校验和。 |
| Documentation | 更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`；新增本次 merge review 并追加本记录。 |
| Approval / delivery | 提交前 staged 门禁为 143 files / `+20209 -118`；`HEAD=main@5d5f157`, `MERGE_HEAD=upstream/main@57914967c`。无 unstaged/untracked/unmerged/marker/features 删除；排除上游 source-freeze patch 后 diff check 退出 0。用户随后授权创建上述 merge commit；未 push、未创建 PR、未部署。 |
| Post-commit local main sync | 本地 `main` 通过 `git pull --ff-only` 从 `github/main` 快进到 `864bb85e80163b7b4601a272e3a92919b102cbba`，再以 `--no-commit --no-ff` 合回本分支。唯一冲突为 `llm-wiki/wiki/README.md`，处理为同时保留 v0.1.160 与子管理员知识条目；当前子管理员 migration、后端 DB 权限/路由白名单、前端路由/侧栏过滤均存在。按用户要求不重复执行测试，第二次 merge 保持未提交等待审核。 |

## 2026-07-19 main sync (v0.1.161; committed)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10161_合并1.161版本` |
| Upstream remote / branch | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git`; `main` |
| Base before merge / first parent | `332fdbd0b84619cfb1da6fcb57b65d4d9263b2e9`（本地 `main`） |
| Merge base | `57914967cbb127ff715719c3879d881c10d75274` |
| Upstream head / second parent | `d4b9797ff72024960a035cf22fdd8f213e149169` |
| Merge commit | `e3e6b52da43a5be351cf59089976759eebc28376`（第一父 `332fdbd0b84619cfb1da6fcb57b65d4d9263b2e9`；第二父 `d4b9797ff72024960a035cf22fdd8f213e149169`） |
| Upstream version / delta | `0.1.161`; 62 commits、257 files、`+13643/-1523`。`v0.1.161` tag commit `19149ca196e` 的 `VERSION` 仍为 `0.1.160`，因此固定采用后续 version-sync commit。 |
| Conflict files | `backend/cmd/server/wire_gen.go`; `backend/internal/server/routes/gateway.go`; `frontend/src/App.vue`; `frontend/src/components/account/CreateAccountModal.vue` |
| Conflict handling | Wire 重新生成并保留本地服务与上游 ingress/auth-cache 生命周期；gateway 保留 RequestArchive/RequestIntercept 并合入 text body limit、Grok video content；App 保留子管理员拒绝恢复并采用上游 branding helper；账号创建同时保留 compatible preset 与 upstream billing auto probe。 |
| Semantic overlap review | 对 merge base、本地 `main` 和固定 upstream 的 43 个双方修改文件逐一三方审查；未发现 4 个文本冲突之外还需修改的语义冲突。 |
| Local features | RequestArchive/Intercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、图片生成/支付、用户并发、compatible preset/options/cache usage、默认 reasoning effort、quota flusher、子管理员权限均保留或组合；`docs/features/` tracked 删除数为 0。 |
| Upstream behavior / issue boundary | 采用上游删除 API Key 明文 deleted-audit 归因与 hash/outbox 安全实现，不恢复旧测试。完整验证未发现需要在本分支修复的 upstream-native bug；本地 `main` 既有 `/auth/me` `admin_permissions: null` contract mismatch 只记录，不在本次合并修复。 |
| Verification | Backend targeted 通过；完整 unit 仅上述本地基线 contract 失败，完整 integration 退出 0；全量 lint 28 条既有债务，增量 lint 0 issues；`go mod tidy -diff` 仅提示 `main` 已有的 6 组 Wire CLI 校验和。Wire 连续生成稳定，SHA-256 `5F2930F8...3237AC`；frontend lint/typecheck、200 files / 1373 tests、981-module build 通过；embed build 通过（149,794,304 bytes）。 |
| Documentation | 更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `data-and-domain.md`, `security-and-reliability.md`、账号组件 README、执行计划与本次 merge review。 |
| Approval / delivery | 最终 staged snapshot 为 267 files / `+14033 -1540`；无 unmerged/unstaged/untracked/marker/whitespace/features 删除，收尾 fetch 确认 `upstream/main=MERGE_HEAD=d4b9797ff`。用户审核后明确接受本地 `main` 既有 `/auth/me` contract 失败暂不修复，并授权创建上述 merge commit；未 push、未创建 PR、未部署。 |

## 2026-07-20 main sync (v0.1.162; awaiting review)

| Item | Value |
|---|---|
| Integration branch | `feature/hy/10162_合并1.162版本` |
| Upstream remote / branch | `upstream` -> `https://github.com/Wei-Shaw/sub2api.git`; `main` |
| Base before merge / first parent | `e52b5c89d07ac058043de5adb983cad8750cab58`（本地 `main`） |
| Merge base | `d4b9797ff72024960a035cf22fdd8f213e149169` |
| Upstream head / second parent | `e625ce3b3b3b955b7c3afc93221f7c5f0ae55aa8` |
| Merge commit | **待用户审核，尚未创建；`MERGE_HEAD=e625ce3b3b3b955b7c3afc93221f7c5f0ae55aa8`** |
| Upstream version / delta | `0.1.162`; 114 commits、190 files、`+9841/-990`。`v0.1.162` tag `27f094e09` 的 `VERSION` 仍为 `0.1.161`, 因此固定采用后续 version-sync commit `e625ce3b3`。 |
| Conflict files | `backend/internal/server/routes/gateway_test.go` |
| Conflict handling | 采用上游 `newGatewayRoutesTestRouterWithConfig(cfg, platform...)` helper, 将本地 RequestArchive/RequestIntercept 路由测试迁入该 helper, 同时采用上游 Grok 根级/`v1` count-tokens 本地估算断言。未修改冲突之外的上游生产实现。 |
| Semantic overlap review | 对 merge base、本地 `main` 与固定 upstream 的 45 个双方修改文件逐一复核；除唯一文本冲突外未发现需要手工改写的语义冲突。功能重叠处采用上游实现, 本地独有调用链继续保留。 |
| Local features | 22 个 tracked `docs/features/` 文件零删除；RequestArchive/Intercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、图片生成/支付、用户并发 preset/`ConcurrencyCacheError`、OpenAI-compatible preset/options/cache usage、默认 reasoning effort、quota flusher 和子管理员权限均保留或与上游组合。 |
| Upstream behavior | 引入客户端 IP 兼容开关与自定义 header、异步生图对象存储后台热配置、Grok 本地 count-tokens/客户端工具缓存、Prompt Audit blocking-intent fail-closed、Codex manifest 401 账号隔离、Agent Identity Team 隔离、`UPDATE_GITHUB_TOKEN`、更新/回退 15 分钟请求 timeout、订阅分钟级到期展示和 SVG branding。无 schema/migration 变化。 |
| Issue boundary | Frontend Vitest 的 `admin.system.rollback.spec.ts` 两条断言未接受实现新增的 `{ timeout: 900000 }`; 实现和测试均与固定 upstream 完全一致, 作为 upstream-native test mismatch 记录且不修。本地 `main` 既有 `/auth/me` 响应多出 `admin_permissions:null` contract mismatch、全量 lint 28 项及 `go mod tidy -diff` 6 组旧 checksum 也不在本次冲突解决范围。 |
| Verification | Backend 完整 unit 为 51 包通过、1 包因上述本地 contract 失败；fresh `GOTMPDIR` 跳过该合同后 52 包通过, integration 47 包通过。全量 lint 28 项, `--new-from-rev=HEAD` 为 0 issues；module 文件与本地 `main` 一致。Frontend lint/typecheck 通过, Vitest 202/203 files、1383/1385 tests, production build 983 modules / 179 files / 5,840,458 bytes。Wire 重新生成无漂移（SHA-256 `81C4EFE8...665F2AF`）；fresh frontend 后 embed build 通过（114,522,112 bytes）。无 Windows file lock 或 Docker/Testcontainers 错误。 |
| Documentation | 更新 `llm-wiki/wiki/README.md`, `backend.md`, `frontend.md`, `ops.md`, `security-and-reliability.md` 并追加本记录；本轮无数据模型或 migration 变化, `data-and-domain.md` 无需修改。 |
| Approval / delivery | 最终 staged snapshot 为 196 files / `+9903 -1051`；0 unstaged、0 untracked、0 unmerged、0 新增冲突标记、0 whitespace error、0 `docs/features` 删除, 且 `upstream/main=MERGE_HEAD=e625ce3b3`。保持 `git merge --no-commit --no-ff`, 未 commit、未 push、未创建 PR、未部署, 等待用户审核。 |
