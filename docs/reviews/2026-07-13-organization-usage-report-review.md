# 组织用量报表实现审核报告

- 审核日期：2026-07-13
- 审核人：Copilot（对照设计文档复核 Codex 实现）
- 工作分支：`feature/hy/0710_组织用量报表`
- 实现提交：`7c346b314 feat: 组织用量报表功能落地`
- 设计文档：`docs/features/organization-usage-report-design-cn.md`
- 审核范围：后端 Handler/Service/Repository、前端页面/API/导出、路由权限、测试覆盖与边界语义

---

## 1. 结论摘要

| 维度 | 评价 |
| --- | --- |
| 需求主链路 | 已落地：独立页面、Summary/Periods、三组织汇总、峰值/Champion、六 Sheet 导出 |
| 分层与权限 | Handler/Service/Repository 边界清晰；挂在 admin auth + compliance 下正确 |
| 测试 | Service/Handler/前端单测较全；Repository 以 SQL 合同 + sqlmock 为主，缺真实 PostgreSQL 集成 |
| 生产稳健性 | **有风险**：Summary/导出重复全量聚合，大数据量下易超时或压库 |
| 边界与体验 | 有若干缺口：筛选陈旧数据、组织反选、排序默认方向、ILIKE 转义、`as_of` 语义名不副实 |

**一句话：**

> 这不是半成品，主流程和设计大项基本齐；更像“中小数据量可用，生产大数据量与若干边界体验仍需补强”的实现。

**上线建议：**

- 中小规模试用可接受。
- 正式面向全量历史/大导出前，至少处理 **P0 性能** 与 **P1 陈旧数据展示 / as_of 语义**。

---

## 2. 审核方法

1. 阅读设计文档 `docs/features/organization-usage-report-design-cn.md`。
2. 对照实现提交 `7c346b314` 的后端/前端关键文件。
3. 重点检查：
   - 业务口径（活跃用户、组织归属、指标、峰值规则）
   - 时间与 `as_of` 快照语义
   - SQL 聚合与分页性能
   - 权限与路由边界
   - 前端筛选/排序/导出边界
   - 测试是否覆盖关键合同与真实数据库行为
4. 未在本轮执行全量 `go test` / 浏览器 E2E；结论以静态代码审查与现有单测结构为主。

---

## 3. 实现覆盖核对

### 3.1 已对齐设计的部分

| 设计点 | 实现结论 | 证据 |
| --- | --- | --- |
| 独立管理端页面 | 已实现 | `frontend/src/views/admin/OrganizationUsageView.vue`，路由 `/admin/organization-usage` |
| 仅管理员入口 | 已实现 | 路由 `requiresAdmin: true`；侧栏仅 admin 菜单 |
| 后端接口前缀 | 已实现 | `/api/v1/admin/usage/organization-report/{summary,periods}` |
| 活跃用户口径 | 已实现 | `deleted_at IS NULL AND status = 'active'` |
| 组织归属 | 已实现 | `LOWER(SPLIT_PART(email,'@',2))` 精确匹配 `xunyou.com` / `wsdashi.com`，其余 `other` |
| 三组织始终返回 | 已实现 | `VALUES ('xunyou'),('wsdashi'),('other')` + LEFT JOIN |
| 零用量活跃用户保留 | 已实现 | 人员主表 LEFT JOIN `usage_totals` |
| 北京时间闭区间 | 已实现 | Service 按 `Asia/Shanghai` 解析，Repository 用 UTC 半开区间 |
| 最大 366 天 | 已实现 | Service 校验 + 前端自定义范围校验 |
| 峰值/Champion 稳定并列 | 已实现 | `total_tokens DESC, actual_cost DESC, requests DESC, user_id ASC, bucket_start ASC` |
| 排序白名单 | 已实现 | Service + Repository allowlist，防 SQL 注入 |
| 六 Sheet 导出 | 已实现 | 报表概览/组织汇总/人员汇总/月周日明细 |
| 导出行数限制 | 已实现 | `MAX_CLIENT_EXPORT_ROWS=100000`，`MAX_XLSX_DATA_ROWS=1048575` |
| Worker 导出与取消 | 已实现 | Web Worker + AbortController + 页面卸载 abort |
| 无 DB migration | 符合设计 | 只读 `users` / `usage_logs` |
| Wire/路由注册 | 已实现 | `wire_gen.go`、`routes/admin.go`、各层 `wire.go` |

### 3.2 实现文件清单（核心）

**后端**

- `backend/internal/handler/admin/organization_usage_handler.go`
- `backend/internal/service/organization_usage_service.go`
- `backend/internal/repository/organization_usage_repo.go`
- `backend/internal/server/routes/admin.go`
- 对应 `*_test.go`

**前端**

- `frontend/src/views/admin/OrganizationUsageView.vue`
- `frontend/src/api/admin/organizationUsage.ts`
- `frontend/src/components/admin/organization-usage/*`
- `frontend/src/utils/organizationUsageReport.ts`
- `frontend/src/utils/organizationUsageExportWorker.ts`
- `frontend/src/utils/organizationUsageExport.worker.ts`

---

## 4. 问题清单

### 4.1 P0：Summary 查询成本过高，导出时会被放大

**现象**

`Summary` 每次请求串行执行 3 次独立 SQL：

1. 组织汇总
2. 人员分页（内含 day/week/month 全量峰值聚合）
3. Champion（再次 day/week/month 全量聚合）

每次都会重新扫描 `usage_logs` 时间窗并做三套周期聚合；`LIMIT/OFFSET` 发生在聚合之后。

导出时 `page_size=500` 多页拉取，复杂度近似：

```text
页数 ×（全量 usage 聚合 + 峰值窗口计算）
```

**影响**

- 366 天 + 活跃用户较多时，管理端易超时（前端 `apiClient` 默认 30s）
- 可能对 PostgreSQL 造成明显压力
- 导出比页面查询更危险，因为会重复打多页 Summary + day/week/month Periods

**建议**

1. Summary 单次请求内合并 CTE，避免 3 次全表聚合。
2. 峰值与 Champion 复用同一套 period 聚合结果。
3. 中长期考虑服务端一次性快照/流式导出，而不是前端分页拼装。

**相关文件**

- `backend/internal/repository/organization_usage_repo.go`
- `frontend/src/api/admin/organizationUsage.ts`

---

### 4.2 P1：设计写了“签名快照”，实现只是普通时间戳

**设计原文**

> Summary 响应在 `range.as_of` 返回 canonical signed snapshot。

**实际实现**

- 校验严格 RFC3339/RFC3339Nano
- 规范为 UTC
- `min(as_of, server_now)`
- **没有 HMAC/签名/服务端会话快照 ID**

**影响**

- 不是权限漏洞（接口本身在管理员认证下）
- 文档与实现语义不一致
- 客户端仍可主动传更早的 `as_of` 做历史回看（若这不是目标，应明确禁止或说明）

**建议**

二选一：

1. 补真正签名或服务端 snapshot id；
2. 或把设计改成“canonical as_of 时间戳”，删除“signed”表述。

**相关文件**

- `backend/internal/service/organization_usage_service.go`
- `docs/features/organization-usage-report-design-cn.md`

---

### 4.3 P1：筛选切换时页面会展示陈旧数据

**现象**

`loadReport()` 开始时：

- 不清空 `report`
- 只给人员表 `loading=true`

因此改组织/日期/排序后，Overview 与组织汇总仍显示上一份结果，直到新请求返回。

**影响**

管理员容易误判“筛选已变、数字未变”。

**建议**

1. loading 时给 overview/summary 加骨架或遮罩；
2. 或在请求开始时清空/标记 stale。

**相关文件**

- `frontend/src/views/admin/OrganizationUsageView.vue`

---

### 4.4 P2：组织汇总点击不能回到“全部”

**现象**

`selectOrganization` 只接受 `xunyou|wsdashi|other`，再次点击已选组织不会取消，只能靠下拉框改回 `all`。

**影响**

交互不对称，筛选路径不完整。

**建议**

再次点击同一组织时切回 `all`，或提供“清除组织筛选”。

**相关文件**

- `frontend/src/views/admin/OrganizationUsageView.vue`
- `frontend/src/components/admin/organization-usage/OrganizationUsageSummary.vue`

---

### 4.5 P2：排序首次点击固定为 `asc`，指标列体验差

**现象**

```ts
emit('sort', sortBy, props.sortBy === sortBy && props.sortOrder === 'asc' ? 'desc' : 'asc')
```

新点一列总是从 `asc` 开始。

**影响**

对 `total_tokens` / `actual_cost` 等指标，通常更期望默认 `desc`。

**建议**

指标列首次点击默认 `desc`，邮箱列可保持 `asc`。

**相关文件**

- `frontend/src/components/admin/organization-usage/OrganizationUsagePeopleTable.vue`

---

### 4.6 P2：邮箱模糊搜索未转义 `%` / `_`

**现象**

```go
return "%" + q + "%"
```

用户输入 `100%`、`_` 会变成 SQL 通配符。

**影响**

搜索结果可能不符合字面预期。

**建议**

转义 `\%` `\_`，并使用 `ESCAPE '\'`。

**相关文件**

- `backend/internal/repository/organization_usage_repo.go`

---

### 4.7 P2：partial 周期指标语义需产品确认

**实现语义**

1. `usage_rows` 先按所选时间窗过滤；
2. 再 `date_trunc` 成日/周/月桶；
3. 展示时裁剪 `period_start/end` 到所选范围，并标 `partial=true`；
4. **指标值只含窗内数据，不是自然周/自然月全量**。

**影响**

若业务期望“完整自然周/自然月冠军”，当前实现不满足；
若业务接受“窗内部分周期峰值”，则与设计一致，但应在页面/导出中更明确说明。

**建议**

与产品确认后，在 UI 文案和设计文档中固定口径。

---

### 4.8 P2：前端导出受 30s 超时与串行分页限制

**现象**

导出流程：

1. 先拉完 Summary 全部分页；
2. 再串行 day / week / month；
3. 每页都走完整聚合。

**影响**

大数据量失败概率高；错误提示未必能区分超时与容量限制。

**建议**

与 P0 一并处理；至少为导出请求提高超时，并区分 timeout / too-large / canceled。

---

### 4.9 P2：Repository 缺真实 PostgreSQL 集成测试

**现状**

已有：

- SQL 字符串合同测试
- sqlmock 扫描/分页测试
- Service 日期 / `as_of` / 白名单测试
- 前端页面与导出单测

缺失：

- 真实 PG 的 `date_trunc('week')` 周一边界
- `AT TIME ZONE 'Asia/Shanghai'` 日界
- `SPLIT_PART` 域名精确匹配
- 零用量用户 LEFT JOIN
- 并列 Champion 稳定性

**建议**

上线前至少补一组 integration，不能只用 sqlmock 替代。

---

## 5. 低优先级观察

| 项 | 说明 |
| --- | --- |
| `actual_cost` 用 `double precision` 聚合 | schema 为 `decimal(20,10)`，转 double 有精度误差；与部分旧统计一致，但报表导出更敏感 |
| `SPLIT_PART(email,'@',2)` | 异常邮箱 `a@b@c.com` 只取 `b`；极端情况归类可能不准 |
| `Asia/Shanghai` 用 `FixedZone(+8)` | 当前中国无 DST，可接受；与 PG IANA 时区在历史日期上偶有差异 |
| 页面查询不带 `as_of` | 符合“导出主用 as_of”的设计，但翻页期间若有新用量，人员表可能轻微漂移 |
| 组织汇总兼容 `xunyou.com` 显示值 | 后端实际返回 `xunyou`，兼容无害，略冗余 |
| `go.sum` 有小变更 | 确认是否必要，避免无关 diff |

---

## 6. 安全与可靠性核对

| 检查项 | 结论 |
| --- | --- |
| 是否仅管理员可访问 | 是，注册在 `/api/v1/admin` + AdminAuth + ComplianceGuard |
| 前端是否限制普通用户入口 | 是，`requiresAdmin: true`，侧栏仅 admin |
| 排序字段是否防注入 | 是，固定 allowlist |
| 组织/粒度枚举是否严格 | 是，Service 白名单校验 |
| Context 是否传播取消 | 是，Handler 使用 `c.Request.Context()`，Repository 使用 `QueryContext/QueryRowContext` |
| 是否新增 migration / 配置项 | 否，符合设计非目标 |
| 是否记录密钥/凭据 | 否 |

---

## 7. 测试覆盖评估

| 层级 | 覆盖情况 | 缺口 |
| --- | --- | --- |
| Service | 较好：日期、366 天、`as_of`、枚举、分页默认值 | 可补更多边界组合 |
| Handler | 较好：参数绑定、400/500 映射 | Context 取消传播可补 |
| Repository | 中等：SQL 合同 + sqlmock | 缺真实 PG integration |
| 前端页面 | 较好：筛选、排序分页、加载/空/错误、导出取消 | 陈旧数据展示未覆盖 |
| Excel | 较好：六 Sheet、行数上限、空工作簿 | 大数据量超时场景难单测 |

---

## 8. 修复优先级建议

| 优先级 | 项 | 建议动作 |
| --- | --- | --- |
| P0 | Summary/导出重复全量聚合 | 合并 SQL CTE，复用 period 聚合；评估服务端导出 |
| P1 | loading 陈旧数据 | 请求中遮罩/骨架，或标记 stale |
| P1 | `as_of` 签名语义 | 补签名/snapshot，或修正设计文档措辞 |
| P2 | 组织筛选可取消 | 再次点击回 `all` |
| P2 | 指标列默认降序 | 区分 email 与 metric 列 |
| P2 | ILIKE 转义 | 转义 `%` `_` |
| P2 | partial 口径确认 | 产品确认后固化文案 |
| P2 | 导出超时体验 | 提高超时并区分错误类型 |
| P2 | PG integration | 补真实数据库边界测试 |

---

## 9. 验收对照（基于当前实现）

| 验收项 | 当前判断 |
| --- | --- |
| 管理员可从侧栏进入 `/admin/organization-usage` | 通过 |
| 普通用户不能访问 | 通过（路由/后端权限） |
| 月报/周报/自定义范围符合北京时间口径 | 通过（静态审查） |
| 三组组织汇总始终存在 | 通过 |
| 组织筛选影响总体、Champion、人员表 | 通过 |
| active 零用量用户出现在人员汇总 | 通过 |
| 禁用/软删除用户不出现 | 通过 |
| 总 Token/实际消费/峰值/Champion 稳定排序 | 通过（SQL 合同） |
| 服务端分页排序，不退化为当前页客户端排序 | 通过 |
| Excel 固定六 Sheet，多页复用同一 `as_of` | 基本通过（`as_of` 非签名） |
| 取消导出能停止网络与 Worker | 通过 |
| 大数据量下可稳定查询/导出 | **未通过风险项** |

---

## 10. 最终意见

**可以合入开发/联调分支继续打磨，不建议在未处理 P0 前直接作为生产全量报表能力发布。**

推荐下一步：

1. 先做 P0 性能收敛（Summary 单次聚合 + 导出路径减负）。
2. 同步修 P1 陈旧数据展示，并统一 `as_of` 文档/实现语义。
3. 补 PostgreSQL integration 与一次真实数据量压测（至少 30 天、90 天、366 天三档）。

---

## 11. 参考路径

- 设计：`docs/features/organization-usage-report-design-cn.md`
- 实现提交：`7c346b314`
- 后端核心：
  - `backend/internal/handler/admin/organization_usage_handler.go`
  - `backend/internal/service/organization_usage_service.go`
  - `backend/internal/repository/organization_usage_repo.go`
- 前端核心：
  - `frontend/src/views/admin/OrganizationUsageView.vue`
  - `frontend/src/api/admin/organizationUsage.ts`
  - `frontend/src/utils/organizationUsageReport.ts`

---

## 12. Codex 复核回复

- 回复日期：2026-07-13
- 回复人：Codex（实现方复核）
- 复核原则：审核意见作为待验证项，最终结论以原始需求、设计文档、源码和现有测试合同为准。

### 12.1 总体回复

本次审核准确识别了主链路覆盖情况以及若干真实风险，但部分问题的优先级需要调整。当前结论是：

1. 筛选期间展示陈旧数据、邮箱搜索通配符未转义属于明确功能问题，需要修复。
2. Summary/导出重复聚合和真实 PostgreSQL 测试缺口需要在生产全量发布前处理。
3. `as_of` 不需要补密码学签名，应修正文档和 wiki 中“签名/签发”的错误表述。
4. `partial` 已由原始需求明确，不需要再次确认产品口径。
5. 组织反选和新排序列默认方向属于可选体验优化，不作为当前需求缺陷。

### 12.2 逐项结论

| 审核项 | 复核结论 | 是否处理 | 调整后优先级与说明 |
| --- | --- | --- | --- |
| 4.1 Summary/导出重复全量聚合 | 风险成立，但静态代码不足以直接判定 P0 | 是 | 生产发布前完成 30/90/366 天真实数据 `EXPLAIN ANALYZE` 和压测；出现超时或明显压库后升级为 P0 阻断 |
| 4.2 “signed snapshot”名不副实 | 文档术语错误，不是权限或安全漏洞 | 是，仅文档 | 删除设计文档及 wiki 中 `signed`、签名、签发等措辞，统一为“服务端裁剪并规范化的 canonical `as_of` 时间戳”；不增加 HMAC 或 snapshot id |
| 4.3 筛选切换展示旧数据 | 成立，管理员可能把旧指标误认为新筛选结果 | 是 | P1；加载期间遮罩、骨架或隐藏 Overview 和组织汇总，并补前端测试 |
| 4.4 组织行不能反选为全部 | 不属于原需求缺失，已有组织下拉框的“全部”和重置入口 | 否 | 可选 UX 优化，不阻塞上线 |
| 4.5 新排序列首次固定为升序 | 实现未违反设计；页面初始仍为总 Token 降序 | 否 | 可选 UX 优化，不作为缺陷 |
| 4.6 邮箱搜索未转义 `%`、`_` | 成立，会导致字面搜索结果错误 | 是 | P2；转义反斜杠、`%`、`_`，并在 `ILIKE` 中明确 `ESCAPE`；该问题不是 SQL 注入 |
| 4.7 `partial` 周期语义 | 审核建议不采纳 | 否 | 设计已明确边界周期只统计所选范围内数据并标记 `partial=true`，页面和 Excel 也已展示 Partial |
| 4.8 导出 30 秒超时与串行分页 | 风险成立，但属于 4.1 的后果，不宜单独通过提高超时或并发请求处理 | 随 4.1 处理 | 先优化和压测聚合路径，再决定导出专用超时及 timeout/too-large/canceled 错误分类；日/周/月盲目并发可能进一步压库 |
| 4.9 缺真实 PostgreSQL integration | 成立，且原测试计划已明确要求 | 是 | 生产发布前补齐；覆盖时区日界、周一边界、域名归类、零用量用户和稳定并列规则 |

### 12.3 性能项补充说明

审核报告将 Summary 描述为 3 次 SQL，源码实际执行 4 次数据库查询：组织汇总、人员总数、人员聚合分页、Champion。导出人员分页时还会重复请求不随页码变化的组织汇总和 Champion；Periods 每页也会执行 count 和 data 两次查询，因此重复成本确实存在。

仓库已经存在 `usage_logs(created_at)` 和 `usage_logs(user_id, created_at)` 索引，时间范围过滤具备基础索引条件。因此当前只能确认“存在生产性能风险”，不能在缺少生产级数据规模、执行计划和耗时证据时直接确认 P0。优化方案应以真实执行计划为依据，不预设必须合并成单条复杂 SQL。

### 12.4 低优先级观察回复

| 观察项 | 回复 |
| --- | --- |
| `actual_cost` 转 `double precision` | 当前报表 API、前端和 Excel 最终都使用浮点数，且页面按固定小数位展示，暂不作为本轮阻断项；若后续要求财务对账级精度，需要整体改为 decimal/string 合同，而不是只改这一条 SQL |
| 异常多 `@` 邮箱 | 正常注册和绑定链路已有邮箱格式校验；仅手工写库或历史异常数据可能触发，暂不处理 |
| `FixedZone(+8)` | 当前业务日期范围下中国不存在 DST，语义可接受；可后续统一为 IANA location，但不属于缺陷 |
| 普通页面分页不带 `as_of` | 与“导出使用 `as_of` 固定用量上界”的设计一致；普通页面允许实时数据轻微变化，不处理 |
| 冗余组织显示兼容 | 无行为风险，可在后续清理，不纳入本轮修复 |
| `go.sum` 变更 | `github.com/google/subcommands` 是 `github.com/google/wire v0.7.0` 的传递依赖，属于 Wire 生成链路所需校验项，保留 |

### 12.5 确认的后续处理范围

1. 修复筛选加载期间的陈旧数据显示，并补前端回归测试。
2. 修复邮箱字面搜索转义，并补 Repository 测试。
3. 修正设计文档及 `llm-wiki/wiki/backend.md`、`frontend.md`、`data-and-domain.md` 中的 `as_of` 签名表述。
4. 基于真实 PostgreSQL 数据执行 30、90、366 天三档查询分析，再确定聚合和导出优化方案。
5. 补组织用量 Repository 的 PostgreSQL integration 测试。

### 12.6 后续处理结果

- 筛选加载期间已隐藏上一响应的 Overview 和组织汇总，人员表继续显示 loading skeleton；新增前端回归测试。
- 邮箱搜索已转义反斜杠、`%`、`_`，两个 `ILIKE` 入口使用显式 PostgreSQL `ESCAPE`；新增单元和真实 PostgreSQL 字面搜索测试。
- 设计文档和 `llm-wiki` 已统一为 canonical `as_of` 时间戳口径，明确不具备密码学签名。
- 新增真实 PostgreSQL integration，覆盖域名/状态/零用量、北京时间边界、partial 周月、聚合和 Champion 并列规则。测试首次发现 Champion 窗口排序的 `user_id` 歧义，已限定为 `pa.user_id` 并验证通过。
- PostgreSQL 18.1 的 600 用户、219,600 logs 合成基线已完成。90 天 Summary items 的病态计划可重复达到约 11 秒，根因是三个 peak CTE 各对 `ranked_periods` 循环扫描 600 次；显式物化 peak 的诊断候选约 418 ms。完整结果见 `docs/features/organization-usage-report-performance-cn.md`。
- SQL 重构决定：第一阶段收敛 peak CTE 连接形状，第二阶段减少导出分页重复查询；当前轮次只完成证据和方案，不把诊断候选直接并入生产 SQL。

验证结果：Repository/Service 单元测试通过；组织用量 PostgreSQL integration 通过；前端 Vitest 161 个文件、1025 个测试通过；typecheck、lint 和 `git diff --check` 通过。
