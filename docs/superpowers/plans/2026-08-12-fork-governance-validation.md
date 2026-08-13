# Fork 治理方案 A 验证与实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不修改业务行为的前提下建立 TD-009 阶段 0 合同基线，实际运行五组验证并记录缺口，再为方案 A 的分阶段结构实施建立可拒绝、可回滚的评审 Gate。

**Architecture:** 本轮只验证现有 Gateway、Settings、Runtime、Admin/Frontend、Compat/Safety 合同，不创建动态插件或提前迁移生产代码。后续结构实施按 `Gateway Extension -> LocalRuntime -> Admin Extension + Frontend Manifest -> Settings Extension -> Compat/Safety 归属收口` 顺序进行，每阶段以相同合同包保护行为。

**Tech Stack:** Go 1.26.5、Gin、Google Wire、Vue 3、TypeScript 5.6、Vitest 2、PowerShell、Git exact-SHA workflow。

## Global Constraints

- 工作分支固定为 `feature/hy/10177_fork_governance_validation`，起点固定为 `github/main@8784a4084268b532ab653774c0dc3999e24ff7c9`。
- 当前文档与 Grok 图谱改动全部保留；不得还原或覆盖 `llm-wiki/.understand-anything/*` 的既有修改。
- 阶段 0 不迁移生产结构；补测发现的生命周期行为缺陷按 RED -> GREEN 做最小加固，不创建扩展目录或改变 Wire 所有权。
- 不在 upstream merge PR 中实施治理重构；不顺手修上游或既有基线问题。
- `CPR=100%`、`PRF=0`、不手改 `wire_gen.go`、环境跳过不算通过。
- 当前五组测试只能证明阶段 0 基线与缺口，不能证明方案 A 已降低合并成本。

---

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `docs/features/upstream-fork-governance-design-cn.md` | 目标架构、五类接缝、迁移和评审 Gate |
| `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md` | v1.1 指标、样本规则、A/B 和最终门槛 |
| `docs/superpowers/plans/2026-08-12-fork-governance-validation.md` | 本轮与后续实施的任务顺序、命令和判定标准 |
| `docs/features/upstream-fork-governance-validation-report-cn.md` | 本轮五组验证的 exact SHA、命令、结果、合同覆盖和缺口 |
| `backend/internal/server/routes/gateway_test.go` | `GW-*` 当前真实 ingress 行为 |
| `backend/internal/handler/admin/setting_handler_*_test.go` | `SET-*` omitted/explicit-empty 行为 |
| `backend/cmd/server/wire_gen_test.go`、`backend/internal/service/*_test.go` | `LIFE-*` 当前 cleanup 与单体 runtime 行为 |
| `backend/internal/service/admin_permission_test.go`、`backend/internal/server/middleware/admin_auth_test.go` | 后端权限最终授权合同 |
| `frontend/src/router/__tests__/*`、`frontend/src/utils/__tests__/adminPermissions.spec.ts`、`AppSidebar.spec.ts` | 前端路由、landing 和导航合同 |
| `backend/internal/service/openai_gateway_responses_compat_test.go`、`openai_ws_v2_passthrough_adapter_effort_test.go`、`prompt_risk_judge_test.go` | `COMP-*` 与 `SAFE-*` 行为 |

---

### Task 1: 冻结分支和验证环境

**Files:**
- Modify: `docs/features/upstream-fork-governance-validation-report-cn.md`

**Interfaces:**
- Consumes: `github/main@8784a4084268b532ab653774c0dc3999e24ff7c9`
- Produces: 可复现的 branch、HEAD、Go、Node、pnpm 和测试缓存元数据

- [x] **Step 1: 验证分支没有混入旧 hotfix 提交**

Run:

```powershell
git branch --show-current
git rev-parse HEAD
git rev-list --left-right --count github/main...HEAD
```

Expected: 分支为 `feature/hy/10177_fork_governance_validation`；HEAD 等于固定 main SHA；ahead/behind 为 `0 0`。

- [x] **Step 2: 记录工具链**

Run:

```powershell
go version
node --version
pnpm --version
git --version
```

Expected: 命令均可执行；实际版本写入验证报告，不用文档预设值冒充实测值。

- [x] **Step 3: 建立仓库约定的隔离 Go cache**

Run:

```powershell
$backend = (Resolve-Path backend).Path
$cacheRoot = Join-Path $backend '.gocache'
$env:GOCACHE = Join-Path $cacheRoot 'review-cache'
$env:GOPATH = Join-Path $cacheRoot 'review-gopath'
$env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
$env:GOTMPDIR = Join-Path $cacheRoot 'run-tmp-fork-baseline'
New-Item -ItemType Directory -Force $env:GOCACHE,$env:GOMODCACHE,$env:GOTMPDIR | Out-Null
git check-ignore backend/.gocache/review-cache
```

Expected: `go env GOVERSION` 为 `go1.26.5`；cache 路径位于工作区且被 Git ignore。每组测试换新的 `backend/.gocache/run-tmp-fork-*`，统一使用 `-p 1 -count=1`，避免 Windows `.test.exe` 锁污染结论。

---

### Task 2: 验证 Gateway ingress 合同

**Files:**
- Test: `backend/internal/server/routes/gateway_test.go`

**Interfaces:**
- Consumes: 当前 `RegisterGatewayRoutes` 和真实 middleware
- Produces: `GW-ARCHIVE-01`、`GW-ORDER-01`、`GW-GUARD-01`、`GW-ROUTES-01` 证据

- [x] **Step 1: 运行 Gateway 定向合同**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./internal/server/routes -run 'TestGatewayRoutes(RequestArchiveRunsForOpenAIResponsesAlias|RequestInterceptRunsAfterArchive|GeminiModelActionGuardRunsBeforeRequestIntercept|ResponsesSubpathRejectsNonConformingSubpaths|ResponsesSubpathGuardRunsBeforeRequestIntercept|OpenAIResponsesCompactPathIsRegistered|OpenAIAlphaSearchPathsAreRegistered|OpenAIImagesPathsAreRegistered|AsyncImagesPathsAreRegistered|GrokImagesAndVideosPathsAreRegistered)$' -v
```

Expected: 所列测试全部 PASS；任何漏跑或 `no tests to run` 均判无效。

- [x] **Step 2: 记录合同映射**

Expected mapping:

| 合同 | 证据测试 |
| --- | --- |
| `GW-ARCHIVE-01` | `RequestArchiveRunsForOpenAIResponsesAlias` |
| `GW-ORDER-01` | `RequestInterceptRunsAfterArchive` |
| `GW-GUARD-01` | Gemini/Responses guard-before-intercept 两项 |
| `GW-ROUTES-01` | compact、alpha search、images、Grok media 路由集 |

---

### Task 3: 验证 Settings omitted / explicit-empty 合同

**Files:**
- Test: `backend/internal/handler/admin/setting_handler_partial_payload_test.go`
- Test: `backend/internal/handler/admin/setting_handler_stepup_switch_test.go`
- Test: `backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`

**Interfaces:**
- Consumes: 通用 `PUT /api/v1/admin/settings` 当前 field-presence 实现
- Produces: `SET-PATCH-01`、`SET-EMPTY-01` 证据

- [x] **Step 1: 运行 Settings 定向合同**

Run from `backend`:

```powershell
go test -tags unit -p 1 -count=1 ./internal/handler/admin -run '^(TestUpdateSettingsPartialPayloadKeepsUnsentKeys|TestUpdateSettingsFullPayloadStillClearsSentEmptyFields|TestUpdateSettingsForwardedClientIPHeadersOmittedPreservesAndEmptyClears|TestSettingHandler_UpdateSettings_PreservesOmittedAuthSourceDefaults)$' -v
```

Expected: 四项全部 PASS；同时证明 omitted 保留和显式空值清除，不能只测其中一侧。`setting_handler_partial_payload_test.go` 带 `unit` build tag，缺少 `-tags unit` 会静默漏跑其中两项。

---

### Task 4: 补齐当前 Runtime 生命周期合同

**Files:**
- Test: `backend/cmd/server/wire_gen_test.go`
- Test: `backend/internal/service/token_analysis_auto_index_test.go`
- Test: `backend/internal/service/user_platform_quota_flusher_test.go`
- Test: `backend/internal/service/user_concurrency_preset_runner_test.go`（若不存在则记录缺口，不伪造 PASS）

**Interfaces:**
- Consumes: 当前 `provideCleanup` 和三个本地 runtime 对象
- Produces: `LIFE-START-01`、`LIFE-STOP-01` 当前行为证据

- [x] **Step 1: 运行 cleanup 最小依赖测试**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./cmd/server -run '^TestProvideCleanup_WithMinimalDependencies_NoPanic$' -v
```

- [x] **Step 2: 运行本地 Stop/Flush 行为测试**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./internal/service -run '^(TestTokenAnalysisAutoIndexStopCancelsRunningRound|TestFlusher_NilSafe|TestFlusher_StopPreventsFlush)$' -v
```

Expected: 四项现有测试全部 PASS。

- [x] **Step 3: 对覆盖结论做降级判定**

Expected: 如果没有可观察地证明 preset runner 创建/启动、quota 最终 flush、prompt metrics 停止及 Redis/Ent 关闭顺序，则 `LIFE-START-01` 和 `LIFE-STOP-01` 只能标 `partial`，阶段 0 `CPR` 不得写 100%。

- [x] **Step 4: RED -> GREEN 补齐三个合同缺口**

实际补齐：Gateway 三 alias 归档；Token Analysis/preset/quota/Prompt Metrics 启用、禁用及重复启动；quota 最终 flush；重复 cleanup 幂等；Redis/Ent 关闭顺序。RED 暴露的生命周期缺陷只做最小加固。目标 `backend/internal/localext/runtime.NewRuntime` 测试移到 Gateway Extension 通过后的第二结构阶段，不作为阶段 0 的循环前置条件。

---

### Task 5: 验证 Admin / Frontend 权限与入口合同

**Files:**
- Test: `backend/internal/service/admin_permission_test.go`
- Test: `backend/internal/server/middleware/admin_auth_test.go`
- Test: `frontend/src/router/__tests__/subAdminRoutes.spec.ts`
- Test: `frontend/src/router/__tests__/organizationUsageRoute.spec.ts`
- Test: `frontend/src/utils/__tests__/adminPermissions.spec.ts`
- Test: `frontend/src/components/layout/__tests__/AppSidebar.spec.ts`
- Test: `frontend/src/api/__tests__/admin.tokenAnalysis.spec.ts`
- Test: `frontend/src/api/__tests__/admin.requestIntercept.spec.ts`

**Interfaces:**
- Consumes: 后端 fail-closed 权限矩阵和前端路由/菜单/API client
- Produces: `AUTH-MATRIX-01`、`EXT-ADMIN-01` 跨端证据

- [x] **Step 1: 运行后端权限合同**

Run from `backend`:

```powershell
go test -tags unit -p 1 -count=1 ./internal/service -run '^(TestNormalizeAdminPermissions|TestCanAccessAdminRoute|TestCanAccessAdmin|TestSubAdminWriteWhitelistStaysNarrow)$' -v
go test -tags unit -p 1 -count=1 ./internal/server/middleware -run '^(TestAdminAuthSubAdminUsesLatestDatabasePermissions|TestAdminAuthAdminAPIKeyBypassesSubAdminRouteCatalog)$' -v
```

Expected: 权限矩阵、未知路由 fail-closed、写操作白名单、数据库最新权限和 admin API key 边界均 PASS。执行前可用 `go test -tags unit -list` 核对真实测试名；不存在的测试名或 `no tests to run` 均不得计入结果。

- [x] **Step 2: 运行本地管理能力 handler 合同**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./internal/handler/admin -run '^(TestUserConcurrencyPresetHandlerCreateAndApply|TestOrganizationUsageHandlerSummary_BindsStrictQueryAndReturnsEnvelope|TestTokenAnalysisHandlerSummaryParsesDateRange|TestRequestInterceptHandlerSaveListAndTest)$' -v
```

Expected: 并发 preset、组织用量、Token Analysis 和 Request Intercept 四类入口均执行真实 handler 合同并 PASS。

- [x] **Step 3: 运行前端路由、landing、侧栏和 API 合同**

Run from `frontend`:

```powershell
cmd.exe /c node_modules\.bin\vitest.cmd run src/router/__tests__/subAdminRoutes.spec.ts src/router/__tests__/organizationUsageRoute.spec.ts src/utils/__tests__/adminPermissions.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts src/api/__tests__/admin.tokenAnalysis.spec.ts src/api/__tests__/admin.requestIntercept.spec.ts
```

Expected: 六个文件全部 PASS，且输出显示实际执行测试数。当前 Windows/Codex 环境中 `pnpm exec vitest` 未解析本地 shim，因此使用仓库现有 `node_modules/.bin/vitest.cmd`；不安装或变更依赖。

---

### Task 6: 验证 Compat / Safety 合同

**Files:**
- Test: `backend/internal/service/openai_gateway_responses_compat_test.go`
- Test: `backend/internal/service/openai_ws_v2_passthrough_adapter_effort_test.go`
- Test: `backend/internal/service/prompt_risk_judge_test.go`
- Test: `backend/internal/securityaudit/coordinator_test.go`
- Test: `backend/internal/service/openai_default_reasoning_effort_test.go`
- Test: `backend/internal/service/openai_reasoning_effort_policy_test.go`
- Test: `backend/internal/service/openai_oauth_passthrough_test.go`
- Test: `backend/internal/service/openai_responses_lite_tools_test.go`

**Interfaces:**
- Consumes: compatible cache usage、WS reasoning effort 和 Prompt Risk judge
- Produces: `COMP-USAGE-01`、`COMP-REASON-01`、`SAFE-ORDER-01`、`SAFE-FAIL-01` 证据

- [x] **Step 1: 运行默认 build-tag 合同**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./internal/service -run '^(TestOpenAIGatewayService_ResponsesCompatPreservesCompatibleCacheUsage|TestPromptRiskJudge_FailOpenOnUnreachable|TestRunPromptRiskJudge_(ConcurrencyLimitFailOpen|SuccessBodyTooLargeFailOpen|Non2xxFailOpen|TimeoutFailOpen|BadJSONFailOpen))$' -v
```

- [x] **Step 2: 运行 Prompt 安全链顺序合同**

Run from `backend`:

```powershell
go test -p 1 -count=1 ./internal/securityaudit -run '^(TestCoordinatorModesAndPriority|TestCoordinatorBlockingPriorityCoversBothEngineDecisionMatrix|TestCoordinatorAsyncEnqueueFailuresNeverChangeResponseOrDownstreamDispatch)$' -v
go test -p 1 -count=1 ./internal/service -run '^(TestPromptRiskJudge_(FusionDowngradesOnNone|FusionKeepsBlockOnHigh|ExemptionShortCircuits|ContextInFlightSkips|InternalHTTPRequestSkipsPromptRiskStage|InternalHeaderIgnoredWhenJudgeDisabled|DisabledNoOp)|TestContentModerationCheck_(PreBlockKeywordHitSkipsUpstreamCall|KeywordsIgnoredInObserveMode))$' -v
```

Expected: Prompt Risk/judge 在 Content Moderation 前置阶段的 block/observe/fail-open 边界稳定；Prompt Audit 的 off/async/blocking 模式和 legacy 优先级矩阵稳定。

- [x] **Step 3: 运行跨路径 reasoning 合同**

Run from `backend`:

```powershell
go test -tags unit -p 1 -count=1 ./internal/service ./internal/handler -run '^(TestApplyDefaultOpenAIReasoningEffort|TestApplyOpenAIReasoningEffortPolicy|TestOpenAIReasoningEffortPolicyForCompositeTarget|TestForwardAsRawChatCompletions_PreservesMappedGPT56MaxEffort|TestOpenAIGatewayService_APIKeyPassthrough_StripsTopLevelThinkingAndKeepsReasoning|TestOpenAIGatewayServiceForward_NormalizesResponsesLiteToolsForOAuth|TestWSPassthroughUsageMeta_(InitFromFirstFrame_MappedModelCandidate|InitFromFirstFrame_NonGPT56FallsBackToXHigh|UpdateFromResponseCreate_MappedModelCandidate))$' -v
```

Expected: cache usage 保留 read/create 两桶；reasoning 默认值不覆盖显式值，策略上限按合同生效，API Key Chat/Responses、OAuth Responses 和 WS 多轮路径保持既定优先级；judge 故障均 fail-open。

---

### Task 7: 汇总阶段 0 结果并决定下一 Gate

**Files:**
- Create: `docs/features/upstream-fork-governance-validation-report-cn.md`
- Modify: `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md`
- Modify: `llm-wiki/wiki/README.md`

**Interfaces:**
- Consumes: Tasks 1-6 的当前命令输出
- Produces: 当前 `CPR`、合同状态、缺口和是否允许进入 Gateway Extension RED 测试

- [x] **Step 1: 逐合同填写状态**

只允许 `pass / fail / partial / unknown / not-run`。`partial` 不计入 CPR 分子；`unknown/not-run` 不视为通过。

- [x] **Step 2: 计算当前 CPR**

```text
CPR = pass 合同数 / 14
```

同时列出测试数和命令；不得把多条测试等同于多份合同。

- [x] **Step 3: Gate 判定**

| 结果 | 下一步 |
| --- | --- |
| 14/14 pass | 可进入 Gateway Extension 的 RED 测试与最小实现 |
| 任一 partial/unknown | 先补阶段 0 合同缺口，不改生产结构 |
| 任一 fail | 先定位是基线缺陷还是合同定义错误，单独处理 |

- [x] **Step 4: 文档验证**

Run:

```powershell
git diff --check
tools\check-understand-status.cmd
```

Expected: diff check PASS；Understand 在 wiki 未提交/未刷新时可以是 `PARTIAL`，但必须记录具体检查项，不得写 READY。

---

## 后续结构实施 Gate

阶段 0 已以 `97/97` 顶层测试、`CPR=14/14` 完成。后续按以下顺序分别建立新计划或继续本计划；每个阶段都执行 RED -> GREEN -> 全合同回归：

| 阶段 | 首个 RED | 最小实现 | 阶段证据 |
| --- | --- | --- | --- |
| Gateway Extension | 调用目标 `NewExtension(...).Handlers(coreGuards...)`，当前因接口不存在失败 | 固定 `Archive -> coreGuards -> Intercept` | `GW-*` + routes package |
| LocalRuntime | 构造目标 `NewRuntime(...)`，当前因接口不存在失败 | strong-typed members、`sync.Once`、明确 Stop 顺序 | `LIFE-*` + Wire 生成无漂移 |
| Admin Extension | 调用 `LocalAdmin.Register(admin)`，当前因聚合不存在失败 | 一次注册本地 handlers，API path 不变 | `AUTH-*`、`EXT-*` + frontend |
| Frontend Manifest | 从 manifest 生成 route/nav，当前因 manifest 不存在失败 | 静态 feature list + icon key 映射 | router/sidebar/landing tests |
| Settings Extension | namespaced 与 legacy adapter 等价测试先失败 | 单一 typed domain patch | `SET-*` + API contract |
| Compat/Safety 归属收口 | 对迁移后 package 的外部行为测试先失败 | 移动纯函数和显式调用点 | `COMP-*`、`SAFE-*` |
| A/B | 同一代表性 exact SHA 两个隔离 worktree | 无额外生产改动 | `SDP/SDR/RSD/ARM/AMR/CPR/PRF` |

只有 A/B 为 `effective`，才进入连续三轮真实同步；只有三轮满足 v1.1 最终门槛，才把 TD-009 改为 `mitigated`。
