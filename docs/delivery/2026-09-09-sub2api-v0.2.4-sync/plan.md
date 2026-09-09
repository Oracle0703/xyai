# 实施计划

| 字段 | 内容 |
|---|---|
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 目标分支 | `feature/hy/10204_merge_sub2api_204` |
| 负责人 | Codex 控制器 |

## 任务清单

| 状态 | 任务 | 文件 / 模块范围 | 验证方式 |
|---|---|---|---|
| 已完成 | 刷新并核对本地、自有 GitHub main 与 exact upstream SHA | Git refs | SHA、远端 URL、VERSION、merge base |
| 已完成 | 三方预演并盘点本地 features | 固定 Git 对象 | 2 个双方路径、0 个文本冲突、24 个 features 基线 |
| 已完成 | 创建规定分支并执行未提交 merge | Git index / worktree | branch、`MERGE_HEAD`、`git ls-files -u` |
| 已完成 | 审查双方修改与上游全部 19 个路径 | README、OpenAI image/service/pricing、deploy、前端模型白名单 | base/local/upstream/merge 四方 diff |
| 已完成 | 更新 wiki、ledger 和交付记录 | `llm-wiki/**`、`docs/**` | 文档 diff、LF、ledger 尾部 |
| 已完成 | 运行专项、完整测试、lint 和 build | `backend/`、`frontend/` | `test-review.md` |
| 已完成 | 刷新 Wiki 图谱并完成最终审计 | 图谱、Git index | graph status、features、markers、whitespace |
| 待开始 | 提交前用户审核 | 待提交 merge | 用户明确批准后才 commit |

## 实施步骤

| 顺序 | 动作 | 范围边界 |
|---|---|---|
| 1 | 从 `main@f6bce5db` 创建 `feature/hy/10204_merge_sub2api_204`。 | 不改写 main。 |
| 2 | 运行 `git merge --no-commit --no-ff 98d86915...`。 | 不创建 merge commit。 |
| 3 | 审查文本冲突、2 个双方修改文件和 17 个仅上游路径。 | 只处理冲突/必要接口适配，不修上游 bug。 |
| 4 | 确认 24 个 tracked `docs/features/` 文件零删除。 | 真正重叠功能优先上游，独有功能保留。 |
| 5 | 更新 `llm-wiki/wiki/`、append-only ledger 与交付证据。 | ledger 只追加，不覆盖历史。 |
| 6 | 执行验证并对失败进行 first-parent / exact-upstream 归因。 | 不为上游失败修改生产代码。 |
| 7 | 固化 staged `MERGE_HEAD` 快照并通知用户。 | 不 commit、不 push、不建 PR、不部署。 |

## 验证命令

```powershell
# 后端专项（backend/）
go test -p 1 -count=1 ./internal/pkg/openai ./internal/service -run 'GPTImage25|OpenAIImage|Images|ImageModel|Pricing'

# 后端完整套件与构建（backend/，使用任务专用 GOCACHE/GOTMPDIR）
go test -p 1 -count=1 ./...
go test -tags=unit -p 1 -count=1 ./...
go test -tags=integration -p 1 -count=1 ./...
go build ./...
go build -tags=embed ./...
go mod tidy -diff
golangci-lint run --new-from-rev=HEAD ./...

# 前端（仓库根目录）
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run src/composables/__tests__/useModelWhitelist.spec.ts
pnpm --dir frontend exec vitest run
pnpm --dir frontend run build
```

## 审查关卡

| 关卡 | 必需证据 | 状态 |
|---|---|---|
| 规格符合性审查 | 每条成功标准映射到当前实现和验证证据 | 已完成 |
| 合并语义审查 | 冲突、双方修改、仅上游路径和本地 feature 均有结论 | 已完成 |
| 最终验证 | 必需命令通过，或失败经边界归因并记录 | 已完成 |

## 回退

当前计划停在未提交 merge；若用户审核拒绝，只在用户明确授权后执行 `git merge --abort`。未获授权前不做破坏性回退。
