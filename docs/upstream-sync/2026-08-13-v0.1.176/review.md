# Sub2API 0.1.176 上游同步记录（固定提交）

合并日期 2026-08-13 ｜ 工作分支 `feature/hy/10176_merge_sub2api_176`（自本地 `main@cf5b7ee5a15cd04bb6134a372d20e86534660b93` 创建，未 pull/rebase）｜ 上游 `Wei-Shaw/sub2api main@0e82efe48951cb7da1f8554639afdeab05bf16b8`（VERSION 0.1.176，merge base `48eb3766d2da` = 上游 0.1.173）｜ 合并提交 `e2d3a4984087e69ae778b1237dfcc14bca9f1749`（父 `cf5b7ee5a` + `0e82efe48`）｜ 状态：经 PR #40（`65bd97fe3`，2026-08-13）合入 main

本文是该轮同步的唯一记录。执行计划 `docs/superpowers/plans/2026-08-13-sub2api-v176-upstream-merge.md` 中仍有价值的决策与验证结果已并入本文，计划已移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-08-13 main sync (v0.1.176; awaiting review)` 节（"待用户审核，尚未创建"是提交前快照，实际合并提交见上）。

## 1. 范围

- merge base 到固定上游 107 commits、194 paths（`+10986 / -667`）；固定提交自身 VERSION 已是 `0.1.176`，不以更晚的 `upstream/main@fbfdcef81...`、tag 或中间提交替代。
- 合并前用 `git merge-tree --write-tree --messages main 0e82efe4...` 预演；三方路径集合 local 513、upstream 194、both 43（4 个文本冲突 + 39 个自动合并交叠），按网关/计费/安全、DTO/仓储/Wire/Ent、前端合同分组审查。
- 约束：只解决冲突与合并必需的兼容，不修上游 bug；功能完全重叠时采用上游；Ent/Wire 从合并源重新生成并证明二次生成无漂移。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 保留本地拆分后的 `ResponsesToChatCompletionsRequestWithOptions` 实现，把上游 `x_search` tool-choice 声明检查迁入现行文件，不恢复重复函数 |
| `backend/internal/service/openai_gateway_response_handling.go` | usage 解析同时保留上游 Cline `data.*` candidate 与本地 compatible cache 互斥计费桶归一化 |
| `backend/internal/service/openai_gateway_chat_completions_raw_test.go` | 本地 cache usage 测试与上游 Grok missing-usage guard 测试并存；本地测试遗漏的两个 `account` 参数按上游新签名适配 |
| `frontend/src/views/admin/__tests__/UsageView.spec.ts` | stub 同时声明上游 `columns` 与本地 `userClickable` |

自动合并交叠中的判断：
- `openai_silent_refusal.go` 的 reasoning-only failover 是本地独立功能，与上游本轮 empty `response.completed` failover 并存，不按重叠功能删除。
- 最终审查发现上游新增根级 `POST /x_search` 未显式挂本地 `RequestArchive -> RequestIntercept`（`/v1/x_search` 继承 group middleware，不受影响）。按既有"根级 alias 必须显式挂本地横切中间件"的规则补齐两项，并新增实际 Gin handler-chain 顺序回归测试（先因缺 `RequestArchive` 失败，补线后通过）。这是保留本地横切能力的合并适配，不改上游搜索业务。当前源码的 `rootRoute` helper 统一挂 `requestArchive, requestIntercept`。

## 3. 保留的本地能力与语义适配

合并前 24 个 tracked `docs/features/` 文件 24/24 保留。RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、OpenAI-compatible cache usage、默认 reasoning effort、用户并发 preset、quota flusher 和 reasoning-only failover 均保留；真正重叠处采用上游实现。

## 4. 采用的上游行为

- 分组逐模型定价与长上下文开关（migration 221）。
- Grok 4.6 / JWT 订阅档位，未知文本模型价卡回落；原生 `/x_search`；Realtime 音频计费。
- 上游响应模型安全计费；备份分卷与定时备份 leader lock。
- Codex 指纹收敛；OpenAI pool auth / usage / empty-completed / WS audit 加固。
- 管理用量 Request ID 列。

## 5. 验证

| 命令 / 检查（`backend` 用仓库内 Go 缓存 + 全新 `GOTMPDIR`；前端 pnpm 9.15.9） | 结果 |
| --- | --- |
| Ent / Wire 生成 + 二次生成 | 成功，无漂移 |
| 4 组 0.1.176 专项（Responses/x_search/cache；Grok/JWT/Realtime；group pricing/migration 221；backup/leader lock） | 通过 |
| `go test -p 1 -count=1 ./...`（默认 tag，含 `/x_search` 适配后重跑） | 退出 0 |
| `go test -tags=integration -p 1 -count=1 ./...` | 退出 0 |
| `go build ./...`、`go build -tags embed ./...` | 退出 0 |
| `go test -tags=unit -p 1 -count=1 ./...` | 除第一父 `/auth/me` contract（`admin_permissions:null`）外通过；`TestGrokQuotaServiceQueryQuotaFreeFallsBackToGrok45` 在一次全 service 运行中失败，同一最终代码精确重跑通过，实现/测试与固定上游 blob 一致，记为并行测试波动 |
| `golangci-lint run ./...` | 29 项历史债务 |
| `golangci-lint run --new-from-rev=HEAD ./...` | 0 issues |
| 前端 lint、typecheck | 通过 |
| 前端专项 / 完整 Vitest | 5 files / 46 tests；完整 636 suites / 1740 tests 通过 |
| 前端 production build | 通过，1052 modules |
| Git 审计 | 冲突标记、unmerged、whitespace、upstream-only blob 一致性、local-only blob 保留、`docs/features` 删除均检查通过 |
| Wiki 图谱 | 33 nodes / 67 edges、49 wikilinks、0 unresolved；未提交时为预期 `PARTIAL` |

## 6. 遗留问题

- `/auth/me` contract 基线失败与全量 lint 29 项历史债务保持不改（lint 见 `docs/features/technical-debt-board-cn.md` TD-004）。
- `TestGrokQuotaServiceQueryQuotaFreeFallsBackToGrok45` 存在并行运行波动，未改上游代码。
