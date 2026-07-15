# 组织用量报表 PostgreSQL 性能基线

## 结论

组织用量报表需要进入 SQL 重构，不建议仅提高前端超时或增加索引。

真实 PostgreSQL 18.1 测试中，90 天 `summary-items` 查询稳定出现约 11 秒的病态执行计划。根因是 `ranked_periods` CTE 在三个 peak LEFT JOIN 下被 PostgreSQL 规划为嵌套循环，每个 peak 对 600 名用户重复扫描 600 次，并产生约 138 万个临时块读取。`usage_logs` 索引扫描本身只占约 36 ms，不是主要瓶颈。

将 day/week/month peak CTE 显式物化的诊断候选把同一 90 天查询从 11,032 ms 降至 418 ms，约改善 26 倍。因此第一阶段应优先固定 peak 结果的物化和连接形状，再处理导出分页重复查询。

## 测试环境

| 项目 | 值 |
| --- | --- |
| 测试日期 | 2026-07-13 |
| PostgreSQL | 18.1，Windows amd64 可携带运行时 |
| 运行时校验 | Maven Central `embedded-postgres-binaries-windows-amd64-18.1.0.jar`，SHA-256 `0a7e951660cbbee3f382f8b44b2c3ae26c4a17817c3a972dee3378c80fc9b23b` |
| `work_mem` | 4 MB |
| `shared_buffers` | 128 MB |
| `effective_cache_size` | 4 GB |
| `max_parallel_workers_per_gather` | 2 |
| Schema | 仓库 `backend/migrations/` 全量迁移 |
| 活跃用户 | 600，平均分布到 `xunyou.com`、`wsdashi.com`、其他 |
| `usage_logs` | 219,600；每名用户连续 366 天每天 1 条 |
| 查询页大小 | 500 |
| 缓存口径 | 每个查询先执行一次预热，记录第二次 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` |

该数据集是可重复的合成基线，不代表生产真实模型分布、并发、硬件或缓存命中率。它用于识别查询形状问题；生产上线前仍需在脱敏副本上复核。

## 实测结果

下表单位为毫秒；`temp r/w` 是顶层计划累计临时块读写数量。

| 查询 | 30 天执行 | 30 天 temp r/w | 90 天执行 | 90 天 temp r/w | 366 天执行 | 366 天 temp r/w |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Summary 组织汇总 | 12.187 | 0 / 0 | 61.572 | 0 / 0 | 91.554 | 0 / 0 |
| Summary 人员 count | 0.186 | 0 / 0 | 0.192 | 0 / 0 | 0.184 | 0 / 0 |
| Summary 人员 items | 117.009 | 149 / 309 | 11,032.002 | 1,386,172 / 4,045 | 1,454.228 | 18,864 / 12,218 |
| Summary Champion | 129.931 | 149 / 309 | 370.078 | 2,814 / 3,286 | 1,357.780 | 13,012 / 11,056 |
| Periods day count | 14.731 | 0 / 0 | 42.296 | 0 / 0 | 174.127 | 378 / 731 |
| Periods day data | 66.737 | 88 / 157 | 141.391 | 1,440 / 1,118 | 702.404 | 5,668 / 3,049 |
| Periods week count | 16.628 | 0 / 0 | 48.870 | 0 / 0 | 179.607 | 0 / 0 |
| Periods week data | 30.585 | 0 / 0 | 88.990 | 312 / 313 | 186.236 | 458 / 444 |
| Periods month count | 15.928 | 0 / 0 | 45.474 | 0 / 0 | 157.617 | 0 / 0 |
| Periods month data | 25.269 | 0 / 0 | 75.292 | 312 / 313 | 170.929 | 0 / 0 |

90 天 `summary-items` 已重复运行，执行时间分别为 11,367.674 ms 和 11,032.002 ms，异常可复现。366 天反而更快，说明该问题来自基数估算和连接计划切换，不是单纯按日期范围线性增长。

## 慢计划根因

90 天窗口包含 54,000 条用量记录，period 聚合产生 64,200 行。关键慢节点如下：

| 节点 | 行/循环 | 累计时间 | 说明 |
| --- | ---: | ---: | --- |
| `usage_logs` Index Scan | 54,000 / 1 | 36 ms | 时间索引工作正常 |
| `ranked_periods` WindowAgg | 64,200 / 1 | 278 ms | 聚合和排序本身可控 |
| 顶层 Left Nested Loop | 600 / 1 | 11,024 ms | 主要瓶颈 |
| day peak `ranked_periods` CTE Scan | 600 / 600 | 4,027 ms | 每名用户重复扫描 |
| week peak `ranked_periods` CTE Scan | 600 / 600 | 3,457 ms | 每名用户重复扫描 |
| month peak `ranked_periods` CTE Scan | 600 / 600 | 3,427 ms | 每名用户重复扫描 |

因此新增 `usage_logs` 索引不能解决该问题。单纯提高 `work_mem` 可能减少 external merge 和临时文件，但不会消除 `用户数 × ranked_periods` 的重复扫描复杂度。

## 候选方案验证

诊断查询只做以下变化，未修改生产 SQL：

```sql
day_peak AS MATERIALIZED (...),
week_peak AS MATERIALIZED (...),
month_peak AS MATERIALIZED (...)
```

| 90 天 Summary items | 执行时间 | temp read | temp written |
| --- | ---: | ---: | ---: |
| 当前 SQL | 11,032.002 ms | 1,386,172 | 4,045 |
| 物化 peak 候选 | 417.776 ms | 4,826 | 3,978 |

候选方案证明先生成每名用户每个粒度最多一行的 peak 结果，可以避免嵌套循环重复扫描。它仍存在 4 MB `work_mem` 下的临时排序，后续需在结构修复后再判断是否值得做更深聚合改写。

## 重构决定

### 第一阶段：修复 Summary peak 连接形状

1. 优先验证并落地 day/week/month peak CTE 显式物化，或把 `ranked_periods WHERE rn=1` 一次聚合为每用户一行后只 JOIN 一次。
2. 保持峰值并列规则 `total_tokens DESC, actual_cost DESC, requests DESC, user_id ASC, bucket_start ASC` 不变。
3. 补 SQL 合同测试和 PostgreSQL integration，确认日/周/月 peak、零用量用户和稳定排序无回归。
4. 重跑本文件的 30/90/366 天基线，验收标准是消除 peak CTE 的 600 次循环和百万级 temp read。

### 第二阶段：减少导出分页重复查询

1. Summary 首页面返回 overview、organizations、champions、total 和首批 items；后续导出分页只查询人员 items。
2. Summary 和 Periods 已在数据查询中使用 `COUNT(*) OVER()`，评估取消每页独立 count；空页总数通过首响应快照或单次兜底查询处理。
3. day/week/month 保持串行，避免同时触发三组大聚合；完成 SQL 优化后再评估导出专用超时和错误分类。

### 暂不采用

- 不新增数据库索引或 migration：当前证据显示索引扫描不是瓶颈。
- 不只提高 Axios 30 秒超时：这会延长失败等待并放大数据库压力。
- 不直接并发 day/week/month：当前查询仍会产生临时排序和聚合压力。
- 不直接建设服务端流式导出：先完成可验证的最小 SQL 修复，避免提前扩大架构范围。

## 复现入口

测试文件：`backend/internal/repository/organization_usage_explain_integration_test.go`。

```powershell
$env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN = "<temporary PostgreSQL DSN>"
$env:ORGANIZATION_USAGE_RUN_EXPLAIN = "1"
go test -tags=integration -p 1 -count=1 ./internal/repository -run '^TestOrganizationUsageRepositoryExplainAnalyze$' -v
```

可用 `ORGANIZATION_USAGE_EXPLAIN_DAYS=30|90|366` 和 `ORGANIZATION_USAGE_EXPLAIN_QUERY=<query-name>` 限定诊断范围。DSN 和数据库凭据不得写入仓库。
