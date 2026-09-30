# 部门用量与额度重置实施验收

状态：**2026-09-20 S1–S6 已实现，RV1–RV8 本机隔离验收通过。** 完整范围以 [设计](../../features/organization-department-usage-design-cn.md) 为准；未推送、未部署，未合回 main。

## Git 交付

| 要求 | 证据 | 状态 |
| --- | --- | --- |
| 提交原分支和设计 | 原上游合并 `ee829b777`，设计提交 `de5a3e383` | 完成 |
| 合入本地 main | main 已快进至 `de5a3e383` | 完成 |
| 从 main 新建功能分支 | `feature/hy/10207_department_usage` 从该 main 创建 | 完成 |
| 功能提交 | 首版 `a9c53a9f6`；本次修正及证据提交到上述功能分支，具体提交可用 Git 路径日志定位 | 完成 |

未推送远端、未部署；部门功能未合回 main。

## 2026-09-19 旧基线证据（保留）

下列记录只证明当时测试实际覆盖的情形；复核新增缺口按 RV1–RV8 单列。下表的“待补”是当时状态，以后面的本轮复验为准。

| 编号 | 要求 | 验证证据 | 状态 |
| --- | --- | --- | --- |
| D1 | 独立部门页面；组织下一级部门 | 浏览器创建迅游“研发部”和速宝“产品技术部”，页面按组织管理 | 基线通过 |
| D2 | 重名、停用/恢复、未分配 | PG 组织内重名拒绝、跨组织同名允许；UI 停用保留成员且禁止添加，恢复成功 | 基线通过 |
| D3 | 一人一部门、多平台订阅独立 | nullable FK；同一成员三平台订阅仍存在，部门调整不改订阅/API Key | 基线通过 |
| D4 | 成员批量原子化、CAS、审计 | PG 跨组织整批回滚且无成功审计；版本冲突 409；UI 刷新后重新确认；成功审计同事务 | 基线通过 |
| D5 | 跨组织邮箱变更、成员口径 | PG trigger 清部门并递增版本，database_guard 审计；零用量保留，active/未删除口径沿用原集成 | 基线通过 |
| A1 | 独立权限、多部门、无授权拒绝 | service/route 单测；PG 两组织多部门授权；空 grant 数据拒绝，scope 返回空选项 | 基线通过；RV2 待补 |
| A2 | 所有报表面及选项按范围 | PG Summary/Periods/Trend/Champion 同范围；HTTP 越权组织/部门均 403；UI 无外部门数据 | 基线通过 |
| A3 | 部门与全站订阅权限互斥 | 后端 normalize、授权事务及前端互斥控件；授权弹窗保留其他部门，显式切换全站权限 | 基线通过；RV1 待补 |
| A4 | 单项/批量重置、并发、幂等及缓存 | PG 单项和批量隔离；pg_stat_activity 确认撤权等待重置行锁；HTTP 幂等重放不清后来用量，撤权后缓存重放 403；既有缓存专项通过 | 基线通过；RV5 待补 |
| A5 | 负责人不能分配订阅或维护成员 | 14 个真实 HTTP 越权入口全部 403；UI 无分配/延期/撤销/恢复/删除/通用批量按钮和成员管理菜单 | 基线通过 |
| R1 | 部门/平台筛选与统一总量 | UI 三平台合计 2 成员、1 活跃、3 请求、72 Token、$6；单选 Grok 2 成员、1 活跃、36 Token、$3 | 基线通过 |
| R2 | Composite/unknown/软删除/去重 | PG Composite 取实际账号，软删除关联仍保留，unknown、跨平台人数去重；零费用日志沿用原集成 | 基线通过 |
| R3 | 当前成员历史、时间桶/峰值 | PG 调岗后历史随当前归属；原北京时间边界、周/月部分周期、峰值并列/趋势补零六组集成通过 | 基线通过 |
| R4 | scope_version、迟到响应、403/409 | UI/Vitest 范围预检、版本透传、409 最多重试一次、403 清数据；选项迟到响应被作废；PG 变更后旧版本拒绝 | 基线通过；RV3/RV6 待补 |
| E1 | 八 Sheet、同范围、元信息、取消/行限 | 实际下载 XLSX 八表对账，无速宝成员；Vitest 取消、100,000 行含新增汇总、分页冲突中止、公式形态部门名保持字符串 | 基线通过；RV3 待补 |
| U1 | 用户管理部门列、筛选、分配 | 浏览器速宝筛选仅返回速宝成员，分配选项仅其组织；PG 用户 DTO/列表版本与组织部门筛选 | 基线通过；RV4 待补 |
| U2 | 菜单、登录默认页、中英文 | 负责人实际登录直达组织报表，只有授权管理菜单；i18n、路由/landing 专项及全量前端通过 | 基线通过 |
| T1 | migration、Ent/Wire、依赖一致 | 隔离库实际应用 239/240；重跑生成前后 SHA256 无漂移，go mod tidy -diff 通过，go.mod/go.sum 无增量 | 基线通过 |
| T2 | Go 单测与真实 PG | default 与 unit 完整通过；真实 PG 9 组及 12 组性能全部通过，无跳过 | 基线通过 |
| T3 | 前端 lint/typecheck/Vitest/i18n/build | typecheck、lint、328 files / 2,448 tests 全通过；build 含 i18n 检查通过 | 基线通过 |
| T4 | 管理员 → 负责人 → 报表/重置/导出 | 真实浏览器完成；补验部门停用/恢复、外部撤权后清空报表和订阅、恢复授权重新加载 | 基线通过 |
| T5 | 30/90/366 天 SQL 与分页性能 | 600 用户/219,600 日志，12 组 Summary p95 全部 < 3 秒；9 组周期导出首末页计时，见 performance.md | 基线通过；RV4/RV7 待补 |
| T6 | 原订阅、API Key、计费与角色回归 | 完整 default/unit、前端全量及既有订阅/计费/路由测试全部通过 | 基线通过；RV1/RV2/RV5/RV7 待补 |
| W1 | Wiki、组件 README、图谱、初始化/回退 | 本轮设计/实施计划及 wiki 已修订；图谱按本轮文档刷新；代码实现后的组件 README 与最终证据待收口 | RV8 待补 |

## 2026-09-20 复核修正验收 RV1–RV8

执行依据：[实施方案](../../features/organization-department-usage-implementation-plan-cn.md)。本表采用实际查询范围的版本合同；审核报告未要求全站查询忽略新成员。默认口径下，全站新增 active 成员可以改变版本；只有采用 RV3 创建时间上界备选时，才排除上界之后的新注册。“grant 非空即禁止全站订阅”和“管理员无条件跳过所有锁”也不作为本方案的验收要求。

| 编号 | 对应审核项 | 需补证的正反例 | 当前状态 |
| --- | --- | --- | --- |
| RV1 | P1-4 | 编辑打开/切换时获取详情与 admin_access_version，失败禁止保存；仅改资料不发送角色/权限/版本，权限变化携带同一详情版本；旧窗口不能恢复全站权限，缺/旧版本不写入，合法组合正常 | 通过：PG `TestUserAdminAccessIntegration_StaleEditorAndLifecycle`、UserEditModal/Vitest 与真实 HTTP 六项合同；浏览器旧窗口只改备注不恢复权限，权限冲突刷新后需再确认 |
| RV2 | P1-1 | 降级/软删除同事务清其持有 grants、再提权无复活；与在途授权/重置并发；临时停用/恢复按既定规则 | 通过：PG 降级/软删除清 grant、再提权不复活及角色审计；两种生命周期事务提交后排队的授权写均被拒绝；重置持锁期间降级/停用/撤权等待，随后越权操作被拒 |
| RV3 | P1-2 | 单部门忽略无关变化；本范围邮箱/部门名/成员变化被检测；默认全站新增 active 成员允许失效；catalog_version 不混用 admin_access_version/scope_version，Summary/Trend/Excel/重置快照衔接 | 通过：PG 无关注册/username/排序稳定、相关邮箱/部门名及全站注册失效；View/订阅测试与 HTTP 证明目录/数据版本分开，浏览器八 Sheet 对账 |
| RV4 | P1-3/P2-2 | SQL 过滤分页及批量统计、查询次数有界；管理员普通订阅查询不为授权全量装载成员，指定部门仍过滤；列表/详情/重置同数据前后计时与 EXPLAIN | 通过：PG 部门页 4 次/成员页 3 次/全站订阅 scope 1 次；SQL 计划与 50 样本前后比较已入库，成员/订阅列表 p95 未改善如实保留 |
| RV5 | P2-5 | 验证 command 费用二选一，记录重置 users→订阅与计费订阅→users 的反序及混合扣费失效条件；真实 PG 覆盖重置 vs 余额扣费/转岗/撤权及身份变化，保留幂等、缓存和历史日志保障 | 通过：PG 真实锁等待覆盖余额扣费、转岗、跨组织邮箱、停用、降级、撤权；既有 command 费用互斥单测、HTTP 幂等/历史不变与浏览器重置通过 |
| RV6 | P2-3 | 权限版本、部门版本、重名、范围变化、部门停用、跨组织、确认替换全站权限均按 code 给出正确提示；一般 409 不触发范围重载 | 通过：业务 reason/string code 适配与真实错误信封测试；权限冲突中文浏览器验证；仅 REPORT_SCOPE_CHANGED 触发范围重试 |
| RV7 | 回归/性能 | 修复后的 Go default/unit、PG、前端完整检查、浏览器/HTTP/八 Sheet、构建；原 600 用户/219,600 日志/12 组 p95 3 秒基线复验，扩展压力仅作诊断 | 通过：Go default/unit、11 组 PG、前端 329 files / 2,459 tests、lint/typecheck/build、normal/embed build；HTTP/浏览器/XLSX、12 组原规模 p95 门槛全部通过 |
| RV8 | P0-2/P1-5/P2-1/P3 | 设计/计划/验收/wiki/组件文档同步，图谱更新；复用既有 trigger 审计断言；必要合成产物纳入功能提交，无秘密、链接可达 | 通过：设计/计划/wiki/组件文档同步，合成产物与 SQL/性能证据纳入本次提交；图谱按本次文档投影刷新，239/240 原审计断言保留 |

5,000 用户/200 部门不是已经约定的生产容量或 3 秒 SLA；是否开展该规模的扩展诊断按实施方案记录。当前成员口径已写入设计，本次不增设独立书面审批事项。用户已明确同意开始代码修改。两个可选项仍未启用。

| 备选验证 | 仅选择采用后追加的断言 | 当前状态 |
| --- | --- | --- |
| RV3-O 成员创建上界 | 所有报表面共用 `users.created_at <= canonical as_of`；上界后新注册不中断导出，相关旧成员停用/转岗仍失效，零用量人数与明细一致；不影响订阅/重置 | 备选，未启用，不计入当前必需门禁 |
| RV4-O SQL 摘要 | SQL 内有序聚合的字段编码、空集合、特殊字符和变更敏感性符合投影合同；同事务、EXPLAIN/内存/耗时有证据，不能仅看 Go 返回体变小 | 实现备选，未选择 |

## 本轮实测产物

- [HTTP 新合同](artifacts/rv-http-contracts.json)：版本必填/旧版本拒绝/资料差量/显式权限切换/目录与查询版本区分。
- [HTTP 隔离与幂等复验](artifacts/rv-http-legacy.json)：14 个越权入口 403，撤权后读/重放拒绝，外部门额度与历史不变。
- [权限冲突截图](artifacts/rv-access-conflict.png)：旧窗口中文提示、刷新后撤销未经确认的权限勾选。
- [负责人重置截图](artifacts/rv-subscriptions-reset.png)：只显示研发部 4 个订阅；Grok 单项日额 $1→$0，批量日限重置成功，周/月用量保持。
- [实际 Grok 工作簿](artifacts/rv-department-grok.xlsx)：八 Sheet、2 人/1 活跃/1 请求/36 Token/$3，含零用量成员，无外部门数据。
- [性能](performance.md)、[计时 JSON](artifacts/rv-performance.json)、[SQL 计划](artifacts/rv-query-plans.json)。

### 本轮命令与本机日志

| 验证 | 结果及日志 |
| --- | --- |
| Go default | `go test -p 1 -parallel 4 -count=1 ./...` 通过，`rv-go-default-final-v2.log` |
| Go unit | `go test -tags=unit -p 1 -count=1 ./...` 通过，`rv-go-unit-final.log` |
| 真实 PG | integration 标签的 11 组测试通过，无跳过，`rv-pg-final.log`；最终角色审计断言专项 `rv-access-audit-final-v2.log`，排队授权竞态 `rv-lifecycle-queued-final-v2.log`；最终三组受影响集成复验 `rv-test-instrumentation-final.log` |
| 性能 | 原报表 12 组门槛通过；管理接口 50 样本对比，详见 performance.md |
| lint/依赖/build | Go 2.13 增量 lint 0 issues：`rv-go-lint-final.log`，integration 标签仓储 lint 亦为 0 issues：`rv-integration-lint-final-v2.log`；`go mod tidy -diff` 无输出：`rv-tidy-final.log`；normal/embed build 退出 0：`rv-build-final.log` |
| 前端全量 | 隔离副本 `pnpm test:run` 329 files / 2,459 tests；`rv-isolated-ui-final.log` |
| 前端 lint/typecheck/build | `pnpm lint:check`、`pnpm build`（含 i18n、vue-tsc、Vite）通过，`rv-ui-lint-final.log`、`rv-ui-build-final.log` |
| schema/生成 | 本次无 Ent schema/provider/migration 变化，不重复生成；首版生成无漂移的旧证据保留 |
| 实际交互 | 旧权限窗口、负责人登录、三平台/单平台报表、导出、单项/批量重置；合成测试库与隔离 UI |

首次失败与边界：SQLite 域名计数夹具通过带 PG 行锁的 repo.Delete 造数据而失败，已改为 Ent soft-delete 夹具，删除事务仍由 PG 测试覆盖；完整 default/unit 后续通过。未改动的缓存/流 keepalive 用例曾有时序波动，缓存专项 10 次及后续完整回归通过；原失败日志保留。最后审计测试第一次使用了不存在的精确名称，报告 no tests to run，不算通过；修正为真实测试名后 PASS。排队授权测试首次预期删除后 400，实际行锁检查返回更严格的 403；修正断言后通过，未修改业务行为。额外 integration 标签 lint 发现测试辅助类类型断言未检查，修正后 lint 与三组受影响 PG 用例均通过。

主工作区另一任务新增截图/Excel 时曾因 html2canvas 依赖未就绪构建失败；用户要求保留其改动，因此本次在 a9c53a9f6 加本次源码的隔离副本完成前端测试和构建。截图/Excel、Vite 分包等并行改动不纳入本次提交或验收。仅共享文件中的部门相关片段纳入提交。

## 实测产物（旧基线）

- [HTTP 权限与幂等验收](artifacts/http-acceptance.json)：14 个越权入口、all 默认范围、幂等重放、外部门额度及历史用量不变、撤权后重放拒绝。
- [实际 Grok 工作簿](artifacts/test-department-grok.xlsx)：八个 Sheet；2 成员/1 活跃/1 请求/36 Token/$3，零用量成员保留，无速宝数据。
- [负责人报表截图](artifacts/leader-report.png)、[负责人订阅截图](artifacts/leader-subscriptions.png)：均为合成测试账号。
- [性能记录](performance.md)：固定数据和阈值，明确仓储与导出分页的测量边界。

## 命令与本机日志

本机 `.git/codex-department/` 日志未作为原始全量日志入库；可共享结论在本表和产物中，不包含测试密码或 token。

| 验证 | 命令/结果日志 |
| --- | --- |
| Go default | backend 下 `go test -p 1 -count=1 ./...`；`backend-default-final.log` |
| Go unit | `go test -tags=unit -p 1 -count=1 ./...`；`backend-unit-final-v2.log` |
| 真实 PG | `-tags=integration`，三组 Department 与六组 OrganizationUsageRepository；`backend-integration-final.log` |
| 性能 | `DEPARTMENT_USAGE_RUN_PERFORMANCE=1`；`performance-v3.log`，包含 EXPLAIN |
| Ent/Wire | `go generate ./ent`、`go generate ./cmd/server`，生成文件前后散列一致；`generation-final.log` |
| 依赖 | `go mod tidy -diff`；`tidy-final.log`，工具临时 checksum 已移除 |
| Go lint | golangci-lint 2.13.0 / Go 1.27，`run --new-from-rev=HEAD --max-same-issues=0 --max-issues-per-linter=0 --timeout=10m ./...`；`backend-lint-final-v3.log` 为 0 issues |
| 后端构建 | `go build ./cmd/server` 和 `go build -tags=embed ./cmd/server`，独立输出文件；`backend-build-final.log`，均退出 0 |
| 前端 | `pnpm.cmd typecheck / lint:check / test:run / build`；`frontend-typecheck-final.log`、`frontend-lint-final.log`、`frontend-tests-final-v2.log`、`frontend-build-final.log` |
| HTTP | `http-acceptance.py`（测试环境专用，不入库），结果入 artifacts |

2026-09-19 基线回归中修正了旧 auth/me golden 缺少 `admin_permissions:null`、路由夹具缺失规范化 query，以及 Ollama 旧测试在 Windows 同一时钟 tick 未构造不同状态的问题；没有改变这些业务响应或限流逻辑。Ollama 保留原断言，改用原 reset 加一秒构造新状态，20 次专项复跑通过。最初红灯日志保留；以上为旧基线修正，本轮结果见前述 RV 表。

## 环境与边界

- 专用 PostgreSQL 18.1（department_test / department_ui）、Redis 8.10.2、隔离 API/Vite；只用合成账号、空凭据禁用上游账号。未连接业务库，未调用实际模型。
- 管理报表按当前 active 且未删除成员归属；调岗重分类历史，平台配置也不是发生时快照。生产组织名单需主管理员确认后配置。
- `scope_version` 不是授权凭证或数据库快照；已返回/下载内容无法追回。性能实测不是生产 HTTP SLA 或并发容量保证。
- 本轮只启动自身隔离 PostgreSQL/Redis/API/Vite；收尾关闭临时标签页及自身服务、移除验证 worktree，保留本机日志和隔离数据。清理记录 `.git/codex-department/rv-cleanup-final.json`。

## 2026-09-21 深度审核与精简

本轮业务代码净减少 156 行，移除重复查询校验、内存二次筛选、保留授权重复锁/插入、旧并行趋势状态和无版本导出/用户详情回退；保留并发权限、幂等和原子审计。Go default/unit 全量、12 组真实 PG、前端 331 files / 2,480 tests、最后导出专项 54 项、lint/typecheck/build 和 normal/embed 已通过。新增授权锁范围、请求迟到、错误传播、参数边界及导出快照反例。

源码、锁保留理由、精确验证边界及本机日志见[代码审核报告](../../reviews/organization-department-usage-code-review-cn.md)。未重新执行浏览器验收，旧截图/HTTP 证据不冒充本轮结果；本轮未提交、推送或部署。
