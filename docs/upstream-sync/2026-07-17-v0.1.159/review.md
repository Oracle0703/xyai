# Sub2API 0.1.159 上游同步记录

合并日期 2026-07-17 ｜ 工作分支 `feature/hy/10157_同步sub2api主线`（第一父 `b5c9726262ca65bddc8ca4bc5a35ed26e1208cb3`，即 0.1.156 合并后的同分支）｜ 上游 `Wei-Shaw/sub2api main@c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc`（VERSION 0.1.159，`git describe` 为 `v0.1.157-33-gc2c19a7cb`，merge base `d515c3045ce8`）｜ 合并提交 `d4256d471cc3f087ca01406a4c92010d4528473f`（父 `b5c972626` + `c2c19a7cb`）｜ 状态：经 PR #22（`5d5f15785`，2026-07-17）合入 main

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）与 `docs/upstream-sync/reviews/2026-07-17-sub2api-v0.1.159-merge-review.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-17 main sync (v0.1.159; awaiting review)` 节（该节"待用户审核，尚未创建"是提交前快照，实际合并提交见上）。

## 1. 范围

- 上游 114 commits、399 files（`+28829 / -1666`）；提交前 staged 快照 416 files（`+29294 / -1677`）。10 个文本冲突。
- 目标是"收尾时最新的 `upstream/main`"：会话中上游两次前进（`bc2244c83` → `9b3c96dc6` → `c2c19a7cb`），每次都保存无提交 tree 后中止旧 merge、重新合并新目标并恢复冲突解，再核对"新快照相对旧快照的差异正好等于上游新增增量"（最后一次为 16 files `+262/-33`）。收尾再 fetch 确认 `upstream/main = MERGE_HEAD`。
- 用户范围决定：只合并并保留本地 features；**上游原生问题等待远端修复，不在本分支改生产代码**。执行中曾做过 SSRF、批量限额和 Prompt Metrics 的主动补丁，按该决定全部撤销，并核对 `image_storage*` 与 `MERGE_HEAD` 一致、batch-limit 生产文件与 merge-tree 一致。
- `concurrency=0` 继续表示不限并发。

## 2. 冲突与处理

| 文件 | 双方意图 | 处理 |
| --- | --- | --- |
| `backend/cmd/server/wire_gen.go` | 上游新增 Audit/StepUp/AsyncImage/BillingProbe DI；本地有 PromptMetrics、TokenAnalysis、OrganizationUsage、RequestIntercept、并发 preset、quota flusher | 不选 ours/theirs，从合并后的双方 provider 源图重新生成；连续两次生成 SHA-256 一致（`92D6F616...12A61`） |
| `backend/internal/server/http.go` | 上游增加 audit/step-up 注入；本地保留 prompt metrics 等依赖 | 组合构造参数并传入 `SetupRouter` |
| `backend/internal/server/router.go` | 上游 Audit/StepUp/SessionBinding；本地 Prompt Metrics 管理路由 | 保留本地入口与上游路由注册，叠加 0.1.159 `SessionBindingContext(cfg)` 客户端 IP 口径 |
| `backend/internal/server/routes/gateway.go` | 上游 async image/task 与 API Key billing；本地 RequestArchive/RequestIntercept | 所有新增入口继续经过本地 archive/intercept 链 |
| `backend/internal/server/routes/gateway_test.go` | 双方各自扩展路由与 middleware 断言 | 合并路由清单与本地链路断言 |
| `backend/internal/service/openai_gateway_forward.go` | 上游重构 failover helper；本地 response header clone、413 换账号、`thinking` 清理 | 采用上游 helper 结构，保留本地错误语义 |
| `frontend/src/api/admin/index.ts` | 上游 Audit API；本地 Token Analysis、组织用量等导出 | 导出并集 |
| `frontend/src/components/layout/AppSidebar.vue` | 上游 Audit Logs；本地 Prompt Metrics、RequestIntercept、OrganizationUsage、TokenAnalysis | 入口并集，保留 simple-mode gate |
| `frontend/src/i18n/locales/en/admin/index.ts`、`zh/admin/index.ts` | 上游 Audit locale；本地功能 locale | key 并集 |

接口兼容：上游 repository 接口新增 `BatchUpdateLimits`，只在 2 个本地用户并发 preset 测试桩中补实现，不改上游批量限额生产代码。

## 3. 保留的本地能力与语义适配

| 能力 | 决策 | 复核 |
| --- | --- | --- |
| RequestArchive / RequestIntercept | 组合 | 上游 async image、billing、image/video 路由经过本地链路 |
| Prompt Metrics / Prompt Risk | 保留/组合 | 管理入口、payload、合规 guard、采集、LLM judge 与 moderation 组合；不额外改造上游审计链 |
| Token Analysis | 保留 | handler/service/repository/UI/Wire |
| 组织用量 | 保留 | 独立 API、SQL、导出与侧栏入口 |
| 用户并发 preset / `ConcurrencyCacheError` | 保留 | 仅测试桩适配 `BatchUpdateLimits` |
| OpenAI-compatible preset / options / cache usage | 组合 | 与上游新 failover 共同保留 |
| user-platform quota flusher | 保留 | 启动与 shutdown cleanup 在组合 Wire 中 |

未发现可由上游完整替代并安全删除的本地能力；`docs/features/` tracked 删除为 0。

## 4. 采用的上游行为

- 异步图片生成/编辑任务、Redis 状态与 S3-compatible 结果转存。
- 操作审计日志、IP/UA 会话绑定、敏感操作 step-up TOTP。
- API Key 计费倍率自省、OpenAI API Key 上游 billing probe 与倍率感知调度。
- 用户批量 concurrency/RPM 限额：`POST /api/v1/admin/users/batch-limits`，`all=true` 优先并忽略同时传入的 `user_ids`，0 并发为不限。
- 账号/分组/Channel Monitor 复制；图片输入 token 单价、usage token/cost 拆分、订阅币种标签。
- alpha search 恢复 API Key 账号调度、Grok function-tool prompt cache、Stripe SDK 懒加载。
- 统一 API Key ACL、session binding、audit log 的 forwarded client IP 信任开关；账号上游站点安全跳转；migrations 177–181（只新增，Ent 与生成代码一致）。

## 5. 验证

| 命令 / 检查 | 结果 |
| --- | --- |
| `MERGE_HEAD` / unmerged / `git diff --check`（worktree、cached） | `c2c19a7cb`；unmerged 0；通过 |
| Wire 连续两次 `go generate ./cmd/server` | SHA-256 一致 |
| `go test -tags=unit -p 1 -count=1 ./...` | 退出 0 |
| `go test -tags=integration -p 1 -count=1 ./...` | 跑到结尾；5 个 `.test.exe` 被本机 360 在启动前拒绝：`tlsfingerprint`、`urlvalidator` 换全新 `GOTMPDIR` 后通过，`anthropicfp`、`antigravity`、`logredact` 编译到系统临时目录直接执行通过 |
| `golangci-lint run --new-from-rev HEAD ./...` | 0 issues |
| `go mod tidy -diff` | 非零，只建议删除 12 行继承的旧 `go.sum` 校验项；按"只合并"不整理 |
| `pnpm run lint:check`、`typecheck`、`build` | 通过；build 961 modules，仅既有 chunk/dynamic import warning |
| `pnpm exec vitest run` | 188 files / 1300 tests 通过；1 个上游 suite 无法收集（见第 6 节），退出 1 |
| `go build -tags embed -trimpath ./cmd/server` | 通过，产物 148,425,728 bytes |
| staged 门禁 | 无 unmerged/unstaged/untracked/冲突标记/`docs/features` 删除；唯一 token-like 命中是上游审计脱敏单测的合成 fixture |

## 6. 遗留问题（上游原生，合并时只记录）

以下问题均可在 `MERGE_HEAD` 原始文件中复现，非冲突解决引入。

| 级别 | 问题 | 上游路径 | 当前状态（HEAD `26f6363af`） |
| --- | --- | --- | --- |
| P1 | 异步图片结果 URL 用默认 HTTP client 下载，无私网/metadata/DNS rebinding/redirect 防护 | `backend/internal/service/image_storage.go` | 已由上游 `ab71e12ec`（随 0.2.18 合入）修复：下载走自定义 dialer 并用 `urlvalidator.IsBlockedHost` 拦截 |
| P2 | async image 任务只在进程内 goroutine 执行，重启后 Redis `processing` 记录可能悬挂到 24h TTL | `handler/image_task_handler.go`、`service/image_task.go` | 材料中无后续结论，未复核 |
| P2 | 批量限额 cache 删除底层吞掉 Redis 错误；全量 user ID 展开查询有 PostgreSQL bind 参数上限风险 | `api_key_auth_cache_impl.go`、`api_key_repo.go`、`admin_user.go` | 材料中无后续结论，未复核 |
| P2 | locale compile 测试直接导入未声明的 `@intlify/message-compiler`，pnpm 严格链接下 suite 无法收集 | `frontend/src/i18n/__tests__/localesMessageCompile.spec.ts`、`frontend/package.json` | 已由上游在 0.1.160 声明依赖（`frontend/package.json` 现有 `@intlify/message-compiler`） |
