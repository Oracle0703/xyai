# Sub2API 0.1.170 上游同步记录（固定提交）

合并日期 2026-08-03 ｜ 工作分支 `feature/hy/10170_merge_upstream_v170`（自本地 `main@12770bc5da8c0ef6a2dd02b0e80a37f6eb26d408` 创建）｜ 上游 `Wei-Shaw/sub2api main@7e2e9ba05026b7126318aa0754c1afa0ac00bc58`（VERSION 0.1.170，merge base `5a6143097db1`）｜ 合并提交 `17ef8727c2317920ff10fbd99610f857db12d61c`（父 `12770bc5d` + `7e2e9ba05`）｜ 状态：经 PR #33（`7a537cffb`，2026-08-03）合入 main

本文是该轮同步的唯一记录。执行计划 `docs/superpowers/plans/2026-08-03-merge-upstream-v170.md` 中仍有价值的决策与验证结果已并入本文，计划已移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-08-03 main sync (v0.1.170; merged via PR #33)` 节；该节"Merge commit"一栏写的 `7a537cff` 是 PR #33 合入 main 的提交，真正的上游合并提交是 `17ef8727c`。

## 1. 范围

- 上游 99 commits、294 files（`+16749 / -1179`），其中 0.1.169 增量 72 files、0.1.170 增量 242 files；固定提交自身 VERSION 已是 `0.1.170`。fetch 时 `upstream/main` 已到 `954d44c19c24`，不跟随。
- 三方路径集合：本地修改 463、上游修改 294；only-local 411、only-upstream 242、both 52。6 个文本冲突；24 个后端网关和 22 个外围自动合并路径逐路径三方审查，未发现可定位的自动合并缺陷。
- 提交前 staged 快照 307 files（`+17197 / -1229`）。
- 约束：只处理 Git 冲突及其必需的接口一致性，不修复上游自身缺陷；`docs/features/` 本地能力不得删除（23 个 tracked 文件，上游该目录为空）；功能重叠时采用上游实现。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/internal/service/content_moderation.go` | 叠加上游内容审核代理能力，保留本地 Prompt Risk / LLM judge |
| `backend/cmd/server/wire_gen.go` | 为 Content Moderation 注入 `ProxyRepository`；4 个本地测试调用补 `nil` 参数 |
| `backend/internal/server/routes/gateway.go` | 同时保留本地 RequestArchive/RequestIntercept 与上游 path guard / profit-control；**path guard 必须位于 `RequestIntercept` 之前** |
| `backend/internal/server/routes/gateway_test.go` | 合并双方路由与链路断言 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 同时保留本地兼容映射与上游 media tool-output |
| `frontend/src/views/admin/RiskControlView.vue` | 保留本地 Prompt Risk 面板并接入上游代理选择 |

最终审查后续修复：本地 `RequestIntercept` 与 Gemini wildcard 路由组合会让 path guard 被短路。修复后 `/v1beta/models/*modelAction` 与 `/antigravity/v1beta/models/*modelAction` 先执行共用的 model/action 解析、action allowlist 与安全 model segment 校验。

## 3. 保留的本地能力与语义适配

23 个合并前 tracked `docs/features/` 文件零删除。RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、OpenAI-compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher 均保留；功能重叠处采用上游实现。

## 4. 采用的上游行为

- 分组利润控制（`backend/migrations/192_group_profit_control.sql`、`193_group_profit_control_auth_cache_invalidation.sql`）。
- 跨平台 API Key 上游账单探测与倍率同步；账号跨页全选/批量删除。
- 内容审核代理（状态与 1 分钟缓存 TTL 的真实语义已同步写入安全 wiki）。
- Responses/Gemini 上游 path allowlist；OAuth Responses namespace opt-in 摊平；compact home。
- 订阅配额窗口到期对齐；proxy stream circuit 禁用开关；容器部署加固。

## 5. 验证

| 命令 / 检查（`backend` / `frontend`） | 结果 |
| --- | --- |
| 合并前基线：默认 tag `go test ./...`、前端 lint/typecheck/Vitest | 退出 0；unit tag 仅 `/auth/me` fixture 既有失败；Vitest 有既有 `GroupsView` mock unhandled rejection 但退出 0 |
| `go generate ./ent`、`go generate ./cmd/server` | 退出 0，`backend/ent` 与 `wire_gen.go` 无漂移 |
| `go test -p 1 -count=1 ./...`（默认 tag） | 退出 0 |
| `go test -tags=integration -p 1 -count=1 ./...` | 退出 0 |
| `go build ./cmd/server` | 退出 0 |
| `go test -tags=unit -p 1 -count=1 ./...` | 退出 1，仅 `/auth/me` fixture 比实际响应少 `admin_permissions:null`，与第一父一致 |
| `golangci-lint run ./...` | 退出 1，28 项经 blob/引用审计均为第一父既有 |
| 增量 lint | golangci-lint v2.9.0 在仓库根/Go module 组合下报 `no go files to analyze`，**不记为 0 issues** |
| 前端 lint、typecheck、build | 退出 0；Vite 1017 modules |
| 前端完整 Vitest | 退出 1：1567/1569 通过，2 条失败为第一父既有 rollback timeout 参数断言；另有 10 个第一父既有 `GroupsView` mock unhandled rejection |
| deploy fixture（2 个） | Git Bash login shell 下退出 0；直接调用 Bash 因 PATH 缺 `dirname` 未启动 |
| 冲突/diff 审计 | `git ls-files -u` 为空，无冲突标记与 whitespace error；23/23 `docs/features` 保留 |
| Wiki 图谱 | 刷新为 33 nodes / 67 edges、49 wikilinks、0 unresolved；未提交工作树下状态为预期 `PARTIAL` |

最终 scoped re-review 结论 `APPROVED`：新 Critical/Important 为 0。

## 6. 遗留问题

- `rawOpenAIResponsesRequestPathSuffix` 用 `strings.LastIndex(normalizedPath, "/responses")` 截取后缀，存在路径重锚风险；该实现同时存在于第一父和固定上游，按"不修上游/既有问题"约束未改。当前源码 `backend/internal/service/openai_gateway_request_body.go` 仍是该实现。
- 已知基线失败（`/auth/me` fixture、28 项 lint、2 条 rollback timeout 断言、`GroupsView` mock rejection）按用户约束未修复。lint 债务见 `docs/features/technical-debt-board-cn.md` TD-004。
