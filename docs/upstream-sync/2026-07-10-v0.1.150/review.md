# Sub2API 0.1.150 上游同步记录

合并日期 2026-07-10 ｜ 工作分支 `feature/hy/0621_敏感词过滤` ｜ 上游 `Wei-Shaw/sub2api main@9a2f11b4e21763cb7003ea29921d9a672ab50b1f`（VERSION 0.1.150，merge base `ddb1a210ce67`，即上一轮 0.1.149 边界）｜ 合并提交 `61fec21ade8c1214f2426158ea698b8f2f5f1e9a`（父 `4b1bb6cb0960` + `9a2f11b4e217`），修复提交 `9f39cd007` ｜ 状态：已经 PR #15（`faf8379b7`，2026-07-10）合入 main

本文是该轮同步的唯一记录。原始审核材料 `docs/upstream-sync/reviews/2026-07-10-upstream-9a2f11b-merge-review.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-10 main sync follow-up (v0.1.150)` 节。

## 1. 范围

- 0.1.149 全量验证期间上游前进，本轮是同一分支上的增量合并：13 commits、32 files（`+703 / -172`），Go 工具链保持 `1.26.5`。
- 主要上游提交：`4a2b10c94` / `383f61d0e` / `062af81fb`（GPT-5.6 cache write 计费与官方价格、保留显式价格）、`0a5f34a2e`（Windows websocket reset 识别）、`b9b013a08`（WS passthrough effort 候选）、`dda8f7873` / `ea9f40b63`（admin user breakdown `request_type`）、`0dec1ad29`（`isOpenAIGPT56Model` 去重到 `openai_model_alias.go`）、`9a2f11b4e`（VERSION）。
- 2 个文本冲突；另有 7 个双方修改路径：`apicompat/chatcompletions_responses_bridge.go`、`apicompat/types.go`、`openai_gateway_messages.go`、`openai_gateway_request_body.go`、`openai_gateway_response_handling.go`、`openai_gateway_service_test.go`、`openai_gateway_usage.go`。
- 同日随后在同一分支以 `5c13b077d` 再合入上游 `6dd3274aafbc`（PR #3948 GPT-5.6 billing usage integrity，7 commits，VERSION 仍为 0.1.150）；该次合并在台账中没有独立条目，0.1.151 台账以 `6dd3274aafbc` 为 merge base。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/internal/pkg/apicompat/chatcompletions_responses_test.go` | 双边保留：本地 `TestChatCompletionsToResponses_PromptCacheKey` 与上游 `TestUsageConversionsPreserveCacheWriteTokens`。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | `extractCCStreamUsage` 采用上游 `openAIUsageFromGJSON` 统一解析 input/output/cache read/cache write，再调用本地 `applyOpenAICompatibleCacheUsageFromJSON` 补第三方字段（`prompt_cache_hit_tokens`、顶层 `cache_read_input_tokens` / `cache_creation_input_tokens`、Anthropic 5m/1h 明细）。 |

### 语义冲突：本地 helper 把上游 cache write 清零（已修复）

合并提交 `61fec21ad` 文本上"两边都保留"，但语义互相覆盖：本地 helper 只认顶层 `cache_creation_input_tokens` 和 ephemeral 5m/1h，不认上游新增的嵌套 `input_tokens_details.cache_write_tokens` / `prompt_tokens_details.cache_write_tokens` / `cache_write_tokens` / `cache_write_input_tokens`，且对 `CacheCreationInputTokens`、`CacheReadInputTokens` 直接赋值，会用 0 覆盖 `openAIUsageFromGJSON` 已解析出的正值。

- 影响入口：`extractCCStreamUsage`（冲突文件）和更关键的统一入口 `extractOpenAIUsageFromJSONBytes`（非流式/SSE usage 提取与计费）。返回嵌套 `cache_write_tokens` 的上游会少记 cache creation，GPT-5.6 cache write 计费（本轮主功能）和看板偏差；仅顶层字段或仅 `prompt_cache_hit_tokens` 的兼容上游不受影响；apicompat 的 Chat↔Responses 转换本身正确。
- 证据：合并提交上 `TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes` 失败（`expected 4, actual 0`），`..._CompatibleCacheFieldsFallback` 通过。
- 修复（`9f39cd007 fix: preserve cache write usage after upstream merge`，`backend/internal/service/openai_gateway_response_handling.go`）：helper 复用上游 `openAICacheCreationTokensFromUsage` / `openAICacheReadTokensFromUsage`，ephemeral 5m/1h 只作 cache creation 回退，只在解析结果 `> 0` 时写回；`prompt_cache_hit_tokens` 保留在 cache read 候选；补 compatible cache read 时原位更新 Responses/Chat details，避免替换对象丢失已有 cache write。当前源码 `applyOpenAICompatibleCacheUsageFromJSON` 仍是这一形态。

原则（已写入 playbook）：本地兼容 helper 与上游统一解析器叠加时，helper 只能补缺，不能无条件覆盖上游已解析的正值。

## 3. 保留的本地能力与语义适配

| 能力 | 状态 |
| --- | --- |
| Prompt Risk / 敏感词过滤与 LLM judge | 保留，路由、service、judge fail-open、i18n 与测试未被覆盖 |
| RequestArchive / RequestIntercept | 保留，网关中间件、运行时设置和管理路由 |
| Token Analysis | 保留，admin route、handler/service/repository 与索引入口 |
| 图片生成 | 保留，`/image-gen` 与 `/batch-image` 并存 |
| 用户并发方案 | 保留，preset route/service/runner 与缓存失效逻辑 |
| OpenAI compatible cache usage | 保留并与上游 cache write 统一：`cache_read_input_tokens` / `cached_tokens` / `prompt_cache_hit_tokens` 与 `cache_write_tokens` / `cache_creation_input_tokens` 同时解析 |

双边长行存续扫描：apicompat bridge/types 的 cache write 映射、billing/pricing GPT-5.6 cache write、WS passthrough effort 候选、`isOpenAIGPT56Model` 去重、本地 `prompt_cache_hit` 路径均在。

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| GPT-5.6 计费 | cache write token 解析、官方 cache write 价格、渠道显式价格保留和用量统计；普通 input / cache read / cache write 改为互斥计费桶 |
| OpenAI WS | WS passthrough reasoning effort 候选 `upstreamModel -> billingModel -> originalModel`；Windows connection reset/abort 识别为网络错误 |
| 管理用量 | `GetUserBreakdown` 使用 `ParseUsageRequestType`，前端 `UserBreakdownParams.request_type` 改为 `UsageRequestType` |
| 可靠性 | 稳定 expired lock reconciliation integration test |

## 5. 验证

| 命令（`backend` 目录，仓库内 Go 缓存 + 全新 `GOTMPDIR`） | 结果 |
| --- | --- |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat` | 通过 |
| `go test -tags=unit -count=1 ./internal/service -run "TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes|TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback|StreamPreservesCompatibleCacheUsage|TestParseSSEUsage_CompatibleCacheFieldsFallback"` | 修复后通过 |
| `go test -tags=unit -count=1 ./internal/pkg/apicompat -run "UsageConversionsPreserveCacheWrite|ChatUsageToResponses|PromptCacheKey"` | 通过 |
| Responses / Chat cache details 保留回归 + compatible cache 专项 | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 最终代码完整通过（`internal/service` 95.8s） |
| `go test -tags=integration -p 1 -count=1 ./...` | 最终代码完整通过（`internal/service` 56.1s） |
| `pnpm --dir frontend run lint:check`、`run typecheck` | 通过 |
| `pnpm --dir frontend exec vitest run` | 151 files / 955 tests 通过 |
| 冲突标记、未解决文件、`git diff --check` | 通过 |

Windows 首轮 unit/integration 偶发 `.test.exe` `Access is denied` / file-in-use，按 `llm-wiki/wiki/ops.md` 换全新 `GOTMPDIR` 串行重跑后通过，未改业务代码。
