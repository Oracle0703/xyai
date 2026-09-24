# sub2api v0.1.135 上游合并审查报告

审查日期: 2026-06-09

审查对象: `feature/hy/0609_合并1.135版本`

合并提交: `d187587c6e8c440a120532221463981c02efc76d`

## 结论摘要

| 项目 | 结论 |
| --- | --- |
| 分支形态 | 基本符合要求。当前分支为 `feature/hy/0609_合并1.135版本`, HEAD 是一个双父 merge commit。 |
| 合并来源 | 已包含本地 `main` 与上游 `v0.1.135`。`git merge-base --is-ancestor main HEAD` 与 `git merge-base --is-ancestor v0.1.135 HEAD` 均通过。 |
| 冲突解决 | 四个显式冲突文件没有残留冲突标记, Wire 注入链同时保留了本地 `TokenAnalysisService` / `UserConcurrencyPresetRunner` / `UserPlatformQuotaUsageFlusher` 与上游 `ProxyExpiryService`。未发现明显“只保留一边、丢另一边”的冲突解决。 |
| 测试复核 | 前端 typecheck、相关 Vitest、后端关键包单测均通过。后端全量 `go test -tags=unit -p 1 -count=1 ./...` 主体通过, 但全量命令最终因 Windows 临时 `.test.exe` 执行权限问题返回非零; 两个失败包单独补跑通过。 |
| 合并前建议 | 不建议直接合入 `main`。至少应补齐强制上游合并记录, 并澄清/修正 `backup_proxy_id` 的 Ent 唯一约束与 SQL migration 不一致问题。 |

## Git 证据

| 检查项 | 证据 |
| --- | --- |
| 当前分支 | `git branch --show-current` 输出 `feature/hy/0609_合并1.135版本`。 |
| HEAD | `d187587c6e8c440a120532221463981c02efc76d`。 |
| 父提交 | `125296debcba26ae6fb3964d26a8d1429edd4656 8c782bcc81bd0661aeaf8da74499005c251c2925`。 |
| merge message | `Merge upstream v0.1.135 into feature/hy/0609_合并1.135版本`。 |
| 本地 main 包含性 | `git merge-base --is-ancestor main HEAD` 通过。 |
| 上游 tag 包含性 | `git merge-base --is-ancestor v0.1.135 HEAD` 通过。 |
| 工作区 | 审查结束前 `git status --short` 为空。 |
| 冲突标记 | `rg -n "^(<<<<<<<\|=======\|>>>>>>>)"` 无真实冲突标记命中。 |
| diff 基础格式 | `git diff --check main...HEAD` 通过。 |

## 显式冲突文件审查

merge commit 标注的冲突文件:

| 文件 | 审查结论 | 证据 |
| --- | --- | --- |
| `backend/cmd/server/wire.go` | 合理保留双边 cleanup 参数和 stop 逻辑。 | `provideCleanup` 参数同时包含 `tokenAnalysis *service.TokenAnalysisService` 与 `proxyExpiry *service.ProxyExpiryService`; cleanup steps 同时包含 `TokenAnalysisAutoIndex` 和 `ProxyExpiryService`。 |
| `backend/cmd/server/wire_gen.go` | 生成结果与手写 Wire 结构一致。 | `initializeApplication` 同时创建 `tokenAnalysisService`, `proxyExpiryService`, `userConcurrencyPresetRunner`, `userPlatformQuotaUsageFlusher`, 并传入 `provideCleanup`。 |
| `backend/cmd/server/wire_gen_test.go` | cleanup 最小依赖测试已补上 `ProxyExpiryService` 与 `TokenAnalysisService`。 | `TestProvideCleanup_WithMinimalDependencies_NoPanic` 构造 `tokenAnalysisSvc` 和 `proxyExpirySvc`。 |
| `backend/internal/service/wire.go` | ProviderSet 同时包含本地和上游新增 provider。 | `ProvideTokenAnalysisService` 启动自动索引, `ProvideProxyExpiryService` 启动代理过期服务, ProviderSet 中两者均存在。 |

判断: 这四个文件未发现不合理冲突解决。关键验证 `go test -tags=unit -p 1 -count=1 ./cmd/server` 通过。

## 发现的问题

### P1: 未按项目规则追加上游合并记录

| 项目 | 内容 |
| --- | --- |
| 严重级别 | 高 |
| 类型 | 流程/文档合规 |
| 证据 | `docs/features/sub2api -merage-list.md:7` 明确要求每次合并远程仓库后必须追加记录, `:9` 要求包含合并分支、上游提交、合并提交、冲突文件、处理方式和验证结果。 |
| 当前状态 | 搜索 `feature/hy/0609`, `v0.1.135`, `d187587c`, `8c782bcc` 在该文件中无命中; 当前分支也没有修改该文件。 |
| 风险 | 后续无法审计本轮上游合并的冲突文件、处理方式、验证命令和剩余风险; 违反 `AGENTS.md` 的强制要求。 |
| 建议 | 在合入 `main` 前追加一条 `v0.1.135` 合并记录, 至少包含日期、工作分支、上游 tag/提交、合并提交、4 个冲突文件、处理策略、验证命令与结果、已知风险。 |

### P2: `backup_proxy_id` 的 Ent schema 与 SQL migration 约束不一致

| 项目 | 内容 |
| --- | --- |
| 严重级别 | 中 |
| 类型 | 数据模型/迁移一致性 |
| 证据 | `backend/ent/schema/proxy.go:76-78` 的 `backup_proxy` edge 对 `backup_proxy_id` 使用 `Unique()`; 生成的 `backend/ent/migrate/schema.go:1111` 显示 `backup_proxy_id` 列 `Unique: true`。但 SQL migration `backend/migrations/149_proxy_expiry_fallback.sql:4,7` 只是添加普通外键列和普通索引, 没有唯一约束。 |
| 风险 | 新库或 Ent 自动迁移场景可能把 `backup_proxy_id` 当成唯一列, 限制多个代理共用同一个备用代理; 已迁移库则不会有该唯一约束, 导致不同环境行为不一致。对于“多个过期代理回退到同一个健康备用代理”这个业务场景, 唯一约束也不合理。 |
| 建议 | 如果业务允许多个代理共用同一个备用代理, 去掉 `edge.To(...).Unique()` 并重新生成 Ent 代码; 如果业务确实要求一对一, 则需要补 SQL migration 的唯一约束, 并在前端/导入逻辑给出冲突提示。建议优先按“多对一备用代理”修正。 |

### P3: `llm-wiki` 未记录本轮上游新增的重要后台任务和数据字段

| 项目 | 内容 |
| --- | --- |
| 严重级别 | 中 |
| 类型 | 项目知识库维护 |
| 证据 | 本轮新增/合入了 `ProxyExpiryService`, `proxies.expires_at/fallback_mode/backup_proxy_id/expiry_warn_days`, `accounts.proxy_fallback_origin_id`, API key exclusive group 校验, OpenAI transport failover 临时摘除等跨模块行为; 但 `llm-wiki/wiki/*.md` 在当前分支没有相对 `main` 的变更。 |
| 风险 | 后续 AI 或维护者按 wiki 进入任务时, 不知道代理过期回退后台任务、账号手动回切、告警指标和数据模型约束, 容易遗漏 Wire cleanup、migration、前端字段或运维告警。 |
| 建议 | 合入前更新 `llm-wiki/wiki/backend.md`, `data-and-domain.md`, `security-and-reliability.md` 或 `ops.md` 中相关章节, 简洁记录代理过期回退、手动回切入口、OpenAI transport failover 和验证命令。 |

## 关键功能复核

| 上游 1.135 主题 | 当前落地情况 | 审查意见 |
| --- | --- | --- |
| 代理有效期与失败回退 | 后端新增 `ProxyExpiryService`, `SweepExpiredProxies`, `ResolveProxyFallbackTarget`, `RevertProxyFallback`; 前端 `ProxiesView.vue` 增加有效期/回退模式 UI, `AccountsView.vue` 增加回切入口。 | 逻辑链路基本完整; 需要修正 P2 的 schema/migration 约束不一致。 |
| OpenAI transport error failover | `handleOpenAIUpstreamTransportError` 在 Responses fallback、raw/passthrough 路径中接入; 持久网络/代理错误会临时摘除账号。 | `internal/service` 单测通过, 未发现合并导致的明显丢失。 |
| API Key exclusive group auth | `APIKeyAuth` 中增加用户不再允许 exclusive group 时拒绝访问; 对应 middleware 单测存在并通过。 | 合理。 |
| usage cache token split | `UsageLogStats` 和 repository 聚合拆分 `cache_creation_tokens` / `cache_read_tokens`; 前端 i18n 增加相关文案。 | 合理。 |
| 5h reset stale 修复 | `UsageProgressBar.vue` 过期窗口但仍有 utilization 时显示 `usage.resetPending`, utilization 为 0 时显示 `usage.resetNow`。 | 相关 Vitest 通过。 |
| 非流式 Content-Type | `openai_gateway_handler.go` / gateway service 路径保留 JSON Content-Type 处理。 | 未发现冲突覆盖。 |

## 验证记录

| 命令 | 结果 |
| --- | --- |
| `git diff --check main...HEAD` | 通过。 |
| `rg -n "^(<<<<<<<\|=======\|>>>>>>>)"` | 无真实冲突标记。 |
| `cmd.exe /c npm run typecheck` in `frontend` | 通过。 |
| `cmd.exe /c npx vitest run src/utils/__tests__/proxyExpiry.spec.ts src/components/account/__tests__/UsageProgressBar.spec.ts` | 通过, 2 个文件 14 个测试通过。 |
| `go test -tags=unit -p 1 -count=1 ./cmd/server` in `backend` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/server/middleware` in `backend` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/service` in `backend` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/repository` in `backend` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/server` in `backend` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./...` in `backend` | 主体通过, 但最终因 Windows 临时测试可执行文件 `geminicli.test.exe` / `openai_compat.test.exe` `Access is denied` 返回非零。两个包已单独补跑通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/geminicli` in `backend` | 补跑通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/openai_compat` in `backend` | 补跑通过。 |

## 合入前建议清单

| 优先级 | 建议 |
| --- | --- |
| 必须 | 补写 `docs/features/sub2api -merage-list.md` 的 `v0.1.135` 合并记录。 |
| 必须 | 明确 `backup_proxy_id` 是否允许多个代理共用同一个备用代理; 按结论修正 Ent schema 或 SQL migration, 并重新生成代码/补测。 |
| 建议 | 更新 `llm-wiki/wiki` 中代理过期回退、账号回切、OpenAI transport failover、API key exclusive group 校验等稳定知识。 |
| 建议 | 在干净 CI 或非 Windows 文件锁环境中再跑一次完整 `go test -tags=unit ./...` 和前端完整 `npm run test:run`/`npm run lint:check`。 |

