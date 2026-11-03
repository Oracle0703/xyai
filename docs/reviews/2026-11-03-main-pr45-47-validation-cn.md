# PR #45–#47 合入 main 后的完整项目验证

| 项目 | 内容 |
| --- | --- |
| 日期 / 执行方 | 2026-11-03（北京时间）/ Codex |
| 用户要求 | Claude 的三个 PR 合入 main 后，实际运行检查整个项目是否正常 |
| 被测提交 | `0157f2c7999b429d1db3273b8fe62f445a725276`；开始与收尾均核对 GitHub main 为同一 SHA |
| 合并前基线 | `7d0f760fc4a3fa8e52d4e99fe36787423f7e1cdf` |
| 结论 | **本次改动的针对性测试、前端全量检查、两种后端构建及隔离 PostgreSQL 专项通过；整个项目尚未全绿。** 两个仓储 SQL mock 失败和 Command Code 不稳定测试在合并前代码上复现；默认全量另有一次 WebSocket 时序失败，后续未再复现，尚未完成根因闭环 |
| 本轮变更 | 验证报告及 ops 验证记录；未修改业务代码，未提交、推送、部署或操作遗留分支 |
| 原始证据 | 本机忽略目录 `.localdev/validation-20261103-pr45-47/`，保留每项命令、退出码、日志、JSON 统计和基线 overlay，不包含生产配置 |

## 1. 本次合并范围

| PR | 合并提交 | 内容 |
| --- | --- | --- |
| [#45](https://github.com/Oracle0703/xyai/pull/45) | `2b3f5920b` | 默认 reasoning effort 为 `none` 时不注入请求 |
| [#46](https://github.com/Oracle0703/xyai/pull/46) | `016a33180` | 取消周期时间轮任务后，正在执行的回调不再注册下一轮 |
| [#47](https://github.com/Oracle0703/xyai/pull/47) | `0157f2c79` | 子管理员订阅响应投影、显式部门授权参数、停用分组新建订阅限制、错误传播、分页上界、导出快照和重置错误恢复 |

两端共 23 个路径、`+426/-31`。检查期间 main 未改变，原有三份未提交评审文档保留。

## 2. 环境与隔离

| 项目 | 实际环境 |
| --- | --- |
| 操作系统 | Windows；没有可用 Docker。不能等同 Linux CI 或生产运行验收 |
| Go | 显式使用仓库缓存 Go 1.27.3；`GOROOT` 与 `go.exe` 一致，`GOTOOLCHAIN=local` |
| Go 缓存 | `backend/.gocache/review-cache`、`backend/.gocache/review-gopath/pkg/mod`；每条命令独立 `GOTMPDIR`，完整 default/unit/integration 顺序执行 |
| 前端 | Node 24.13.0、pnpm 9.15.4；frozen lockfile 安装成功。CI 的 Node 20 未在本轮复现 |
| Go lint | golangci-lint 2.14.0 |
| 竞态检查 | MinGW GCC，时间轮现有源文件与测试文件集，`CGO_ENABLED=1`、`-race -count=3` |
| PostgreSQL | 使用已有 18.1 二进制创建全新临时集群，只监听 127.0.0.1，并新建 `postmerge_tests` 数据库；未复用既有开发或生产数据库 |
| 数据库收尾 | `pg_ctl -m fast -w stop` 成功，确认 `server stopped`；日志与临时数据目录留在本机忽略目录 |
| 外部服务 | 不提供真实 OpenAI/TypeSafe API Key 或现有测试 DSN；不访问生产账号、支付、SMTP 或上游模型服务 |

本机现有 Redis 二进制为 5.0.14.1，低于项目 Redis 7+ 要求。本轮未启动完整应用进行真实账号/浏览器端到端验收；二进制 `-version` 正常返回 0.2.19 只证明可执行文件可运行，不替代完整启动与业务验证。

## 3. 正式验证结果

Go 测试数量按**顶层测试**统计，子测试数量另注；没有测试文件的包不计为通过测试。不同 tag 的用例有重叠，不能相加当作独立覆盖总量。

| 检查 | 结果 | 数量 / 说明 |
| --- | --- | --- |
| 前端 frozen install | 通过 | 退出 0，锁文件未改动 |
| 前端 `lint:check` | 通过 | 退出 0，不使用自动修复 |
| 前端 `typecheck` | 通过 | 退出 0 |
| 前端完整 Vitest | 通过 | **423 个测试文件、3456 个测试全部通过，0 skip**；JSON 的 suite 数包含 describe 分组，文件数取 `testResults.length` |
| 前端生产 build | 通过 | 退出 0；有 Browserslist 数据较旧和 chunk 超过 500 kB 的非阻断警告 |
| 后端默认全量 | **失败** | 7531 pass / 1 fail / 14 skip；53 个有测试包通过、service 包失败。唯一失败为下述 WebSocket 时序用例 |
| 后端 unit 全量 | **失败** | 12844 pass / 3 fail / 35 skip；59 包通过、repository/service 两包失败 |
| 后端 integration 全量 | **有覆盖缺口的通过** | 命令退出 0，7045 pass / 18 skip；repository 因无 Docker 整包未执行，不能宣称完整集成通过 |
| PostgreSQL 部门/成员/订阅专项 | 通过 | **38 pass / 0 fail / 2 skip**；含子用例 102 PASS。两项 skip 为显式开关控制的部门性能测试 |
| PostgreSQL 受限管理密钥专项 | 通过 | **57 pass / 0 fail / 0 skip**；含子用例 108 PASS，涵盖 repository/service |
| 时间轮竞态专项 | 通过 | 现有 10 个顶层测试重复 3 轮，共 30 PASS，未报 data race；这是文件集专项，不是整个后端 `-race` |
| 后端普通 build | 通过 | `CGO_ENABLED=0`，Windows 原生构建 |
| 后端 embed build | 通过 | 编译本轮生成的前端产物，Windows 原生构建 |
| 两种二进制 `-version` | 通过 | 均正常输出 0.2.19 |
| Go 全量 lint | **失败** | 34 issues：errcheck 20、gofmt 1、staticcheck 4、unused 9 |
| Go 相对合并前的增量 lint | 通过 | `--new-from-rev=7d0f760fc`，**0 issues**；全量 lint 涉及的 19 个文件均未被三个 PR 修改 |
| PR diff 空白检查 | 通过 | `git diff --check 7d0f760fc..main` |

受限管理密钥的首次补跑漏带 `unit` tag，只得到“no tests to run”；该结果不计为通过。正式结果是随后明确使用 `-tags=unit`、提供临时库 `SCOPED_ADMIN_TEST_DSN` 的 57 项测试。

integration 的 18 个显式 skip 涉及 Redis、Prompt Audit PostgreSQL/配置、外部抓包对照、真实 API、TypeSafe live、插件包等前提；本轮补跑的部门和管理密钥 SQL 不替代其余集成范围。

## 4. 失败项与归属

### 4.1 两个仓储 SQL mock：合并前已存在

| 用例 | 现象 |
| --- | --- |
| `TestDashboardAggregationRepositoryCleanupUsageLogsPartitionedSortsAndInvalidatesEachDropBeforeSync` | mock 期待先 `DROP TABLE`，实现先执行 `mark_user_usage_rollup_dirty_day`，SQL 顺序不匹配 |
| `TestDashboardAggregationRepositoryCleanupUsageLogsPartitionFailureRollsBackAndStops` | 同一前置 SQL 失配，使测试无法到达它期望的 drop partition 错误 |

证据：`backend/internal/repository/dashboard_aggregation_group_usage_test.go:270`、`:324`，以及 `go-unit.stdout.log`、`go-repository-baseline.stdout.log`。

两项在当前 unit 全量中失败，在还原合并前 Go 源码的基线 overlay 上各自再次失败，错误一致。应核对并更新测试与现行“先标记 rollup dirty 再删分区”的合同；本轮没有修改测试或删除业务步骤来消除红灯。

### 4.2 Command Code 协议用例：两端均可复现不稳定

`TestCommandCodeGatewayPassesThroughCatalogProtocols` 在 `backend/internal/service/model_protocol_catalog_test.go:374` 失败：目录不可用的场景预期转到 `/provider/v1/chat/completions`，实际为 `/provider/v1/responses`。

| 运行 | 结果 |
| --- | --- |
| 当前 main unit 全量 | 失败 |
| 当前 main 单独 `-count=3` | 2 pass / 1 fail |
| 合并前源码单独 `-count=3` | 1 pass / 2 fail |

该测试及模型协议目录实现未被三个 PR 修改。实测证实合并前已存在，但**本轮没有完成其不稳定原因的修复**，不能把“偶尔通过”写成已经解决。证据日志为 `go-commandcode-current-repeat.stdout.log` 和 `go-commandcode-baseline-repeat.stdout.log`。

### 4.3 WebSocket 关闭时序：首次全量失败，后续未再复现

`TestPassthroughLifecycle_ResponseCreatedTimeoutClosesWithoutFailover` 在 `backend/internal/service/openai_ws_v2_passthrough_lifecycle_test.go:850` 失败：预期 `websocket.CloseError`，实际读取等到 `context deadline exceeded`。该轮用例耗时约 6.30 秒，测试中的关闭帧读取预算为 2500 毫秒。

| 运行 | 结果 |
| --- | --- |
| 当前 main 默认全量 | 失败 |
| 当前 main unit / integration 全量 | 均通过 |
| 当前 main 默认标签单独 `-count=3` | 3/3 通过 |
| 合并前源码默认标签单独 `-count=3` | 3/3 通过 |

用例与直接的 passthrough 实现路径未被三个 PR 修改。本轮没有发现持续性回归证据，但也没有在合并前复现这次失败，**不把它定性为已证明的旧失败或已修复问题**。需保留全量时序敏感性的后续排查项；当前证据不能证明原因只是机器负载。

### 4.4 全量 lint：现有质量门禁仍为红

34 项分布在 19 个未被本轮 PR 修改的文件；相对 `7d0f760fc` 的增量检查为 0。主要包括未检查错误、格式、旧辅助函数/字段未使用等。详细位置见 `go-lint.stdout.log`。

这是当前全量质量门禁的实际失败，不应因为增量通过就写成全仓 lint 通过。

## 5. 三个 PR 的针对性证据

| 变更 | 验证结果 |
| --- | --- |
| 默认 effort `none` 不注入 | unit 全量中的 `TestApplyDefaultOpenAIReasoningEffort` 通过，原报告记录的该项失败已不再出现 |
| 取消执行中的周期任务 | `TestTimingWheelService_CancelWhileRecurringCallbackRunsPreventsReschedule` 通过；时间轮文件集 `-race -count=3` 通过 |
| 子管理员响应裁剪、批量与重放 | `TestSubAdminSubscriptionHidesMemberAccountAndAssignment`、`TestSubAdminSubscriptionReplayAndBulkUseSubAdminView` 通过；受限密钥相关 SQL 专项通过 |
| 显式 `department_ids` 与现行部门权限 | `TestDepartmentSetAccessRequiresExplicitDepartmentIDs` 通过；真实 PostgreSQL 的部门、成员管理、授权生命周期回归通过 |
| 停用分组只禁止新建订阅 | `TestAssignSubscriptionInactiveGroupBlocksOnlyNewSubscriptions` 通过，保留已有订阅续期/幂等返回语义 |
| 进度错误保留 | `TestGetSubscriptionProgressKeepsRepositoryError` 通过 |
| 分页、页面导出与重置状态恢复 | 相关后端测试及前端完整 Vitest 通过；尚未做真实账号浏览器验收 |

## 6. 辅助 CI 检查

| 检查 | 结果与范围 |
| --- | --- |
| Apple container / release shell 语法 | 均通过 `bash -n` |
| Compose security、gateway env、runtime resources、Caddy cache 脚本 | 四项通过 |
| Compose simple-mode 渲染 | 无 Docker，不能执行 `docker compose config`；不是业务断言失败。Python 入口已纠正后仍因缺 Docker 退出 |
| release helpers Python | 10 项中 9 pass / 1 fail；Linux executable-mode 用例在 Windows 读到 0666 而非 0755，属于平台前提未满足 |
| Apple container 生命周期 fixture | 依赖 macOS `stat -f` 与权限行为，未在 Windows 伪装执行；需由对应 macOS CI 验证 |

这些辅助项与三个 PR 的改动范围无交集；本轮没有安装 Docker、WSL 或修改系统服务来消除环境差异。

## 7. 复现方式

下列命令均使用上述固定工具链环境。数据库变量仅指向本轮创建的临时库，不得直接复制到生产 DSN。

```powershell
# backend 下，依次执行；每条使用独立 GOTMPDIR
go test -p 1 -count=1 -json ./...
go test -tags=unit -p 1 -count=1 -json ./...
go test -tags=integration -p 1 -count=1 -json ./...

# 已创建隔离 PostgreSQL 后分别设置所需变量
# SUB2API_POSTGRES_ONLY_INTEGRATION_DSN：部门/订阅专项
go test -tags=integration -p 1 -count=1 -json ./internal/repository -run 'TestDepartment|TestOrganizationUsageDepartment|TestUserSubscriptionRepoSuite|TestUserAdminAccess'
# SCOPED_ADMIN_TEST_DSN：受限管理密钥 SQL 专项
go test -tags=unit -p 1 -count=1 -json ./internal/repository ./internal/service -run 'CustomAdmin|Scoped'

# 时间轮专项使用 CGO_ENABLED=1、有效 MinGW CC
go test -race -p 1 -count=3 -json ./internal/service/timing_wheel_service.go ./internal/service/timing_wheel_service_test.go

golangci-lint run --timeout=30m ./...
golangci-lint run --new-from-rev=7d0f760fc --timeout=30m ./...

# frontend 下
pnpm install --frozen-lockfile
pnpm run lint:check
pnpm run typecheck
pnpm exec vitest run --maxWorkers=4 --minWorkers=1
pnpm run build
```

失败对照使用 Go `-overlay`：把本轮修改的全部 13 个 Go 文件恢复成 `7d0f760fc` 的字节内容，未切换或覆盖 main 工作树；Go module 文件和其他后端源码在两端无此次 PR 差异。overlay 与映射文件保存在证据目录，适用于这几项读取编译后源码的用例，不是对整个旧版本重新做了全量验收。

完整执行参数和持续时间以 `*.result.json` 为准；Go 事件日志为 `*.stdout.log`，统计为 `*.stdout.summary.json`，前端原始结果为 `frontend-vitest.json`。库服务已停止，如需重跑数据库专项应重新启动对应隔离实例或另建测试库。

## 8. 后续建议

| 优先项 | 建议 |
| --- | --- |
| 全量测试门禁 | 修正两项仓储 mock 与现行 dirty 标记顺序的预期，定位 Command Code 不稳定测试的隔离/时序原因 |
| WebSocket 未闭环项 | 保留首次全量日志，针对关闭帧时序进一步验证；不根据本轮复跑全过宣布已解决 |
| 全量 lint | 单独清理 34 项现有问题，避免混入已验证的三个修复 PR |
| 环境覆盖 | 在 Linux + Docker/Redis 7+ 和相应 macOS CI 中补齐未执行项；真实账号、浏览器完整流程与生产行为仍需各自验收 |

本轮结果可以支持“当前三个修复的针对性验证通过、未发现明确由其引入的新失败”；不能支持“整个项目所有测试全绿”或“生产已经验收”。
