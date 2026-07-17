# sub2api v0.1.159 上游同步执行计划

| 状态 | 任务 | 负责人 | 验证 |
| --- | --- | --- | --- |
| 已完成 | 锁定最新上游边界 | 交付控制器 | 当前 `MERGE_HEAD=upstream/main=c2c19a7cb` |
| 已完成 | 解决 10 个冲突并组合双方能力 | 交付控制器 | `git ls-files -u` 为空 |
| 已完成 | 撤销本地上游缺陷修复，仅保留远端生产代码 | 交付控制器 | image storage 与 `MERGE_HEAD` 一致；batch limits 与 merge-tree 一致 |
| 已完成 | 保留本地用户并发 preset 的接口兼容 | 交付控制器 | 仅 2 个本地测试桩适配 `BatchUpdateLimits` |
| 已完成 | 规格、最新增量和代码质量审查 | 只读审查智能体、交付控制器 | 有效 findings 已核实；上游原生问题单列 |
| 已完成 | 更新 wiki、merge review、delivery 和 ledger | 交付控制器 | 事实已回填；ledger 仅 EOF 追加 |
| 已完成 | 完整生成、测试、lint 与构建 | 交付控制器 | `test-review.md`；上游/环境失败单列披露 |
| 已完成（等待审核） | 最终 staged 硬门和 commit 前通知 | 交付控制器 | 416 files / `+29294 -1677`；无 unmerged/unstaged/untracked/marker/features 删除/未解析敏感模式 |

## 验证命令

~~~powershell
# backend: repo-local cache + fresh GOTMPDIR
go generate ./cmd/server
go test -tags=unit -p 1 -count=1 ./...
go test -tags=integration -p 1 -count=1 ./...
golangci-lint run --new-from-rev HEAD ./...
go mod tidy -diff
go build -tags embed -trimpath ./cmd/server

# frontend
pnpm run lint:check
pnpm run typecheck
pnpm exec vitest run
pnpm run build
~~~

## 审查关卡

| 关卡 | 状态 |
| --- | --- |
| 规格符合性审查 | 已完成 |
| 代码质量审查 | 已完成 |
| 最终验证 | 已完成 |
| Commit 前审核 | 等待用户审核 |

用户审核前保持未提交 merge；不执行 reset、push、PR 或部署。
