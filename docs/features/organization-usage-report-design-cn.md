# 组织用量报表设计

更新：2026-11-04。状态：**已实现并在 main 运行（HEAD `26f6363af`）。报表、趋势图、展示调整、运营看板补强、日汇总（迁移 246）均已落地；日汇总的两项业务口径（保留期语义、费用末位精度）仍待产品确认（§14）。**

本文是组织用量报表的唯一文档，覆盖报表口径与 SQL、接口、筛选、页面展示、趋势折线图、Excel 导出、日汇总（rollup）表与回填、PostgreSQL 性能基线和代码位置。原先分散的首版设计、趋势图设计、日汇总设计、性能基线、三份审核与分流、四份执行计划/规格和一组交付记录已并入本文；审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看，例如 `git show 26f6363af:docs/features/organization-usage-trend-chart-design-cn.md`）。凡是对后续修改有约束的审核结论都已写进正文或 §11。

边界：部门实体、部门/未分配维度的数据范围、负责人与组织负责人权限、平台筛选与平台/部门拆分的业务规则、`/scope` 接口、`scope_version` 合同和对应错误码，由 [组织、部门与多平台用量报表设计](organization-department-usage-design-cn.md) 负责；组织负责人见 [组织负责人权限设计](organization-leader-design-cn.md)。本文只在需要处链接，写报表本身的计算、存储和展示。

阅读顺序建议：
- 改指标、峰值、日期或 `as_of`：先读 §2、§3，再读 §11；
- 改接口或 SQL：读 §4、§5，Trend 另读 §4.4、§5.4；
- 改日汇总、触发器或保留期清理：读 §6 和 §11.3；
- 查慢查询：读 §7；
- 改页面或趋势图：读 §8、§11.4；改导出读 §9；
- 定位代码：读 §10；跑测试读 §13。

## 1. 范围与边界决策

管理端已有使用记录和 Token 分析页面，但它们面向请求明细、排行或分析，不适合按组织、人员、月/周/日统一汇总并导出。本功能是独立页面 `/admin/organization-usage`，按当前邮箱域名把用户归入组织，统计指定日期范围内每个人的请求数、各类 Token、实际消费和日/周/月峰值，提供趋势折线图并导出 Excel。

| 决策 | 结论 | 依据 |
| --- | --- | --- |
| 独立页面，不扩展 `/admin/usage` | 采用 | 使用记录页以请求明细和通用筛选为主，加入组织汇总、峰值和多 Sheet 导出后职责过重 |
| 独立 Handler/Service/Repository | 采用 | 避免继续扩张 `UsageLogRepository`、`UsageService` 和使用记录 Handler |
| 组织规则写死三组 | 采用 | 首版不建组织表、不加配置；后续增加组织时再评估受控配置或组织表并同时定义历史归属口径 |
| 趋势图独立接口 | 采用 | 见 §4.4 和 §12 |
| 日汇总镜像原始日志 | 采用 | 见 §6 |

| 项 | 值 |
| --- | --- |
| 管理端路由 | `/admin/organization-usage` |
| 后端接口前缀 | `/api/v1/admin/usage/organization-report`（`summary`、`periods`、`trend`；`scope` 属部门文档） |
| 固定时区 | `Asia/Shanghai`，不跟随服务端时区配置 |
| 数据库 | 读 `users`、`usage_logs`、`groups`、`accounts`；迁移 `backend/migrations/246_organization_usage_daily_rollups.sql` 新增日汇总三张表和触发器 |
| 配置 | `organization_usage.rollup_enabled`（默认 `true`，环境变量 `ORGANIZATION_USAGE_ROLLUP_ENABLED`） |
| 首版实现提交 | `7c346b314 feat: 组织用量报表功能落地` |

非目标：
- 不回溯用户历史状态或历史邮箱；用户 active 状态、邮箱、组织和部门归属都以查询时的当前值为准（部门口径见部门文档 §7.3）。
- 不做用户级趋势下钻、模型/账号维度拆分、多组织分色折线、成本趋势、时间对比、预算或预警。
- 不替代使用记录、Token 分析或用户排行页面。
- 不提供财务对账级精度或封账快照（§3.2、§6.7）。

## 2. 业务口径

### 2.1 用户范围

活跃账号：

```sql
users.deleted_at IS NULL AND users.status = 'active'
```

角色同时包含普通用户和管理员；禁用和软删除账号不参与统计。部门/负责人范围在此基础上再按授权成员收窄（`/* department_members */` 占位，§5.2）。

- `active_users`：满足上述条件且符合邮箱搜索、组织（及部门）筛选的账号数，含零用量用户。页面和 Excel 标为“成员人数”。
- `used_users`：其中在所选用量范围（日期、平台）内至少有一条用量记录的账号数，按用户去重。页面和 Excel 标为“活跃人数”。
- 这两个只是展示名，后端字段名和计算不变。历史上 2026-08-09 曾把 `active_users` 标为“注册人数”，部门功能落地后改为“成员人数”，以 locale 为准。

### 2.2 组织归属

按当前 `users.email` 第一个 `@` 之后的完整域名做大小写不敏感精确匹配（`LOWER(SPLIT_PART(email, '@', 2))`，函数 `organizationUsageOrganizationExpression`）：

| 邮箱域名 | API 值 | 页面/Excel 显示 |
| --- | --- | --- |
| 精确等于 `xunyou.com` | `xunyou` | 迅游 |
| 精确等于 `wsdashi.com` | `wsdashi` | 速宝 |
| 其他域名、无域名、子域名 | `other` | 其他（英文界面 Other） |

例：`USER@XUNYOU.COM` 归 `xunyou`；`user@sub.xunyou.com` 归 `other`。异常多 `@` 邮箱（`a@b@c.com` 只取 `b`）只可能来自手工写库，注册与绑定链路已有格式校验，不处理。

显示名由唯一的纯展示函数 `formatOrganizationUsageOrganization(value, otherLabel?)` 生成（定义在 `frontend/src/utils/organizations.ts`，`organizationUsageReport.ts` 再导出）：`xunyou`/`xunyou.com` → 迅游，`wsdashi`/`wsdashi.com` → 速宝，其他回落调用方传入的本地化“其他”（默认“其他”）。品牌名中英文界面都显示中文。

### 2.3 指标

| 字段 | 计算 |
| --- | --- |
| `requests` | 用量记录行数；汇总路径为 `SUM(requests)`，原始路径为 `COUNT(*)`；不额外套 `actual_cost > 0` 过滤 |
| `input_tokens` / `output_tokens` / `cache_creation_tokens` / `cache_read_tokens` | 对应列之和 |
| `total_tokens` | 四类 Token 之和（行级先算再求和） |
| `actual_cost` | `SUM(usage_logs.actual_cost)`；SQL 内全程 numeric，只在最终输出列转 `double precision`（§6.7） |

`total_tokens` 包含缓存创建和缓存读取，因此不等于页面上可见的输入、输出两条曲线之和。

### 2.4 峰值与 Champion

个人峰值和团队 Champion 都以周期内 `total_tokens` 最大为首要条件，稳定并列规则：

```text
total_tokens DESC, actual_cost DESC, requests DESC, user_id ASC, bucket_start ASC
```

- 个人峰值：同一用户、同一粒度内取第一名；零用量用户的个人峰值为 `null`。
- 团队 Champion：在符合当前组织、部门、平台和邮箱筛选的所有用户中取第一名。全局 Champion 必然是某个用户的个人峰值，所以当前实现直接从三组已物化的个人峰值里再排序（`organizationUsageChampionMetadataCTE`），不再对整个范围二次聚合（§5.3）。

### 2.5 周期与 `partial`

日、周、月周期先按北京时间聚合；周为周一到周日（`date_trunc('week', ...)`）。周期跨越所选范围边界时，响应的 `period_start`/`period_end` 裁剪到所选范围，并返回 `partial=true`。

指标只含所选范围内的数据，不是完整自然周/自然月的全量。这是原始需求明确的口径，页面和 Excel 都展示 Partial；2026-07-13 审核建议“与产品确认”未采纳（§12）。

Periods 明细和 Excel 只返回有用量的周期，不生成全零行；只有 Trend 补零（§4.4）。

## 3. 时间与快照语义

### 3.1 日期范围

- `start_date`、`end_date` 固定 `YYYY-MM-DD`，两端都包含；`end_date` 不能早于 `start_date`，闭区间最多 366 个自然日（`organizationUsageCalendarDays`）。
- Service 按 `Asia/Shanghai` 解析，再转为 UTC 半开区间：`created_at >= start AND created_at < end + 1 day`（`repositoryRange`）。
- 时区对象是 `service.OrganizationUsageLocation()`（`FixedZone(+8)`）。中国无夏令时，与 PostgreSQL IANA 时区在业务日期上等价；日汇总仓储必须复用同一个函数，不另建时区。

### 3.2 `as_of` 快照

`as_of` 是可选的严格 RFC3339/RFC3339Nano 时间戳（正则 `organizationUsageRFC3339Pattern` 先校验格式）。

1. 调用方生成候选 `as_of` 发给服务端；
2. Service 转 UTC，若晚于服务端当前时间则裁剪到当前时间（`clampAsOfToServerNow`）；
3. 响应 `range.as_of` 回写裁剪并规范化后的 canonical UTC 值（`time.RFC3339Nano`）；不传则不回写；
4. 用量查询上界取“`end_date` 次日零点”与 `as_of` 的较早者；`as_of` 早于开始时间时钳制为空区间；
5. 后续分页、Periods、Trend 都复用这个 canonical 值。

边界：
- `as_of` 只冻结用量记录上界，不冻结 `users`、部门或授权；用户状态、邮箱、组织、部门仍按每次查询的当前值。成员/授权变化由部门文档的 `scope_version` 检测。
- `as_of` 不冻结迟到写入或被清理的日志，不是数据库历史快照。
- `as_of` 不是签名或服务端 snapshot id，没有防篡改能力；管理员可以显式传入更早的合法时间戳来限制上界。首版设计曾写“canonical signed snapshot”，2026-07-13 审核指出名不副实，结论是只改文档不加 HMAC（§12）。代码里 `clampAsOfToServerNow` 的局部变量名 `signed` 和测试名 `...SignedAsOf` 是历史遗留，不代表签名语义。

## 4. 接口合同

三个接口都注册在 `backend/internal/server/routes/admin.go` 的管理员 `usage` 组（`/api/v1/admin` + 管理员认证 + 合规 guard），Trend 与 Summary/Periods 同组、同权限，没有独立权限位；子管理员与负责人通过部门文档定义的组织报表权限白名单访问，三个 GET 必须同步放行。Handler 先要求 `service.DepartmentActorID(ctx) > 0`，否则返回 `ErrDepartmentScopeDenied`；所有请求在仓储里走 `beginScoped` 的只读 repeatable-read 事务（§5.5）。

### 4.1 公共查询参数

| 参数 | 必填 | 默认 | 约束 |
| --- | --- | --- | --- |
| `start_date` | 是 | — | `YYYY-MM-DD` |
| `end_date` | 是 | — | `YYYY-MM-DD`，不早于开始日期，闭区间 ≤366 天 |
| `as_of` | 否 | — | 严格 RFC3339/RFC3339Nano |
| `organization` | 否 | `all` | `all\|xunyou\|wsdashi\|other`（大小写不敏感） |
| `q` | 否 | 空 | 邮箱模糊搜索，字面匹配（§11.1） |
| `page` | 否 | `1` | 正整数；显式 `0`、负数、非整数由 Handler 直接 400；`(page-1)*page_size` 溢出返回 400 |
| `page_size` | 否 | `20` | `1..1000` |
| `department_id` / `platform` / `scope_version` | 否 | — | 合同见部门文档 §8.2；平台值由 `OrganizationUsageDepartmentFilters.Normalize` 白名单校验 |

`organization`、`sort_by`、`sort_order`、`granularity`、`platform` 都是严格 allowlist，排序 SQL 由 `organizationUsageOrderBy` 的固定映射生成，不拼接任意字符串。参数非法返回 400（`OrganizationUsageValidationError` → `response.BadRequest`）；其余错误经 `response.ErrorFrom`。仓储错误统一包装为 `query organization usage <surface>: ...`。

### 4.2 Summary

`GET /api/v1/admin/usage/organization-report/summary`

| 附加参数 | 默认 | 允许值 |
| --- | --- | --- |
| `sort_by` | `total_tokens` | `email`、`requests`、`input_tokens`、`output_tokens`、`cache_creation_tokens`、`cache_read_tokens`、`total_tokens`、`actual_cost`、`peak_day_tokens`、`peak_week_tokens`、`peak_month_tokens` |
| `sort_order` | `desc` | `asc\|desc` |

排序后统一追加 `user_id ASC` 作为稳定次序；`email` 按 `LOWER(email)`，峰值列按 `COALESCE(peak.total_tokens, 0)`。

| 响应字段 | 内容 |
| --- | --- |
| `range` | `start_date`、`end_date`、可选 canonical `as_of` |
| `overview` | 当前筛选下的成员人数、活跃人数和聚合指标 |
| `organizations` | 组织汇总：只返回当前组织筛选（`all` 时为全部）且在授权范围内的组织 |
| `champions` | `day`、`week`、`month` 团队 Champion，可为 `null` |
| `items` | 当前页人员汇总及个人 `peak_day/peak_week/peak_month`，含 `department_id/department_name` |
| `pagination` | `total`、`page`、`page_size`、`pages`（最少 1） |
| `departments`、`platforms`、`scope_version`、`applied_filters` | 部门/平台拆分与范围版本，口径见部门文档 |

组织汇总的筛选行为已变化：首版是“组织汇总忽略组织筛选、始终返回三组”，部门功能落地后改为“所有区块统一按有效范围过滤，不返回范围外组织”（`scopedSummary` 在算完 overview 之后按 `applied_filters.organization` 和授权组织过滤）。`overview` 由 `organizationUsageOverview` 在 Go 中按所选组织对组织汇总行求和，不是独立 SQL。

### 4.3 Periods

`GET /api/v1/admin/usage/organization-report/periods`，附加 `granularity=day|week|month`（默认 `day`）。响应 `range`、`granularity`、有用量的“用户 × 周期”`items`（含组织、部门、`partial`）、`pagination`，按 `period_start DESC, user_id ASC` 排序。只用于 Excel 导出明细，不补零，页面不调用。

### 4.4 Trend

`GET /api/v1/admin/usage/organization-report/trend`，参数为公共参数（无分页、无排序）加 `granularity`（默认 `day`，服务端不做自动推断）。

```json
{
  "range": { "start_date": "2026-07-01", "end_date": "2026-07-03", "as_of": "2026-07-03T12:00:00Z" },
  "data_through": "2026-07-03",
  "granularity": "day",
  "points": [
    { "period_start": "2026-07-01", "period_end": "2026-07-01", "partial": false,
      "requests": 10, "input_tokens": 1000, "output_tokens": 200, "cache_creation_tokens": 0,
      "cache_read_tokens": 50, "total_tokens": 1250, "actual_cost": 0.12 },
    { "period_start": "2026-07-02", "period_end": "2026-07-02", "partial": false,
      "requests": 0, "input_tokens": 0, "output_tokens": 0, "cache_creation_tokens": 0,
      "cache_read_tokens": 0, "total_tokens": 0, "actual_cost": 0 }
  ]
}
```

另有 `scope_version`、`applied_filters`（部门文档）。合同：

- **只按周期汇总，无用户维度**：一次返回全部点，不分页；响应不含邮箱或 `user_id`。
- **`data_through`**：`effective_as_of` = canonical `as_of`（省略时为服务端当前时间）；`data_through = min(end_date, effective_as_of 的北京日期)`。只生成 `start_date..data_through` 的桶，未来日期不补 0（当前月/周筛选通常含未来日期，补 0 会画出虚假下跌）。当天桶包含截至精确 `as_of` 的数据。
- **`data_through < start_date`**（整个范围都在快照之后）：返回 `points: []` 且省略 `data_through`。无部门 actor 时 Service 直接短路不访问仓储；有 actor 时（所有真实请求）仍走 `scopedTrend` 完成鉴权与 `scope_version` 计算后返回空点。
- **补零**：`data_through` 之前无数据的周期返回全 0 点，保证 X 轴连续；整个可达范围都无用量时也返回连续 0 点。
- **`partial`**：用未裁剪的桶边界计算，`bucket_start < start_date OR bucket_end > end_date OR bucket_end > data_through`；响应 `period_start = GREATEST(bucket_start, start_date)`，`period_end = LEAST(bucket_end, end_date, data_through)`。所以当前未结束的周/月也标 partial；day 桶只表达日历裁剪，不表达日内进度，精确时刻看 `range.as_of`。
- **点数上限**：day 最多 366；366 个闭区间自然日最多跨 54 个周桶（例 `2024-01-07..2025-01-06`）；month 最多 13。
- **点数合同**（`data_through >= start_date` 时）：day = `start_date..data_through` 含首尾天数；week/month = 该区间跨越的不同周/月桶数（含首尾 partial 桶）。

week 合同示例：范围 `2026-01-30..2026-02-03`（周五到周二），`data_through=2026-02-03` → 2 个点：`2026-01-30..2026-02-01 partial` 和 `2026-02-02..2026-02-03 partial`；month 粒度同范围 → `2026-01-30..2026-01-31 partial` 和 `2026-02-01..2026-02-03 partial`。

一致性（在同一 canonical `as_of`、用户 fixture 不变时成立）：

```text
trend.range.as_of          == summary.range.as_of
sum(points.*.total_tokens) == overview.total_tokens
sum(points.*.requests)     == overview.requests
```

## 5. 后端查询实现

### 5.1 分层

| 层 | 职责 | 文件 |
| --- | --- | --- |
| Handler | 读查询参数、严格解析分页、要求部门 actor、映射 400/500、传播 `c.Request.Context()` | `backend/internal/handler/admin/organization_usage_handler.go` |
| Service | 日期/枚举/分页/排序/粒度校验；北京时间转换；`as_of` 规范化与裁剪；Trend 的 `data_through` 派生 | `backend/internal/service/organization_usage_service.go`、`organization_usage_department.go` |
| Repository | 范围事务、活跃用户 CTE、组织分类、读取计划、聚合、峰值、Champion、补零、分页 | `backend/internal/repository/organization_usage_repo.go`、`organization_usage_department_repo.go`、`organization_usage_rollup_plan.go` |
| Routes/Wire | 注册路由；`ProvideOrganizationUsageRepository` 注入 `rollup_enabled` | `routes/admin.go`、`repository/wire.go`、`cmd/server/wire_gen.go` |

### 5.2 CTE 积木

所有查询由同一组片段拼出，改公式只改一处：

| 片段 | 作用 |
| --- | --- |
| `organizationUsageActiveUsersCTE` → `active_users` | active、未删除用户 + 组织表达式 + 邮箱 `ILIKE $3 ESCAPE E'\\'`；含 `/* department_members */` 占位，范围事务内替换为 `AND u.id=ANY($n::bigint[])` |
| `selected_users AS MATERIALIZED` | `$4 = 'all' OR organization = $4` |
| `organizationUsageDaysCTE` → `usage_days` | “北京日 × 用户 × 分组 × 账号”粒度：已发布且非脏的日期读 `usage_user_daily_rollups`，其余区间按同形状从 `usage_logs` 聚合，`UNION ALL` 合并（§6.5）。`{{rollup_start}}` 等占位由读取计划绑定为参数 |
| `organizationUsageRowsCTE` → `usage_rows` | `usage_days` 左连 `groups`/`accounts`，计算行级 `total_tokens`；含 `/* department_platform */` 占位，替换为基于 `usageLogEffectivePlatformExpr` 的平台条件 |
| `organizationUsageTotalsCTE` → `usage_totals` | 按用户汇总 |
| `organizationUsagePeriodBucketSQL(granularity)` | 桶边界唯一来源：day `local_date`；week `date_trunc('week', local_date::timestamp)::date` 到 `+6 days`；month `date_trunc('month', ...)` 到 `+1 month - 1 day` |
| `organizationUsagePeriodAggregationSQL` / `organizationUsageAllPeriodAggregationsSQL` | 按 `user_id, bucket` 聚合；后者把 day/week/month `UNION ALL` 成一张 `period_aggregates` |

原始日志的时间条件（`created_at` 区间）在聚合前执行，并且原始区间用 `unnest(raw_starts, raw_ends)` 后 JOIN，每个区间各自命中 `created_at` 索引。

### 5.3 Summary 的查询组成

在一个范围事务内依次执行：
1. `organizationUsageOrganizationsQuery`：`VALUES ('xunyou',1),('wsdashi',2),('other',3)` 左连 `active_users`、`usage_totals`，三组都返回（之后在 Go 里过滤，§4.2）；
2. `organizationUsageSummaryItemsCountQuery`：只数 `active_users`，不扫用量；
3. `organizationUsageSummaryItemsSQL(orderBy, includeChampions)`：`period_aggregates AS MATERIALIZED` → `ranked_periods`（`ROW_NUMBER() OVER (PARTITION BY user_id, granularity ...)`）→ `day_peak`/`week_peak`/`month_peak` 各自 `AS MATERIALIZED` → `selected_users` 左连 `usage_totals` 与三个 peak，`COUNT(*) OVER()` 带出总数，`LIMIT $7 OFFSET $8`。范围事务内 `includeChampions=true`，附加 `report_champions` CTE 把 Champion JSON 放进每一行，省掉独立 Champion 查询；`total == 0` 时直接返回空 Champion；
4. 平台/部门拆分 `breakdowns`（部门文档），与 Summary 共用同一读取计划。

`organizationUsageChampionsQuery` 是非范围路径（无部门 actor，只在测试/EXPLAIN 中出现）使用的独立 Champion 查询；Champion 窗口里的列必须写 `pa.user_id` 等限定名（§11.1）。

### 5.4 Trend SQL

`organizationUsageTrendQuery(granularity)` 参数位与 Periods 对齐，禁止重编号：

| 占位 | Periods | Trend |
| --- | --- | --- |
| `$1`/`$2` | UTC 起止（`$2` 可被 `as_of` 收紧） | 同 |
| `$3` | 邮箱 ILIKE pattern（`active_users` 固定用 `$3`） | 同 |
| `$4` | 组织筛选 | 同 |
| `$5`/`$6` | 所选 `start_date`/`end_date` 日历串 | 同 |
| `$7` | `page_size` | `data_through` 日历串（只接收 Service 派生值） |
| `$8` | offset | 不使用 |

形状：`active_users` → `selected_users` → `usage_rows` → `filtered_usage`（JOIN `selected_users`）→ `period_aggregates AS MATERIALIZED`（`organizationUsageTrendAggregationSQL`，**只** `GROUP BY bucket_start, bucket_end`，没有 `user_id`）→ `buckets`（`organizationUsageTrendBucketSeriesSQL`）→ `buckets LEFT JOIN period_aggregates ON bucket_start` 补 0 → `ORDER BY period_start ASC`。

`buckets` 生成规则：day 为 `generate_series($5::date, $7::date, '1 day')`；week 从 `date_trunc('week', $5)` 到 `date_trunc('week', $7)` 步长 7 天（起点是包含 `start_date` 的周一，不是 `start_date` 本身）；month 先 `date_trunc('month', ...)` 再按 1 个月步进（从月中日期直接 `+1 month` 会错位）。

### 5.5 报表事务

`beginScoped`：`BeginTx(ReadOnly, RepeatableRead)` → 执行 `organizationUsageReportSessionSQL`（`set_config('jit','off',true)`、`set_config('work_mem','64MB',true)`，仅事务内生效）→ `resolveDepartmentQueryScope` 解析授权成员与 `scope_version` → 返回带 `query=tx` 的仓储副本。同一请求内范围成员、读取计划、汇总表、原始日志处在同一快照。生产 EXPLAIN 显示 JIT 编译每条查询耗 0.4–1s，默认 `work_mem` 会让共享 CTE 落盘；成员/峰值查询因此从 3.6s 降到 2.75s。

所有新 SQL 必须经 `queryRows`/`queryRow`（内部做占位替换和 `plan.bind`），或像 `breakdowns` 那样显式调用 `plan.bind`；直接 `QueryContext` 会把 `{{...}}` 原样发给数据库。

## 6. 日汇总（rollup）

### 6.1 目标与非目标

报表原先从原始日志实时聚合，耗时与所选范围内日志行数成正比，90/366 天等于扫全表（生产数据见 §7.2）；JIT、`work_mem`、前端并行只能削常数。日汇总把已结束的北京日预聚合，使查询量只随天数增长。

目标：
- 默认本月加载 Summary < 0.5s、Trend < 0.3s，366 天 < 1.5s（本机实测见 §7.3，生产待复测）；
- 结果与纯原始路径逐字段一致：口径、筛选、`as_of`、当前成员/组织/部门/平台归属语义不变；唯一差异是 `actual_cost` 末位精度（§6.7）；
- 不阻塞网关写 `usage_logs`，热路径不加行锁；
- 汇总缺失、落后或关闭时自动退回原始日志，改配置并重启即可回退。

非目标：
- 不把组织、部门、平台写进汇总；归属仍在查询时按 `users`/`groups`/`accounts` 当前值计算。
- 不保留超出原始日志保留期的数据：汇总严格镜像 `usage_logs`，日志被清理后汇总同步消失（保留期部署默认 90 天，`dashboard_aggregation.retention.usage_logs_days`）。
- 不改上游 `/admin/groups` 的 `usage_group_daily_rollups` 及其触发器。

### 6.2 数据模型（迁移 246）

| 表 | 要点 |
| --- | --- |
| `usage_user_daily_rollups` | 主键 `(bucket_date, user_id, group_id, account_id)`；`bucket_date` 为北京自然日；`group_id` 用 0 表示 `usage_logs.group_id IS NULL`（可空列不能进主键）；`requests`、四类 Token 为 BIGINT；`actual_cost DECIMAL(24,10)`（源列 `decimal(20,10)`，留累加余量）；`computed_at` |
| `usage_user_rollup_state` | 单行（`id = 1` CHECK）；`closed_before`：早于该日期的日桶都已至少完整发布一次，NULL 表示未初始化；`reconciled_at`：最近一次集群范围对账时间 |
| `usage_user_rollup_dirty_days` | `id BIGSERIAL`、`bucket_date`、`marked_at`；只追加，同一天可有多行，日期不设唯一约束；索引 `idx_usage_user_rollup_dirty_days_bucket_date` |

维度选择：

| 报表需要 | 汇总如何满足 |
| --- | --- |
| 成员、组织、部门筛选 | 只存 `user_id`；组织、部门、active 在查询时按当前 `users` 计算 |
| 日/周/月桶 | 存北京日；周、月由 `date_trunc(local_date::timestamp)` 推导，与按时间戳分桶结果相同 |
| 平台筛选与拆分 | 存 `group_id`、`account_id`，查询时左连 `groups`/`accounts` 用 `usageLogEffectivePlatformExpr` 算当前有效平台 |
| `as_of` 精确上界 | 只有完整的日读汇总，`as_of` 当天读原始日志截到精确时刻 |

主键以 `bucket_date` 开头，按日期范围读取直接走主键范围扫描；负责人范围很小时可再评估 `(user_id, bucket_date)` 索引，当前未加。生产实测（2026-10-23，13 个完整北京日）：61.7 万条原始日志对应 4,237 行汇总，每天 70–459 行、平均 326 行，压缩约 145 倍；外推 30 天约 1 万行、366 天约 12 万行，因此不需要改成按平台汇总。

### 6.3 触发器（登记脏日期）

全部触发器通过 `mark_user_usage_rollup_dirty_day(p_day)` 登记：每个事务对每天插入一行自己的标记，并用事务级设置 `sub2api.user_rollup_dirty_YYYYMMDD` 保证同一事务同一天只插一次（随子事务回滚撤销）。

| 触发器 | 级别 | 行为 |
| --- | --- | --- |
| `usage_logs_user_rollup_dirty_insert` | 语句级 + transition table | 每条批量 INSERT 执行一次；登记 `(created_at 北京日) < (clock_timestamp() + 10 分钟 的北京日)` 的日期，即所有过去日期，以及 23:50 之后写入的当天日期（宽限窗口）。当天普通网关写入不写标记 |
| `usage_logs_user_rollup_dirty_delete` | 行级 AFTER DELETE | 登记 `OLD` 日期；行级是为了覆盖外键级联和直接写分区；同一事务同一天只写一次，保留期每批 5000 行不会写 5000 次 |
| `usage_logs_user_rollup_dirty_update` | 行级 `AFTER UPDATE OF` 九个相关列 + `WHEN` 过滤 | 登记新旧日期，覆盖直接写分区 |
| `usage_logs_user_rollup_dirty_update_statement` | 语句级 `REFERENCING OLD TABLE ... NEW TABLE` | 按 `id` 配对比较相关列，覆盖分区表上跨分区移动的 UPDATE（被转换为 DELETE + INSERT，行级 UPDATE 与语句级 INSERT 都不触发）；当前代码没有更新这些列的路径，属兜底 |
| `usage_logs_user_rollup_reset_truncate` | 语句级 TRUNCATE | 清空汇总表和脏标记，`closed_before`、`reconciled_at` 置 NULL，作业重新初始化 |

按月 DROP 分区不触发任何触发器：`dropUsageLogsPartitionWithRollupInvalidation`（`dashboard_aggregation_repo.go`）在同一事务里调用 `markOrganizationUsageRollupRangeDirty`，把该 UTC 月覆盖的全部北京日期登记为脏，再 DROP。

### 6.4 后台作业

`OrganizationUsageRollupService`（`backend/internal/service/organization_usage_rollup.go`），仓储 `organizationUsageRollupRepository`（`organization_usage_rollup_repo.go`）。生命周期参照本地后台任务（`Start`/`Stop`，幂等），不挂在上游 `DashboardAggregationService` 上：不受 `dashboard_aggregation.enabled` 影响，也避免上游合并冲突。`ProvideOrganizationUsageRollupService` 在 `rollup_enabled=false` 时不启动作业。

调度：启动时先跑一次，此后每 60 秒一次；`tryAcquireSingletonLeaderLock`（键 `organization-usage:rollup:leader`，TTL 2 分钟）保证多实例只有一个在跑；每次运行预算 45 秒，超时剩余工作留到下一次（没有单独的天数上限）。每一步是一个短 READ COMMITTED 事务，先 `SELECT ... FROM usage_user_rollup_state FOR UPDATE` 作为数据库层二次互斥。

`RunOnce` 依次：
1. **初始化**（`InitializeOrganizationUsageRollups`）：`closed_before IS NULL` 时，清空汇总表，把水位设为 `MIN(usage_logs.created_at)` 的北京日期，表为空时设为今天。**不清空脏标记**（§11.3）。
2. **逐日关闭**（`CloseNextOrganizationUsageRollupDay`，循环到无可关闭日期）：当 `closed_before < (clock_timestamp() - 10 分钟) 的北京日期`（`organizationUsageRollupClosableDateSQL`）时，对 D = `closed_before` 执行 `rebuildOrganizationUsageRollupDay`：先删该天全部已提交脏标记（认领），删旧汇总，`INSERT ... SELECT ... GROUP BY user_id, COALESCE(group_id,0), account_id`（走 `created_at` 索引，一天约 3.5 万行），再 `closed_before + 1`。即午夜后 10 分钟才关闭前一天。
3. **重建脏日期**（`RebuildNextDirtyOrganizationUsageRollupDay`）：每次取 `bucket_date < closed_before` 最早的一个脏日期，同样经 `rebuildOrganizationUsageRollupDay` 认领并重算。`>= closed_before` 的标记先保留，关闭该天时一并认领。
4. **ANALYZE**：本次关闭 + 重建 ≥ 7 天时执行 `ANALYZE usage_user_daily_rollups`。依据：本机实测刚批量写入、尚无统计信息的汇总表会让个别报表查询出现 12–19 秒的异常计划，补 ANALYZE 后消失。
5. **对账**（`ReconcileOrganizationUsageRollups(3 天, 1 小时)`）：用 `UPDATE usage_user_rollup_state SET reconciled_at = NOW() WHERE reconciled_at <= NOW() - 1h ... RETURNING` 抢占节奏，集群范围每小时最多一次；在单条语句快照内比较 `[closed_before - 3, closed_before)` 每天的汇总合计与原始合计（请求数、四类 Token、费用，`IS DISTINCT FROM`），不一致且尚无脏标记的日期补登记为脏，并打 `organization_usage_rollup_reconcile_mismatch` 警告。这是 §6.6 残余风险的自愈兜底。

首次回填：生产约 90 天，每天一个事务约 0.1–0.3 秒，总计约 1 分钟，分摊在启动后的前几次运行里；回填期间读路径对未发布日期自动走原始日志，不会比改造前更慢。

与上游分组汇总的区别（也是不照搬的原因）：上游同步事务在整个重建期间持有状态行 `FOR UPDATE`，网关 INSERT 触发器要拿同一行的 `FOR KEY SHARE`，重建期间日志写入排队；上游保留期清理会把水位拉回最早日期，每 6 小时触发一次全量重建。本方案触发器不碰状态行、不锁标记行，保留期清理只重建被删的那几天。

### 6.5 读取计划

`withRollupPlan` 在报表同一 repeatable-read 事务内调用 `loadOrganizationUsageRollupState` 读水位和 `[start_date, closed_before)` 内的脏日期，再由 `buildOrganizationUsageRollupPlan` 生成 `organizationUsageRollupPlan`：

- `endTime` 沿用 `min(as_of, end_date + 1 天 00:00)`；
- `rollupEnd = min(closed_before, endTime 的北京日期向下取整)`，不含上界；汇总未初始化、开关关闭、或结果不晚于 `start_date` 时 `rollupEnd = start_date`，即全部走原始日志；
- 读汇总的日期：`[start_date, rollupEnd)` 去掉脏日期（SQL 里 `r.bucket_date <> ALL(dirty_dates)`）；
- 读原始日志的区间：范围内每个脏日期整天，加尾段 `[max(startTime, rollupEnd 00:00), endTime)`；相邻区间合并后作为 `raw_starts`/`raw_ends` 两个数组参数；
- `as_of` 落在过去（导出续页）时 `rollupEnd` 自动截到 `as_of` 所在日，当天截至 `as_of` 的部分读原始日志。

Summary、拆分、Periods、Trend、导出共用同一份计划；一个请求只建一次计划（`withRollupPlan` 对已带计划的副本原样返回）。下游聚合统一是 `SUM(requests)`，分桶表达式基于 `local_date`。读路径不依赖作业是否在运行：作业落后时原始尾段变长，查询变慢但结果正确。

### 6.6 正确性

不变量：在读事务快照中，任一满足 `D < closed_before` 且 D 不在脏日期表中的日期，汇总表中 D 的各行之和等于 `usage_logs` 在 `[D 00:00, D+1 00:00)` 内同维度聚合的结果。读路径只在满足不变量的日期上读汇总。

快照一致性：报表在一个 RR 只读事务内读 state、脏日期、汇总和原始日志；作业在同一事务里更新汇总、state、脏标记；触发器的标记与引起它的日志变更同事务提交。读者要么同时看到“变更 + 标记”，要么都看不到。

迟到写入真实存在（批量图片结算、OpenAI Live 以原始时间写 `created_at`；网关日志异步批量插入）：

| 情形 | 结果 |
| --- | --- |
| 迟到行在作业聚合语句开始前提交 | 被聚合（READ COMMITTED 每条语句取新快照，认领在聚合之前） |
| 迟到行在聚合语句开始后提交 | 其标记一起提交并留存；读者对该日改读原始日志；下次运行重建 |
| 作业认领时写入方标记未提交 | 认领只删已提交标记、不等待；写入方提交后标记仍在，该天读原始并再次重建（`WriterMarkSurvivesConcurrentRebuild`） |
| 两事务按相反顺序修改同两天 | 标记互不冲突、不加锁、不死锁（`CrossDayWritersDoNotDeadlock`） |
| 初始化时并发提交早于 `MIN(created_at)` 的日志 | 标记保留，该天在水位之前保持脏并被重建（`InitializationKeepsEarlierConcurrentMark`） |
| 某天最后 10 分钟写入的当天数据 | 触发器提前登记（宽限窗口），关闭该天时一并认领 |
| 跨午夜的写入事务 | 漏数须同时满足：触发器在 23:50 前执行（未登记）且事务在 00:10 后才提交，即事务打开超过 20 分钟。网关写入是短事务；即便出现，每小时对账也会检出并重建 |

删除与修改：保留期清理、管理员清理任务和单条删除都经 DELETE 触发器登记日期；被删光的日期重建后为零行。

### 6.7 数值精度

改造前先把每行 `actual_cost` 转 double 再求和；改造后无论来自汇总还是原始日志都以 numeric 精确求和，只在最终输出列转一次 double，汇总开、关两种模式结果逐位相同（等价性测试用 `require.Equal` 比较整个结果）。与改造前的旧结果比：生产 80 万行样本底层差异最大约 3.3e-10，但 1,343 个分组中有 2 个按 6 位小数舍入时跨边界（例 454.173373 → 454.173374）。新值是精确值，不能承诺固定小数位展示与旧版本永远相同；Token 和请求数完全一致。API、前端和 Excel 最终仍是浮点数，若要财务对账级精度需整体改为 decimal/string 合同。

### 6.8 开关、上线与回退

- 配置 `organization_usage.rollup_enabled`（`OrganizationUsageConfig.RollupEnabled`，`viper.SetDefault` 为 `true`；环境变量 `ORGANIZATION_USAGE_ROLLUP_ENABLED`）。只在进程启动时读取，修改需重启。`false` 时仓储 `rollupDisabled=true`，读取计划全部走原始日志，结果与改造前路径一致；作业不启动，但触发器仍会为过去日期写脏标记，重新开启后由作业认领重建。这个开关同时是测试里的参照实现。
- 上线：执行迁移 246（`CREATE TRIGGER` 会对 `usage_logs` 短暂加 SHARE ROW EXCLUSIVE 锁，建议低峰）→ 部署代码，作业从启动起逐日回填 → 对已发布日期逐日比较汇总与原始合计、浏览器 Network 复测 Summary/Trend、对新 SQL 做 `EXPLAIN (ANALYZE, BUFFERS)`。
- 回退：设 `rollup_enabled: false` 并重启，无需改库。彻底移除须新增迁移删除触发器和表，不能修改 246（checksum 校验）。

### 6.9 备选方案

| 方案 | 结论 |
| --- | --- |
| `created_at` 覆盖索引（INCLUDE 用户、分组、账号、Token、费用） | 预计提速 2–3 倍，但仍随范围线性增长；要在已有 26 个索引的热表上再加约 300MB 写放大。只适合作过渡，不采用 |
| 照搬上游分组汇总的加锁水位 | 重建期间阻塞网关写入；保留期清理每 6 小时全量重建。不采用 |
| 按“日 × 用户 × 平台”汇总 | 行数更少，但平台被固化为汇总时刻的值，还要给 `groups`/`accounts` 加触发器。实测行数（§6.2）不需要，不采用 |
| 报表结果缓存 | 首次访问仍慢，不能根治。不采用 |

## 7. PostgreSQL 性能基线

### 7.1 2026-07-13 合成基线（peak CTE 病态计划）

环境：PostgreSQL 18.1 Windows 可携带运行时（Maven Central `embedded-postgres-binaries-windows-amd64-18.1.0.jar`，SHA-256 `0a7e951660cbbee3f382f8b44b2c3ae26c4a17817c3a972dee3378c80fc9b23b`）；`work_mem` 4MB、`shared_buffers` 128MB、`effective_cache_size` 4GB、`max_parallel_workers_per_gather` 2；全量迁移；600 名 active 用户均分三组织，`usage_logs` 219,600 行（每人连续 366 天每天 1 条）；页大小 500；每条查询预热一次后记录第二次 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`。这是可重复的合成集，只用于识别查询形状问题，不代表生产分布、并发或硬件。

| 查询（毫秒；temp 为读/写块数） | 30 天 | 90 天 | 366 天 |
| --- | ---: | ---: | ---: |
| Summary 组织汇总 | 12.2 | 61.6 | 91.6 |
| Summary 人员 count | 0.19 | 0.19 | 0.18 |
| Summary 人员 items | 117.0（149/309） | **11,032.0（1,386,172/4,045）** | 1,454.2（18,864/12,218） |
| Summary Champion | 129.9 | 370.1 | 1,357.8 |
| Periods day count / data | 14.7 / 66.7 | 42.3 / 141.4 | 174.1 / 702.4 |
| Periods week count / data | 16.6 / 30.6 | 48.9 / 89.0 | 179.6 / 186.2 |
| Periods month count / data | 15.9 / 25.3 | 45.5 / 75.3 | 157.6 / 170.9 |

90 天 items 重复两次（11,367.7ms、11,032.0ms）可复现；366 天反而更快，说明是基数估算导致的连接计划切换，而非线性增长。慢计划：90 天窗口 54,000 条日志、64,200 行 period 聚合；`usage_logs` Index Scan 只占 36ms，`ranked_periods` WindowAgg 278ms，顶层 Left Nested Loop 11,024ms——day/week/month 三个 peak 各对 `ranked_periods` CTE Scan 循环 600 次（4,027 / 3,457 / 3,427ms）。新增索引或单纯调高 `work_mem` 都消除不了“用户数 × ranked_periods”的重复扫描。

诊断候选只把三个 peak 改为 `AS MATERIALIZED`：90 天 items 从 11,032ms 降到 417.8ms（temp 4,826/3,978），约 26 倍。结论与落地：
- 第一阶段（修 peak 连接形状）**已落地**：当前 `period_aggregates`、`selected_users`、`day_peak`/`week_peak`/`month_peak` 都是 `AS MATERIALIZED`，并列规则不变（随部门功能提交 `a9c53a9f6` 进入 main）；
- 第二阶段（减少导出分页重复查询）**部分落地**：范围事务内 Champion 改为复用三组 peak 并内嵌到 items 行（§5.3），不再单独聚合；Summary 每页仍会重算组织汇总和全部 period 聚合，Periods 仍是 count + data 两条，导出时 day/week/month 保持串行；
- 2026-07-13 后合成基线没有在新 SQL 上重跑，当前性能结论以 §7.2、§7.3 为准。

### 7.2 生产观测（2026-10-23，PostgreSQL 15.13）

| 项 | 值 |
| --- | --- |
| `usage_logs` | 约 328 万行，堆 1.7GB，索引 3.1GB（26 个） |
| 本月（10-01~10-23） | 80 万行，约 3.5 万行/天，占全表 24% |
| active 用户 | 135 |
| 浏览器耗时（改造前） | scope 40ms，Summary 5.19s，Trend 1.77s |

Summary 的组织汇总、成员/峰值、平台拆分三条和 Trend 当时都对 `usage_logs` 整表 Seq Scan（每条读约 20 万页/1.6GB 后再按时间过滤）。已上线的常数优化：前端 Summary/Trend 并行；报表事务 `jit=off`、`work_mem=64MB`（§5.5）。根治靠日汇总（§6）；日汇总上线后的生产复测尚未记录（§14）。

### 7.3 日汇总本机实测（2026-10-23，嵌入式 PostgreSQL 16.9，非生产 SLA）

`TestOrganizationUsageRollupIntegration_ProductionShapedBenchmark`（需手动开启）：135 用户、3 账号，每天 4.7 万条、共 30 天（141 万行）；查询“本月 + 当天截至 15:00”，管理员全量范围；预热 1 次后取 7 次样本。29 天汇总共 11,745 行（约 405 行/天），发布耗时 1.1 秒。

| 接口 | 原始日志 中位/最大 ms | 日汇总 中位/最大 ms |
| --- | ---: | ---: |
| Summary（组织汇总 + 成员/峰值 + 拆分） | 3068 / 3122 | 138 / 160 |
| Trend（按日） | 1028 / 1057 | 35 / 36 |

两种模式结果逐位一致。部门性能测试夹具每用户每天约 1 条日志，汇总几乎不压缩，只用来确认无退化：366 天全量中位数 2526ms → 1986ms，所有档位 p95 < 3 秒。

Trend 的本机期望量级（非 SLO）：30 天 day < 100ms，90 天 < 200ms，366 天 < 500ms；若 366 天 day 远超 Summary 组织汇总一个数量级，先排查是否误走用户维 SQL。

### 7.4 复现入口

```powershell
# 合成基线 EXPLAIN（DSN 只放本进程环境变量，不写入仓库或日志）
$env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN = "<临时 PostgreSQL DSN>"
$env:ORGANIZATION_USAGE_RUN_EXPLAIN = "1"
go test -tags=integration -p 1 -count=1 ./internal/repository -run '^TestOrganizationUsageRepositoryExplainAnalyze$' -v
```

可用 `ORGANIZATION_USAGE_EXPLAIN_DAYS=30|90|366` 和 `ORGANIZATION_USAGE_EXPLAIN_QUERY=<query-name>` 限定范围（`organization_usage_explain_integration_test.go`）。

## 8. 前端页面

### 8.1 组件

| 模块 | 文件 |
| --- | --- |
| 页面与请求编排 | `frontend/src/views/admin/OrganizationUsageView.vue` |
| 筛选栏 | `frontend/src/components/admin/organization-usage/OrganizationUsageFilters.vue`（含 `OrganizationUsageSegmented.vue`） |
| 概览与 Champion | `OrganizationUsageOverview.vue` |
| 趋势图 | `OrganizationUsageTrendChart.vue` |
| 组织汇总 | `OrganizationUsageSummary.vue` |
| 部门/平台拆分 | `OrganizationUsageBreakdowns.vue`（部门文档） |
| 人员宽表 | `OrganizationUsagePeopleTable.vue`、`organizationUsageTable.css` |
| API client | `frontend/src/api/admin/organizationUsage.ts` |
| 日期、粒度推断、工作簿 | `frontend/src/utils/organizationUsageReport.ts` |
| 导出 Worker | `organizationUsageExportWorker.ts`、`organizationUsageExport.worker.ts` |
| 行数常量 | `frontend/src/constants/organizationUsage.ts` |
| 文案 | `frontend/src/i18n/locales/{zh,en}/admin/organizationUsage.ts` |
| 组件说明 | `frontend/src/components/admin/organization-usage/README.md` |

布局顺序：Header + 筛选 → 页面级错误条 → 内容区（概览 → 趋势图 → 组织汇总/拆分 → 人员表）。入口在管理员侧栏；路由守卫与负责人 landing 见部门文档。

### 8.2 筛选、默认值与导出绑定

| 模式 | 默认范围 |
| --- | --- |
| 月报 | 当前自然月 1 日到月末 |
| 周报 | 当前自然周周一到周日 |
| 自定义 | 最近 30 个自然日（含当天），最长 366 天（`validateCustomDateRange`） |

筛选还包括组织、部门、平台和邮箱。查询按钮应用草稿并把页码重置为 1；重置恢复月报、默认组织/部门范围、空邮箱、默认排序（总 Token 降序）和每页 20 人。点击组织汇总行立即按该组织筛选概览、趋势、Champion 和人员表，保留已应用的日期与邮箱。组织下拉的选项 value 仍是 `all/xunyou/wsdashi/other`，只有 label 用显示名。

导出与已查询结果绑定：页面维护“当前草稿”与“最后已应用筛选”的指纹（`filterFingerprint`，取模式、当前模式的日期值、组织、`q.trim()` 等已应用字段），两者一致才允许导出；查询、重置、组织行点击都同步已应用指纹。不一致时 `hasPendingFilters` → Filters 的 `exportDisabled`，导出按钮禁用并显示待查询提示。依据：2026-08-10 看板补强前，改了日期但未点查询时导出会按草稿取数，Excel 与页面已展示结果不一致。

### 8.3 页面状态

- Summary 加载中：隐藏上一响应的概览和组织汇总，人员表显示固定高度骨架行。依据：2026-07-13 审核发现筛选切换时旧概览仍显示，管理员会把旧数字当成新筛选结果。
- 空数据：保留表头并显示空状态。
- Summary 失败：页面级 `errorMessage` + 重试；内容区（含趋势图）整体不渲染，重试同时重拉 Summary 和 Trend。
- Trend 单独失败：只在图表区显示错误和重试，不写页面级错误。
- 排序和分页全部由服务端执行，表头同步 `aria-sort`；新点一列首次为升序（未改，§12）。
- 移动端：筛选栏换行；人员表保持宽表，在表格容器内横向滚动，不改卡片。

### 8.4 概览与展示合同

概览六项 KPI：成员人数（`active_users`）、活跃人数（`used_users`）、活跃率（`used_users / active_users`，分母为 0 显示 `0.0%`）、请求数、总 Token、实际成本（`formatCostFixed`）。不再展示输入 Token。成员人数、活跃人数、总 Token 旁用 `HelpTooltip` 解释口径；概览日期旁显示 `range.as_of` 作为数据截止时间；Champion 组织列用 `formatOrganizationUsageOrganization` 显示中文名。

组织显示名覆盖筛选器、组织汇总、人员表、Champion 和 Excel 全部组织列，展示函数不反向参与请求参数、筛选值或聚合。人员表保留缓存读取 Token 列，去掉缓存创建列；API 和 Excel 仍保留两类缓存 Token。

### 8.5 趋势图

**粒度推断**（`inferOrganizationUsageTrendGranularity`）：含首尾自然日 `≤31 → day`，`≤120 → week`，否则 `month`；复用 `organizationUsageReport.ts` 既有的含首尾天数计算（与 `validateCustomDateRange`、后端 `organizationUsageCalendarDays` 同口径），禁止用浏览器本地 `Date` 差值。月报（28–31 天）和周报都推断为 day；90 天 → week；366 天 → month；手动选 day + 366 天合法。日期范围变化或重置回到自动模式；仅组织/邮箱变化保留当前模式；用户点粒度切到手动。

**请求状态**：Summary 用 `reportController` + `loading`；Trend 是独立的 controller、`trendLoading`、`trendError`、最近一次成功的点和元数据。`trendLoading` 只控制图表区骨架，不驱动概览卸载。

| 触发 | 中止 Summary | 中止 Trend | 快照 | 结果 |
| --- | --- | --- | --- | --- |
| 挂载 / 应用筛选 / 重置 / 组织行点击 / 页面重试 | 是 | 是 | `loadFullReport` 递增 `reportCycleId`，生成新候选 `as_of` | Summary、Trend 并行；图表区先显示骨架，避免旧筛选曲线 |
| 人员排序 / 翻页 / 改页大小 | 是 | **否** | 复用 `snapshotAsOf` | 只重拉 Summary，已成功的曲线保持可见 |
| 手动切换粒度 / 图表区重试 | **否** | 是 | 复用 `snapshotAsOf` | 只重拉 Trend |
| 卸载 | 是 | 是 | — | — |

**共享快照协调**：
1. 完整加载用同一个候选 `as_of` 并行请求 Summary 和 Trend；
2. Summary 成功时 `range.as_of` 必须非空，作为页面权威快照写入 `snapshotAsOf`；缺失视为协议错误，进入页面级错误；
3. 任何 Trend 成功响应都必须带非空 `range.as_of`（以及部门文档要求的 `scope_version`），否则清空点并显示局部错误；
4. Summary 尚未返回时，可先展示带非空 `as_of` 的 provisional 曲线（并行 fast path）；Summary 返回后，若 Trend 的 `as_of` 或 `scope_version` 与 Summary 不同，先 `clearTrendSuccessState` 清掉不匹配的曲线，再用 Summary 快照**仅重拉一次**（`reconciledCycleId` 防重复）；
5. 重拉后必须严格相等，缺失或仍不同 → 清空 + 局部错误，不再循环；
6. 只有 `reportCycleId`、controller、signal 都仍有效时才写状态，旧周期的 Summary、Trend 与重拉响应全部丢弃。

正常时两者并行、无额外往返；只有客户端时钟超前导致两次服务端裁剪结果不同，或范围版本变化时，才多一次 Trend 请求。部门功能曾改为“Summary 返回后才请求 Trend”，2026-10-23 起恢复并行（部门文档 §7.4）。

**图表配置**（vue-chartjs Line，模式对齐 `frontend/src/components/charts/TokenUsageTrend.vue`，但不复用该组件，因为它绑定 `TrendDataPoint` 和缓存命中率）：
- 默认四个数据集，顺序固定：输入 Token、输出 Token、总 Token（左轴 `y`）、请求数（右轴 `yRequests`，`drawOnChartArea: false`）；图例文案复用 `metrics.*`；不画缓存创建、缓存读取和实际消费；
- 两轴 `beginAtZero: true`；请求数轴 `ticks.precision: 0`；Token 轴用紧凑数字格式；
- `cubicInterpolationMode: 'monotone'`（默认三次平滑会过冲），`interaction.mode: 'index'`，图例点击可隐藏；
- X 轴为 `period_start`，`maxTicksLimit` 在 12–16；点数 > 60 时 `pointRadius: 0`，否则 2，`pointHoverRadius` 始终 4；
- tooltip 附 `period_end` 和 partial 提示，最后一个已到达桶附格式化的 `range.as_of`；
- 全零数据仍画坐标轴；`maintainAspectRatio: false`，固定高度；
- 明暗主题通过 `MutationObserver` 触发配色重算，不用无依赖的静态 computed。

## 9. Excel 导出

文件名 `organization_usage_<start_date>_to_<end_date>.xlsx`，由 `buildOrganizationUsageWorkbook` 生成，固定八个工作表（首版六个，部门功能新增部门汇总、平台汇总）：

| 顺序 | Sheet | 内容 |
| --- | --- | --- |
| 1 | 报表概览 | 日期范围、归属口径（“当前启用成员及当前部门归属”）、筛选组织/部门/平台、成员人数、活跃人数、总体指标、日/周/月 Champion |
| 2 | 人员汇总 | 用户、组织、部门、总指标、个人日/周/月峰值；含零用量 active 成员；设列宽和自动筛选 |
| 3 | 组织汇总 | 组织、成员人数、活跃人数、指标 |
| 4 | 部门汇总 | 组织、部门 ID、部门、成员人数、活跃人数、指标 |
| 5 | 平台汇总 | 平台、活跃人数、指标 |
| 6–8 | 月度 / 周度 / 日度明细 | 有用量的“用户 × 周期”行，含 Partial、组织、部门；不生成全零行 |

组织列一律经 `formatOrganizationUsageOrganization` 显示中文名；Sheet 名、数值类型与行上限受 Workbook 测试保护。

导出协调（`fetchAllOrganizationUsageData`）：
- API 分页固定 500；先拉完 Summary 全部页，再依次串行拉 day、week、month Periods（不并发，避免同时触发三组大聚合）；
- Summary 首响应的 canonical `as_of` 和 `scope_version` 用于后续所有分页；途中范围变化（409）或权限失效（403）立即中止，不拼接不同版本（部门文档 §7.4）；
- 导出路径不得调用 Trend；
- 人员、组织、部门、平台和三类明细合计 ≤ `MAX_CLIENT_EXPORT_ROWS`（100,000）；单 Sheet ≤ `MAX_XLSX_DATA_ROWS`（1,048,575，为表头预留一行）；
- Workbook 构建和 `XLSX.write` 在 Web Worker 中执行；用户取消或页面卸载时终止网络请求和 Worker，卸载时不提示“用户主动取消”；
- 没有任何数据时也生成表头完整的合法工作簿。

已知限制（2026-07-13 审核确认，之后未专门处理）：导出受前端 API 默认 30 秒超时与串行分页影响，错误提示未区分超时、超限和取消；处理顺序是先优化聚合路径（§7），再决定导出专用超时和错误分类，不单独调高超时（§12）。

## 10. 代码地图

后端：

| 位置 | 内容 |
| --- | --- |
| `backend/internal/handler/admin/organization_usage_handler.go` | `OrganizationUsageHandler.Summary/Periods/Trend`、`parseOrganizationUsagePagination`、`writeOrganizationUsageError` |
| `backend/internal/service/organization_usage_service.go` | DTO、`OrganizationUsageRepository` 接口、`OrganizationUsageService.Summary/Periods/Trend`、`normalizeOrganizationUsageQuery`、`repositoryRange`、`clampAsOfToServerNow`、`organizationUsageCalendarDays`、`organizationUsageCalendarDate`、`validOrganizationUsageSortBy`、`OrganizationUsageLocation` |
| `backend/internal/service/organization_usage_department.go` | `OrganizationUsageDepartmentFilters.Normalize`（平台白名单）、`OrganizationUsageAppliedFilters`、部门/平台拆分 DTO |
| `backend/internal/repository/organization_usage_repo.go` | `ProvideOrganizationUsageRepository`；`organizationUsageActiveUsersCTE`、`organizationUsageDaysCTE`、`organizationUsageRowsCTE`、`organizationUsageTotalsCTE`、`organizationUsagePeriodBucketSQL`、`organizationUsageSummaryItemsSQL`、`organizationUsageChampionsQuery`、`organizationUsagePeriodsQuery`、`organizationUsageTrendQuery`、`organizationUsageTrendBucketSeriesSQL`、`organizationUsageOrderBy`、`organizationUsageSearchPattern`、`organizationUsageOverview` |
| `backend/internal/repository/organization_usage_department_repo.go` | `beginScoped`、`organizationUsageReportSessionSQL`、`scopedQuery`（占位替换）、`scopedSummary/Periods/Trend`、`organizationUsageChampionMetadataCTE`、`breakdowns` |
| `backend/internal/repository/organization_usage_rollup_plan.go` | `organizationUsageRollupPlan.bind`、`buildOrganizationUsageRollupPlan`、`loadOrganizationUsageRollupState`、`withRollupPlan` |
| `backend/internal/repository/organization_usage_rollup_repo.go` | `InitializeOrganizationUsageRollups`、`CloseNextOrganizationUsageRollupDay`、`RebuildNextDirtyOrganizationUsageRollupDay`、`rebuildOrganizationUsageRollupDay`、`markOrganizationUsageRollupRangeDirty`、`ReconcileOrganizationUsageRollups`、`AnalyzeOrganizationUsageRollups` |
| `backend/internal/service/organization_usage_rollup.go` | `OrganizationUsageRollupService`（`Start`/`Stop`/`RunOnce`）、调度常量 |
| `backend/internal/service/wire.go` | `ProvideOrganizationUsageRollupService`（开关关闭时不启动） |
| `backend/internal/repository/dashboard_aggregation_repo.go` | `dropUsageLogsPartitionWithRollupInvalidation` |
| `backend/internal/config/config.go` | `OrganizationUsageConfig.RollupEnabled` 及默认值 |
| `backend/internal/server/routes/admin.go` | 三个 `organization-report` 路由 |
| `backend/migrations/246_organization_usage_daily_rollups.sql` | 三张表、`mark_user_usage_rollup_dirty_day` 与五个触发器 |

前端见 §8.1；关键函数：`OrganizationUsageView.vue` 的 `loadFullReport`、`reportCycleId`、`reconciledCycleId`、`snapshotAsOf`、`clearTrendSuccessState`、`filterFingerprint`、`hasPendingFilters`；`organizationUsage.ts` 的 `getOrganizationUsageTrend`、`fetchAllOrganizationUsageData`；`organizationUsageReport.ts` 的 `inferOrganizationUsageTrendGranularity`、`validateCustomDateRange`、`buildOrganizationUsageWorkbook`；`organizations.ts` 的 `formatOrganizationUsageOrganization`。

## 11. 关键约束与易错点

### 11.1 口径与 SQL

- **邮箱搜索是字面匹配**：`organizationUsageSearchPattern` 依次转义 `\`、`%`、`_` 再包 `%...%`，两个 `ILIKE` 入口都写 `ESCAPE E'\\'`。依据：首版直接 `"%"+q+"%"`，输入 `100%`、`_` 会变成通配符（不是注入，但结果错）。
- **Champion 窗口里的列要带表别名**（`pa.user_id` 等）。依据：真实 PostgreSQL 集成测试首次运行时发现 `user_id` 歧义，sqlmock 和字符串合同都没测出。
- **三个 peak 与 `period_aggregates` 必须保持 `AS MATERIALIZED`**，或改成“每用户每粒度一行后只 JOIN 一次”的等价形状。依据：未物化时 90 天 items 出现约 11 秒的嵌套循环计划（§7.1）。不能用加索引或调高超时代替结构修复。
- **桶边界只从 `organizationUsagePeriodBucketSQL` 取**，Periods、峰值、Trend 共用；Trend 的 `generate_series` 起点必须先 `date_trunc` 到周一/月首。
- **Trend 不得按用户聚合**：SQL 合同测试断言含 `generate_series`、不含 `GROUP BY user_id`。也不得在应用层折叠 Periods 结果。
- **Trend 参数位 `$1..$7` 不变**（§5.4），`$7` 只接收 Service 派生的 `data_through`，不信任客户端。
- **不生成未来桶**：`data_through` 是 Service 算的，仓储只补已到达的桶。
- **新增报表 SQL 必须经 `queryRows`/`queryRow` 或 `plan.bind`**，否则 `{{...}}` 和部门占位不会被替换；占位值一律绑定为参数，不拼接用户输入或授权 ID。
- **费用全程 numeric**，只在最终输出列 `::double precision`；中间 CTE 提前转 double 会让汇总开/关两种模式不再逐位一致。
- **组织汇总在 Go 里按范围过滤**，`organizationUsageOverview` 要在过滤前用完整三组计算；改顺序会让负责人看到的概览漏算或多算。
- **sqlmock 不是日期桶、补零、时区边界的证据**。依据：趋势图首轮实现只有字符串断言和 sqlmock，被审核判为合并阻断，补真实 PostgreSQL 集成后才关闭。

### 11.2 快照与请求编排

- **Trend 响应必须 fail-closed**：成功响应缺 `range.as_of`（或范围版本）一律当失败；重拉后用严格相等判断。依据：曾实现为 `if (trendMeta.range.as_of && ... !== canonical)`，缺失 `as_of` 时条件为假，未对齐的点被当成成功展示（2026-07-27 审核 F3）。
- **修 fail-closed 时保留并行 fast path**：不能要求“Trend 返回时 Summary canonical 必须已存在”，Trend 可能先回来；先展示 provisional，再对齐。
- **对齐前先清掉不匹配的 provisional 曲线**（`clearTrendSuccessState`）。依据：自审发现 Trend 先到、Summary 后到且不一致时，重拉期间会短暂显示错误快照的曲线。
- **每个加载周期最多重拉一次 Trend**，仍不一致就局部报错，不循环。
- **人员排序/翻页不得中止或重拉 Trend**，也不得清空已成功曲线；粒度切换不得请求 Summary。
- **只有完整加载才创建新快照**；分页、排序、粒度、图表重试都复用 `snapshotAsOf`。
- **导出只能用已应用筛选**（§8.2），且不得调用 Trend。

### 11.3 日汇总

- **脏标记只追加**：不能给 `usage_user_rollup_dirty_days.bucket_date` 加唯一约束。依据：用 `ON CONFLICT DO NOTHING` 时标记已存在的写入方不会留下自己的标记，作业若在其提交前认领并重建该天，这次改动永久丢失（典型是保留期分批删最老的一天）；改 `ON CONFLICT DO UPDATE` 虽不丢，但写入方按天持有标记行锁，跨日事务按相反顺序修改会死锁（复审 F2 已复现）。
- **初始化不清空脏标记**。依据：若有早于读取到的 `MIN(created_at)` 的日志并发提交，它的标记要让该天在水位前保持脏并被重建，而不是被当作空的已发布日期（复审 F1）。
- **触发器不得读写状态行或加锁**；当天普通写入不写标记。照搬上游加锁水位会让重建期间网关写入排队。
- **按月 DROP 分区必须在同一事务里调用 `markOrganizationUsageRollupRangeDirty`**；任何新增的批量清理路径如果绕过行级 DELETE 触发器，也要自己登记日期。
- **批量发布 ≥7 天后 ANALYZE**；删除这一步会重现 12–19 秒的异常计划。
- **关闭前一天要等到午夜后 10 分钟**，触发器的 23:50 宽限窗口与之配套，两个常量要一起改。
- **时区一律用 `service.OrganizationUsageLocation()`**，SQL 里写 `'Asia/Shanghai'`。
- **迁移 246 不能修改**（checksum 校验）；结构变更或移除走新迁移。
- **汇总只存用户、分组、账号**；不要把组织、部门、平台写进去，否则改变“按当前值归属”的语义。

### 11.4 展示

- **显示名函数只用于展示**，请求参数和筛选值始终是 `xunyou`/`wsdashi`/`other`。
- **`total_tokens` 包含两类缓存 Token**，不能解释为输入、输出两条可见曲线之和；默认不画缓存曲线。
- **人数标签以 locale 为准**：`active_users` 当前显示“成员人数”，`used_users` 显示“活跃人数”；不改 i18n key 名，概览、组织汇总和 Excel 自动同步。
- **Excel 与页面同步**：改展示文案时 Workbook 测试必须同时覆盖概览、组织、部门、平台、人员和三类明细的组织列。

## 12. 已否决与暂不做

- 已否决：扩展 Periods 增加 `scope=aggregate`/`mode=trend`，原因是 Periods 是分页的用户 × 周期导出模型，聚合 + 补零会让合同变胖，导出客户端可能误用，测试矩阵翻倍。
- 已否决：前端分页拉 Periods 再 group-by，原因是 366 天 × 数百用户可达数十万行，与导出超时同源，补零和 partial 裁剪难在客户端复现。
- 已否决：把趋势点塞进 Summary 响应，原因是人员翻页会重复计算趋势，Summary 本身已含峰值聚合。
- 已否决：趋势图单轴，原因是 Token 与请求数相差数个数量级。
- 已否决：给 `as_of` 加 HMAC 或服务端 snapshot id，原因是接口已在管理员认证下，需求只要求固定用量上界；只修正文档措辞。
- 已否决：对 `partial` 周期再做产品确认、改成完整自然周/月口径，原因是原始需求已明确只统计所选范围内数据并标 partial。
- 暂不做：组织行再次点击回到“全部”、新排序列首次默认降序，原因是已有组织下拉“全部”和重置入口，页面初始已是总 Token 降序；属可选体验优化。
- 已被后续设计覆盖：2026-07-13 曾决定“页面普通分页不带 `as_of`，允许实时漂移”；趋势图上线后页面完整加载统一传候选 `as_of`，分页、排序复用快照（§8.5）。
- 暂不做：只提高前端超时、并发拉 day/week/month、服务端流式导出，原因是先做可验证的 SQL 修复，盲目并发会进一步压库。
- 暂不做：新增 `usage_logs` 索引，原因是证据显示索引扫描不是瓶颈（§7.1）；覆盖索引方案见 §6.9。
- 暂不做：多组织分色趋势线，若产品确认强需求另开设计，不预留未使用合同。
- 暂不做：`actual_cost` 改 decimal/string 合同，只有财务对账级需求才整体改。
- 暂不做：处理异常多 `@` 邮箱、把 `FixedZone(+8)` 换成 IANA location。

## 13. 验证

### 13.1 命令

Windows 下按 `llm-wiki/wiki/ops.md` 使用仓库内 `GOCACHE`/`GOMODCACHE`、每轮新建 `GOTMPDIR`、`-p 1 -count=1`。CI 单测门禁带 `-tags=unit`。

```powershell
cd backend
# 单元：Service / Repository（SQL 合同、sqlmock、读取计划）/ Handler / Routes / Config / 作业编排
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes ./internal/config -run 'OrganizationUsage|RegisterUsageRoutes'
# 迁移 246 结构与无行锁断言（只在 unit 标签下编译）
go test -tags=unit -p 1 -count=1 ./migrations -run TestMigration246
# 真实 PostgreSQL：Testcontainers，或在本进程设置 SUB2API_POSTGRES_ONLY_INTEGRATION_DSN 指向新建的空库
go test -tags=integration -p 1 -count=1 ./internal/repository -run 'OrganizationUsage' -v

cd ../frontend
node_modules\.bin\vitest.cmd run organizationUsage OrganizationUsage
pnpm run typecheck
pnpm run lint:check
```

在 linked worktree 中 `pnpm --dir frontend exec vitest` 曾无法解析 vitest，改用 `frontend` 目录下的 `node_modules\.bin\vitest.cmd`。DSN 和凭据不得写入仓库或日志。

### 13.2 测试清单

| 层 | 覆盖 | 主要测试 |
| --- | --- | --- |
| Service | 日期解析与北京时间区间、366 天上限、枚举/排序/分页校验与默认值、`as_of` 规范化与服务端裁剪、早于范围钳制为空区间、Trend 默认粒度与 `data_through` 派生、仅未来范围短路 | `organization_usage_service_test.go` |
| Repository SQL 合同 | 精确域名与 active 条件、搜索转义、零用量峰值为 NULL、排序白名单与 `user_id` 次序、周一分桶与裁剪、Trend 补零且无用户维度 | `organization_usage_repo_test.go` |
| Repository sqlmock | 参数绑定、扫描、空页保留总数、错误包装 | 同上 |
| 读取计划 | 无水位全走原始、已发布日 + 原始尾段、脏日期走原始并合并相邻区间、历史范围无原始段、`as_of` 早于范围、占位绑定为参数 | `organization_usage_rollup_plan_test.go` |
| 作业 | 回填后 ANALYZE、排空后进入对账、预算截止与错误上报、Start/Stop 幂等 | `organization_usage_rollup_test.go` |
| PostgreSQL 集成：报表 | 域名/状态/零用量、字面搜索、北京时间边界与周/月 partial、Trend 补零与 `data_through`、54 周上界、Champion 并列取最小 `user_id` | `organization_usage_repo_integration_test.go` |
| PostgreSQL 集成：日汇总 | 汇总开/关结果相等（`ReportsMatchRawAggregation`）、触发器只登记过去日期、同步生命周期与对账、写入方标记存活、跨日无死锁、初始化保留早期标记、跨分区 UPDATE 登记两天、DROP 分区登记整月、生产形态基准（手动） | `organization_usage_rollup_integration_test.go` |
| PostgreSQL 集成：范围 | 各接口同一范围、报表事务 `jit=off` 与 `work_mem` | `organization_usage_department_integration_test.go` |
| Handler / Routes / Config | 严格绑定、显式 0 分页拒绝、400/500 映射、非法 `as_of`、Trend 无分页、三个路由注册、`rollup_enabled` 配置与环境变量 | 对应 `_test.go` |
| 前端 | View 状态机与快照协调、导出绑定已应用筛选、Filters/Overview/PeopleTable/Summary、TrendChart 数据集与轴配置、粒度推断边界 31/32/120/121、API client（导出不调 Trend）、Workbook 全部 Sheet 与组织列、Worker、路由、双语 locale | `frontend/src/**/__tests__/*organizationUsage*`、`*OrganizationUsage*` |

验收要点：管理员能从侧栏进入、普通用户不能访问；月报/周报/自定义请求符合北京时间；组织筛选正确影响概览、趋势、Champion 和人员表；active 零用量用户出现、禁用/删除用户不出现；排序分页在服务端；当前月/周不出现未来 0 点；人员翻页时趋势不闪不空；Summary 失败整页报错、Trend 失败只影响图表区；Excel 多页使用同一快照，取消能停止网络与 Worker；加载、空、错误、桌面和移动状态无页面级横向溢出。

### 13.3 最近结果

本文合并时（2026-11-04）只核对了源码，没有重跑测试。历史记录：

| 日期 | 范围 | 结果 |
| --- | --- | --- |
| 2026-07-13 | 审核修复（搜索转义、加载隐藏旧概览、`as_of` 措辞、首批 PG 集成） | Repository/Service 单元通过；组织用量 PG 集成通过；前端 161 文件 / 1025 测试通过；typecheck、lint、`git diff --check` 通过 |
| 2026-07-27 | 趋势图 | 前端组织用量相关 53 测试、后端单元通过；当时 Trend 的 PG 集成用例已写但未在本机运行 |
| 2026-08-10 | 展示调整 | 专项 6 文件 / 64 测试，全量 246 文件 / 1704 测试；typecheck、lint 通过；桌面/移动视觉检查因无管理员会话和后端数据受阻 |
| 2026-08-10 | 看板补强 | 计划所列专项测试、typecheck、lint、`git diff --check` 均已勾选完成，具体计数未入库 |
| 2026-10-23 | 日汇总 | 本机单元与 `-tags integration` 日汇总用例通过；生产形态基准见 §7.3 |

## 14. 状态与待办

已完成：首版报表（`7c346b314`）；2026-07-13 审核修复；peak 物化与 Champion 复用；趋势图；展示调整与看板补强；部门/平台范围接入（部门文档）；日汇总（迁移 246、作业、读取计划、配置开关）。

待确认（日汇总，需产品或客户决定；当前按“严格镜像原始日志、费用精确求和”实现）：
1. 数据保留语义：日志超过保留期被删后，报表同样查不到。若要求报表保留更久，需要单独设计（例如保留期清理的删除不登记脏日期），并补充口径说明。
2. 费用精度：接受 `actual_cost` 末位可能变化（变得更精确，§6.7）。

待办：
- 日汇总生产上线后复测浏览器 Summary/Trend 耗时，对已发布日期逐日比较汇总与原始合计，观察 `organization_usage_rollup_reconcile_mismatch` 警告；
- 在有 PostgreSQL 的环境确认 Trend 集成用例（`TrendZeroFillAndDataThrough`、`TrendWeekBucketUpperBound54`）在当前代码上通过，并记录结果；
- 在具备管理员会话和数据的环境补做桌面/移动视觉检查（人数标题截断、图例、表格重叠、主题切换）；
- 导出专用超时与 timeout/too-large/canceled 错误分类（§9），在聚合路径优化后再定；
- 可选：在新 SQL 上重跑 §7.1 合成基线，或在生产脱敏副本上对 Summary/Trend 做 `EXPLAIN (ANALYZE, BUFFERS)`。
