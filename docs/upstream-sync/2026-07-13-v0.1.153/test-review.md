# 测试与审查

## 测试矩阵

| 验收标准 | 验证方式 | 结果 | 证据 |
|---|---|---|---|
| SC-1 / 151 基线 | merge parents | 已通过 | 第一父 `5e6e85568792` |
| SC-2 / 固定上游 | ancestor checks | 已通过 | 第二父 `55ed0ab0d`；`7d239d62e` 非祖先 |
| SC-3 / 冲突 | combined diff、聚焦 Go/Vitest、独立复核 | 已通过 | 6 个冲突文件无标记，未发现 P0-P3 可执行问题 |
| SC-4 / 双侧能力 | 入口扫描、unit/integration、Vitest | 已通过 | 后端全量和前端 997 个用例通过 |
| SC-5 / 文档 | append-only diff、路径检查 | 已通过 | ledger 仅尾部追加；wiki、review 和组件 README 已补齐 |
| SC-6 / 全量验证 | Go、pnpm、build、lint | 已通过 | 差异 lint 0 issues；构建与测试均通过 |

## 规格符合性审查

| 严重程度 | 问题 | 证据 | 状态 |
|---|---|---|---|
| 无阻断 | 未发现后续上游提交误入 | ancestor check 对 `7d239d62e` 返回 1 | 已关闭 |
| 无阻断 | 6 个冲突的组合语义经独立复核 | merge combined diff、入口与测试对照 | 已关闭 |

## 代码质量审查

| 严重程度 | 问题 | 文件 / 行号 | 状态 |
|---|---|---|---|
| 历史债务 | 全仓 golangci-lint 报 29 项，均在 151 第一父已有 | `docs/features/golangci-lint-debt-cleanup-plan-cn.md` | 不归因本次合并 |
| 无新增 | 从第一父开始的差异 lint 为 0 issues | `golangci-lint run --new-from-rev HEAD^1 ./...` | 已关闭 |

## 验证日志

| 命令 / 检查 | 结果 | 备注 |
|---|---|---|
| `go mod tidy -diff` | 通过 | 当前依赖图稳定 |
| 冲突标记扫描 | 通过 | 0 个真实冲突标记 |
| `git diff --check` | 通过 | 无 whitespace error |
| `golangci-lint run ./...` | 未通过：29 issues | 151 基线既有，不是本次新增 |
| `golangci-lint run --new-from-rev HEAD^1 ./...` | 通过：0 issues | 本次 merge 未增加 lint 问题 |
| `go test -tags=unit -p 1 -count=1 ./...` | 通过 | 首次被安全软件占用 `web.test.exe`；fresh `E:\tmp\xyai-unit-final-*` 完整重跑通过，`internal/service` 101.002s |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过 | `internal/service` 57.799s |
| `pnpm --dir frontend run lint:check` | 通过 | ESLint 无新增错误 |
| `pnpm --dir frontend run typecheck` | 通过 | `vue-tsc --noEmit` |
| `pnpm --dir frontend exec vitest run` | 通过 | 156 files / 997 tests |
| `pnpm --dir frontend run build` | 通过 | 926 modules / 15.56s；仅既有 import/chunk warning |
| 前端 build 后 `go build -tags embed -trimpath ./cmd/server` | 通过 | 最终产物 145,468,416 bytes |

## 残余风险

| 风险 | 影响 | 缓解 / 后续动作 |
|---|---|---|
| 本机缺少 `bash` | Apple container shell test 无法在 Windows 本地运行 | `.github/workflows/backend-ci.yml` 已在 macOS job 执行 syntax 与 fixture test |
| 151 基线 lint 债务仍存在 | 全仓 CI lint job 可能保持红色 | 本次用差异 lint 阻止新增；后续单独执行既有清理计划 |
