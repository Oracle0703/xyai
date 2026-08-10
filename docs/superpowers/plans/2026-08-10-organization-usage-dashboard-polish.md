# Organization Usage Dashboard Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以纯前端小改动保证导出条件与已查询结果一致，并补齐运营概览的活跃率、成本、截止时间和口径说明。

**Architecture:** 保留现有 Summary/Trend 双请求和 API 数据合同。页面只新增 mode-aware 筛选指纹；Overview 只消费已有 `overview`、`range` 和 `champions` 字段，不增加服务端请求。

**Tech Stack:** Vue 3、TypeScript、Vue I18n、Vitest、Vue Test Utils、Tailwind CSS。

## Global Constraints

- 分支必须为 `feature/hy/10175_org_usage_dashboard_polish`。
- 不修改后端、数据库、API、DTO、趋势数据集、人员表或 XLSX 结构。
- 不新增依赖；组织内部键和 API 筛选值保持不变。
- 生产代码前必须先看到对应测试因目标行为缺失而失败。

---

### Task 1: Bind Export To Applied Filters

**Files:**
- Modify: `frontend/src/views/admin/OrganizationUsageView.vue`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsageFilters.vue`
- Modify: `frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts`
- Modify: `frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts`
- Modify: `frontend/src/i18n/locales/zh/admin/organizationUsage.ts`
- Modify: `frontend/src/i18n/locales/en/admin/organizationUsage.ts`

**Interfaces:**
- Consumes: existing `OrganizationUsageFilterDraft`, `apply/reset/export` emits.
- Produces: optional `exportDisabled?: boolean` prop and internal `filterFingerprint(draft): string` behavior.

- [x] **Step 1: Write the failing View test**

Add a test that changes `month-input` from `2026-07` to `2026-06` without applying, asserts the export button is disabled and `fetchAll` remains uncalled, then applies the filter and asserts export uses `{ start_date: '2026-06-01', end_date: '2026-06-30' }`.

- [x] **Step 2: Write the failing Filters test**

Mount with `exportDisabled: true` and assert `data-testid="export-report"` has `disabled` plus the localized pending-filter title.

- [x] **Step 3: Verify RED**

```powershell
cmd.exe /c node_modules\.bin\vitest.cmd run src\views\admin\__tests__\OrganizationUsageView.spec.ts src\components\admin\organization-usage\__tests__\OrganizationUsageFilters.spec.ts --reporter=dot
```

Expected: both new assertions fail because no export dirty-state contract exists.

- [x] **Step 4: Implement minimal dirty-state behavior**

In the View, fingerprint only `[mode, active date value(s), organization, q.trim()]`. Save the fingerprint after successful apply intent, reset, and organization row selection. Pass `hasPendingFilters` to Filters. In Filters, disable export when `exporting || exportDisabled` and set the pending-filter title.

- [x] **Step 5: Verify GREEN**

Run the Step 3 command and require all tests to pass.

### Task 2: Present Operational Overview Context

**Files:**
- Create: `frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageOverview.spec.ts`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsageOverview.vue`
- Modify: `frontend/src/i18n/locales/zh/admin/organizationUsage.ts`
- Modify: `frontend/src/i18n/locales/en/admin/organizationUsage.ts`
- Modify: `frontend/src/components/admin/organization-usage/README.md`

**Interfaces:**
- Consumes: existing `OrganizationUsageOverview`, `OrganizationUsageRange`, `OrganizationUsageChampions`.
- Produces: six rendered KPI values, report cutoff label, tooltip content, formatted Champion organization.

- [x] **Step 1: Write failing KPI tests**

Use literal fixtures to assert the visible KPI set is registered users, active users, `40.0%`, requests, total tokens and `$12.3400`, while input tokens is absent. Add a zero-denominator fixture expecting `0.0%`.

- [x] **Step 2: Write failing context tests**

Assert the rendered cutoff contains the `trend.asOf` key, the three `HelpTooltip` contents match registered/active/total definitions, and Champion organization values render as `迅游`/`速宝`/`其他`.

- [x] **Step 3: Verify RED**

```powershell
cmd.exe /c node_modules\.bin\vitest.cmd run src\components\admin\organization-usage\__tests__\OrganizationUsageOverview.spec.ts --reporter=dot
```

Expected: assertions fail because active rate, actual cost, cutoff, tooltip and organization labels are absent.

- [x] **Step 4: Implement the minimal Overview change**

Import `HelpTooltip`, `formatCostFixed`, and `formatOrganizationUsageOrganization`; derive active rate locally; switch to a responsive six-item grid; render cutoff and organization labels using existing fields.

- [x] **Step 5: Update locale and README**

Add Chinese/English keys for active rate, pending-export hint and the three metric definitions. Document the applied-filter export rule and Overview KPI set.

- [x] **Step 6: Verify GREEN**

```powershell
cmd.exe /c node_modules\.bin\vitest.cmd run src\components\admin\organization-usage\__tests__\OrganizationUsageOverview.spec.ts src\i18n\__tests__\organizationUsageLocale.spec.ts --reporter=dot
```

Expected: all targeted tests pass.

### Task 3: Review And Verify

**Files:**
- Modify: `docs/delivery/2026-08-10-organization-usage-dashboard-polish/delivery-status.md`
- Create: `docs/delivery/2026-08-10-organization-usage-dashboard-polish/test-review.md`
- Create: `docs/delivery/2026-08-10-organization-usage-dashboard-polish/delivery-report.md`

**Interfaces:**
- Consumes: requirements, specs, implementation diff and test results.
- Produces: spec-compliance verdict, code-quality findings and final evidence.

- [x] **Step 1: Run focused regression**

Run the seven organization usage test files and require all tests to pass.

- [x] **Step 2: Run static checks**

```powershell
cmd.exe /c npm run typecheck
cmd.exe /c npm run lint:check
git diff --check
```

- [x] **Step 3: Request independent review**

Review SC-1 through SC-6 and confirm no backend/API/trend/people/workbook behavior changed.

- [x] **Step 4: Complete delivery artifacts**

Record exact test counts, commands, findings, residual visual risk and Git state without claiming push or deployment.
