# Token Analysis Selected User Trend Design

## 状态

| 项目 | 状态 |
| --- | --- |
| 后端精确选人查询、参数校验与缓存隔离 | 已实现并通过定向单测；PostgreSQL integration 测试已编译，待可用 PostgreSQL 环境运行 |
| 前端 API、选择交互与折线图 | 已实现并通过定向测试、typecheck、lint:check |
| 真实 `EXPLAIN ANALYZE` | opt-in 测试已实现；当前机器无 Docker、PostgreSQL 或外部 DSN，尚未取得运行结果 |
| 提交状态 | 按用户要求不提交、不推送 |

## 目标

在现有管理端 Token 分析页面增加“计费用量趋势”，管理员可从用户排行中选择最多 5 名用户，按日或按小时比较每人的总 Token 用量。

## 数据口径

- 权威数据源固定为 `usage_logs`，不从 `token_analysis_request_summaries` 反推。
- 图表指标首版只展示总 Token：`input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`。
- 用户排行仍是已归档样本口径，趋势图标题明确为“计费用量趋势”，避免与排行数字混淆。
- 时间分桶固定使用 `Asia/Shanghai`：日为北京时间自然日，小时为北京时间整点。
- 原始 `usage_logs` 默认保留 90 天，因此日趋势最多查询 90 个自然日；小时趋势首版只允许同一自然日。

## 方案选择

### 方案 A：扩展现有 `/admin/dashboard/users-trend`（采用）

增加可选的 `user_ids=7,8` 参数。传入时精确查询所选用户；未传入时保持现有 Top N 行为。该方案复用 Dashboard Service、Repository、30 秒快照缓存和现有 `UserUsageTrendPoint`，不新增数据库表或迁移。

### 方案 B：前端并发调用单用户 `/admin/dashboard/trend`

后端无需修改，但最多会产生 5 个并发请求、5 次聚合和独立缓存项，错误与取消处理更复杂，不采用。

### 方案 C：新增 Token Analysis 专属趋势接口

接口边界更贴近页面，但会重复用量领域逻辑，并容易误用归档样本口径，不采用。

## 后端合同

接口保持：

```http
GET /api/v1/admin/dashboard/users-trend
```

新增可选参数：

| 参数 | 规则 |
| --- | --- |
| `user_ids` | 逗号分隔的正整数，显式空值或空片段非法；去重并升序规范化，最多 5 个唯一用户 |
| `start_date` | 精确选人模式必填，格式 `YYYY-MM-DD` |
| `end_date` | 精确选人模式必填，格式 `YYYY-MM-DD`，闭区间 |
| `granularity` | `day` 或 `hour` |
| `limit` | 仅未传 `user_ids` 的 Top N 模式生效 |

精确选人模式验证：

- 非法、空片段、非正数或超过 5 个用户返回 `400`。
- 显式传入 `user_ids=` 与完全不传 `user_ids` 语义不同：前者返回 `400`，后者保持旧 Top N 模式。
- `end_date < start_date` 返回 `400`。
- `day` 最多 90 个自然日。
- `hour` 必须 `start_date == end_date`。
- 缓存键包含规范化后的 `user_ids`，顺序不同但集合相同的请求命中同一缓存项。
- 精确选人模式将 `limit` 规范化为 `0`，不同 `limit` 不产生重复缓存项。

响应继续使用现有 `UserTrendResponse` 和 `UserUsageTrendPoint`，不扩展 DTO。

## 聚合查询

精确选人模式使用 `(user_id, created_at)` 复合索引，查询形状为：

```sql
SELECT
    TO_CHAR(u.created_at AT TIME ZONE 'Asia/Shanghai', '<whitelisted format>') AS date,
    u.user_id,
    COALESCE(us.email, '') AS email,
    COALESCE(us.username, '') AS username,
    COUNT(*) AS requests,
    COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) AS tokens,
    COALESCE(SUM(u.total_cost), 0) AS cost,
    COALESCE(SUM(u.actual_cost), 0) AS actual_cost
FROM usage_logs u
LEFT JOIN users us ON us.id = u.user_id
WHERE u.user_id = ANY($1)
  AND u.created_at >= $2
  AND u.created_at < $3
GROUP BY date, u.user_id, us.email, us.username
ORDER BY date ASC, u.user_id ASC;
```

`user_ids` 通过 `pq.Array` 绑定；粒度继续走 `safeDateFormat` 白名单。未传 `user_ids` 时保留现有 Top N SQL，不改变管理端 Dashboard。

## 页面交互

- 用户排行增加第一列复选框；无 `user_id` 的归档行不可选择。
- 用户排行提供分页控件，选择状态独立于当前页并在翻页后保留。
- 最多选择 5 人；达到上限后，其他未选复选框禁用，已选项仍可取消。
- 选择至少 1 人后，在排行与请求明细区域下方显示独立趋势图面板。
- 面板包含标题、已选数量和“按日 / 按小时”分段控制。
- 小时模式只在筛选开始日期等于结束日期时可用。
- 图表每名用户一条线，纵轴为总 Token；颜色顺序稳定跟随选择顺序。
- 前端根据日期范围生成完整横轴，后端缺失的用户/周期填 0；完全无用量也显示合法零值折线。
- 用户切换、粒度切换和页面刷新使用请求序号丢弃迟到响应，加载时清空旧趋势，失败时显示错误状态并允许重试。

## 性能验收

- 已有 migration 提供 `idx_usage_logs_user_created (user_id, created_at)`，符合“用户等值过滤在前、时间范围在后”的复合索引顺序，本功能不新增 migration。
- `backend/internal/repository/user_usage_trend_explain_integration_test.go` 对 1/5 人、90 天日粒度和 1 天小时粒度运行 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`。
- 验收要求执行计划命中 `idx_usage_logs_user_created`，不得因选择 1～5 人扫描全部历史 `usage_logs`；同时记录 planning/execution time、rows 与 buffer 指标。
- 当前机器缺少 PostgreSQL 运行环境时，只能确认 integration 测试编译通过，不能据此声称真实执行计划已通过。

## 测试

- Repository：精确用户集合、北京时间日界线、小时分桶、Token 求和、未选模式保持 Top N、参数数组化。
- Handler/缓存：参数解析、去重排序、5 人上限、90 天限制、小时同日限制、缓存键区分集合且忽略顺序。
- 前端 API：数组序列化为逗号分隔 `user_ids`。
- 页面：复选框、多页选择保留、5 人上限、按日/小时切换、零值补齐、迟到响应、错误与空状态。
- 回归：现有管理端 Dashboard Top N 用户趋势不变。

## 非目标

- 不新增用户小时/日汇总表，不支持 90 天以前的用户趋势。
- 不新增输入、输出、缓存、费用等指标切换。
- 不改变 Token Analysis 用户排行的归档样本口径。
- 不新增页面或路由，不修改全局 Chart.js 组件。
- 不提交、不推送或合并当前分支。
