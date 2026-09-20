# 部门与组织用量报表设计入口

状态：2026-09-20 S1–S6 已完成，RV1–RV8 本机隔离验收通过；未推送、部署或合回 main。

实施方案：`docs/features/organization-department-usage-implementation-plan-cn.md`；验收：`docs/delivery/2026-09-19-department-usage/acceptance.md`。验收表保留旧基线并单列本轮修正证据。

完整设计：`docs/features/organization-department-usage-design-cn.md`，含关系图、管理/鉴权/导出流程图、数据模型、接口合同、验收及估算。

| 项目 | 保留的业务边界 |
| --- | --- |
| 组织与部门 | 沿用邮箱识别 `xunyou` / `wsdashi` / `other`；组织下独立维护一级部门，名称不写死 |
| 管理入口 | 已新增独立 `/admin/departments` 页面，与用户管理并列，仅完整管理员可用；组织选择器切换数据，成员与负责人用弹窗维护。详见完整设计第 4.1.1–4.1.2 节 |
| 人员与订阅 | 一人一个当前部门，可有多个平台订阅；部门不从 allowed groups 或订阅自动推导 |
| 负责人 | 已确认保留部门内订阅额度重置，禁止分配订阅；新增 `admin.organization_usage`、`admin.department_subscriptions`，共用独立部门授权 |
| 订阅范围 | 复用现有订阅页面及单项/筛选批量重置；新部门订阅权限与全站 `admin.subscriptions` 互斥；无部门授权时拒绝，不能回退全站 |
| 报表 | Summary、Periods、Trend、选项、峰值及 Excel 共用服务端授权范围；默认汇总所有平台 |
| 历史 | 本次实施按当前成员归属统计，不作为部门历史结算 |
| 平台 | 沿用当前有效平台表达式，Composite 取具体账号平台，缺失归 unknown，不按模型名猜测 |
| 一致性 | 保留查询版本；已按实际筛选范围收窄，目录 `catalog_version`、用户授权 `admin_access_version` 与数据快照 `scope_version` 分开，保留邮箱/部门名等影响结果的字段；as_of 不冻结日志 |
| 估算 | 基础 14–21 人日，含约 20% 预留建议 17–26 人日；包含订阅查询与重置的部门隔离，历史归属、多级部门另估 |

## 已实现的复核修正

| 范围 | 约束 |
| --- | --- |
| 权限入口 | 通用用户修改与部门授权同事务共用角色/权限/grant 版本校验，旧编辑窗口不能恢复全站权限；不按 grant 非空一刀切拒绝全站订阅组合 |
| 生命周期 | 降级/软删除清其持有 grants，重新提权不复活；当前为软删除，不改 created_by 外键；临时停用保留授权但禁止访问 |
| 查询 | SQL 过滤分页、批量统计；管理员普通订阅查询去掉不必要全量装载，保留指定部门筛选及最新鉴权 |
| 并发与错误 | 保留行锁和成员版本，补计费/转岗/撤权并发；错误按业务 reason/string code 区分，不把全部 409 当范围变化 |
| 证据 | RV1–RV8 已补证通过；保留原 600 用户基线，扩展压力场景不自动成为容量承诺 |
| 备选 | 报表成员创建时间上界 RV3-O 与 SQL 内摘要 RV4-O 默认未选用，选择采用后才追加相应验证 |

稳定合同见 [[backend]]、[[frontend]]、[[data-and-domain]]、[[security-and-reliability]]，验证入口见 [[ops]]。对应正文及组件 README 已同步；按当前成员统计，不用于固定历史结算。
