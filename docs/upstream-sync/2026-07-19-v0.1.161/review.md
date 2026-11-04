# Sub2API 0.1.161 上游同步记录（固定提交）

合并日期 2026-07-19 ｜ 工作分支 `feature/hy/10161_合并1.161版本`（自本地 `main@332fdbd0b84619cfb1da6fcb57b65d4d9263b2e9` 创建）｜ 上游 `Wei-Shaw/sub2api main@d4b9797ff72024960a035cf22fdd8f213e149169`（VERSION 0.1.161，merge base `57914967cbb1`）｜ 合并提交 `e3e6b52da43a5be351cf59089976759eebc28376`（父 `332fdbd0b` + `d4b9797ff`）｜ 状态：经 PR #25（`e52b5c89d`，2026-07-19）合入 main

本文是该轮同步的唯一记录。原始材料 `docs/upstream-sync/reviews/2026-07-19-sub2api-v0.1.161-merge-review.md`、`docs/upstream-sync/reviews/2026-07-19-sub2api-v0.1.161-conflict-resolution-audit.md` 与执行计划 `docs/superpowers/plans/2026-07-19-sub2api-v0.1.161-sync.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-19 main sync (v0.1.161; committed)` 节。

## 1. 范围

- 上游 62 commits、257 files（`+13643 / -1523`）；提交前 staged 快照 267 files（`+14033 / -1540`）。
- 边界：tag `v0.1.161` 指向 `19149ca196ee`，VERSION 仍为 `0.1.160`，因此固定其后的 version-sync 提交 `d4b9797ff`；收尾 fetch 确认 `upstream/main = MERGE_HEAD`。
- 4 个文本冲突、43 个双方修改文件（`57914967c..main` 与 `57914967c..d4b9797ff` 文件集合求交）逐一三方审查；合并后另有独立复核：用 `git merge-tree` 重放确认冲突集合，并对 43 个文件做"ours 独有行 / theirs 独有行是否仍在结果中"扫描。
- 用户边界：只解决冲突和保留本地功能所需兼容，不修复上游或本地基线自身缺陷。

## 2. 冲突与处理

| 文件 | 本地意图 | 上游意图 | 处理 |
| --- | --- | --- | --- |
| `backend/cmd/server/wire_gen.go` | Redis 7+ 校验错误返回、Prompt Metrics、Token Analysis、组织用量、用户并发 preset、RequestIntercept、quota flusher、本地 moderation provider 及完整 cleanup | Auth cache invalidation outbox/worker、Ops ingress reject aggregator、UserHandler/SettingHandler 新依赖、Prompt Audit 生命周期 | 以合并后的 provider 源图重新生成；`ProvideRedis` 保留 `err` 返回，`provideCleanup(...)` 同时包含双方 runner/worker；连续两次生成 SHA-256 一致（`5F2930F8...3237AC`），相对 index 无漂移 |
| `backend/internal/server/routes/gateway.go` | 所有网关路由挂 RequestArchive / RequestIntercept | `textBodyLimit`（`TextMaxBodySize`）、Grok `/videos/:request_id/content` 同源代理 | 保留本地 middleware 顺序；`textBodyLimit` 用于 `/alpha/search`、`/embeddings` 等上游指定入口；接入 `videoContentHandler`。根路径 content 路由也挂了 archive/intercept，二者对非 POST 直接跳过，无害但略冗余 |
| `frontend/src/App.vue` | 子管理员 `admin-permission-denied` 事件、权限刷新、退出与路由恢复、`canAccessAdmin` 判断 | 统一用 `utils/branding.ts` 的 `updateFavicon` 更新标题和 favicon | 采用上游 branding helper，完整保留子管理员恢复流程 |
| `frontend/src/components/account/CreateAccountModal.vue` | OpenAI-compatible provider preset、base URL、placeholder、credentials、endpoint capability | OpenAI API Key 自动 upstream billing probe 开关（`upstreamBillingAutoProbeEnabled`）、payload、创建后首次 `probeUpstreamBilling` 与失败 warning | UI、state、reset、submit payload 并集；自动 probe 不覆盖 compatible preset |

自动合并交集抽查结论：

| 文件/面 | 结论 |
| --- | --- |
| `cmd/server/wire.go`、`service/wire.go`、`handler/wire.go`、`repository/wire.go` | 双方 provider 并存 |
| `config.go` / defaults | 双方 mapstructure 与 `SetDefault` 无缺失；本地 reasoning effort、archive/intercept 与上游 ingress、step-up、cache 设置并存 |
| `settings_view.go`、`dto/settings.go`、前端 types、`SettingsView.vue`、i18n、settings API | 字段并集完整 |
| `http.go` / `router.go` | 上游 `MaxHeaderBytes` + ingress recorder，本地 Prompt Metrics middleware/routes |
| `routes/admin.go` | 上游 ingress/auth-cache 健康路由与 backup restore step-up；本地 Token Analysis / RequestIntercept / 并发 preset / 组织用量保留 |
| `openai_gateway_{chat_completions,forward,passthrough,cc_pipeline}.go` | 上游 stream read error / shouldDisable failover 进入；本地默认 reasoning effort 仍在 |
| `openai_gateway_handler.go` | 相对上游少掉的 handler 内 `isSafeRetryAfter` 是本地既有重构，统一用 `service.IsSafeRetryAfter` |
| `api_key_repo.go` / `user_repo.go` | 采用上游不再落明文 deleted-audit 与 allowed-groups 增量同步；保留本地 `AdminPermissions` 映射与读写 |
| `vite.config.ts` | 保留本地去掉 `vite-plugin-checker`，合入上游 branding 注入 |

没有发现 4 个文本冲突之外需要修改的语义冲突，也没有借交集清理或重构上游代码。复核结论：文本冲突选边错误 0，自动合并丢变更 0，不建议回滚。

## 3. 保留的本地能力与语义适配

| 本地能力 | 决策 | 说明 |
| --- | --- | --- |
| RequestArchive / RequestIntercept | 组合 | 冲突路由保留 middleware 并接入上游新增入口 |
| Prompt Metrics | 保留 | middleware、管理入口、生命周期未被上游 Ops 能力替代 |
| Prompt Risk / LLM judge | 保留 | 仍走本地 `ProvideContentModerationService`，规则、judge、fail-open、防递归合同存在 |
| Token Analysis | 保留 | handler/service/repository/UI 与后台索引 |
| 组织用量 | 保留 | API、聚合、导出、页面 |
| 图片生成与支付 | 组合 | 采用上游 Grok 受保护视频增量 |
| 用户并发 preset / `ConcurrencyCacheError` | 保留 | runner、缓存错误、503 合同 |
| OpenAI-compatible preset / options / cache usage | 组合 | 与上游自动 billing probe、兼容桥增量独立生效 |
| 默认 reasoning effort | 保留 | 配置默认值与注入路径 |
| user-platform quota flusher | 保留 | dirty snapshot、批量落库、shutdown |
| 子管理员权限 | 保留 | DB 权威权限、Gin 路由模板白名单、侧栏过滤、拒绝恢复 |

"上游完整替代"项为 0；`docs/features/` tracked 删除为 0。

## 4. 采用与未采用的上游行为

| 领域 | 内容 |
| --- | --- |
| HTTP ingress | header 读取超时/大小基线、纯文本 body 上限、拒绝原因有界聚合与 Ops 健康查询 |
| API Key 鉴权缓存 | migration 184 增加只存 key SHA-256 的 durable invalidation outbox；worker、Redis subscriber 与二次失效让多实例 L1/L2 收敛 |
| 安全开关 | session binding 与 step-up 默认关闭；关闭 step-up 需二次验证；旧客户端省略字段保持现值 |
| Grok media | 生成请求绑定原 user/API key 与实际账号；status/content 强制所有权并由网关代理签名内容 |
| 调度 | 已知模型时临时不可调度规则只隔离 `(account, model)`；401 或未知模型保留账号级语义 |
| OpenAI WS / Responses | passthrough lifecycle、读错误与 Anthropic/Responses 流式兼容修复 |
| 前端 / 部署 | 首屏与运行时 branding 统一；Docker cross-build、Caddy/Redis edge 配置 |
| 安全变更（采用） | 删除 API Key 不再把明文 key 复制到 deleted audit，并删除旧明文归因测试 `ops_repo_lookup_deleted_key_audit_integration_test.go`；不恢复旧测试 |

新增管理路由授权决策（未扩权）：上游新增 `GET /api/v1/admin/ops/ingress-rejections`、`GET /api/v1/admin/ops/ingress-rejections/health`、`GET /api/v1/admin/ops/auth-cache-invalidation/health`。本地 `admin_auth.go` 对 `sub_admin` 走 `service.CanAccessAdminRoute`，未知 `METHOD + FullPath` 默认拒绝；`AdminPermissionUsage` 只放行 `/ops/errors*`、`/ops/request-errors*`、`/ops/upstream-errors*` 只读接口。这三条接口包含 `client_ip`、`user_id`、`api_key_id` 及 outbox/Redis subscriber/limiter 安全运行态，前端也未调用，因此保持仅完整管理员可访问。当前源码 `backend/internal/service/admin_permission.go` 仍未放行它们。

## 5. 验证

| 命令 / 检查 | 结果 |
| --- | --- |
| 后端 6 个高风险包定向 | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 跑到结尾退出 1：仅 `internal/server` 的 `TestAPIContracts/GET_/api/v1/auth/me` 失败，实际比 golden 多 `"admin_permissions": null`。本地 main 既有问题（`dto.User` 字段无 `omitempty`、`UserFromService` 已映射），本轮 `git diff --cached HEAD -G "admin_permissions|AdminPermissions"` 无生产映射改动 |
| `go test -tags=integration -p 1 -count=1 ./...` | 退出 0 |
| 全量 `golangci-lint run ./...` | 28 条本地 main 既有债务（15 errcheck、1 gofmt、3 staticcheck、9 unused） |
| `golangci-lint run --new-from-rev main ./...` | 0 issues |
| `go mod tidy -diff` | 退出 1，只建议删除 6 组 12 行 Wire CLI 传递校验和，均存在于本地 main，未改模块文件 |
| 前端定向 | 5 files / 27 tests 通过 |
| 前端 lint、typecheck、全量 Vitest、build | 通过；200 files / 1373 tests；build 981 modules / 17.81s（Browserslist、mixed import、大 chunk warning 不影响退出码） |
| embed build | 通过，产物 149,794,304 bytes |
| staged 门禁 | 无 unmerged/unstaged/untracked/冲突标记/whitespace/`docs/features` 删除；worktree/cached/HEAD 三种 `diff --check` 退出 0 |

用户审核后明确接受 `/auth/me` contract 失败暂不修复，并授权创建 merge commit。

## 6. 遗留问题

- `/auth/me` `admin_permissions: null` contract mismatch：0.2.6、0.2.7 同步时仍为第一父既有失败；部门功能提交 `a9c53a9f6` 把 `"admin_permissions": null` 写入 `backend/internal/server/api_contract_test.go` golden，随 `273b76c35`（2026-09-28）进入 main，当前源码已不再失配。
- 三条新增 Ops 路由是否向 `admin.usage` 子管理员开放，需要独立的产品与安全决策；若开放，应先确认最小披露范围，再改 `admin_permission.go` 并补权限测试和前端可见性测试。
- `frontend/src/App.vue` 中 `onAdminPermissionDenied` 等 23 行使用 Tab 缩进（文件主体为 2 空格），本地 main 既有，可在独立风格整理中处理。
- root `/embeddings` 未挂 archive/intercept、组织用量部分路由未进子管理员白名单，均为合并前既有策略，非本轮引入。
