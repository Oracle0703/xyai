# 实施计划

| 字段 | 内容 |
|---|---|
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 分支 | `feature/hy/10173_merge_sub2api_173` |
| 负责人 | Codex 控制器及只读审查智能体 |

## 任务清单

| 状态 | 任务 | 负责人 / 智能体 | 文件 / 模块范围 | 验证方式 |
|---|---|---|---|---|
| 已完成 | 核对本地与自有 GitHub main，创建功能分支 | 控制器 | Git refs | SHA 与远端实时查询 |
| 已完成 | 合并固定上游提交并解决 9 个冲突 | 控制器 | 9 个冲突文件及必要测试 stub | focused Go tests、冲突索引 |
| 已完成 | 三方重叠与上游版本风险审计 | 重叠审查、上游审查智能体 | 固定 Git 对象，只读 | 79 个双方路径分类与风险报告 |
| 已完成 | 冲突语义复核 | 冲突审查智能体 | 9 个冲突文件，只读 | index/marker/whitespace 审查 |
| 已完成 | 重新生成并核对 Wire/Ent | 控制器 | `backend/ent/**`、`backend/cmd/server/wire_gen.go` | 两个 generate 均通过且无工作树漂移 |
| 已完成 | 更新 wiki、ledger、交付记录和 Wiki 图谱 | 控制器、Wiki 智能体 | `llm-wiki/**`、`docs/**` | 文档 diff、图谱状态 |
| 已完成 | 后端、前端、专项测试与构建 | 控制器 | 全仓验证 | `test-review.md` |
| 已完成 | 最终 Git/features/whitespace 审计 | 控制器 | 暂存树 | 静态检查与状态快照 |
| 待开始 | 提交前用户审核 | 用户 | 待提交 merge | 用户明确批准后才 commit |

## 详细步骤

### 任务 1：生成物与冲突一致性

| 允许修改 | 禁止修改 |
|---|---|
| 由当前 schema/provider 生成的 Ent/Wire 文件；合并所需测试桩。 | 为通过测试而修复上游生产缺陷。 |

- [x] 记录合并前后基线验证。
- [x] 解决 9 个文本冲突并清空 unmerged index。
- [x] 运行 `go generate ./ent` 与 `go generate ./cmd/server`。
- [x] 核对生成物不存在意外漂移。

### 任务 2：知识与合并记录

| 允许修改 | 禁止修改 |
|---|---|
| `llm-wiki/wiki/*.md`、Wiki 图谱、append-only ledger、本交付目录。 | 覆盖或删除 ledger 历史；把未经验证猜测写入 wiki。 |

- [x] 更新 0.1.173 稳定架构、前端、配置、数据与安全事实。
- [x] 追加 2026-08-09 合并条目，merge commit 标为审核后补录。
- [x] 刷新并检查 Wiki 图谱。

### 任务 3：验证与审核

| 允许修改 | 禁止修改 |
|---|---|
| 验证产生的预期构建产物；测试证据文档。 | commit、push、PR、部署。 |

- [x] 后端 default/unit/integration、lint、build 与专项测试。
- [x] 前端 lint、typecheck、Vitest、build 与专项测试。
- [x] 对失败做合并前/固定上游归因，不越界修复。
- [x] 最终检查 Git 状态、冲突标记、whitespace 和 23 个 features 文件。

## 命令

```powershell
cd backend
go generate ./ent
go generate ./cmd/server
go test -p 1 -count=1 ./...
go test -tags=unit -p 1 -count=1 ./...
go test -tags=integration -p 1 -count=1 ./...
go build ./...

pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run
pnpm --dir frontend run build
```

## 审查关卡

| 关卡 | 必需证据 | 状态 |
|---|---|---|
| 规格符合性审查 | 每条验收标准映射到实现和验证证据 | 已完成 |
| 代码质量审查 | 冲突语义与双方重叠路径已审查，剩余结果进入风险表 | 已完成 |
| 最终验证 | 必需命令通过，或失败经基线归因并记录 | 已完成 |

## 回退

当前 merge 尚未提交；若用户审核拒绝，应由用户明确授权后再执行 `git merge --abort`。在获得该授权前不做破坏性回退。
