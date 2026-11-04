# Sub2API 0.1.146 上游同步记录

合并日期 2026-07-08 ｜ 工作分支 `feature/hy/0621_敏感词过滤` ｜ 上游 `Wei-Shaw/sub2api main@6f43986c376d76144cb39c7a562c179e19ac7439`（VERSION 0.1.146，merge base `a5638a4e5408`）｜ 合并提交 `ae11166ed13e3309121e982ef52a1e552ff8f967`（父 `62e6a2e99` + `6f43986c3`）｜ 状态：已经 PR #13（`587b7c1b6`，2026-07-08）合入 main

本文是该轮同步的唯一记录。原始审核材料 `docs/upstream-sync/reviews/sub2api-v0.1.146-merge-review-cn.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-08 main sync` 节（该节"合并提交：未提交"是写台账时的快照，实际合并提交见上）。

## 1. 范围

- 上游 180 commits，656 files（`+118779 / -100144`），本地版本 0.1.143 → 0.1.146。
- 11 个文本冲突文件。上游本轮做了大规模"纯移动拆分"重构（`d9e514f98` / `bb5d2e84a` / `50043b117` / `4d23ad4ba` 等）：`setting_handler.go`（3957→468 行）、`setting_service.go`（5471→263）、`openai_gateway_service.go`（4872→1095）等拆成领域文件，`i18n` 的 `en.ts` / `zh.ts` 拆成按域模块。这是本地新增内容最容易"被拆没"的区域，也是本轮复核重点。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/cmd/server/wire_gen.go` | 保留本地 `tokenAnalysisService`、`userConcurrencyPresetService`/`Runner`（注册为后台任务）、`RequestIntercept*`、`promptmetrics.ProviderSet`、`extension`，同时加入上游 `batchImageCleanupService`、`batchImageWorkerRuntime`。 |
| `backend/go.mod` | 采用上游 AWS SDK 升级（`aws-sdk-go-v2 v1.41.5`、`s3 v1.97.3`、`eventstream v1.7.8`）和 `tiktoken-go/tokenizer` 直接依赖；保留本地 `golang.org/x/sys`、`golang.org/x/text` 直接依赖、`smithy-go` 降为间接。`go.sum` 含全部上游版本。 |
| `backend/internal/handler/admin/setting_handler.go` | 接受上游拆分，RequestArchive settings DTO/handler 迁到 `setting_handler_runtime.go`；路由仍挂在 `routes/admin.go`。 |
| `backend/internal/service/setting_service.go` | 接受上游拆分，RequestArchive runtime cache/singleflight 迁到新文件 `setting_request_archive.go`（无重复定义）。 |
| `backend/internal/handler/endpoint.go` | 本地 `/chat/completions`、裸 `/responses`、`/backend-api/codex/responses` 别名被上游更完整的 `EndpointResponsesCompact` 拆分方案吸收；保留根级 Responses alias，`/responses/compact` 采用上游独立归一化。 |
| `backend/internal/service/openai_gateway_service.go` | 接受上游拆分；`sanitizeOpenAIResponsesOfficialRequestBody` 等迁到 `openai_gateway_request_body.go`，`applyOpenAICompatibleCacheUsageFromJSON` 进入各 CC 管线文件；本地 `zeroCostBreakdown` / `usageCostTotal` 改名或内联为 `openAIUsageZeroCostBreakdown` + `cost.TotalCost`（`openai_gateway_usage.go`），保留本地图片计费模式特殊逻辑。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 保留 `shouldDropOpenAICompatibleResponsesTemperature`、`openai_compat_drop_temperature` 和 Responses fallback usage 写回。 |
| `backend/internal/service/concurrency_service_test.go` | 同时保留本地 `TestAcquireUserSlot_CacheError` / `TestAcquireAccountSlot_CacheError` 与上游 API key slot 测试。 |
| `frontend/src/components/layout/AppSidebar.vue` | 同时保留本地 `/image-gen` 与上游 `/batch-image` 菜单。 |
| `frontend/src/i18n/locales/en.ts` / `zh.ts` | 接受上游域模块拆分并删除旧单文件；本地 `nav.imageGeneration` 迁入 `common.ts`，本地 tokenAnalysis / requestIntercept / promptMetrics 文案拆成独立模块（`en|zh/admin/{tokenAnalysis,requestIntercept,promptMetrics}.ts`）。 |

### i18n 拆分中的本地文案丢失及修复

提交前复核发现后端零丢失，但前端 i18n 拆分迁移有真实丢失；以下问题均已在合并提交中修复（已对 `ae11166ed` 核实文件存在、键可检索）：

| 问题 | 影响 | 修复 |
| --- | --- | --- |
| 6 个新 i18n 模块（en/zh × tokenAnalysis/requestIntercept/promptMetrics）和 `frontend/src/i18n/__tests__/adminMergeKeys.spec.ts` 未加入暂存区，而已暂存的 `admin/index.ts` 已 import 它们 | 直接提交会漏文件，前端构建失败、三项功能文案全丢 | 全部纳入提交 |
| `admin.riskControl.promptRisk` 整个子树（约 57 个配置键 + `judge` 子树约 16 键）及 4 个兄弟键 `tabs.promptRisk`、`result.observe`、`action.promptRiskBlock`、`action.promptRiskObserve` 丢失 | `PromptRiskPanel.vue` 73 处引用落空，敏感词/Prompt 风险面板显示原始 key | 从旧 `en.ts` / `zh.ts` 取回，补入拆分后的 `riskControl` 结构（`en|zh/admin/channels.ts` + `settings.ts`） |
| `resetDailyQuota`、`resetDailyQuotaTitle`、`resetDailyQuotaConfirm`、`resetDailyQuotaDesc` 丢失（上游同对象 `resetQuota*` 保留，本地追加的兄弟键被丢） | `SubscriptionsView.vue` 重置日限按钮文案破损 | 补回 `en|zh/admin/accounts.ts` |
| `accounts.openai.compatibleProvider` / `compatibleProviderHint` 丢失 | `CreateAccountModal.vue` OpenAI 兼容服务商文案破损 | 补回 `en|zh/admin/accounts.ts` 的 `openai` 对象 |

`requestArchive.maxRequestBodyHint` 曾疑似丢失，核实为 EN 值破折号 `—` 规范成 `-`，非丢键。

教训：`vue-tsc` 和现有 i18n 守卫测试对缺键无感（vue-i18n 运行期回显 key），拆分类合并必须对本地组件引用的 key 做存在性检索，并检查 `git status` 中 `frontend/src/i18n` 无 `??` 文件。

## 3. 保留的本地能力与语义适配

逐一提取本地在冲突文件中新增的函数/类型/常量，在合并后全树检索存续；`base..HEAD` 本地新增文件（`prompt_risk*.go`、`token_analysis_*.go`、`promptmetrics/*`、`user_concurrency_preset*.go`、`request_archive*.go` 及前端视图/store/api）缺失计数为 0。

| 能力 | 状态 |
| --- | --- |
| Prompt Risk / 敏感词过滤与 LLM judge | 保留，风险控制入口与文案完整 |
| RequestArchive / RequestIntercept | 保留，setting runtime、service cache 与路由中间件语义不变 |
| Token Analysis | 保留，用户排行按用户聚合、隐藏 key 的局部改动继续存在 |
| 图片生成 | 保留，`/image-gen` 与上游 `/batch-image` 并存 |
| 用户并发方案 | 保留，preset runner 与 cache-error 语义测试 |
| OpenAI compatible cache usage | 保留，Chat/Responses fallback 继续识别 `cache_read_input_tokens` / `cached_tokens` / `prompt_cache_hit_tokens` |

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| Batch image | batch image 任务/事件/下载/结算/清理/worker runtime，路由 `/v1/images/batches*`，用户侧 `/batch-image` 指引页 |
| 后端拆分 | `setting_service.go`、`setting_handler.go`、`admin_service.go`、`gateway_service.go`、`openai_gateway_service.go`、`openai_ws_forwarder.go`、`usage_log_repo.go` 等按领域拆文件 |
| 网关 | shared CC pipeline 拆分、compact body-signal 路由修复；OpenAI official payload sanitizer 删除顶层 `thinking`、保留 `reasoning` |
| 调度/管理 | 账号 scheduler score 改为前端列可见时 opt-in 请求，避免默认列表触发高成本计算 |
| 前端 | i18n 域模块拆分并增加 key collision 测试 |

## 5. 验证

| 命令（`backend` 目录，Go 缓存指向仓库内目录） | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./internal/service` | 通过（95s） |
| `go test -tags=unit -p 1 -count=1 ./cmd/server ./internal/handler ./internal/server/routes` | 通过 |
| `go test -tags=unit ... -run 'TestOpenAIGatewayService_ResponsesCompatPreservesCompatibleCacheUsage'`、`'Test(ParseSSEUsage_CompatibleCacheFieldsFallback|ExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback|OpenAIGatewayService_APIKeyPassthrough_StripsTopLevelThinkingAndKeepsReasoning)$'` | 通过 |
| `go test -tags unit ./... -run '^$'`（编译全部测试） | 仅 `logger`、`proxyutil` 因 Windows 测试 `.exe` 被占用报错，属环境问题 |
| `pnpm --dir frontend run typecheck` | 通过 |
| 前端 i18n 守卫测试（5 files / 15 tests） | 通过 |
| `git diff --name-only --diff-filter=U`、冲突标记检索、`git diff --check` | 无未解决文件、无冲突标记 |

## 6. 遗留问题

- 复核只覆盖冲突解决正确性与本地内容存续，未对上游新功能本身做代码审查。
- 复核建议补一条守卫用例，断言 `PromptRiskPanel.vue` / `SubscriptionsView.vue` / `CreateAccountModal.vue` 引用的关键 key 在 en/zh 中存在；合并提交包含 `adminMergeKeys.spec.ts`，其覆盖范围未在材料中记录。
