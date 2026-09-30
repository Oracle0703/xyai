# sub2api v0.1.155 上游同步 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将固定的 `upstream/main` `7c717365e` 合并到基于本地 `main` 的分支，同时保留上游与本地功能并完成文档和全量验证。

**Architecture:** 使用普通 Git merge 保留双亲历史。先解决 3 个显式冲突，再审查 23 个双方同时修改但可能自动合并的文件，最后通过生成一致性、后端/前端测试、静态检查和构建收口。

**Tech Stack:** Git、Go 1.26.5、Gin、Ent、PostgreSQL、Redis、Vue 3、TypeScript、Vite、Vitest、pnpm 9。

---

| 字段 | 内容 |
|---|---|
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 分支 | `feature/hy/10155_同步sub2api主线` |
| 负责人 | 主会话控制器 |

## 任务清单

| 状态 | 任务 | 负责人 | 文件 / 模块范围 | 验证方式 |
|---|---|---|---|---|
| 已完成 | 保存旧分支并创建目标分支 | 控制器 | Git refs/index | commit 与 branch SHA |
| 已完成 | 合并固定上游并解决冲突 | 控制器 | 全仓库；重点 3 个冲突文件 | `git diff --check`、冲突标记扫描 |
| 已完成 | 审查 23 个双边修改文件和本地关键功能 | 控制器 | 网关、配置、Redis、路由、内容审核、前端账号页 | base/ours/theirs diff、定向测试 |
| 已完成 | 更新 wiki 与 merge ledger | 控制器 | `llm-wiki/wiki/*.md`、`docs/features/sub2api -merage-list.md` | 文档 diff、追加性检查 |
| 已完成 | 生成一致性与完整验证 | 控制器 | backend/frontend | 命令退出码、工作树 diff |
| 已完成 | 最终规格与代码质量复核 | 控制器 | 全部改动 | `test-review.md`、`delivery-report.md` |

## 详细步骤

### 任务 1：合并与冲突解决

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| 合并产生的文件、3 个冲突文件、为恢复双边合同所需的测试 | 与本次合并无关的重构、删除本地功能、改写历史 |

**执行步骤**

- [ ] 执行 `git merge --no-ff upstream/main` 并记录冲突。
- [ ] 分别查看 3 个冲突文件的 base/ours/theirs。
- [ ] 按调用链组合双方逻辑并删除全部冲突标记。
- [ ] 运行 `git diff --check` 和冲突标记扫描。

**命令**

```powershell
git merge --no-ff upstream/main
git diff --name-only --diff-filter=U
git diff --check
rg -n "^(<<<<<<<|=======|>>>>>>>)" --glob '!docs/delivery/**'
```

### 任务 2：自动合并交集与关键功能审查

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| 23 个双方修改文件及其直接测试 | 未经证据扩展到无关模块 |

**执行步骤**

- [ ] 对每个交集文件检查本地 diff、上游 diff和合并结果。
- [ ] 搜索 RequestArchive、RequestIntercept、Token Analysis、内容审核、Responses→Chat、本地 cache usage 和并发错误等本地合同。
- [ ] 对路由、Wire、配置和 Redis provider 做依赖注入与生命周期检查。
- [ ] 对发现的真实回归补最小修复和回归测试。

### 任务 3：文档同步

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| `llm-wiki/wiki/*.md`、merge ledger、交付文档 | 覆盖或删除 merge ledger 既有记录 |

**执行步骤**

- [ ] 从最终源码和测试提取稳定变化。
- [ ] 更新 README 最近同步及相关 backend/frontend/ops/data/security 文档。
- [ ] 追加本次日期、分支、上游 SHA、merge SHA、冲突、处理和验证结果。

### 任务 4：生成、测试、静态检查与构建

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| 生成代码、测试揭示的真实缺陷及其文档 | 为让测试变绿而弱化断言或跳过测试 |

**执行步骤**

- [ ] 运行 Ent 和 Wire 生成，确认生成结果一致。
- [ ] 运行后端 unit 和 integration 全包测试。
- [ ] 运行 `golangci-lint`。
- [ ] 运行前端 lint、typecheck、Vitest 全量和 production build。
- [ ] 构建后端 embed 二进制并检查最终工作树。

**命令**

```powershell
Set-Location backend
go generate ./ent
go generate ./cmd/server
go test -tags=unit -p 1 -count=1 ./...
go test -tags=integration -p 1 -count=1 ./...
golangci-lint run ./...
Set-Location ..
cmd.exe /c pnpm --dir frontend run lint:check
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend exec vitest run
cmd.exe /c pnpm --dir frontend run build
```

## 审查关卡

| 关卡 | 必需证据 | 状态 |
|---|---|---|
| 规格符合性审查 | 每条成功标准映射到实现和验证证据 | 已完成 |
| 代码质量审查 | 冲突与 23 个交集文件已检查，发现项已修复或记录 | 已完成 |
| 最终验证 | 必需命令通过，或环境阻塞有原始证据 | 已完成 |

## 回滚

合并提交前可使用 `git merge --abort` 回到目标分支起点；合并提交后如需放弃，应新建 revert 提交，不改写已共享历史。本轮不执行 push 或部署。
