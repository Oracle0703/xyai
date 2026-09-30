# `feature/hy/10207_department_usage` 分支上线审核报告

- 审核日期：2026-09-29
- 审核分支：`feature/hy/10207_department_usage`
- 当前 HEAD：`273b76c3558e2f3a63d35d851e0e9783f7a66adb`
- 当前版本：`0.2.8`
- 审核方式：只读代码审查、Git 基线核对、局部测试、构建、集成测试入口和发布门禁检查
- 最终结论：**NO-GO，不建议直接上线**

## 一、范围与分支内容

当前分支不是单一功能分支，HEAD 同时包含以下主要改动：

| 功能范围 | 主要内容 |
|---|---|
| 部门与组织用量 | 部门管理、成员归属、负责人授权、组织用量报表、导出、部门订阅范围及额度重置 |
| 子管理员订阅能力 | 部门范围订阅查询及额度重置权限 |
| 订阅自助日重置 | 自助重置策略、计次、幂等、事件记录和前端入口 |
| GPT 额度展示 | `c-`/`d-` 账号双列额度展示、快照、采集计划、管理员配置 |
| 上游同步 | Sub2API `0.2.8` exact-SHA 合并及相关 Wire/插件接线 |

工作区检查结果：工作区干净，当前分支已跟踪远程分支。

## 二、已通过的检查

| 检查项 | 结果 | 证据 |
|---|---|---|
| Git 工作区 | 通过 | `git status --short --branch` 无未提交修改 |
| 补丁格式 | 通过 | `git diff --check` 退出码 0 |
| 后端普通构建 | 通过 | `go build ./...` |
| 后端嵌入构建 | 通过 | `go build -tags=embed ./...` |
| 后端重点测试 | 通过 | service、handler、middleware、routes、cmd/server 定向测试通过 |
| 前端类型检查 | 通过 | `pnpm.cmd typecheck` |
| 前端 lint | 通过 | `pnpm.cmd run lint:check` |
| 前端生产构建 | 通过 | `pnpm.cmd run build`，1133 modules transformed，构建完成 |
| 知识图谱状态 | 通过 | `tools\\check-understand-status.cmd` 返回 `Status: READY` |

这些结果只能证明编译、静态检查和部分测试路径可运行，不能单独证明生产可上线。

## 三、发布阻断项

### P0：前端全量测试失败

执行命令：

```text
frontend\\pnpm.cmd test:run
```

结果：

```text
Test Files  372 passed (372)
Tests       2789 passed (2789)
Errors      1 error
Process exited with code 1
```

未处理异步错误来源：

```text
src/views/admin/__tests__/AccountsView.selectAllResults.spec.ts
useTableLoader.ts:59
TypeError: Cannot read properties of undefined (reading 'items')
```

影响：测试虽然所有断言通过，但 Vitest 捕获到未处理 rejection，进程退出码为 1。发布门禁不能把该结果视为全量通过。

处理要求：修复异步错误来源或补齐测试 mock 的响应合同，然后重新执行完整 Vitest，并确认无 `Errors`、无 unhandled rejection、退出码为 0。

### P1：后端全量测试失败

执行命令：

```text
backend\\go test -p 1 -count=1 ./...
```

结果：3 个 `backup_pg_dumper` 用例失败：

```text
TestPgDumperHoldsMigrationLockThroughReaderClose
TestPgDumperReleasesMigrationLockWhenProcessFails
TestPgDumperReportsUnlockFailureAndDiscardsConnection
```

共同错误：

```text
start pg_dump: exec: "sh": executable file not found in %PATH%
```

影响：这次失败看起来属于 Windows 测试环境缺少 Git `sh.exe`，不一定是业务实现错误；但当前证据仍然不能称为后端全量测试通过。

处理要求：在 CI 或配置好 Git for Windows `usr\\bin` 的环境中重新执行；必须保留真实退出码和完整日志。如果修复环境后仍失败，再按代码缺陷处理。

### P1：数据库集成测试未实际执行

当前环境执行集成测试时输出：

```text
docker is not available; skipping integration tests
```

因此以下高风险路径没有得到当前环境的有效真实数据库证据：

- 部门和授权事务
- 组织用量查询、分页和导出
- migration 239、240、241、242、243 的从零应用与幂等重跑
- GPT 额度快照、槽位领取和并发采集
- 订阅自助重置并发、回滚、幂等和跨实例缓存失效

退出码为 0 不能把“跳过”解释为“通过”。

处理要求：在 PostgreSQL、Redis 和 Docker/CI 可用环境中执行完整 integration suite，并核对每组用例确实出现 `--- PASS`，没有 skip。

### P1：GPT 额度真实上游采集未验收

`docs/features/gpt-account-quota-display-design-cn.md` 第 13.4 节明确记录：

> 未执行：真实 OpenAI 账号采集（需上线后用隔离账号验证）

目前已有的证据主要是：

- fake 上游和服务层测试
- PostgreSQL 快照/迁移测试
- JWT、权限、配置冲突和页面验收
- 只读路径未访问真实 OpenAI 的验证

尚未证明：

- 真实 OAuth 账号的 wham/usage 请求可成功完成
- 真实代理、token provider 和上游响应解析可用
- 真实 5 小时/7 天窗口数据能稳定落库和展示
- 限流、Retry-After、过期 token 和实际采集失败状态符合预期

处理要求：上线前使用隔离的真实 OpenAI OAuth 账号完成一次单账号采集和一次页面读取；确认不修改 `accounts.extra`、不触发 reset-credit、不改变账号调度状态。

### P1：安全扫描未完成

当前环境未安装 `govulncheck`，命令检查结果为：

```text
govulncheck: NOT INSTALLED
```

项目 CI 要求执行 Go 漏洞扫描，当前没有本轮的有效扫描结果。

处理要求：安装并执行项目要求版本的 `govulncheck ./...`，同时执行前端生产依赖审计及既有 audit exception 检查。

## 四、已确认的代码问题

### P2：省略 `department_ids` 不会清理旧授权

位置：

- `backend/internal/repository/department_repo.go:499`
- `backend/internal/service/department_service.go` 的授权输入处理

当前 SQL：

```sql
DELETE FROM department_access_grants
WHERE user_id=$1 AND NOT (department_id=ANY($2))
```

当 JSON 请求省略 `department_ids` 时，Go 侧得到 `nil`，通过 `pq.Array(nil)` 绑定为 SQL `NULL`。在 SQL 三值逻辑下，`department_id = ANY(NULL)` 不是 TRUE，`NOT (...)` 也不会命中旧授权，因此旧 grant 会保留。

影响：

- UI 正常发送 `[]` 时路径安全
- Admin API Key 或裸 JSON 省略字段时，旧授权不会被清理
- 后续再次打开部门权限时，旧 grant 可能重新生效
- 与降级/软删除后 grant 不复活的生命周期合同不一致

整改要求：明确“全量替换”语义，将缺省 `department_ids` 规范化为空切片，或把字段改为可区分“省略”和“显式空数组”的类型；补充真实 SQL 测试覆盖缺省、`[]`、非空保留列表和 API Key 场景。

### P3：订阅进度接口吞掉所有错误

位置：`backend/internal/service/subscription_service.go:1259`

当前逻辑将 `GetByID` 的所有错误统一转换为 `ErrSubscriptionNotFound`：

```go
sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
if err != nil {
    return nil, ErrSubscriptionNotFound
}
```

影响：

- 部门负责人访问范围外订阅时，越权错误被伪装为 404
- 数据库中断、连接错误等也被伪装为 404
- 运维和客户端无法区分资源不存在、无权限和服务故障

整改要求：只在确实不存在时映射 404；越权错误和数据库错误应原样保留。补充 scoped `ErrDepartmentScopeDenied`、数据库错误和真实不存在三类测试。

### P3：组织用量分页缺少乘法溢出保护

位置：

- `backend/internal/service/organization_usage_service.go:432`
- `backend/internal/repository/organization_usage_repo.go:181`

`normalizeOrganizationUsageQuery` 校验了 `page >= 1` 和 `page_size <= 1000`，但未校验：

```text
(page - 1) * page_size
```

在 `page` 接近 `math.MaxInt` 时可能发生整数溢出，导致 PostgreSQL 收到负 OFFSET 或错误的正 OFFSET。

影响：报表和导出可能返回错误页，仍然属于授权范围内，但违反分页确定性合同。

整改要求：在进入仓储前增加与部门列表一致的溢出检查，并补充 `page=math.MaxInt` 的服务单测，确保不会访问数据库。

### P3：订阅分配候选接口未过滤停用分组

位置：`backend/internal/handler/admin/subscription_assignment_options.go:47`

接口注释称返回 active subscription groups，但实现只判断订阅类型：

```go
if !group.IsSubscriptionType() {
    continue
}
```

未检查 `group.Status`。因此直接调用接口或后端分配路径可能使用停用分组；管理页面的额外前端过滤不能替代后端校验。

整改要求：后端候选接口和 `assignOrExtendSubscription` 共同拒绝非 active 分组，并补充 API 和 service 测试。

## 五、分支基线风险

当前版本：`0.2.8`。

当前 `upstream/main`：`9a62841fd`，已同步 `0.2.9`。因此需要在发布前确认：

- 本次上线是否明确锁定 `0.2.8`
- 是否必须先同步上游 `0.2.9`
- `0.2.9` 是否包含安全修复、兼容修复或数据库变更
- 若不升级，是否有已批准的版本冻结记录

未完成该决策前，不应把当前分支视为最终发布候选。

## 六、上线前必做清单

| 顺序 | 门禁 | 通过标准 |
|---:|---|---|
| 1 | 修复前端 unhandled rejection | Vitest 退出码 0，无 `Errors` 和 unhandled rejection |
| 2 | 解决后端 `sh.exe` 环境边界 | `go test ./...` 退出码 0，3 个 pg dumper 用例实际执行并通过 |
| 3 | 执行真实数据库集成测试 | PostgreSQL/Redis 可用，migration 和高风险事务用例无 skip |
| 4 | 修复 P2 授权缺省语义 | 缺省、空数组、非空列表行为有测试证据 |
| 5 | 修复 P3 错误映射和分页溢出 | 对应单测通过，错误码和分页合同明确 |
| 6 | 修复停用分组分配 | API 和 service 均 fail-closed，并有回归测试 |
| 7 | 完成真实 OpenAI 采集验收 | 隔离账号单条采集、快照落库、用户读取、只读副作用检查通过 |
| 8 | 完成安全扫描 | `govulncheck` 和前端 audit 结果无未处理高危项 |
| 9 | 确认上游版本策略 | 明确锁定 0.2.8 或完成 0.2.9 同步复审 |
| 10 | 重新生成发布证据 | 记录命令、退出码、日志位置、测试数量和未执行项 |

## 七、最终判断

当前分支具备较好的代码完整度和局部验证覆盖，核心构建、类型检查、lint 和多组定向测试通过；但全量测试存在失败，数据库集成测试被跳过，GPT 真实上游采集未验证，安全扫描未完成，并且存在一个 P2 授权语义问题及多个 P3 合同问题。

因此当前结论为：

> **NO-GO：不得直接部署到生产。**

完成上述阻断项并重新生成全量证据后，才能进行下一轮上线审核。
