# Sub2API 0.1.160 上游同步记录（固定提交）

合并日期 2026-07-17 ｜ 工作分支 `feature/hy/10160_合并1.160版本`（自本地 `main@5d5f157854b9a88cc57da1600095bb404b78ed45` 创建）｜ 上游 `Wei-Shaw/sub2api main@57914967cbb127ff715719c3879d881c10d75274`（VERSION 0.1.160，merge base `c2c19a7cbe84`）｜ 合并提交 `8a022e9756d9dab36a3963b1e023d77d6fce0c75`（父 `5d5f15785` + `57914967c`）｜ 状态：经 PR #24（`332fdbd0b`，2026-07-19）合入 main

本文是该轮同步的唯一记录。原始审核材料 `docs/upstream-sync/reviews/2026-07-17-sub2api-v0.1.160-merge-review.md` 与执行计划 `docs/superpowers/plans/2026-07-17-sub2api-v0.1.160-sync.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-17 main sync (v0.1.160; committed)` 节。

## 1. 范围

- 上游 20 commits、133 files（`+19766 / -113`）；提交前 staged 快照 143 files（`+20209 / -118`）。
- 边界：tag `v0.1.160` 指向 `8bfbc5ca99bf`，该提交 VERSION 仍为 `0.1.159`；随后的 version-sync 提交 `57914967c` 才是本轮边界。审核期间上游继续前进不跟随。
- 4 个文本冲突 + 14 个自动合并的双方修改文件，共 18 个交集逐项对照 merge base / 本地 main / 上游审查。
- 用户边界：只解决冲突和保留本地功能所需的兼容，**不修复随上游带入的 bug**。
- 子管理员功能分支不在第一父历史中，不属于本轮保留范围；提交后另行合回（见第 6 节）。

## 2. 冲突与处理

| 文件 | 本地意图 | 上游意图 | 处理 |
| --- | --- | --- | --- |
| `backend/cmd/server/wire.go` | `promptmetrics.ProviderSet`，Token Analysis / 用户并发 preset / Prompt Metrics / quota flusher 及 cleanup | `securityaudit.ProviderSet`、`Application.PromptAudit`、`PromptAuditService.Shutdown` 生命周期 | ProviderSet、Application 字段与 cleanup 语义并集 |
| `backend/cmd/server/wire_gen.go` | 本地扩展的真实构造与清理链 | 构造 Prompt Audit 并注入 handler/server | 组合双方现有生成结果；不借解决生成冲突修正上游 ProviderSet（见第 6 节） |
| `frontend/src/components/layout/AppSidebar.vue` | 图片生成、请求拦截、Prompt Metrics、Token Analysis、组织用量及权限过滤 | 新增 Security Audit 分组与 Prompt Audit 入口 | 采用上游 Security Audit 分组容纳 Risk Control / Prompt Audit，Request Intercept 等本地入口独立保留 |
| `frontend/src/router/index.ts` | `/admin/request-intercept` 与全部本地 admin 路由/守卫、backend-mode 边界 | `/admin/prompt-audit` 与 risk-control gate | 路由并存，Prompt Audit 使用 `requiresRiskControl` |

自动合并交集：`backend/cmd/server/main.go`、`backend/cmd/server/wire_gen_test.go`、`backend/internal/handler/content_moderation_helper.go`、`backend/internal/handler/gemini_v1beta_handler.go`、`backend/internal/handler/handler.go`、`backend/internal/handler/openai_gateway_handler.go`、`backend/internal/handler/wire.go`、`backend/internal/server/routes/admin.go`、`frontend/package.json`、`frontend/pnpm-lock.yaml`、`frontend/src/i18n/locales/{en,zh}/admin/index.ts`、`frontend/src/i18n/locales/{en,zh}/common.ts`。

唯一额外语义适配：把本地 `X-Sub2API-Prompt-Risk-Judge` 签名头从 handler 的 `securityaudit.Request` 继续传给 legacy moderation。否则 judge 经本网关 HTTP 回环时，新 coordinator 会丢掉本地防递归合同。由 `security_audit_helper_test.go`、`coordinator_legacy_test.go` 两个定向测试覆盖；属于保留本地功能的兼容，不是上游 bug 修复。

## 3. 保留的本地能力与语义适配

| 本地能力 | 决策 | 说明 |
| --- | --- | --- |
| RequestArchive / RequestIntercept | 保留 | 网关 middleware 与 `/admin/request-intercept` |
| Prompt Metrics | 保留 | 与上游 Prompt Audit 职责不同，ProviderSet、middleware、UI、shutdown 均保留 |
| Prompt Risk / LLM judge | 组合 | 既有规则/judge 作为 legacy moderation 接入新 coordinator，防递归签名头完整传递 |
| Token Analysis | 保留 | handler/service/repository/UI 与后台索引 |
| 组织用量 | 保留 | API、SQL 口径、导出与页面 |
| 图片生成与支付 | 组合 | 本地 ImageGen/支付保留，采用上游 Grok media eligibility 与 explicit intent |
| 用户并发 preset / `ConcurrencyCacheError` | 保留 | runner、缓存错误、503 合同 |
| OpenAI-compatible preset / options / cache usage | 保留 | 未被替代 |
| 默认 reasoning effort | 保留 | 配置与注入路径 |
| user-platform quota flusher | 保留 | dirty snapshot、批量落库、shutdown |

"上游替代"项为 0；`docs/features/` tracked 删除为 0。

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| Prompt Audit | 默认关闭；off / async audit / blocking 三档；OpenAI-compatible Guard 节点、策略/group 范围、runtime、事件查询与筛选删除由管理端配置 |
| 存储 | migration 181 新增 `prompt_audit_jobs` / `prompt_audit_events`，async 扫描原文在 Redis 最长 30 分钟；migration 182 把未脱敏 `full_prompt` 持久化到 event，上限 65,536 rune |
| 前端 | `/admin/prompt-audit` 位于 Security Audit 分组：policy、endpoint pool、runtime、event workspace/detail、筛选删除 |
| Grok media | generation 调度引入账号资格 capability，scheduler cache 保留资格/billing snapshot，image reference 字段规范为 `url` |
| S3 step-up | `BackupView.vue` 保存 S3 配置接入 step-up TOTP |
| 前端依赖 | 显式加入 `@intlify/message-compiler@9.14.5`，locale compile suite 在 pnpm strict linking 下可收集（解决 0.1.159 遗留） |

## 5. 验证

| 命令 / 检查 | 结果 |
| --- | --- |
| 合并前基线：`go test -tags=unit ./internal/config ./internal/server/routes ./internal/handler`、前端 typecheck | 通过 |
| 后端定向 `./cmd/server ./internal/securityaudit ./internal/server/routes ./internal/handler` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 退出 0 |
| `go test -tags=integration -p 1 -count=1 ./...` | 退出 0 |
| 全量 `golangci-lint run ./...` | 28 条本地 main 既有债务 |
| `golangci-lint run --new-from-rev main ./...` | 0 issues |
| `go mod tidy -diff` | 只建议删除既有 Wire CLI 传递校验和，未改 `go.mod/go.sum` |
| `go generate ./cmd/server` | 按预期退出 1，报缺 `securityaudit.PromptAdminService` provider（上游问题，见第 6 节） |
| 前端定向 Prompt Audit / layout / router | 14 files / 94 tests 通过 |
| 前端 lint、typecheck、全量 Vitest、build | 通过；194 files / 1330 tests；build 979 modules |
| embed build | 通过，产物 149,248,512 bytes |
| staged 门禁 | 无 unstaged/untracked/unmerged、冲突标记或 `docs/features` 删除；排除上游 source-freeze patch 后 cached/HEAD diff check 退出 0 |

## 6. 遗留问题

| 项目 | 说明 | 当前状态 |
| --- | --- | --- |
| Prompt Audit Wire 源图缺接口绑定 | `securityaudit.ProviderSet` 提供 `NewPromptService` 但缺 `wire.Bind(new(PromptAdminService), new(*PromptService))`，`go generate ./cmd/server` 失败；合并时保留现有 `wire_gen.go`，不改上游 ProviderSet | 上游 `b92bbf029`（随 0.1.161 合入）已在 `backend/internal/securityaudit/prompt_module.go` 补上 binding |
| OpenSpec source-freeze patch 自带 whitespace | `openspec/.../source-freeze/aicodex-prompt-audit-tracked.patch` 在无过滤 `git diff --cached --check` 中报 trailing whitespace / space-before-tab | 作为上游冻结证据原样保留；diff check 需排除该 artifact |
| Prompt 原文持久化改变早期设计 | migration 181 注释称 raw prompt 不进 PostgreSQL，但 migration 182 与 repository 实际写入 `full_prompt` | 以源码为准写入 wiki，不改上游实现 |

提交后的本地 main 回合：本地 `main` 经 `git pull --ff-only` 快进到 `github/main@864bb85e8`（含子管理员 PR #23），再以 `--no-commit --no-ff` 合回本分支；唯一冲突 `llm-wiki/wiki/README.md` 同时保留 0.1.160 与子管理员条目。子管理员 migration `backend/migrations/177_add_sub_admin_permissions.sql`、`backend/internal/service/admin_permission.go`、后端权限/路由白名单、`frontend/src/utils/adminPermissions.ts` 与路由/侧栏过滤均进入合并树。按用户要求该次只做未解决索引、冲突标记、diff check 与关键文件存在性检查，没有重跑测试。
