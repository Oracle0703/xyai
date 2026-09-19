# 部门与组织用量报表设计入口

状态：2026-09-19 已授权实施，尚未完成验收。以下是目标合同，不是已落地能力。

完整设计：`docs/features/organization-department-usage-design-cn.md`，含关系图、管理/鉴权/导出流程图、数据模型、接口草案、验收及估算。

| 项目 | 建议边界 |
| --- | --- |
| 组织与部门 | 沿用邮箱识别 `xunyou` / `wsdashi` / `other`；组织下独立维护一级部门，名称不写死 |
| 管理入口 | 拟新增独立 `/admin/departments` 页面，与用户管理并列，仅完整管理员可用；组织选择器切换数据，成员与负责人用弹窗维护。详见完整设计第 4.1.1–4.1.2 节 |
| 人员与订阅 | 一人一个当前部门，可有多个平台订阅；部门不从 allowed groups 或订阅自动推导 |
| 负责人 | 已确认保留部门内订阅额度重置，禁止分配订阅；拟新增 `admin.organization_usage`、`admin.department_subscriptions`，共用独立部门授权 |
| 订阅范围 | 复用现有订阅页面及单项/筛选批量重置；新部门订阅权限与全站 `admin.subscriptions` 互斥；无部门授权时拒绝，不能回退全站 |
| 报表 | Summary、Periods、Trend、选项、峰值及 Excel 共用服务端授权范围；默认汇总所有平台 |
| 历史 | 本次实施按当前成员归属统计，不作为部门历史结算 |
| 平台 | 沿用当前有效平台表达式，Composite 取具体账号平台，缺失归 unknown，不按模型名猜测 |
| 一致性 | 拟新增 scope_version 检测导出途中成员/授权变化；as_of 仍不冻结用户关系或用量写入 |
| 估算 | 基础 14–21 人日，含约 20% 预留建议 17–26 人日；包含订阅查询与重置的部门隔离，历史归属、多级部门另估 |

现有事实仍以 [[backend]]、[[frontend]]、[[data-and-domain]]、[[security-and-reliability]] 与源码为准。实施后需更新这些正文及组件 README，不能把本设计草案当作已落地接口。
