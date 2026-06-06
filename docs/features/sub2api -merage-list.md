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
