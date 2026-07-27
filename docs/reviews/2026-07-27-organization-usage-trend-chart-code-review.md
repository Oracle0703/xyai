# 组织用量趋势图实现审核

## 审核结论

| 项目 | 结论 |
| --- | --- |
| 审核日期 | 2026-07-27 |
| 审核对象 | `feature/hy/org-usage-trend-chart` 工作树相对 `main@26d39c9e6` 的实现 |
| 设计合同 | `docs/features/organization-usage-trend-chart-design-cn.md` |
| 总体结论 | **不建议提交或合并，当前未达到设计文档 Definition of Done** |
| 阻断项 | 4 项：2 个交付/验证缺口、1 个共享快照逻辑缺陷、1 个关键前端测试矩阵缺口 |
| 非阻断收尾项 | 2 项：设计状态未闭环、分支命名不符合本轮明确规范 |
| 业务代码修改 | 本审核未修改业务代码 |

实现主链已经存在：后端新增 Trend Handler/Service/Repository/Route，前端新增 API、粒度推断、双轴图表和 Summary/Trend 双请求编排，中英文与 llm-wiki 也有同步。问题不在“完全没做”，而在于核心文件尚未进入 Git、设计强制的真实 PostgreSQL 证明未实施、共享快照协议仍有放行缺口，并且关键 UI/状态机测试没有覆盖设计列出的高风险路径。

## Findings

### F1 - Blocker：两个核心新增文件仍为 untracked，当前提交路径可能直接漏掉功能主体

**证据**

```text
?? docs/features/organization-usage-trend-chart-design-cn.md
?? frontend/src/components/admin/organization-usage/OrganizationUsageTrendChart.vue
```

- `OrganizationUsageView.vue` 已静态 import `OrganizationUsageTrendChart.vue`，但组件本身未跟踪。
- 设计文档也是本实现的唯一完整合同，但仍未跟踪。
- 如果使用 `git add -u`、基于 tracked diff 生成补丁或从干净 checkout 构建，这两个文件不会进入提交；前端将因缺少组件 import 直接失败。

**判定**

这是交付阻断，不是 Git 外观问题。必须在提交前显式确认 intended paths，并验证干净提交快照包含这两个文件。

**建议**

1. 显式纳入两个新增文件，禁止只用 `git add -u`。
2. 从暂存快照或干净 worktree 重跑 typecheck/build，证明不是仅在本机未跟踪文件存在时可用。

### F2 - High：设计强制的 PostgreSQL Trend integration 完全未实施，SQL 正确性目前只有字符串断言

**设计依据**

- 设计文档第 779 行将 PostgreSQL integration 标为“PR1 必做”。
- 第 830-831 行要求验证真实 `generate_series`、补零、北京时间边界、week/month partial、54 周桶、org/q、禁用/删除用户，以及与 Summary 在同一 canonical `as_of` 下的总和一致性。
- 第 924 行再次把 `backend/internal/repository/organization_usage_repo_integration_test.go` 标为“必改”。

**实现证据**

- `backend/internal/repository/organization_usage_repo_integration_test.go` 没有工作树 diff。
- 文件现有四个 integration case 仅覆盖 Summary、搜索、Periods 北京边界和 Champion tie（入口分别在第 16、74、89、136 行），没有任何 `repo.Trend(...)` 调用。
- 新增的 `TestOrganizationUsageTrendSQL_ZeroFillsWithoutUserDimension` 只用 `require.Contains/NotContains` 检查 SQL 文本；`TestOrganizationUsageRepositoryTrend_ScansZeroFilledPoint` 使用 sqlmock 自行提供结果行。两者都不能证明 PostgreSQL 实际桶边界、补零或聚合结果。

**风险**

Trend 的核心价值正是 PostgreSQL 日期序列和时区聚合。字符串测试即使全绿，也可能漏掉类型推断、时区、月末步进、partial 裁剪、过滤总和或真实扫描行为。设计已明确写出“不得把 sqlmock 当作日期桶/补零正确性的证据”。

**建议**

按设计矩阵补真实 integration，至少覆盖 day 空桶、未来桶裁剪、跨月 week、month 月末、54 周上界、org/q、禁用/删除用户、Summary/Trend 同快照总和。没有这些用例前，不应宣称后端合同完成。

### F3 - High：canonical Trend 重拉后缺少 `range.as_of` 会被当成成功并展示，违反共享快照硬合同

**设计依据**

设计的共享快照规则要求：

- Summary canonical 是页面权威快照。
- Trend 不一致时仅重拉一次。
- canonical 重拉后若仍缺失或不同，应清空 points、显示局部错误，且不得循环。
- 最终验收要求 `trend.range.as_of == summary.range.as_of`。

**实现证据**

`frontend/src/views/admin/OrganizationUsageView.vue`：

- 第 238-243 行在接收 Trend 响应时直接写入 `trendPoints/trendMeta`，没有先验证 `response.range.as_of`。
- 第 281 行的终态校验是：

```ts
if (trendMeta.value?.range.as_of && trendMeta.value.range.as_of !== summaryCanonical) {
```

该条件只有在 `as_of` 为 truthy 且不同才报错；若 canonical 重拉返回的 `range.as_of` 缺失或空字符串，条件为 false，响应 points 会继续显示。

**为什么不合理**

这与 Summary 路径第 195 行的 fail-closed 处理不对称，也破坏了 K6 的最终一致性保证。部署版本不一致、代理裁剪字段或服务端协议回归时，页面会把无法证明同快照的数据当成已对齐。

**建议**

Trend 成功写状态前先要求非空 canonical；reconciliation 终态使用严格相等判断：缺失或不同都进入局部错误。增加“首次缺失”“重拉后缺失”“重拉后仍不同且只请求一次”三个回归测试。

### F4 - High：关键前端测试矩阵未实施，现有 71 项通过不能证明图表组件和 reconciliation 状态机

**设计依据**

设计文档第 842-843 行明确要求：

- TrendChart：loading/error/retry、粒度 emit、partial/as_of tooltip、运行时主题切换、双轴零基线、requests 整数轴、单调插值、密集点半径、maxTicksLimit、全零仍画轴。
- View：candidate 双拉、Summary canonical 缺失、canonical 不同只对齐一次、重拉仍不同、粒度不打 Summary、旧 cycle、局部失败/retry、filter abort Trend、retry-load 新 cycle。
- 第 939-940 行明确列出 `OrganizationUsageTrendChart.vue + spec` 和 View 状态机测试。

**实现证据**

- `frontend/src/components/admin/organization-usage/` 下新增了组件，但没有对应 `__tests__/OrganizationUsageTrendChart.spec.ts`。
- `OrganizationUsageView.spec.ts` 全局 mock 掉 TrendChart，只渲染一个空 stub，因此不会执行 Chart.js 配置、tooltip、主题 observer 或组件交互。
- View 新增断言主要覆盖初次 Summary/Trend 参数、人员排序分页不重拉 Trend、卸载 abort；没有 canonical reconciliation、粒度切换、Trend 局部失败/retry 和旧 Trend cycle 的断言。

**风险**

当前 Vitest 绿灯只证明已写用例通过，不能覆盖本功能最复杂的状态机和图表运行时行为。F3 正是因为相关设计用例没有落地而未被发现。

**建议**

补独立 TrendChart spec，并按设计第 843 行逐项补 View 用例。对 reconciliation 使用可控 Promise，明确断言请求次数、AbortSignal、终态 points/error 和“不循环”。

### F5 - Medium：主设计仍标记 Draft，与 wiki 的“已实现”陈述不一致

**证据**

- `docs/features/organization-usage-trend-chart-design-cn.md:7` 仍是 `状态 | Draft`。
- 设计 PR3 清单第 954 行要求“本文状态 Draft → 已实现 + 提交哈希”。
- `llm-wiki/wiki/backend.md` 与 `frontend.md` 已按已落地能力描述 Trend endpoint 和页面状态机。

**风险**

权威设计合同与快速知识库对功能成熟度给出不同信号，后续 AI 可能把未完成验收的实现当成稳定基线。

**建议**

在 F1-F4 闭环且形成提交后，再把状态改为“已实现/待合并”并记录准确提交；在此之前不要让 wiki 表述超过真实验收状态。

### F6 - Medium：当前功能分支名不符合本轮明确的功能分支规范

**证据**

- 当前分支：`feature/hy/org-usage-trend-chart`。
- 本轮明确规范：新增功能分支必须使用 `feature/hy/10xxx_XXX`。

**判定与建议**

这是交付流程不合规，不影响本地运行，但会影响分支治理和后续审计。提交前应按实际需求编号改为符合规范的名称；编号不能从源码可靠推断，应由负责人确认。

## 已合理实施的部分

| 设计项 | 审核结果 | 依据 |
| --- | --- | --- |
| 独立 Trend endpoint | 已实施 | Handler、Service、Repository、`registerUsageRoutes` 均有增量 |
| 不复用 user×period 导出接口 | 已实施 | Trend SQL 无 user 维，`fetchAll` 未调用 Trend |
| 服务端补零与未来桶边界结构 | 代码结构已实施，真实 DB 未证明 | `generate_series` + LEFT JOIN + `$7 data_through`；受 F2 限制 |
| day/week/month 与自动推断 | 已实施 | Service allowlist；前端复用含首尾自然日 helper，31/32、120/121 测试存在 |
| 双轴与默认四系列 | 已实施 | Token 左轴、requests 右轴；未画 total/cache_creation/cost；两轴 `beginAtZero` |
| 密集点与主题响应 | 已实施，缺运行时测试 | `>60` 点半径 0；MutationObserver 驱动颜色重算 |
| 人员翻页/排序不拉 Trend | 已实施且有测试 | `loadReport()` 只打 Summary；View test 断言 Trend 调用数不变 |
| 路由认证边界 | 已实施 | 与 summary/periods 注册在同一 admin usage 路由组，无新 permission 位 |
| i18n / README / llm-wiki | 已更新 | zh/en、组件 README、backend/frontend wiki 均有增量 |

## 验证记录

| 验证 | 结果 | 说明 |
| --- | --- | --- |
| `go test -p 1 -count=1 ./internal/service ... -run OrganizationUsage` | 部分首轮通过 | service、routes 通过；repository、handler 首轮被 Windows `.test.exe` 文件锁阻断 |
| repository 独立 fresh `GOTMPDIR` 重跑 | 通过 | `ok .../internal/repository` |
| handler 独立 fresh `GOTMPDIR` 重跑 | 通过 | `ok .../internal/handler/admin` |
| `vitest run organizationUsage`（`frontend` 工作目录） | 通过 | 10 个 test files，71 tests passed |
| `pnpm --dir frontend run typecheck` | 通过 | `vue-tsc --noEmit` exit 0 |
| `git diff --check` | 通过 | exit 0 |
| PostgreSQL Trend integration | **未实施/未验证** | 无 Trend integration case；本机无 Docker 命令，且未设置外部 integration DSN |
| 浏览器手工验收 | 未执行 | 本轮审核未启动应用或写入业务环境 |

说明：现有通过项不消除 F2/F4，因为“测试通过”与“设计要求的测试存在”是两个独立门禁。

## 建议修复顺序

| 顺序 | 动作 | 完成标准 |
| --- | --- | --- |
| 1 | 修 F3，并补 reconciliation 回归测试 | 缺失/不同 canonical 均 fail-closed；每周期最多一次重拉 |
| 2 | 补 PostgreSQL Trend integration | 设计第 830-831 行矩阵全部有真实 DB 断言 |
| 3 | 补 TrendChart 和 View 状态机测试 | 设计第 842-843 行关键场景逐项落地 |
| 4 | 处理 F1 与分支规范 | intended files 全部进入提交；分支名合规 |
| 5 | 更新设计状态与准确验证记录 | design/wiki/提交状态一致，再做最终 build 和干净快照验收 |

## 最终 Gate

当前结论为 **Changes requested**。F1-F4 任一未关闭都不应合并；F5-F6 应在交付前收尾。关闭后需重新执行聚焦 Go、真实 PostgreSQL integration、前端 Vitest、typecheck/build、`git diff --check`，并在干净提交快照上确认新增组件和设计文档均存在。
