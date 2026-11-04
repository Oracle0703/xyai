# Sub2API 0.1.135 上游同步记录

合并日期 2026-06-09 ｜ 工作分支 `feature/hy/0609_合并1.135版本` ｜ 上游 `Wei-Shaw/sub2api` tag `v0.1.135`（`8c782bcc81bd0661aeaf8da74499005c251c2925`，merge base `635ad81cdcad`）｜ 合并提交 `d187587c6e8c440a120532221463981c02efc76d`（父 `125296debcba` + `8c782bcc81bd`）｜ 状态：已经 PR #7（`04bc40bf6`，2026-06-10）合入 main

本文是该轮同步的唯一记录。原始审核材料 `docs/upstream-sync/reviews/sub2api-v0.1.135-merge-review-cn.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-06-09` 节。

## 1. 范围

- 上游 `v0.1.134` 之后的 26 个增量提交，合并共改动 96 个文件。
- 显式冲突 4 个文件，均集中在 Wire 依赖注入；ent schema、生成的 ent 代码、i18n 文案自动合并。

## 2. 冲突与处理

4 个冲突都是本地与上游在同一位置各自新增独立 provider，统一双边保留（本地 `TokenAnalysisService` + 上游 `ProxyExpiryService`）。合并后复核：无残留冲突标记（`rg -n "^(<<<<<<<|=======|>>>>>>>)"`），`git diff --check main...HEAD` 通过。

| 文件 | 处理 |
| --- | --- |
| `backend/internal/service/wire.go` | ProviderSet 同时注册 `ProvideTokenAnalysisService`（启动自动索引）与 `ProvideProxyExpiryService`（启动代理过期服务）。 |
| `backend/cmd/server/wire.go` | `provideCleanup` 参数与 cleanup steps 同时包含 `tokenAnalysis` / `TokenAnalysisAutoIndex` 与 `proxyExpiry` / `ProxyExpiryService`。 |
| `backend/cmd/server/wire_gen.go` | 先解决源 `wire.go`，再用 `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server` 重新生成；`initializeApplication` 同时创建 `tokenAnalysisService`、`proxyExpiryService`、`userConcurrencyPresetRunner`、`userPlatformQuotaUsageFlusher` 并传入 `provideCleanup`。`go generate ./cmd/server` 的 `main.go` 指令缺 `-mod=mod`，会因 `google/subcommands` 缺 go.sum 条目失败；`-mod=mod` 引入的 wire/entc 工具传递依赖已从 `go.sum` 还原。 |
| `backend/cmd/server/wire_gen_test.go` | `TestProvideCleanup_WithMinimalDependencies_NoPanic` 同时构造 `tokenAnalysisSvc` 与 `proxyExpirySvc`，按签名顺序传入。 |

另按 llm-wiki 规则执行 `go generate ./ent`，输出与自动合并结果一致。

## 3. 保留的本地能力与语义适配

- Token 分析自动索引、用户并发 preset runner、用户平台额度用量 flusher 的 Wire 接线与 cleanup 全部保留。
- 未发现"只保留一边、丢另一边"的冲突解决。

## 4. 采用的上游行为

| 主题 | 内容 | 复核结论 |
| --- | --- | --- |
| 代理有效期与失败回退 | `ProxyExpiryService`（每分钟 `SweepExpiredProxies`）、`ResolveProxyFallbackTarget` / `RevertProxyFallback`；`proxies` 新增 `expires_at` / `fallback_mode` / `backup_proxy_id` / `expiry_warn_days`，`accounts` 新增 `proxy_fallback_origin_id`（手动回切来源）；前端 `ProxiesView.vue` 有效期/回退模式 UI，`AccountsView.vue` 回切入口。 | 链路完整；约束不一致见第 6 节。 |
| OpenAI transport failover | `handleOpenAIUpstreamTransportError`（`openai_upstream_transport_error.go`）接入 Responses fallback、raw/passthrough 路径；持久网络/代理错误临时摘除账号。 | `internal/service` 单测通过。 |
| API Key exclusive group | `APIKeyAuth` 在用户不再被授权独占分组时拒绝访问，附 middleware 单测。 | 合理。 |
| 用量缓存 token 拆分 | `UsageLogStats` 与 repository 聚合拆分 `cache_creation_tokens` / `cache_read_tokens`，前端增加缓存创建/命中/命中率文案。 | 合理。 |
| Ops 告警 | 新增 `account_temp_unscheduled_count` 告警指标（`OpsAlertRulesCard.vue` + evaluator）。 | — |
| 其他修复 | 5h reset stale 同步 `SessionWindowEnd`（`UsageProgressBar.vue` 显示 `usage.resetPending` / `usage.resetNow`）；非流式响应强制 `application/json`；切组后剥离失配 `previous_response_id`；`Select.vue` 下拉高度；新增 `skills/sub2api-admin`。 | 相关 Vitest 通过；非流式 Content-Type 未被冲突覆盖。 |

## 5. 验证

| 命令（`backend` / `frontend` 目录） | 结果 |
| --- | --- |
| `go build ./...`、`go generate ./ent`、wire 重生成 | 通过 |
| `go test ./cmd/server -run Wire` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./cmd/server ./internal/server/middleware ./internal/service ./internal/repository ./internal/server` | 逐包通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 主体通过；Windows 临时 `.test.exe` 被锁（`geminicli.test.exe` / `openai_compat.test.exe` / `internal/pkg/ip` `Access is denied`）导致非零，相关包单独重跑通过，属环境问题 |
| `go test -tags=integration ./...` | 编译通过；本机无 Docker，testcontainers 跳过 |
| `pnpm --dir frontend run typecheck` | 通过 |
| `vitest run src/utils/__tests__/proxyExpiry.spec.ts src/components/account/__tests__/UsageProgressBar.spec.ts` | 2 files / 14 tests 通过 |
| `pnpm --dir frontend exec vitest run`（全量） | 733 通过 / 4 失败；失败为 `UsageTable.spec.ts` / `UsageView.spec.ts` 图片用量行用例，在合并前 main `125296de` 上同样失败（`billing_mode=null` 历史行未走 `image_count`/模型名兜底，显示成 "Token"），非本次合并引入，团队决定暂不修 |

## 6. 遗留问题

- `proxies.backup_proxy_id` 唯一约束不一致（上游 0.1.135 自带）：当时 `backend/ent/schema/proxy.go` 的 `backup_proxy` edge 用 `.Unique()` 且无反向边，生成的 `ent/migrate/schema.go` 把列标为 `Unique: true`，而 `backend/migrations/149_proxy_expiry_fallback.sql` 只建普通外键和普通索引。项目不使用 Ent auto-migrate，真实库为非唯一列，多个代理可共用同一备用代理，运行无影响；团队决定只记录不改。**当前状态**：上游提交 `e835f7981`（2026-09-08，`fix(proxies): make backup references directed and shareable`，随 0.2.4 合入）已增加反向边 `primary_proxies`，`ent/migrate/schema.go` 不再标记唯一，该不一致在当前源码中已消除。
- 审核时提出的"补写合并台账"和"更新 llm-wiki 代理过期回退/transport failover 知识"两项已在后续完成（台账 `2026-06-09` 节、`llm-wiki/wiki/data-and-domain.md`）。
