# Organization Usage Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复组织用量报表审核确认的问题，补齐真实 PostgreSQL 合同测试，并用 30/90/366 天执行计划决定是否需要 SQL 重构。

**Architecture:** 保持现有 Handler/Service/Repository 和独立页面边界不变。前端在新筛选请求完成前隐藏旧概览数据；后端将邮箱查询转成 PostgreSQL `ILIKE ... ESCAPE` 的字面匹配；真实数据库合同通过现有 integration Testcontainers harness 验证。性能阶段只采集并记录证据，不在缺少执行计划时改写聚合 SQL。

**Tech Stack:** Vue 3、Vitest、Go、PostgreSQL、sqlmock、Testcontainers、PowerShell。

---

### Task 1: 筛选加载期间隐藏陈旧概览

**Files:**
- Modify: `frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts`
- Modify: `frontend/src/views/admin/OrganizationUsageView.vue`
- Modify: `frontend/src/components/admin/organization-usage/README.md`

- [x] **Step 1: 写失败测试**

在首次请求返回后，让第二次筛选请求保持 pending；断言人员表进入 skeleton，旧 Overview 和组织汇总不再可见。

- [x] **Step 2: 运行 RED 验证**

Run: `cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/OrganizationUsageView.spec.ts`

Expected: 新测试因旧 Overview/组织汇总仍存在而失败。

- [x] **Step 3: 最小实现**

用统一的 loading 条件控制 Overview 和组织汇总渲染，保留人员表现有 loading skeleton；请求完成后再展示新响应。

- [x] **Step 4: 运行 GREEN 验证**

Run: `cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/OrganizationUsageView.spec.ts`

Expected: 全部通过。

### Task 2: 邮箱搜索使用字面匹配

**Files:**
- Modify: `backend/internal/repository/organization_usage_repo_test.go`
- Modify: `backend/internal/repository/organization_usage_repo.go`

- [x] **Step 1: 写失败测试**

新增 `%`、`_`、反斜杠组合输入，期望 pattern 按反斜杠、百分号、下划线顺序转义；同时断言两个 `ILIKE` 入口都声明 PostgreSQL `ESCAPE`。

- [x] **Step 2: 运行 RED 验证**

Run: `go test -tags=unit -p 1 -count=1 ./internal/repository -run OrganizationUsage`

Expected: pattern 仍返回未转义文本，测试失败。

- [x] **Step 3: 最小实现**

使用 `strings.NewReplacer` 将 `\`、`%`、`_` 分别替换为 `\\`、`\%`、`\_`，外层继续包裹 `%...%`；SQL 使用 `ILIKE $n ESCAPE E'\\'`。

- [x] **Step 4: 运行 GREEN 验证**

Run: `go test -tags=unit -p 1 -count=1 ./internal/repository -run OrganizationUsage`

Expected: 全部通过。

### Task 3: 统一 canonical as_of 文档口径

**Files:**
- Modify: `docs/features/organization-usage-report-design-cn.md`
- Modify: `llm-wiki/wiki/backend.md`
- Modify: `llm-wiki/wiki/frontend.md`
- Modify: `llm-wiki/wiki/data-and-domain.md`

- [x] **Step 1: 替换错误术语**

删除 `signed`、签名、签发措辞，统一描述为“服务端裁剪到当前时间并规范化为 UTC RFC3339Nano 的 canonical `as_of` 时间戳”。明确它冻结 usage 上界但不冻结用户表，不声称具备防篡改能力。

- [x] **Step 2: 扫描残留**

Run: `rg -n "signed snapshot|signed|签名快照|as_of 签名|签发" docs/features/organization-usage-report-design-cn.md llm-wiki/wiki/backend.md llm-wiki/wiki/frontend.md llm-wiki/wiki/data-and-domain.md`

Expected: 无错误术语残留。

### Task 4: 补真实 PostgreSQL Repository 合同

**Files:**
- Create: `backend/internal/repository/organization_usage_repo_integration_test.go`

- [x] **Step 1: 写 integration 测试**

使用现有 `integration_harness_test.go` 和唯一邮箱前缀隔离数据，覆盖三种域名及大小写/子域名、零用量 active 用户、disabled/soft-deleted 排除、北京时间日界线、周一边界、跨月 partial、Token/actual cost 聚合和稳定并列规则。

- [x] **Step 2: 运行真实 PostgreSQL 测试**

Run: `go test -tags=integration -p 1 -count=1 ./internal/repository -run OrganizationUsage`

Expected: Docker 可用时由 Testcontainers PostgreSQL 执行并通过；本机无 Docker 时必须如实记录环境阻塞，不能用 sqlmock 冒充 integration。

### Task 5: 采集 30/90/366 天执行计划

**Files:**
- Create: `docs/features/organization-usage-report-performance-cn.md`

- [x] **Step 1: 记录环境与数据基线**

记录 PostgreSQL 版本、`users`/`usage_logs` 行数、目标时间窗行数和关键索引，禁止记录 DSN、用户名或密码。

- [x] **Step 2: 执行查询分析**

对 Summary 的组织汇总、人员聚合分页、Champion 以及 Periods 数据查询执行 `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)`，分别使用 30、90、366 天范围。

- [x] **Step 3: 给出重构门槛结论**

按最慢查询、重复扫描、临时排序/磁盘 spill、buffer reads 和单次耗时给出“当前无需重构”或“进入 SQL 重构”的证据化结论；环境不可用时文档明确标记未执行和所需命令，不填写推测数值。

### Task 6: 全量验证

**Files:**
- Verify all modified files.

- [x] **Step 1: 后端验证**

使用 `llm-wiki/wiki/ops.md` 的仓库内 `GOCACHE`/`GOMODCACHE`、每轮 fresh `GOTMPDIR`、`-p 1 -count=1` 运行 Repository 和 Service。

- [x] **Step 2: 前端验证**

Run: `cmd.exe /c pnpm --dir frontend run test:run`

Run: `cmd.exe /c pnpm --dir frontend run typecheck`

Run: `cmd.exe /c pnpm --dir frontend run lint:check`

- [x] **Step 3: 差异验证**

Run: `git diff --check`

检查仅包含本计划文件、审核回复及五项修复范围内文件，不覆盖用户已有改动。
