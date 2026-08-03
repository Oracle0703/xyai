# 合并上游 0.1.170 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于本地 `main@12770bc5da8c0ef6a2dd02b0e80a37f6eb26d408`，将 `Wei-Shaw/sub2api main@7e2e9ba05026b7126318aa0754c1afa0ac00bc58` 合并到 `feature/hy/10170_merge_upstream_v170`，仅解决合并冲突并在提交前交由用户审核。

**Architecture:** 使用普通双亲 merge 保留本地与上游历史。冲突按三方语义分析：不重叠的本地功能继续保留；与上游能力重叠时采用上游实现；不对上游自动合入代码做缺陷修复。

**Tech Stack:** Git、Go 1.26.5、Gin、Ent、Vue 3、TypeScript、pnpm 9.15.9、Vitest。

## Global Constraints

- 目标分支固定为 `feature/hy/10170_merge_upstream_v170`。
- 上游目标固定为 `7e2e9ba05026b7126318aa0754c1afa0ac00bc58`，不得使用后续 `upstream/main`。
- 仅处理 Git 冲突及冲突解决所必需的接口一致性，不修复上游自身缺陷。
- `docs/features/` 中本地新增能力不得因合并被删除；功能重叠时采用上游实现。
- 必须追加 `docs/features/sub2api -merage-list.md`，并更新受影响的 `llm-wiki/wiki/*.md`。
- 完成验证后保留 `MERGE_HEAD`，不得执行 `git commit`，等待用户审核。

---

### Task 1: 固定基线与目标提交

**Files:**
- Inspect: `AGENTS.md`
- Inspect: `llm-wiki/wiki/*.md`
- Inspect: `Makefile`
- Inspect: `backend/Makefile`
- Inspect: `frontend/package.json`

**Interfaces:**
- Consumes: 本地 `main@12770bc5da8c0ef6a2dd02b0e80a37f6eb26d408`。
- Produces: 可复核的合并前测试结果、固定的上游提交对象和变更清单。

- [x] **Step 1: 验证工作区和分支基线**

  Run: `git status --short --branch`

  Expected: 工作区无修改，分支为本地 `main`。

- [x] **Step 2: 从本地 main 创建工作分支**

  Run: `git checkout -b feature/hy/10170_merge_upstream_v170 main`

  Expected: 新分支 HEAD 为 `12770bc5da8c0ef6a2dd02b0e80a37f6eb26d408`。

- [x] **Step 3: 运行合并前验证**

  Run: `go test ./...`（工作目录 `backend`，使用仓库要求的 Go 工具链）

  Run: `go test -tags=unit -p 1 -count=1 ./...`（工作目录 `backend`）

  Run: `corepack pnpm@9.15.9 --dir frontend run lint:check`

  Run: `corepack pnpm@9.15.9 --dir frontend run typecheck`

  Run: `corepack pnpm@9.15.9 --dir frontend exec vitest run`

  Evidence: 默认后端测试、前端 lint/typecheck 和完整 Vitest 退出码均为 0；带 `unit` tag 的后端测试仅有既有 `GET /api/v1/auth/me` 合同断言失败，原因是本地响应多出 `admin_permissions`；Vitest 存在既有 `GroupsView` mock 未处理 rejection，但测试进程退出码为 0。

- [x] **Step 4: 获取并验证固定上游提交**

  Run: `git fetch upstream main`

  Run: `git cat-file -e 7e2e9ba05026b7126318aa0754c1afa0ac00bc58^{commit}`

  Run: `git merge-base HEAD 7e2e9ba05026b7126318aa0754c1afa0ac00bc58`

  Evidence: 固定提交存在且 `backend/cmd/server/VERSION=0.1.170`；merge base 为 `5a6143097db142b72a6fc848c214e97214470bdd`。fetch 后 `upstream/main` 已推进到 `954d44c19c24cb27df7f593349c5bf8a3dd99aa2`，本轮仍只合入固定目标。

### Task 2: 审查上游增量与本地功能

**Files:**
- Inspect: `docs/features/**`
- Inspect: `backend/**`
- Inspect: `frontend/**`
- Inspect: `deploy/**`

**Interfaces:**
- Consumes: merge base、本地 HEAD、固定上游提交。
- Produces: 上游增量摘要、双方修改文件清单、本地 features 保留清单。

- [x] **Step 1: 生成三方变更集合**

  Run: `git diff --name-status <merge-base>..HEAD`

  Run: `git diff --name-status <merge-base>..7e2e9ba05026b7126318aa0754c1afa0ac00bc58`

  Evidence: 本地修改 463 路径、上游修改 294 路径；only-local 411、only-upstream 242、both 52。高风险交集集中在 Wire/Ent 生成文件、配置、网关路由、内容审核、OpenAI 转发/usage 和管理前端。

- [x] **Step 2: 建立 features 保留基线**

  Run: `git ls-tree -r --name-only HEAD -- docs/features`

  Evidence: 本地 `docs/features` 有 23 个 tracked 文件，目标上游该目录为空；23 个文件全部列入合并后零删除核对。

- [x] **Step 3: 阅读上游提交历史和关键 diff**

  Run: `git log --oneline --no-merges <merge-base>..7e2e9ba05026b7126318aa0754c1afa0ac00bc58`

  Evidence: 0.1.169 增量 72 文件、0.1.170 增量 242 文件；整体 294 文件、`+16749/-1179`。已按架构、配置、数据、安全、计费、网关和前端归类，未修改自动合入代码。

### Task 3: 执行合并并解决冲突

**Files:**
- Modify: Git 报告为 unmerged 的全部路径。
- Preserve: 不重叠的本地新增功能路径。

**Interfaces:**
- Consumes: Task 2 的三方差异和保留清单。
- Produces: 无未合并索引项、保留 `MERGE_HEAD` 的待提交 merge 工作树。

- [x] **Step 1: 执行非提交 merge**

  Run: `git merge --no-ff --no-commit 7e2e9ba05026b7126318aa0754c1afa0ac00bc58`

  Expected: 自动合入全部无冲突文件，冲突路径保留三方 stage。

- [x] **Step 2: 逐个冲突做三方语义分析**

  Run: `git ls-files -u`

  Run: `git show :1:<path>` / `git show :2:<path>` / `git show :3:<path>`

  Expected: 对每个冲突记录 base/local/upstream 意图；重叠功能取 upstream，不重叠本地能力做语义并集。

- [x] **Step 3: 清除全部冲突标记并暂存解决结果**

  Run: `rg -n "^(<<<<<<<|=======|>>>>>>>)" --glob '!docs/superpowers/plans/*'`

  Run: `git diff --check`

  Run: `git diff --name-only --diff-filter=U`

  Expected: 无冲突标记、无空白错误、无 unmerged 路径。

### Task 4: 更新合并知识与验证证据

**Files:**
- Modify: `docs/features/sub2api -merage-list.md`
- Modify: `llm-wiki/wiki/README.md`
- Modify: `llm-wiki/wiki/ops.md`
- Modify: 由上游增量实际影响的其他 `llm-wiki/wiki/*.md`

**Interfaces:**
- Consumes: 最终冲突清单、上游能力摘要、测试结果。
- Produces: 追加型合并记录和与源码一致的稳定 wiki 基线。

- [x] **Step 1: 追加合并记录**

  Expected: 包含日期、工作分支、上游分支/提交、待创建的 merge commit、冲突文件、处理方式和验证结果；不覆盖历史条目。

- [x] **Step 2: 更新 wiki**

  Expected: 把 0.1.170 的稳定架构、配置、路由、数据和安全变化写入对应页面，并保持 LF。

- [x] **Step 3: 刷新并检查 wiki 图谱**

  Run: `tools\refresh-understand-wiki.cmd`

  Run: `tools\check-understand-status.cmd`

  Expected: 图谱可解析；工作树未提交时允许状态为 PARTIAL，但不得报告损坏。

### Task 5: 完整验证与提交前审核

**Files:**
- Inspect: 全部 staged merge diff。

**Interfaces:**
- Consumes: 已解决冲突的 merge 工作树与更新后的文档。
- Produces: 完整验证证据和供用户审核的预提交状态。

- [x] **Step 1: 运行后端测试和构建**

  Run: `go test -count=1 ./...`（工作目录 `backend`）

  Run: `go test -tags=integration -count=1 ./...`（工作目录 `backend`）

  Run: `go build -o NUL ./cmd/server`（工作目录 `backend`）

  Evidence: 修订后的 staged snapshot 上三条命令均退出 0。此前完整 unit tag 仅有第一父既有 `/auth/me` fixture mismatch，Go lint 28 项均为第一父既有；两类既有问题按约束未修复。

- [x] **Step 2: 运行前端完整检查和构建**

  Run: `corepack pnpm@9.15.9 --dir frontend run lint:check`

  Run: `corepack pnpm@9.15.9 --dir frontend run typecheck`

  Run: `corepack pnpm@9.15.9 run test:run`（工作目录 `frontend`）

  Run: `corepack pnpm@9.15.9 --dir frontend run build`

  Evidence: lint、typecheck 和 production build 均退出 0，Vite 转换 1017 modules。完整 Vitest 为 1567/1569，2 条失败均是第一父既有 rollback timeout 参数断言；另有已审计的 10 个第一父既有 `GroupsView` mock unhandled rejection，均按约束未修复。

- [x] **Step 3: 审计 merge 拓扑和 features 保留情况**

  Run: `git diff --cached --stat`

  Run: `git diff --cached --name-status`

  Run: `git diff --cached --diff-filter=D -- docs/features`

  Run: `git rev-parse MERGE_HEAD`

  Evidence: 最终 staged snapshot 为 307 files、`+17197/-1229`；`MERGE_HEAD` 精确等于固定上游提交，0 unmerged、0 unstaged、0 untracked、0 whitespace error，新增 diff 中无冲突标记；23/23 个第一父 `docs/features` 路径保留且零删除。

- [x] **Step 4: 停在 commit 前通知用户审核**

  Run: `git status --short --branch`

  Evidence: 最终 scoped re-review 为 `APPROVED`，新 Critical/Important 为 0；保持 merge 待提交状态，未运行 `git commit`、push 或创建 PR，本轮在此通知用户审核。
