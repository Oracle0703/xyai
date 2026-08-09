# 最终交付报告

## 摘要

| 字段 | 内容 |
| --- | --- |
| 日期 | 2026-08-10（Asia/Shanghai） |
| 分支 | `feature/hy/10174_org_usage_report_display` |
| 基线 | `github/main=1558cd9f156e034f9d1aa139e6b19780264b4406` |
| 实现审查基线 | `6828cc1668e5a49d872c7ab634653bce504dde9c` |
| 交付状态 | 功能、文档、自动化、静态和范围验证已完成；页面级视觉检查受阻 |

## 已交付行为

| 区域 | 结果 |
| --- | --- |
| 人数标题 | `active_users` 显示为“注册人数 / Registered users”，`used_users` 显示为“活跃人数 / Active users” |
| 组织展示 | `xunyou(.com)` / `wsdashi(.com)` 显示为“迅游 / 速宝”，未知值本地化回落 |
| 趋势图 | 展示 input、output、total Token 与 requests，不再展示 cache Token 曲线 |
| XLSX | 六 Sheet 同步人数标题与组织显示名，数值、Sheet 名和导出限制不变 |
| 合同边界 | API、后端、筛选请求值、View `as_of` 状态机和 export worker 不变 |

## 验证证据

| 验证 | 结果 |
| --- | --- |
| 专项 Vitest | 6/6 files、64/64 tests，exit 0 |
| 完整前端 Vitest | 246/246 files、1704/1704 tests，exit 0 |
| TypeScript | `typecheck` exit 0 |
| ESLint | `lint:check` exit 0，无 lint warning |
| Git 范围 | `diff --check` exit 0；24 个固定范围路径；backend/API/受保护控制器路径无 diff |
| 规格 / 质量审查 | SC-1 至 SC-6 全部符合；findings：无问题 |

测试输出不是 pristine：专项/全量包含既有 `caniuse-lite` 过期 warning，全量还包含测试预期路径产生的 Vue/i18n/异常 stderr；这些输出未导致失败。详细证据见 `test-review.md`。

## 视觉与试用

| 项目 | 状态 |
| --- | --- |
| 试用 URL | `http://127.0.0.1:5174/admin/organization-usage` |
| HTTP | 根路径与目标路由均 200，响应 431 bytes |
| Dev server | 已保持运行；未关闭成功启动的服务 |
| 页面级桌面/移动检查 | 受阻：缺少可用管理员会话和后端数据；Vite 日志记录 public config fetch failed |

## 文件与范围

- Tasks 1-5 修改 24 个计划内路径，包括 14 个前端路径、稳定文档/wiki 与 linked worktree ignore。
- Task 6 只修改/新增本目录的 `plan.md`、`delivery-status.md`、`test-review.md`、`delivery-report.md`。
- 本轮未修改业务代码、wiki、README、后端或 API。

## 已知限制

- 仍需在具备管理员会话和真实后端数据的环境中人工确认桌面/移动标题截断、图例外观和表格重叠。
- `frontend/node_modules` 为依赖 junction；未执行依赖安装或升级，不构成产品失败。
- 未 push、未创建 PR、未合并。
