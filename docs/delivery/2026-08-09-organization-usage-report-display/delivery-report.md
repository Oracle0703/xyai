# 最终交付报告

## 摘要

| 字段 | 内容 |
| --- | --- |
| 日期 | 2026-08-10（Asia/Shanghai） |
| 分支 | `feature/hy/10174_org_usage_report_display` |
| 基线 | `github/main=1558cd9f156e034f9d1aa139e6b19780264b4406` |
| 最终审查 HEAD | `43e5d9a9918a3e4d4b309ec6d2178a33e5c92ebf` |
| 修复复审 HEAD | `29047994a6917d9ce32a7e47e068e0890ec0bddc` |
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
| Git 范围 | `diff --check` exit 0；26 个固定范围路径；backend/API/View 控制器/export worker 无 diff |
| 规格 / 质量审查 | SC-1 至 SC-6 全部符合；最终审查 Ready to merge；1 项 Minor 文档偏差已修复并通过限定复审 |

测试输出不是 pristine：专项/全量包含既有 `caniuse-lite` 过期 warning，全量还包含测试预期路径产生的 Vue/i18n/异常 stderr；这些输出未导致失败。详细证据见 `test-review.md`。

## 视觉与试用

| 项目 | 状态 |
| --- | --- |
| 试用 URL | `http://127.0.0.1:5174/admin/organization-usage` |
| HTTP | 根路径与目标路由均 200，响应 431 bytes |
| Dev server | 仅 `127.0.0.1:5174` 保持运行；误启动的 `3000` 已停止 |
| 桌面浏览器 | 目标路由重定向到 `/login?redirect=/admin/organization-usage`，只可见登录表单 |
| 移动视口 | 390x844 下同样进入登录门槛，无法看到报表内容 |
| 页面级桌面/移动检查 | 受阻：缺少可用管理员会话和后端数据；未输入凭据或读取浏览器存储；Vite 日志记录 public config fetch failed |

## 文件与范围

- 最终分支相对固定基线共有 26 个路径：14 个前端路径、12 个 docs/wiki/ignore 路径。
- 最终审查修复只校正文档中的缓存字段边界，没有修改业务代码或测试。
- 后端、API、`OrganizationUsageView.vue` 状态机和 export worker 均无差异。

## 已知限制

- 仍需在具备管理员会话和真实后端数据的环境中人工确认桌面/移动标题截断、图例外观和表格重叠。
- 完整原始测试输出未提交到仓库；收尾阶段已新鲜重跑并读取退出码与最终汇总。
- `frontend/node_modules` 为依赖 junction；未执行依赖安装或升级，不构成产品失败。
- 未 push、未创建 PR、未合并。
