# sub2api v0.1.156 测试与审查记录

## 验收矩阵

| 验收标准 | 证据 | 结论 |
| --- | --- | --- |
| AC-1 / AC-2 固定双父边界 | `HEAD/main/ORIG_HEAD=4c456aad3`；`MERGE_HEAD=d515c3045`；`v0.1.156=12f991d`；后续 `eb2b8632d` 排除 | 通过 |
| AC-3 冲突与交集审查 | 3 个文本冲突、29 个双方修改交集；后端、网关、前端独立审查 | 通过，无开放 P0-P3 |
| AC-4 本地功能保留 | merge review 中 10 类“保留/组合/上游替代”映射；`docs/features` 无删除 | 通过 |
| AC-5 文档 | 6 个 wiki、追加型 ledger、merge review 与交付材料 | 通过 |
| AC-6 测试与构建 | 下方验证日志 | 已执行；一项 integration 二进制受 360 环境阻断 |
| AC-7 commit 前审核 | `MERGE_HEAD` 保留，未执行 commit/push/PR | 通过 |

## 规格与代码审查

| 级别 | 结论 |
| --- | --- |
| P0 / P1 | 无开放问题。Wire 缺 binding 的阻断已修复并稳定生成。 |
| P2 | native failover header、风控总开关快照、`Retry-After` 三类问题均完成 RED -> GREEN 和二次复核。 |
| P3 | 无开放问题；非阻塞抽象建议未扩入本次合并。 |

## 验证日志

| 命令或检查 | 工作目录 | 真实结果 |
| --- | --- | --- |
| Prompt Risk runtime snapshot 定向回归 | `backend` | 缓存读取、即时 config 更新、总开关双向切换通过。 |
| `UseKeyModal.spec.ts` | `frontend` | RED 1/9 失败；修复 provider 选择后 9/9 通过。 |
| 合并相关前端定向 Vitest | `frontend` | 19 files / 131 tests passed。 |
| `Retry-After` service / handler 定向回归 | `backend` | RED：service 12/14、handler 0/2；GREEN：service 14/14、handler 2/2。 |
| `go test -tags=unit -p 1 -count=1 ./...` | `backend` | 完整重跑退出 0：50 个测试包通过、53 个无测试包、失败 0；`internal/service` 116.717s。 |
| `go test -tags=integration -p 1 -count=1 ./...` | `backend` | 完整运行到结尾：44 个测试包通过、57 个无测试包；`internal/pkg/openai_compat` 因本机 360 持续拒绝启动 `.test.exe` 而最终 `FAIL`。 |
| integration 环境核对 | `backend` | `go list` 证明 `openai_compat` 在 unit/integration tag 下均为同一 `upstream_capability.go` / `upstream_capability_test.go`；完整 unit 已通过。自定义 integration test binary 编译退出 0，但执行仍被 360 拒绝。 |
| `golangci-lint run ./...` | `backend` | 退出 1：29 条既有债务，含 15 errcheck、2 gofmt、3 staticcheck、9 unused。 |
| `golangci-lint run --new-from-rev main ./...` | `backend` | 退出 0：`0 issues`。 |
| `go mod tidy -diff` | `backend` | 退出 1：只建议删除本地 `main` 已存在的 Wire CLI 传递校验和；当前 `go.mod/go.sum` 与第一父相同。 |
| `pnpm exec vitest run` | `frontend` | 176 files / 1214 tests passed。 |
| `pnpm run lint:check` | `frontend` | 退出 0。 |
| `pnpm run typecheck` | `frontend` | 退出 0。 |
| `pnpm run build` | `frontend` | 退出 0：944 modules，13.57s；仅既有 import/chunk warning。 |
| 前端 build 后 `go build -tags embed -trimpath` | `backend` | 退出 0；产物 147,534,336 bytes。 |
| 连续两次 `go generate ./cmd/server` | `backend` | 两次退出 0；`wire_gen.go` SHA-256 均为 `233A9D6C1D53E981CD6069A198E0D8B74A85864F62415BA73BAE77215914601E`。 |
| gofmt 与 `git diff --check` | 仓库根 | 12 个未暂存 Go 文件无 gofmt 差异；worktree/cached/HEAD 三种 diff check 退出 0。 |
| 最终 staged Git / 边界 / LF / 敏感信息硬门 | 仓库根 | 通过：274 files / `+32475 -1728`；`HEAD/main/ORIG_HEAD=4c456aad3`，`MERGE_HEAD=d515c3045`；无 unstaged/untracked/unmerged、冲突标记、whitespace error、CRLF/BOM、敏感模式或 `docs/features` 删除。 |

## 残余风险

| 风险 | 处置 |
| --- | --- |
| 360 拦截 integration 版本 `openai_compat.test.exe` | 未关闭安全软件、未添加白名单、未修改业务代码规避。保留完整失败日志、文件集合等价证据和 unit 通过证据，由用户决定是否接受后提交。 |
| 全量 lint 29 条既有债务 | 相对 `main` 的合并差异 lint 为 0；不在上游同步中清理历史债务。 |
| `go mod tidy -diff` 与 Wire 工具校验和循环 | 当前模块文件与本地 `main` 完全相同，Wire 连续生成稳定；不制造无关 go.sum churn。 |
| 前端构建 warning | 构建退出 0；warning 属于既有 chunk/import 结构，本次未扩范围重构。 |
