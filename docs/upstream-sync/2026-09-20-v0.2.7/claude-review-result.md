# Sub2API 0.2.7 Claude 独立复审结果

日期：2026-09-20。按 `claude-review-request.md` 只读审查当前暂存合并候选。未修改源码、索引、测试或运行配置；未 commit / push / 部署。

## 结论

**GO。** 在固定 SHA、仅解决冲突、按用户选择移除 Codex ticket 的范围内，未发现阻断本次冲突合并的缺陷。

GO 只覆盖已验证范围，不等于上游没有 bug、不等于数据库集成全覆盖，也不构成 commit、push 或上线授权。

| 基线 | 现场值 |
| --- | --- |
| 工作分支 | `feature/hy/10207_merge_sub2api_207` |
| HEAD / 第一父 / main | `de5a3e383cd8eb197c1a83f12a71fb04d9e4e049` |
| MERGE_HEAD | `fbb9006adef852c46f0c7f18b0a8a740722cfac7` |
| merge base | `efe9aab1e4ec89a42ba45e8dac20e882c5409a6a` |
| VERSION | `0.2.7` |
| 无 unmerged / unstaged | 是 |
| 代码子树 | backend `4e25d22da54a7c602a679c1c3799d636b83d9003`；frontend `31b1fc950acc10951f627c5108a304423d69c5ce`；deploy `c3fd269d62e89f74000ba6436a5d02af5b9668b0`，与任务书快照一致 |
| 根 tree | `d2bc43e371b7c84b5f2e7e3b2f533734f7ec286f`。相对任务书 `7fa3eebd...` 仅新增本目录 `claude-review-request.md`；backend/frontend/deploy 无漂移 |
| 暂存路径 | 165（任务书写的 164 是加入审核任务书之前的候选） |

## Findings

**无新增本轮合并 finding。**

下面条目都不是“这次冲突处理把合同弄坏”，按归属单列。

## Ticket 移除与普通 turn-state

授权范围成立：上游 `8b69738d7...fbb9006ad` 为 forced update；旧 0.2.6 ticket 不在目标历史。候选按用户选择对齐目标，而不是把旧上游 ticket 当本地功能恢复。

| 核查 | 结果 |
| --- | --- |
| 删除完整性 | `backend`/`frontend`/`deploy` 中无 `CodexTicket` / `openai_codex_ticket` / harvest 命中。专属实现与测试已删；settings/DTO/账号展示/config.example 同步去掉 |
| SettingService | 相对目标补回本地 `onRiskControlUpdate` 与 `requestArchiveRuntimeCache`；ticket cache/singleflight 字段不在。`SetRiskControlUpdateCallback` 仍由 `service/wire.go` 注册，`setting_update.go` 在成功保存后调用 |
| 普通 turn-state | `guardOpenAICodexTurnStateEcho` 仍在 passthrough 出站路径；`openai_codex_turn_state.go` 与测试保留 |
| 误删 | HTTP/2 keepalive 与目标 blob 一致；fingerprint / model rewrite / 租户隔离未随 ticket 删除 |
| 数据边界 | 无 ticket 清理 migration；旧 settings/extra 未改真实库 |

相对第一父，passthrough 只去掉 `applyOpenAICodexTicket`，并带入上游 `needModelReplace` 不再先 `Contains(mappedModel)` 的 SSE 重写。这是目标行为，不是本地回归。

## Prompt Risk 与 collector 接口

相对第一父，`prompt_risk_input.go` 只多 14 行薄转接，newest/full、连续 user items、尾部 tool output、wrapper 剥离仍是原函数。三个 helper 一律 `filterReminders=true`，对第一父“`addModerationText` 遇 `<system-reminder>` 丢弃、Anthropic reminder 用 prefix 判断”的合同等价。

上游关键词路径 `extractContentModerationKeywordText` 仍走 `filterReminders=false`，本地适配没有把这条路径改回过滤。

`content_moderation.go` 仍在语义引擎前调用 `evaluatePromptRiskStage`；独立配置、密钥掩码、judge 回环头、runtime hash、`prompt_risk_block` / `prompt_risk_observe` 筛选与封禁计数排除都在。TypeSafe/OpenAI 引擎切换与本地前置阶段并存，不能互相替代。

## Wire 与插件账号目录

目标 `wire_gen.go` 有 `pluginManager.SetAccountDirectory(openAIGatewayService)`；目标与本地 `wire.go` 源图都没有等价接线。候选保留该生成物行，没有补改上游源图，符合“不修上游固有问题”。

相对目标，本地 `wire.go` 仍多 21 行（Token Analysis、并发预设、Prompt Metrics、quota flusher 等 cleanup）。生成物同时有 `NewPluginKVStore`、`SetAccountDirectory`、组织用量/Token Analysis/RequestIntercept/Prompt Metrics/`UserPlatformQuotaUsageFlusher`。ticket harvester cleanup 已从源图去掉。

**未来重生成风险（上游固有，P2）：** 在当前源图上 `go generate ./cmd/server` 会删掉 `SetAccountDirectory`。账号目录为 nil 时，有清单范围的插件拿不到 ListAccounts / ResolveOutboundIdentity。本轮不要为了“生成物与源图一致”删掉这行。

## 本地功能

| 能力 | 结论 |
| --- | --- |
| RequestArchive / RequestIntercept | `gateway.go` 的 `/v1` 链仍是 archive → `guardResponsesSubpath(intercept)`。Seedance 四组别名走 `rootRoute`，带 API Key、allowlist、archive、intercept。相对第一父只新增这 12 条 Seedance 路由 |
| Prompt Risk / LLM judge | 见上；前端 `RiskControlView.vue` 仍挂 `PromptRiskPanel`，独立保存未并进 TypeSafe 草稿 |
| Prompt Metrics / Token Analysis / 组织用量 / 并发预设 / quota flusher | Wire provider 与 cleanup 在链上；repository ProviderSet 仍含对应仓储 |
| 子管理员 | `frontend/src/types/index.ts` 相对目标保留 `UserRole`/`AdminPermission`。插件 `GET /admin/plugins/:id/status` 挂在 `adminAuth` 下，不在子管理员白名单，默认拒绝；只是对完整管理员免 step-up 的只读通道，没有绕过已有权限模型 |
| OpenAI-compatible | 账号弹窗仍 import 本地 preset；默认 capabilities 为 chat/embeddings，`seedance` 显式勾选。passthrough 相对目标多出本地清洗增量 |
| Seedance | 创建走 `BindGrokMediaVideoRequestAccount`；查询/删除走 `IsVideoLookupRequest` 的 user/API Key/group 绑定，缺绑定 404。计费只在 status 成功且 `completion_tokens>0` 时 claim 一次。创建失败不重试。账号需 `seedance` capability |
| 25 个 `docs/features` | 零删除；仅台账追加。部门用量设计未在本轮实现 |

仅上游 backend/frontend/deploy 路径抽查 65 个，index blob 与固定目标一致；加上 `docs/seedance-api.md` 与任务书“66 个仅上游路径”对齐。

## 验证

### 已有日志（`backend/.gocache/merge-207/`，本轮只读）

| 项 | 结果 |
| --- | --- |
| focused / default | 退出 0；default 54 包通过、14 skip |
| unit | 退出 1；失败为 `GET /api/v1/auth/me` 与 Ollama CAS，另有父 `TestAPIContracts` fail |
| integration | 退出 1；53 包通过，repository TestMain 因 `docker is not available (CI=true)` 失败；18 skip |
| build / tidy / lint | normal/embed、`go mod tidy -diff`、golangci-lint 0 issues |
| 前端 | lint/typecheck/build 通过；Vitest 325/326 files、2427/2433 tests |
| 第一父基线 | `baseline-auth.log` 同样缺 `admin_permissions:null`；`baseline-frontend.log` 同 6 个 Pinia 失败；`ollama.log` 当前 `-count=3` 为 3 失败，`baseline-ollama.log` 可见 2 次失败（与“1 通过 / 2 失败”在无 `-v` 时相符） |

未重放 `remove-ticket.patch`，未执行 `verify.ps1`（会覆盖原始日志）。

### 本轮独立执行（`backend/.gocache/claude-review-027/`）

| 命令 | 退出码 |
| --- | --- |
| `go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'PromptRisk\|ContentModeration\|Seedance\|PluginHost\|PluginAccount\|RequestArchive\|RequestIntercept'` | 0；四包 ok |
| `pnpm exec vitest run`：`RiskControlView.spec.ts`、`EditAccountModal.spec.ts`、`BulkEditAccountModal.spec.ts` | 0；3 files / 125 tests |

未重跑完整 unit/integration/前端套件，未连 Docker、真实收费上游或业务库。未在候选上 `go generate`；Wire 源图风险按源码对照确认。

## 上游 / 第一父 / 环境风险

| ID | 等级 | 归属 | 说明 |
| --- | --- | --- | --- |
| W1 | P2 | 上游固有 | `SetAccountDirectory` 只在生成物。以后重生成会丢账号目录注入；有 HostService 账号能力的插件会拿不到目录/出站身份 |
| D1 | P3 | 上游固有 | `AccountInfo` 注释写“非机密”，实现故意保留 Extra 与 Proxy（含代理密码）。wiki 已写明不能当成完全脱敏管理 DTO。`pluginapi/README.md` 的 HostService 只写了 KV，未写账号目录，这是目标文档缺口，不是本轮漏接 |
| T1 | P3 | 第一父 | `/auth/me` golden 缺 `admin_permissions:null` |
| T2 | P3 | 上游/第一父 | Ollama Cloud 429 CAS；实现与测试四方 blob 一致。当前 3/3 失败，第一父曾 1/2。一次转绿不能当修复 |
| T3 | P3 | 第一父 | `SubscriptionsView.userUsageLink.spec.ts` 6 个 no active Pinia |
| E1 | 环境 | Docker | repository integration 整包未跑。不能改 CI 标志后把跳过写成通过 |
| E2 | 环境 | skip | TypeSafe live、OpenAI key、Prompt Audit Redis/PG、插件 fixture 等未覆盖 |

0.2.6 的 ticket F1/F2 随目标移除不再适用于本候选。rollup 时区 F3 仍因无 Docker 未确认，不是本轮回归。

旧 ticket 的 settings/extra 行仍可能留在已部署库里，只是代码不再读取。这是授权的数据边界，不是漏删。

## 文档准确性

- `review.md`、台账 2026-09-20 条目、wiki 0.2.7 增量与现场 SHA/冲突处理/测试归属一致。台账是追加，25 个 feature 文件未删。
- “164 个文件”指加入 `claude-review-request.md` 前的候选；现在 165。三个代码子树未变。
- wiki 的 0.2.6 ticket 段落是历史记录；0.2.7 段已写明已移除。不要只读 0.2.6 段当现行行为。
- 建议（不改已有文件）：后续若修上游文档，把 HostService 账号目录和 Extra/Proxy 可见性写进 `pluginapi/README.md`，并在重生成清单里强制核对 `SetAccountDirectory`。

## 终态

- 分支 `feature/hy/10207_merge_sub2api_207`，`HEAD=de5a3e383...`，`MERGE_HEAD=fbb9006ad...`。
- 索引与代码未改。write-tree 在写入本报告前仍为 `d2bc43e371b7c84b5f2e7e3b2f533734f7ec286f`。
- 本文件按任务要求新建、保持未暂存。测试日志在忽略目录 `backend/.gocache/claude-review-027/`。
- 未 commit、未 push、未创建 PR、未部署。
