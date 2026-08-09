# 实施计划

| 字段 | 内容 |
| --- | --- |
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 详细计划 | `docs/superpowers/plans/2026-08-09-organization-usage-report-display.md` |
| 分支 | `feature/hy/10174_org_usage_report_display` |
| 负责人 | 控制器协调；RD 与 QA 按批准的执行方式分派 |

## 任务清单

| 状态 | 任务 | 负责人 / 智能体 | 文件 / 模块范围 | 验证方式 |
| --- | --- | --- | --- | --- |
| 已完成 | 1. 唯一组织显示映射与页面接入 | RD | report util、Filters、Summary、People、相关测试 | helper + Filters + View Vitest |
| 已完成 | 2. XLSX 全路径同步 | RD | report util、Workbook 测试 | Workbook Vitest，覆盖 Champion/组织/人员/月周日 |
| 已完成 | 3. 人数双语文案 | RD | zh/en locale、locale 测试 | Locale + View Vitest |
| 已完成 | 4. 趋势改为总 Token | RD | TrendChart 及专项测试 | dataset label/data/axis Vitest |
| 已完成 | 5. 稳定文档同步 | RD | 组件 README、两份 feature 设计、frontend wiki | 过期合同扫描 |
| 已完成 | 6. 规格审查、代码审查与完整验证 | QA / 控制器 | 全部变更和交付文档 | 自动化、静态与范围验证完成；页面级视觉检查受环境阻塞并已记录 |

## 测试优先顺序

| 顺序 | RED 证据 | 最小实现 | GREEN 证据 |
| --- | --- | --- | --- |
| 1 | namespace import 断言 helper 缺失，页面仍显示域名 | 新增纯映射并接入三个组件 | helper/Filters/View 通过，request value 仍为 `xunyou` |
| 2 | workbook 仍有旧标题和内部键 | 四类行复用 helper，改两个人数标题 | 六 Sheet 与数值合同通过 |
| 3 | zh/en 仍是旧人数文案 | 只改两个 locale value | 双语精确值通过 |
| 4 | Trend 第三条仍读取 `[5]` cache | label/data 改为 total `[36]` | 四曲线与现有状态测试通过 |

## 文件所有权

| 任务 | 允许修改 | 禁止修改 |
| --- | --- | --- |
| 1 | `organizationUsageReport.ts`、三个组织组件及对应测试 | API 类型、View 控制器、后端 |
| 2 | `organizationUsageReport.ts`、Workbook 测试 | Sheet 名、Worker、分页限制 |
| 3 | 两个 locale 文件和 locale 测试 | i18n key 名、其他域文案 |
| 4 | TrendChart 与专项测试 | Trend API、View `as_of` 状态机 |
| 5 | 明列的四份稳定文档 | backend/domain wiki 的统计定义 |
| 6 | 交付文档；业务文件仅允许修复 review 发现的本需求问题 | 无关重构、依赖升级、发布动作 |

## 验证命令

```powershell
cd frontend
cmd.exe /c node_modules\.bin\vitest.cmd run src/utils/__tests__/organizationUsageReport.spec.ts src/utils/__tests__/organizationUsageWorkbook.spec.ts src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts src/components/admin/organization-usage/__tests__/OrganizationUsageTrendChart.spec.ts src/i18n/__tests__/organizationUsageLocale.spec.ts src/views/admin/__tests__/OrganizationUsageView.spec.ts
cd ..
cmd.exe /c pnpm --dir frontend run test:run
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend run lint:check
git diff --check github/main...HEAD
git diff --name-only github/main...HEAD -- backend frontend/src/api/admin/organizationUsage.ts
```

## 审查关卡

| 关卡 | 必需证据 | 状态 |
| --- | --- | --- |
| 规格符合性审查 | SC-1 至 SC-6 都映射到实现与测试；无额外业务变化 | 已完成，无问题 |
| 代码质量审查 | 唯一映射、回落、XLSX、Chart、双语和请求键均经复核 | 已完成，无问题 |
| 最终验证 | 专项/全量测试、typecheck、lint、diff 与视觉检查均有真实结果 | 已完成；页面级桌面/移动视觉检查因缺少管理员会话和后端数据受阻 |

## 回滚

本功能没有 migration、配置或后端兼容负担。若需要撤销，先用 `git log --oneline github/main..HEAD` 列出本分支提交，由用户明确指定需要撤销的实际 SHA，再在功能分支上用 `git revert` 创建反向提交；不得改写 `main` 历史。回滚后重跑 locale、Workbook、TrendChart 和 View 专项测试，确认恢复旧展示且内部合同始终未变。
