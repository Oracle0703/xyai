# 实施计划

| 字段 | 内容 |
|---|---|
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 分支 | `feature/hy/10153_同步sub2api主线` |
| 负责人 | 控制器 |

## 任务清单

| 状态 | 任务 | 负责人 / 智能体 | 文件 / 模块范围 | 验证方式 |
|---|---|---|---|---|
| 已完成 | 固定 151 基线和 153 上游 SHA | 控制器 | Git refs | parent/ancestor 检查 |
| 已完成 | 合并并解决 6 个冲突 | 控制器 | 冲突文件 | combined diff 与聚焦测试 |
| 已完成 | 补齐 ledger、review、wiki、组件 README | 控制器 | `docs/`、`llm-wiki/`、组件 README | 文档 diff 与路径扫描 |
| 已完成 | 独立冲突与文档复核 | 两个只读智能体 | merge diff 与稳定知识 | findings-first 报告 |
| 已完成 | 最终全量验证与 Git 审计 | 控制器 | 全仓 | 命令退出码与工作树状态 |
| 已完成 | 提交文档收口 | 控制器 | 本轮新增/修改文件 | commit、父链和 status |

## 详细步骤

### 任务 1：固定并验证上游边界

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| 无，Git 只读检查 | 不 fetch 后自动合入 `upstream/main` |

**执行步骤**

- [x] 确认 merge commit 第一父为 151 基线。
- [x] 确认第二父为固定 `55ed0ab0d`。
- [x] 确认 `7d239d62e` 不是当前分支祖先。
- [x] 记录 upstream/main 后续 4 个提交但不合入。

**命令**

```powershell
git rev-list --parents -n 1 HEAD
git merge-base --is-ancestor 55ed0ab0da367183d97c15659e33ae9e83f6ff90 HEAD
git merge-base --is-ancestor 7d239d62e HEAD
```

### 任务 2：文档与依赖收口

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| `backend/go.sum`、merge ledger、review、llm-wiki、组件 README、交付文档 | 已有 ledger 条目、已应用 migration、无关业务代码 |

**执行步骤**

- [x] 运行 `go mod tidy` 并确认 `go mod tidy -diff` 为空。
- [x] 追加 merge ledger，记录完整 SHA、6 个冲突与验证结果。
- [x] 新增 merge review，写明边界和组合语义。
- [x] 更新 6 个 wiki 页面中的稳定事实。
- [x] 创建 account/keys README，更新 common/DataTable README。
- [x] 运行文档与 diff 检查。

**命令**

```powershell
Set-Location backend
go mod tidy -diff
Set-Location ..
git diff --check
rg -n '^(<<<<<<< .+|=======|>>>>>>> .+)$' .
```

### 任务 3：最终验证与提交

**文件范围**

| 允许修改 | 禁止修改 |
|---|---|
| 仅修复最终验证发现的本次 merge 回归；更新交付证据 | 不修 151 基线既有 lint 债务，不推送或部署 |

**执行步骤**

- [x] 使用 fresh `GOTMPDIR` 运行后端 unit 全量测试。
- [x] 使用另一个 fresh `GOTMPDIR` 运行后端 integration 全量测试。
- [x] 运行前端 lint、typecheck、全量 Vitest 和生产构建。
- [x] 运行差异 lint，期望 `0 issues`。
- [x] 运行 `-tags embed` 后端构建。
- [x] 完成父链、版本、冲突、工作树审计并提交文档收口。

**命令**

```powershell
go test -tags=unit -p 1 -count=1 ./...
go test -tags=integration -p 1 -count=1 ./...
cmd.exe /c pnpm --dir frontend run lint:check
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend exec vitest run
cmd.exe /c pnpm --dir frontend run build
```

## 审查关卡

| 关卡 | 必需证据 | 状态 |
|---|---|---|
| 规格符合性审查 | 每条验收标准映射到实现和验证 | 已完成 |
| 代码质量审查 | 冲突组合无高风险回归；历史 lint 债务单列 | 已完成 |
| 最终验证 | 全量命令通过，或失败原因与影响明确记录 | 已完成 |
