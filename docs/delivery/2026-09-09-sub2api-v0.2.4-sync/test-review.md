# 测试与审查

## 测试矩阵

| 验收目标 | 命令 / 检查 | 当前结果 |
|---|---|---|
| exact SHA 与 merge 状态 | Git refs、`MERGE_HEAD`、unmerged index | `HEAD=f6bce5db`；`MERGE_HEAD=98d86915b`；unmerged index 为空 |
| GPT Image 2.5 与 OAuth 生图主控 | 后端专项测试 | 通过 |
| 本地功能保留 | 三方语义审查、24 个 feature 文件清单 | 24/24 保留；两个双方路径语义正确 |
| 后端整体回归 | default/unit/integration、build、tidy、lint | integration/build/lint 通过；default/unit 仅保留已知边界；tidy 为基线差异 |
| 前端整体回归 | lint、typecheck、focused/full Vitest、build | lint/typecheck/focused/build 通过；full Vitest 仅保留 4 个第一父失败 |
| Wiki 与 ledger | 文档 diff、图谱刷新、状态检查 | 六页 wiki 与 ledger 已更新；图谱 READY |

## 规格审查

| 成功标准 | 审查结论 |
|---|---|
| SC-1 / SC-2 | 基于 `main@f6bce5db`，`MERGE_HEAD` 精确指向目标 SHA，unmerged index 为空。 |
| SC-3 | 24/24 feature 文档保留；双方修改文件保留本地独有逻辑，上游重叠实现优先。 |
| SC-4 | 未人工修改业务代码；已知 upstream/first-parent/environment 问题只记录。 |
| SC-5 | 六页 wiki、Wiki 图谱和 append-only ledger 已更新。 |
| SC-6 | 专项、完整测试、build、lint 已执行，结果按真实边界分类。 |
| SC-7 | 当前仍为未提交 merge，未 push、未建 PR、未部署。 |

## 代码审查

| 审查面 | 结论 |
|---|---|
| 文本冲突 | 三方预演与实际 merge 均为 0。 |
| 双方修改 | README 同时保留本地部署/赞助说明与上游图片章节；transform 同时保留本地 reasoning helper 与上游主控模型函数。 |
| 仅上游路径 | 17/17 与 exact target blob 一致，未引入本地修补。 |
| 本地独有功能 | 24 个 `docs/features` 文件零删除；对应本地网关、审计、用量、权限与后台功能未被本轮 19 个路径触碰。 |
| 生成物 | 本轮无 Ent schema、migration、Wire source 或 lifecycle 变化，按 wiki 规则无需 regenerate。 |

## 验证日志

| 日期 | 命令 / 检查 | 结果 |
|---|---|---|
| 2026-09-09 | 远端刷新、SHA、VERSION、merge base、三方 legacy merge-tree | 通过；目标 `0.2.4`，无文本冲突，2 个双方修改路径。 |
| 2026-09-09 | `git merge --no-commit --no-ff 98d86915...` | 通过；自动合并完成并按要求停在 commit 前。 |
| 2026-09-09 | 双方修改与 upstream-only blob 初审 | 两个双方路径保留双方非重叠增量；其余 17 个上游路径与 exact target blob 一致。 |
| 2026-09-09 | `go test -tags=unit -p 1 -count=1 ./internal/pkg/openai ./internal/service -run 'GPTImage25|OpenAIImage|Images|ImageModel|Pricing'` | 通过；两个 package 均为 `ok`。 |
| 2026-09-09 | `go test -p 1 -count=1 ./...` | 除 repository 3 个 Windows 缺 `sh.exe` 失败外其余包通过；`internal/service` 通过。 |
| 2026-09-09 | `go test -tags=unit -p 1 -count=1 ./...` | 除相同 3 个 `sh.exe` 失败和第一父 `/auth/me` golden 差异外其余包通过。 |
| 2026-09-09 | 排除上述 4 个测试后重跑 repository/server | default repository、unit repository、unit server 均通过。 |
| 2026-09-09 | `go test -tags=integration -p 1 -count=1 ./...` | 全量通过。 |
| 2026-09-09 | `go build ./...`；`go build -tags=embed ./...` | 均通过。 |
| 2026-09-09 | `go mod tidy -diff` | 非零；仅输出第一父已有 `go.sum` 旧校验和清理与 `xxh3` 补齐差异，文件未修改。 |
| 2026-09-09 | `golangci-lint v2.13.0 run --new-from-rev=HEAD ./...` | 通过，`0 issues`；PATH 版 v2.9.0 因自身使用 Go 1.26 构建而不兼容，已用 Go 1.27 版替代。 |
| 2026-09-09 | `pnpm --dir frontend run lint:check`；`typecheck` | 均通过。 |
| 2026-09-09 | 模型白名单 focused Vitest | 1 file / 16 tests 通过。 |
| 2026-09-09 | 完整 Vitest | 293/296 files、2161/2165 tests；4 个失败均为未被 0.2.4 修改的第一父测试文件。 |
| 2026-09-09 | `pnpm --dir frontend run build` | 通过；i18n 3/3，1079 modules transformed。 |
| 2026-09-09 | Wiki refresh/status | 33 nodes / 67 edges、49 wikilinks、0 unresolved；`READY`。 |
| 2026-09-09 | 最终 Git 审计 | 34 staged files / `+755/-27`；0 unstaged/untracked/unmerged；24/24 features、0 删除；无冲突标记；cached diff check 通过。 |

## 残余风险

| 风险 | 归因 | 本轮处理 |
|---|---|---|
| 3 个 `backup_pg_dumper` 失败 | Windows PATH 无 `sh.exe`，与第一父一致，相关文件未变。 | 记录；不改实现或测试。 |
| `/api/v1/auth/me` golden 差异 | 第一父实际响应包含 `admin_permissions:null`，golden 未同步。 | 记录；不改合同测试。 |
| 前端完整 Vitest 4 个失败 | Channel Monitor 旧数量断言 1 个；Groups/Subscriptions 测试未安装 Pinia 3 个。 | 记录；相关文件未被 0.2.4 修改。 |
| `go mod tidy -diff` | 第一父已有 go.sum 整理差异，本轮未改依赖。 | 保留固定基线元数据。 |

## PR #6924 补充验证

| 检查 | 结果 |
|---|---|
| GitHub metadata | PR OPEN、MERGEABLE；base `98d86915...`、head `4e5632c3...`；仅 CLA check SUCCESS，未提供完整后端 CI。 |
| 三方与最终审查 | local 520 paths、PR 4 paths、both 0；实际 merge 无冲突，4/4 代码 blob 匹配 PR head；最终 17 files / `+176/-24`，0 unstaged/untracked/unmerged。 |
| UA / identity 专项 | `internal/pkg/openai` 与 `internal/service` 精确过滤测试均通过。 |
| default affected packages | `internal/pkg/openai` 通过；service 初次仅 cyber-policy 既有 1 秒时序用例失败，精确复跑通过，排除该用例后的完整 service 通过。 |
| unit affected packages | 排除已单独通过的 cyber 时序用例后，`internal/pkg/openai` 与 `internal/service` 均通过。 |
| build / lint | `go build ./...`、`go build -tags=embed ./...` 通过；golangci-lint v2.13.0 为 `0 issues`。 |
| module metadata | PR 不改 go.mod/go.sum；`go mod tidy -diff` 仅复现 0.2.4 基线整理差异，未改文件。 |
