# 测试与审查记录

## 审查结论

| 项目 | 结论 |
| --- | --- |
| 日期 | 2026-08-10（Asia/Shanghai） |
| 审查范围 | 实现：`github/main` (`1558cd9f1`) 至 Task 5 基线 `6828cc166`；Task 6 文档：`6828cc166..b1c6525a3` |
| 规格符合性 | SC-1 至 SC-6 全部符合 |
| 代码质量 | 无问题 |
| 严重 / 重要 / 一般 findings | 无问题 |
| Task 6 独立审查 | 规格符合、质量批准；Critical / Important / Minor 均无问题 |
| 最终判断 | 自动化、静态和范围验证通过；页面级桌面/移动视觉检查受登录和数据环境阻塞 |

## 成功标准矩阵

| 标准 | 审查证据 | 结论 |
| --- | --- | --- |
| SC-1 | Overview 继续消费 `active_users` / `used_users`；zh 为“注册人数 / 活跃人数”，en 为 `Registered users / Active users`；Locale 与 View 测试覆盖 | 通过 |
| SC-2 | Organization Summary 复用相同 i18n keys；Workbook 组织汇总表头断言为“注册人数 / 活跃人数” | 通过 |
| SC-3 | Trend datasets 为 input、output、total、requests；`total_tokens` 位于 `y`，requests 位于 `yRequests`；不存在 cache dataset；tooltip/as_of 回调保持 | 通过 |
| SC-4 | 唯一 formatter 将短键及 `.com` 形式映射为“迅游 / 速宝”；Filters、Summary、People 与 View 使用同一 formatter | 通过 |
| SC-5 | 六 Sheet 顺序、人数标题及 Champion/组织/人员/月/周/日组织列均有 Workbook 测试 | 通过 |
| SC-6 | 后端、API、View 控制器与 export worker 无 diff；筛选 option value 和请求值保持 `xunyou/wsdashi`；`as_of` 生产逻辑无 diff | 通过 |

## 代码质量审查

| 检查点 | 证据 | 结论 |
| --- | --- | --- |
| 唯一组织映射 | `formatOrganizationUsageOrganization` 仅在 `organizationUsageReport.ts` 定义一次，页面与 XLSX 均复用 | 无问题 |
| 未知值回落 | formatter 接受 `otherLabel`；未知值测试覆盖英文 `Other`，页面传入当前 locale，XLSX 默认“其他” | 无问题 |
| XLSX 六 Sheet | `SheetNames` 精确断言六个合法名称与顺序；所有组织列及数值合同均覆盖 | 无问题 |
| Chart dataset / axis | 四数据集 label、data、axis、零值与 tooltip/as_of 均有断言 | 无问题 |
| 双语 | zh/en 精确文案断言齐全，没有改 i18n key | 无问题 |
| 请求与状态机 | View 断言展示“迅游”后请求仍发送 `organization: 'xunyou'`；`OrganizationUsageView.vue` 与 API 无 diff | 无问题 |
| 范围控制 | 固定范围共 24 个路径：14 个 frontend、10 个 docs/wiki/ignore；无 backend 或 API diff | 无问题 |

## 验证日志

| 命令 / 检查 | 结果 | Exit code / 计数 |
| --- | --- | --- |
| 从 `frontend` 运行 `cmd.exe /c node_modules\.bin\vitest.cmd run` 加 brief 六文件列表 | 通过；含既有 `caniuse-lite` 过期 warning | 0；6 files / 64 tests |
| 从仓库根运行 `cmd.exe /c pnpm --dir frontend run test:run` | 通过；复跑内存捕获确认总计；输出含既有 Vue/i18n/被测异常 stderr 与 `caniuse-lite` warning | 0；246 files / 1704 tests |
| `cmd.exe /c pnpm --dir frontend run typecheck` | 通过 | 0 |
| `cmd.exe /c pnpm --dir frontend run lint:check` | 通过，无 lint warning | 0 |
| `git diff --check github/main...HEAD` 与固定 `1558cd9f1...6828cc166` | 无输出 | 0 |
| `git diff --name-only github/main...HEAD` | 24 个路径，范围符合计划 | 0 |
| `git diff --name-only github/main...HEAD -- backend frontend/src/api/admin/organizationUsage.ts` | 无输出 | 0 |
| 保护路径 diff：backend、API、`OrganizationUsageView.vue`、export worker | 无输出 | 0 |

## 视觉准备与限制

| 项目 | 结果 |
| --- | --- |
| 端口预检 | `5174` 启动前空闲；未停止任何未知进程 |
| brief 原命令 | 宿主同时存在 `Path/PATH` 时 `Start-Process` 首次报 duplicate key；清除当前启动 shell 的重复大写 `PATH` 后成功。pnpm 将单独 `--` 传给 Vite，实际回落项目默认 `3000` |
| 纠偏启动 | 保留已启动的 `3000` 服务，并以 `Start-Process -WindowStyle Hidden` 运行参数可生效的 `pnpm --dir frontend run dev --host 127.0.0.1 --port 5174` |
| 可访问性 | `http://127.0.0.1:5174/` 和 `/admin/organization-usage` 均 HTTP 200，响应 431 bytes；Vite PID 18200 |
| 日志 | `.superpowers/sdd/2026-08-09-organization-usage-report-display/task-6-vite-5174.stdout.log` / `.stderr.log` |
| 桌面浏览器 | 请求 `/admin/organization-usage` 后重定向到 `/login?redirect=/admin/organization-usage`，只显示登录表单，报表 DOM 不可见 |
| 移动视口 | 390x844 下同样进入登录门槛，无法检查真实报表的标题、图例和表格布局 |
| 安全边界 | 未输入凭据，未读取 Cookie、localStorage 或浏览器配置文件 |
| 页面级视觉 | 受阻：本机无可用管理员会话和后端数据，Vite 日志明确 public config fetch failed；未伪造桌面/移动通过 |
| 未执行项 | 未真实确认人数标题截断、组织筛选交互、图例外观及移动表格重叠；自动化合同已覆盖对应非视觉行为 |

## 环境说明与残余风险

- `frontend/node_modules` 是指向主 checkout 依赖的 junction；本轮未安装或升级依赖，这不是产品失败。
- ignored 扫描 linked `node_modules` 时出现 Windows 长路径 warning 并超时；该探索命令不属于产品验证，正式 Git 范围命令均成功。
- Task 6 独立审查可核对文档与 review package，但完整原始测试输出未随审查包保存；审查者将其判定为不阻塞的可复核性限制。
- 残余风险仅为真实管理员数据下的桌面/移动视觉呈现尚未人工确认。
