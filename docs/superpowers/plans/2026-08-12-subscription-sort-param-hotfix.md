# Subscription Sort Parameter Hotfix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复管理端订阅列表在带筛选条件时因排序表达式丢失时间参数而返回 HTTP 500 的问题。

**Architecture:** 保持现有每日剩余比例排序规则不变，只修正 Ent `ORDER BY` 表达式的参数构造，使排序时间参数参与最终查询的统一占位符编号。使用 PostgreSQL 方言的查询生成回归测试覆盖 `WHERE` 参数在前、排序参数在后的场景，并运行真实 PostgreSQL 集成测试（环境可用时）。

**Tech Stack:** Go 1.26、Ent SQL Builder、PostgreSQL、testify。

## Global Constraints

- 分支必须为 `hotfix/hy/0000_subscription_sort_param`。
- 不修改或清理工作区中原有的无关变更。
- 不修改排序业务口径，不执行手工 SQL，不直接修改生产环境。

---

### Task 1: Add a parameter-binding regression test

**Files:**
- Modify: `backend/internal/repository/user_subscription_repo_query_test.go`

**Interfaces:**
- Consumes: `userSubscriptionAdminOrder(filter service.SubscriptionAdminFilter, startOfDay time.Time) func(*entsql.Selector)`
- Produces: 回归约束，保证既有筛选参数为 `$1` 时排序时间使用 `$2`，且两个参数均进入最终参数列表。

- [ ] **Step 1: Write the failing test**

构造 PostgreSQL selector，先添加 `status = active`，再调用真实排序函数，并断言最终查询包含 `daily_window_start < $2`，参数依次为 `active` 和 `startOfDay`。

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=unit -count=1 ./internal/repository -run TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments -v`

Expected: FAIL，显示排序仍引用 `$1` 或缺少第二个时间参数。

### Task 2: Preserve ORDER BY arguments

**Files:**
- Modify: `backend/internal/repository/user_subscription_repo.go:454`

**Interfaces:**
- Consumes: Ent selector 当前 PostgreSQL dialect 与此前累计参数总数。
- Produces: 带真实参数的 `ORDER BY` expression，参数编号由外层 selector 统一递增。

- [ ] **Step 1: Implement the minimal fix**

将会丢失参数的 `OrderExprFunc` 改为保留 `startOfDay` 参数的表达式构造方式，不改变 CASE、JOIN、次级排序或稳定 ID 排序。

- [ ] **Step 2: Run focused tests**

Run: `go test -tags=unit -count=1 ./internal/repository -run 'TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments|TestListAdminOrganizationFilterExplicitlyExcludesSoftDeletedUsers' -v`

Expected: PASS。

### Task 3: Verify and hand off

**Files:**
- Modify if needed: `llm-wiki/wiki/backend.md`
- Modify if needed: `llm-wiki/wiki/data-and-domain.md`

**Interfaces:**
- Consumes: 修复后的查询和现有订阅列表集成测试。
- Produces: 可审计的测试、构建、Git 状态和提交结果。

- [ ] **Step 1: Run repository package tests and build**

运行 repository 单元测试、相关 PostgreSQL integration（环境可用时）、`go test ./internal/repository` 和 `go build ./...`。

- [ ] **Step 2: Check documentation impact**

确认现有 wiki 已描述排序业务口径；只有可靠性约束缺失时才追加简洁说明，并保持 LF。

- [ ] **Step 3: Review and commit only hotfix files**

检查 `git diff --check`、目标 diff 和现有无关改动，仅暂存并提交本次测试、实现及必要文档。
