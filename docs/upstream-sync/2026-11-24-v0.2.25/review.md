# 0.2.25 上游同步审核

2026-11-24 · `feature/hy/10240_merge_sub2api_225` · 待提交审核

## 1. 范围

| 项目 | 固定版本 |
| --- | --- |
| 第一父 | `acc17da790dc776ec4f48154c520a9144a0783c1`，建分支时与远端 github/main 一致，包含 GPT 降智测试 |
| 第二父 | `Wei-Shaw/sub2api main@0f6671457132425d1b84f3d80536b53e02942424`，VERSION 为 0.2.25 |
| 共同祖先 | `328db26f450edbdf9f8d4344a3ea903ac6396842` |
| 上游增量 | 15 个提交，91 个路径，其中 22 个路径双方均有修改 |

## 2. 冲突与处理

| 路径 | 裁决 |
| --- | --- |
| `frontend/src/components/account/CreateAccountModal.vue` | 文本冲突。保留本地 OpenAI-compatible 供应商选择与重置，同时采用上游分组同步状态、探测联动和请求字段 |
| `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 文本冲突。保留时区选项与混合渠道风险 mock，加入上游分组预览 mock |
| `backend/internal/service/gpt_iq_test_runner.go` | 上游更改覆写函数签名。本地后台探测传出站请求上下文，并使用上游共享请求已有的去用户身份方法；保留静态头 |
| `frontend/src/views/admin/AccountsView.vue` 与创建弹窗 | 上游的分组预览与列表写入叠加本地账号操作员权限。复用 `canManageAccounts` 隐藏两个入口，服务端沿用白名单。受限角色行按钮有回归断言，管理员同步沿用上游测试 |

## 3. 本地功能

28 份 `docs/features/` 文件与第一父逐文件相同。文档更新前，855 个仅本地变更路径中，853 个与第一父一致，另两个是 GPT 降智探测调用及受限角色回归用例。69 个仅上游变更路径与指定提交全部一致。

双方同时修改的路径包括账号管理接口、路由和测试、Grok 配额与 OpenAI 转发入口、前端 API、表单、类型和双语文案。保留本地请求归档/拦截、Token Analysis、Grok 自动强度、时区处理、compatible 字段过滤及配额展示；本轮没有撤除独立本地功能。

## 4. 采用的上游行为

采用上游分组展示与同步、API Key 账号身份变量、WebSocket 依赖更新和透传错误处理。分组同步写入现有账号字段，复用计费探测；预览旧 Antigravity relay 也可使用已保存的连接信息。网关展示与账号设置的约束见对应 wiki。

## 5. 验证

Go 1.27.4，三套全量均在 `backend/` 运行 `go test -json -p 1 -count=1 -timeout 60m ./...`，unit、integration 各加对应的构建标签。以下按顶层用例计数，各套件有重叠。

| 套件 | 通过 | 失败 | 显式跳过 | 退出码 |
| --- | ---: | ---: | ---: | ---: |
| go-default | 8144 | 0 | 14 | 0 |
| go-unit | 14038 | 1 | 41 | 1 |
| go-integration | 7639 | 0 | 18 | 0 |
| pgredis | 879 | 0 | 6 | 0 |
| audit-fresh | 114 | 0 | 0 | 0 |

- 本机无 Docker daemon，标准 integration 的仓储包由 TestMain 跳过。另用隔离 PostgreSQL 18.3 和 Redis 8.10.2 重跑仓储全包；仓库外 Go overlay 给现有 postgres-only 测试夹具提供同一 DSN 与独立 Redis 客户端。安全审计全包使用独立空库、原版夹具。显式跳过明细存于仓库外 `skipped-tests.json`。
- 前端 lint、类型检查及生产构建通过。Vitest 共 449 个文件，448 通过，3790/3791 个用例通过。创建与编辑账号弹窗、管理员列表同步及受限账号操作员测试通过。失败的上游样式断言见第 6 节。
- Go 增量和全量 golangci-lint 各为 0 issues。普通构建、嵌入前端的服务构建与 embed Web 测试通过。二进制输出 0.2.25，使用独立空库、Redis 与回环端口启动后，`/health` 返回 200 和 `{"status":"ok"}`，首页返回 200 HTML。测试服务已停止。
- 部署脚本中 Compose 安全、网关环境、资源限制和 Caddy 缓存四项通过。simple mode 的 4 份 Compose 配置各跑 4 种覆盖值，使用官方 Compose v2.39.4（已核 SHA-256），仅适配 Windows 可执行文件及空文件路径，16 种配置均通过。静态生图页与归档工具 Python 测试 5/5 通过。
- Wiki 图谱已刷新，35 节点、76 边，扫描到的 wiki 链接无悬空项。逐路径核对、父版本快照和原始日志存于 `E:/tmp/xyai-merge-225-20261124/`，详见 `merge-paths.json` 与 `logs/`。

## 6. 遗留问题

| 项目 | 证据及处置 |
| --- | --- |
| 账号列表提示框样式断言（上游固有） | `AccountsView.sparkShadow.spec.ts` 仍要求 Tooltip 带 `self-start`，0.2.25 将组件移入行容器时已去掉该类。相同测试文件在纯上游 9/10，失败信息与合并分支相同，第一父 10/10。保留上游布局及断言，等待上游处理 |
| Linux 发布工具文件权限（本机平台） | Windows 上 `test_linux_context_preserves_binary_executable_mode` 得到 0666，预期 0755，发布套件 9/10。第一父快照同环境得到相同失败 |
| Apple 容器生命周期（本机平台） | 此项 CI 使用 macOS 15 和 BSD `stat -f '%Lp'` 检查 0600；Git Bash 的 GNU stat 在该步骤报错。部署脚本与第一父一致，完整 macOS 生命周期待该平台验证 |
| WebSocket 首 token 时序（两父均复现） | `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode` 在 unit 全量的 visible 子用例测得 1099 ms，断言下限 1100 ms。同环境精确复跑 3 次，第一父 1 过 2 败，指定上游与合并分支均 3 败；测试文件两父相同，保留其阈值和业务行为 |
