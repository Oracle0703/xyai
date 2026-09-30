# 测试与审查

## 测试矩阵

| 验收标准 | 验证方式 | 结果 | 证据 |
|---|---|---|---|
| SC-1 分支基线 | Git ref、status、log | 已通过 | 原分支 `1ed588fc3`；目标分支起点 `20e6379f9` |
| SC-2 固定上游 | merge parent、VERSION | 已通过 | `d294d4937` 双亲正确；VERSION `0.1.155` |
| SC-3 双边功能保留 | 冲突/交集审查、定向与全量测试 | 已通过 | 3 个冲突和 23 个交集文件已审查；后端/前端用例通过 |
| SC-4 文档 | ledger 追加性、wiki diff | 已通过 | `9004a6650` |
| SC-5 完整验证 | 生成、Go、前端 | 已通过 | unit、integration、1055 frontend tests、typecheck |

## 规格符合性审查

| 严重程度 | 问题 | 证据 | 状态 |
|---|---|---|---|
| 信息 | 合并前预演发现 3 个显式冲突和 23 个双边修改文件 | `git merge-tree --write-tree --messages main upstream/main` | 已处理 |

## 代码质量审查

| 严重程度 | 问题 | 文件 / 行号 | 状态 |
|---|---|---|---|
| 无阻塞发现 | 未发现本地合同被静默删除；Wire 生成指令缺 `-mod=mod` 已修复 | 3 个冲突文件及 23 个交集文件 | 已完成 |

## 验证日志

| 命令 / 检查 | 结果 | 备注 |
|---|---|---|
| `git fetch --prune github main` | 通过 | 本地 `main` 与 `github/main` 均为 `20e6379f9` |
| `git fetch --prune upstream main` | 通过 | `upstream/main=7c717365e`，VERSION `0.1.155` |
| `git merge-tree --write-tree --messages main upstream/main` | 发现预期冲突 | 3 个显式冲突；尚未修改工作树 |
| `go test -tags=unit -p 1 -count=1 ./...` | 通过 | Windows 锁文件包以 fresh `GOTMPDIR` 重跑通过 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过 | Access denied 包逐包重跑通过 |
| `pnpm --dir frontend exec vitest run` | 通过 | 163 files, 1055 tests |
| `pnpm --dir frontend run typecheck` | 通过 | `vue-tsc --noEmit` |
| `go generate ./ent` / `go generate ./cmd/server` | 通过 | Wire 生成指令修复后稳定生成 |

## 残余风险

| 风险 | 影响 | 缓解 / 后续动作 |
|---|---|---|
| 完整 `golangci-lint` 有 29 个既有问题 | 不影响本阶段用例验收，但仍是仓库静态债务 | 后续独立清理，不在大型 merge 中扩修 |
| Windows Go 缓存与测试进程文件锁 | 单次全包可能出现假失败 | 已通过 fresh `GOTMPDIR` 重跑收口 |
