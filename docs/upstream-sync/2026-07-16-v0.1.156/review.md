# Sub2API 0.1.156 上游同步记录（固定提交）

合并日期 2026-07-16 ｜ 工作分支 `feature/hy/10157_同步sub2api主线`（自本地 `main@4c456aad32c086bb32c650d0e8c659450cc6de3f` 创建）｜ 上游 `Wei-Shaw/sub2api main@d515c3045ce838976ebedab87846aaaf893dbbf6`（含 tag `v0.1.156` = `12f991dde8a5` 及其后的 VERSION 同步，merge base `7c717365ef72`）｜ 合并提交 `b5b54af2129bd5c7cc3d3b54e941deb8a35f31d9`（父 `4c456aad3` + `d515c3045`）｜ 状态：随同分支 0.1.159 经 PR #22（`5d5f15785`，2026-07-17）合入 main

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）、`docs/upstream-sync/reviews/2026-07-16-sub2api-v0.1.156-merge-review.md`、`docs/upstream-sync/reviews/2026-07-16-sub2api-v0.1.156-acceptance-review.md` 与 `docs/superpowers/plans/2026-07-16-sub2api-v0.1.156-sync.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-16 main sync (v0.1.156, pinned; awaiting review)` 与 `2026-07-16 v0.1.156 merge commit 回填` 两节。

## 1. 范围

- 上游 132 commits、253 files（`+31408 / -1710`）；提交前 staged 快照 274 files（`+32475 / -1728`）。
- 3 个文本冲突、29 个双方修改文件（由 `merge-base..^1` 与 `merge-base..^2` 文件集合求交得到，不只看 Git 报出的冲突）。
- 边界决策：
  - tag `v0.1.156` 指向的 `12f991d` 树里 `backend/cmd/server/VERSION` 仍是 `0.1.155`，其直接子提交 `d515c3045` 只把 VERSION 同步为 `0.1.156`，因此固定边界取 `d515c3045`。首轮误按 tag 目标合并，在人工编辑冲突前撤销并重新合并。
  - 会话期间 `upstream/main` 前进到 `eb2b8632ded6`，明确排除（`git merge-base --is-ancestor eb2b8632ded6 b5b54af2129b` 退出 1）。
  - 第一父严格是本地 `main`，不含 `feature/hy/10156_新增子管理员角色`（源码无 `sub_admin` / `admin_permissions`）。
- 执行方式：`git merge --no-commit --no-ff d515c3045...` 停在提交前供用户审核，用户在知悉 integration 环境限制后批准，才创建 merge commit。
- 合并决策规则（本轮首次明确，已写入 playbook）：仅本地存在→保留；仅上游新增→采用；双方互补→手工组合并验证入口/数据结构/调用链；双方等价且上游完整覆盖→采用上游并移除本地重复实现；上游缺本地安全或业务边界→保留本地边界并把上游能力接入。

## 2. 冲突与处理

| 文件 | 本地意图 | 上游意图 | 处理 |
| --- | --- | --- | --- |
| `backend/internal/config/config.go` | `gateway.openai_default_reasoning_effort` 字段、默认值、env、校验 | first-output timeout、WS client-first-message timeout、token refresh 等配置 | 手工组合双方字段、默认值与校验，未整文件取任一侧 |
| `backend/internal/config/config_test.go` | reasoning effort 空默认、env、非法值测试 | 0.1.156 配置默认值/边界测试 | 双方用例共存并与最终 config 对齐 |
| `backend/internal/service/content_moderation.go` | Prompt Risk、LLM judge、semaphore、日志、fail-open | stale-while-refresh runtime snapshot、TTL/backoff、预编译 keyword matcher | 上游 snapshot/matcher 作热路径，叠加本地 judge；补 `risk_control_enabled` 持久化成功后的即时回调 |

### 29 个双方修改文件

| 领域 | 数量 | 审查重点 | 结果 |
| --- | ---: | --- | --- |
| Wire / repository DI | 3 | provider set、接口 binding、生成代码闭合 | 发现并修复 Grok OAuth binding |
| Config / deploy | 3 | 本地配置不丢、上游默认值/校验/示例同步 | 通过 |
| Handler / routes / contract | 5 | 本地 middleware 链、admin API、Gemini/OpenAI 错误语义 | 通过 |
| API compat / gateway / usage | 12 | options/cache usage、moderation、failover headers、passthrough、计费用量 | 发现并修复 3 个 P2 |
| Frontend / locale | 5 | provider 模板、compatible preset、账号表单、中英文 key | 修复 UseKeyModal 测试选择器 |
| README | 1 | 本地说明与上游文档并存 | 通过 |

文件清单：`backend/cmd/server/wire_gen.go`、`backend/internal/config/config.go`、`backend/internal/config/config_test.go`、`backend/internal/handler/gemini_v1beta_handler.go`、`backend/internal/handler/openai_gateway_handler_test.go`、`backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`、`backend/internal/repository/wire.go`、`backend/internal/server/api_contract_test.go`、`backend/internal/server/routes/admin.go`、`backend/internal/server/routes/gateway.go`、`backend/internal/service/account_usage_service.go`、`backend/internal/service/content_moderation.go`、`backend/internal/service/openai_codex_transform.go`、`backend/internal/service/openai_gateway_cc_pipeline.go`、`backend/internal/service/openai_gateway_chat_completions.go`、`backend/internal/service/openai_gateway_chat_completions_raw.go`、`backend/internal/service/openai_gateway_forward.go`、`backend/internal/service/openai_gateway_messages.go`、`backend/internal/service/openai_gateway_passthrough.go`、`backend/internal/service/openai_gateway_response_handling.go`、`backend/internal/service/openai_oauth_passthrough_test.go`、`backend/internal/service/wire.go`、`deploy/config.example.yaml`、`frontend/src/components/account/CreateAccountModal.vue`、`frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`、`frontend/src/components/keys/UseKeyModal.vue`、`frontend/src/i18n/locales/en/admin/accounts.ts`、`frontend/src/i18n/locales/zh/admin/accounts.ts`、`README_CN.md`。

### 合并引入问题及修复

均先以失败用例复现（RED）再修复（GREEN），修复后独立复核无开放 P0–P3。

| 等级 | 问题 | 根因 | 修复 |
| --- | --- | --- | --- |
| P1 阻断 | `go generate ./cmd/server` 报 `no provider found for service.GrokOAuthTokenService` | 上游把 handler 依赖收窄为接口，遗漏 Wire binding | `backend/internal/service/wire.go` 增加 `wire.Bind(new(GrokOAuthTokenService), new(*GrokOAuthService))`；连续两次生成 SHA-256 一致 |
| P2 | `UseKeyModal.spec.ts` 把普通 Codex 模板 provider 当成 `OpenAI`（1/9 失败） | 上游 WS v2 模板与本地普通模板 provider 合同不同 | 普通模板继续用本地 `xunyou`，只有 WS v2 模板断言 `OpenAI`；生产代码未改，9/9 通过 |
| P2 | native Responses failover 丢 `ResponseHeaders` | failover error 构造未复制上游 header | `openai_gateway_forward.go` 用 `resp.Header.Clone()`，交统一 allowlist 恢复 |
| P2 | `risk_control_enabled` 保存后 snapshot 仍用旧总开关 | config/prompt-risk 有即时 replace，总开关链缺回调 | `SettingService.SetRiskControlUpdateCallback` 在持久化成功后原子替换 snapshot enabled（false↔true 双向用例） |
| P2 | passthrough/failover 可传播无界或注入型 `Retry-After` | handler 与 passthrough 校验不统一、缺上限；初版修复在原始值校验前 trim | 共用 `IsSafeRetryAfter`：先对原始字符串拒绝 CR/LF 与长度 `>128`，再 trim 解析；秒数 `1..604800`，HTTP date 必须在未来且 ≤7 天（service 12/14→14/14、handler 0/2→2/2） |

另外：Prompt Risk 原先每请求旁路读取配置 4 次，已并入 runtime snapshot 并补回归；上游 nanosecond TTL 测试在 Windows 时钟粒度下不稳定，已修正；删除上游 Grok 测试 EOF 多余空行。

## 3. 保留的本地能力与语义适配

| 本地能力 | 决策 | 代码锚点 | 说明 |
| --- | --- | --- | --- |
| RequestArchive / RequestIntercept | 组合 | `server/middleware/request_archive.go`、`request_intercept.go`、`routes/gateway.go` | 继续包裹上游新增或调整后的路由 |
| Prompt Metrics | 保留 | `service/promptmetrics/middleware.go`、`server/router.go`、`PromptMetricsView.vue` | 上游无等价替代 |
| Prompt Risk / LLM judge | 组合 | `content_moderation.go`、`prompt_risk.go`、`prompt_risk_judge.go` | 与上游 snapshot/matcher 合为同一运行时审核链 |
| Token Analysis | 保留 | `token_analysis_handler.go` / `_service.go` / `_repo.go`、`TokenAnalysisView.vue` | API、索引、归档解析、UI |
| 组织用量 | 保留 | `organization_usage_handler.go` / `_service.go` / `_repo.go`、`OrganizationUsageView.vue` | 报表、趋势、导出口径未被覆盖 |
| ImageGen / 支付 | 组合 | `ImageGenView.vue`、`image_generation_intent.go`、`routes/payment.go`、`payment_service.go` | 接入上游图片 intent 与 OpenAI/Grok image 增量 |
| 用户并发 preset / `ConcurrencyCacheError` | 保留 | `user_concurrency_preset_service.go` / `_runner.go`、`concurrency_service.go` | 缓存错误与 503 语义完整 |
| OpenAI-compatible preset / options / cache usage | 组合 | `openaiCompatibleProviderPresets.ts`、`apicompat/responses_to_chatcompletions_request.go`、`openai_gateway_response_handling.go` | cache write/read 继续 fill-missing |
| 默认 reasoning effort | 组合 | `config/config.go`、`openai_gateway_chat_completions.go` | 与上游配置在冲突中组合 |
| user-platform quota flusher | 保留 | `user_platform_quota_flusher.go`、`service/wire.go`、`wire_gen.go` | 脏集快照、批量落库、TimingWheel 与 shutdown 清理仍接入 |

没有识别到可由上游完整替代并安全移除的本地功能（"上游替代"项为 0）；`docs/features/` tracked 文件删除为 0。

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| OpenAI 可靠性 | native Responses 首次语义输出超时保护、attempt 级切号、响应头退避信息保留；WS 首消息超时等配置边界 |
| Grok / token refresh | 凭据失败 scope 隔离、OAuth reconcile、候选游标分页、provider QPS/并发 gate 与周期熔断 |
| 账号 / 调度 | 静态凭据账号安全复制、幂等恢复、默认不可调度副本；scheduler projection/batch 查询与生命周期加固 |
| API 兼容 | Chat Completions ↔ Anthropic bridge、Responses Lite/custom tools、Agent Identity、根级 `/models`，更多 failover/cancellation 保护 |
| 前端 / 部署 | Codex 模板区分本地 `xunyou` 与 WS v2 `OpenAI` provider；DataTable identity cache 加固；嵌入式静态缓存只信任 fingerprint 资源 |

## 5. 验证

| 命令 / 检查（`backend` / `frontend`） | 结果 |
| --- | --- |
| 合并前基线：后端 `config/routes/handler` smoke、前端 typecheck | 通过 |
| 前端合并相关专项 Vitest | 19 files / 131 tests 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 完整重跑退出 0：50 个测试包通过、53 个无测试包（`internal/service` 116.7s）；首轮仅 `internal/repository` 在 fork/exec 前被 Windows 占用，单独与完整重跑均通过 |
| `go test -tags=integration -p 1 -count=1 ./...` | 跑到结尾：44 个测试包通过、57 个无测试包；`internal/pkg/openai_compat` 的 `.test.exe` 被本机 360 持续拒绝启动，命令最终 FAIL。该包 unit/integration 文件集合相同且 unit 已通过；未关安全软件、未加白名单、未改业务代码 |
| `golangci-lint run ./...` | 退出 1：29 条既有债务（15 errcheck、2 gofmt、3 staticcheck、9 unused） |
| `golangci-lint run --new-from-rev main ./...` | 0 issues |
| `go mod tidy -diff` | 退出 1，只建议删除本地 `main` 已有的 Wire CLI 传递校验和；`go.mod/go.sum` 与第一父相同，不制造无关 churn |
| `pnpm exec vitest run` | 176 files / 1214 tests 通过 |
| `pnpm run lint:check`、`typecheck`、`build` | 通过；build 944 modules / 13.57s，仅既有 import/chunk warning |
| 前端构建后 `go build -tags embed -trimpath ./cmd/server` | 通过，产物 147,534,336 bytes |
| 连续两次 `go generate ./cmd/server` | `wire_gen.go` SHA-256 一致 |
| staged 硬门 | 无 unstaged/untracked/unmerged、冲突标记、whitespace、CRLF/BOM、敏感模式或 `docs/features` 删除 |

独立验收在提交前复跑了定向用例：`./internal/config`（OpenAI/WS/reasoning）、`./internal/service`（moderation snapshot / Retry-After / forward）、`./internal/handler -run "RetryAfter|Failover"` 均 ok；Vitest `UseKeyModal` + `admin.accounts.duplicate` + `AccountsView.sparkShadow` 3 files / 19 tests 通过。

## 6. 遗留问题

- integration 版 `openai_compat.test.exe` 被本机 360 拦截，需在换机或加白名单后补跑。
- 全仓 lint 基线债务见 `docs/features/technical-debt-board-cn.md` TD-004；前端构建 chunk/import warning 未处理。
