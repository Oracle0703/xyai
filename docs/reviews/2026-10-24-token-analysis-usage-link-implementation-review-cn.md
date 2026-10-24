# Token Analysis 计费关联修复：独立实现审核

日期：2026-10-24。对象：`feature/hy/10218_organization_usage_rollup`、HEAD `bc45a7e03` 上的未提交实现（含 migration 247）。本轮三个 subagent 分别只读核对链路、前端和存量恢复；主审核在独立本机 PostgreSQL 18 空集群运行真实集成测试。未连接生产，未改业务实现，未提交。

> 最新审核状态：Claude 修复后已完成第二次独立复核，仍有四类遗漏、六个失败场景。见 `docs/reviews/2026-10-24-token-analysis-usage-link-rereview-cn.md`。下文保留首轮证据与 Claude 当时的修复说明。

## 结论

请求明细不再因未关联而隐藏、未关联用量显示未知、合法零值显示、客户端包装清理、新请求关联键、库内补关联和存量恢复文档均已实现。原始 18.5 秒请求的既有集成回归通过。

**关联正确性仍有 5 类缺口，共 6 个实际失败复现，当前不能判定完整交付。** 问题影响分析页的账单归属、用量累计和历史恢复；本轮没有实际扣费重复的证据。多实例认领问题具有多进程条件，不能表述为当前单实例生产已发生。

## F1 / P2：弱时序关联占用账单后，精确归属无法纠正

位置：`backend/internal/service/token_analysis_usage_link.go:92`、`backend/internal/repository/token_analysis_repo.go:154`、`:229-236`、`:249-299`。

两种真实数据库复现均失败：

1. **精确账单晚到。** A 有 client_request_id，开始 t0、完成 t18；邻居 WS B 开始 t1、完成 t18.1，其账单 `resp_review_neighbor` 已落库。A 的 `client:A` 尚未落库时，A 会先按时序认领 B 的账单。后来 A 的精确账单到达，重复索引仍保留错误账单：最终复现日志中期待 ID 18，实际仍为邻居 ID 17。
2. **旧行先抢已知 owner 的账单。** 旧 A 无关联键，开始 t0、完成 t10；新 B 有关联键 b，开始 t1、完成 t5，唯一账单是 `client:b`。按 id 先处理 A，它按 timing 认领 B 的账单；B 的精确查询被占用检查挡住，重复索引后仍未关联。

根因：所谓“先精确再兜底”只在单条请求内部成立。全局上旧行可以先占用另一条请求的精确账单；`usage_match_method='timing'` 又被候选条件永久排除，后续证据无法纠正。

普通 HTTP 混合上述账单命名空间是源码支持的情形：`openai_gateway_forward.go` 可转 `forwardOpenAIWSV2`，该结果有 `OpenAIWSMode=true` 和上游 response ID；`openai_gateway_usage.go:342-345` 随之改用 resp_* 计费键。现有 LateBilling 用例只放了 client: 邻居，未覆盖这一分支。

建议保护具有明确 owner 的精确账单，并允许新精确证据原子纠正旧弱关联。不能把任意 timing 结果当作永久确认。

## F2 / P2：异步计费延迟仍可把旧顺序请求的账单串到下一条

位置：`backend/internal/repository/token_analysis_repo.go:245`、`:255-267`。

复现：存量 A 在 t0 开始、真实转发耗时 1 秒；下一请求 B 在 t2.1 开始。A 的计费任务延迟到 t4 执行，账单 duration=1000ms。代码用 created_at-duration 推算出 t3，把 B 当成更晚的 owner：A 被竞争条件排除，B 认领 A 的账单并记为 timing。最终测试日志观察到 B 关联账单 ID 20，正是 A 的账单。

这是相隔 2.1 秒的顺序请求，不属于文档列出的“亚秒并发”例外。`CreatedAt` 在用量 worker 执行时取 time.Now，减去转发耗时不能消掉队列等待。该估算不能作为确认归属的硬事实；缺少 ID/完成信息的旧记录应保留歧义，或补采已有 response 事实后再判。

## F3 / P2：多实例对同一账单的认领不是原子操作

位置：`backend/internal/repository/token_analysis_repo.go:207-209`、`:342-345`；migration 136 的 usage_log_id 索引非唯一，migration 247 没有新增认领约束。

复现模拟两个索引实例的合法交错：两条无关联键、均有完成事实的旧请求相隔 50ms 开始，两个 FindUsageLink 都在写入前读到同一未占用账单，随后并发执行 SetUsageLink；无需制造重复 client_request_id。两次更新均成功，两个摘要永久关联同一 usage_log_id。再次索引不会清理，因为两条 method 已非空。重复账单检查实际返回 1 组重复，预期 0。

查询时的 NOT EXISTS 无法约束后续 UPDATE；服务内 mutex 只保护一个 service 实例。当前“ConcurrentAndRetriedRequests”用例覆盖的是请求时间重叠，实际关联仍串行，不能证明多索引实例安全。

需要数据库层原子认领/锁/约束及冲突处理。存量已有重复时，新增唯一约束前必须明确清理方式，不可直接假设迁移一定成功。

## F4 / P2：按历史日期逆序索引会丢失跨日请求的完成事实

位置：`backend/internal/repository/token_analysis_repo.go:132-135`；`backend/internal/service/token_analysis_indexer.go:257`；`token_analysis_usage_link.go:92`。

复现使用真实临时 JSONL 文件：D 日 23:59:59 的 request、D+1 日 00:00:20 的 response。先索引 D+1，再索引 D，然后两日分别再运行一次增量索引。第一次 RecordResponse 仅 UPDATE，摘要还不存在时影响 0 行却正常返回；response 水位已经前移。后来插入的 request 永远没有 completed_at。

该请求带 client_request_id，而真实账单采用 WS resp_* 键，因 completed_at 为空始终 ExactOnly，库内补关联无法恢复；实际测试四次索引后仍未关联。正常自动两天升序扫描不触发，但文档提供的按历史日期恢复入口允许该顺序。

应持久保留暂时无 request 的完成事实，或提供可靠的补合并路径，避免要求用户发现后再次回退大文件水位。

## F5 / P2：旧失败请求的错误关联被永久跳过

位置：`backend/internal/repository/token_analysis_repo.go:154-156`、`:134`；`backend/internal/service/token_analysis_usage_link.go:139` 附近的状态推导。

复现：旧摘要已有上一版错误写入的 usage_log_id；升级后继续读取其 502 response，再运行关联轮次。候选 SQL 直接排除非 2xx 行，旧 usage_log_id 没有机会被复核或解除；页面状态又优先按 usage_log_id 返回 linked，错误用量继续参与聚合。测试期望旧错误链接清空，实际仍非空。

禁止为新失败请求随意匹配账单，与清理旧失败请求的错误关联是两件事。应先复核/解除旧弱关联再决定是否可参与新匹配；对确有部分用量的失败请求仍需按真实计费证据处理，不能无条件认为所有失败请求都没有费用。

## 已完成项与验证

| 项目 | 本轮结果 |
| --- | --- |
| 两种勾选状态下完整请求明细 | service 强制 IncludeUnmatched=true；真实 PG 既有回归通过 |
| 未关联显示未知、linked 合法零、旧接口兼容 | 前端实现与专项测试通过 |
| 浏览器上下文包装清理 | 开头完整包装与真实正文分离；引用、未闭合、相似标签保护用例通过 |
| 原始 18.5 秒样例 | 既有集成用例通过 |
| Go 专项 | service / repository / handler/admin / server/middleware 的 TokenAnalysis、RequestArchive 测试通过，默认 vet 随 go test 执行 |
| PostgreSQL 18 既有专项 | 6 个集成测试实际 PASS，无 Docker 跳过 |
| 前端专项 | 4 文件 / 49 测试通过；typecheck、相关 ESLint 通过 |
| 新增独立边界复现 | 6/6 失败，分别覆盖 F1 两种、F2、F3、F4、F5 |
| 工作区检查 | git diff --check 通过；临时 Go 测试已删除，测试集群已停止 |

主要测试命令：

```text
go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/middleware -run 'TokenAnalysis|RequestArchive'
go test -tags integration -p 1 -count=1 ./internal/repository -run 'TokenAnalysisUsageLink|TokenAnalysisListRequestsFiltersByOrigin|TokenAnalysisUserInputQuality' -v
go test -tags integration -p 1 -count=1 ./internal/repository -run '^TestReview' -v
```

集成测试仅使用本轮创建的本机空集群，`SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 指向 127.0.0.1。harness 会执行迁移，不可将其指向生产或未知数据库。

## 交给 Claude 的复现材料

- `docs/delivery/2026-10-24-token-origin-human-input/usage-link-review-repros.go.txt`：六个最小集成复现，复用当前 `token_analysis_usage_link_integration_test.go` 的 fixture。临时复制为 `backend/internal/repository/token_analysis_review_temp_test.go` 后运行 `-run '^TestReview'`，测试结束删除临时文件。
- `docs/delivery/2026-10-24-token-origin-human-input/usage-link-review-results.txt`：本轮真实失败输出，包含预期/实际账单 ID 与未关联/重复断言。

本轮只新增复核文档、证据附件并同步 wiki 的待修状态；未替 Claude 修改实现。没有执行生产 SQL、回填、部署或服务重启。

## 处理结果（2026-10-24，Claude，本地修复，未提交、未部署）

五类缺口在新建 PostgreSQL 18 空库上用附带复现逐一确认属实，均已采纳。

| 发现 | 结论 | 处理 |
| --- | --- | --- |
| F1 弱关联抢占精确账单且无法纠正 | 属实，两种复现均失败 | 归档 response 行记录上游 `response.id`，与 `client:` 一起作为精确键（WS 上游以 response.id 计费）；时序兜底排除他行精确账单；精确证据可夺回仅被时序/旧窗口占用的账单，原行置 `released` 后重关联或解除并重算风险；有关联键的时序关联行保留为候选，出现精确证据即纠正 |
| F2 计费排队把旧顺序请求账单串到下一条 | 属实 | 推算转发开始只作下界；时序兜底增加归属唯一约束：另有未确认请求也能解释同一账单时不关联。只有开始时间的“失败后重试”与“计费排队”无法区分，两者都保持未关联；原先“归重试”的用例改为歧义不关联 |
| F3 多实例认领非原子 | 属实（需多实例条件） | 认领在事务内按账单取 `pg_advisory_xact_lock`，锁内复核占用后写入；未加唯一约束，存量重复由关联轮次复核消化 |
| F4 逆序跨日回填丢失完成事实 | 属实 | 请求行未入库时暂存到 `token_analysis_pending_responses`，每轮关联前合并，7 天未合并的暂存清理；不需要回退水位 |
| F5 旧失败行错误关联被永久跳过 | 属实 | 失败行进入候选但只认精确证据：没有精确账单时解除旧关联并重算风险，有精确账单（真实计费证据）时仍关联 |

验证（新建空库 `review_fix_20261024b`，本机嵌入式 PostgreSQL 18）：

- 附带六个复现：F1 两个、F2、F4、F5 共五个原样通过。F3 复现第一步断言 `FindUsageLink` 能为两条相隔 50 ms、都已完成的存量请求找到同一账单；新的归属唯一约束把这笔账单判为歧义，两条都不返回，断言在进入并发写之前失败。改用持行锁制造“检查后、提交前”交错的用例直接验证认领原子性。
- 永久回归已并入 `token_analysis_usage_link_integration_test.go`（10 个用例），覆盖以上全部场景。
- 变异检查：去掉归属唯一约束、他行精确账单保护、账单咨询锁、精确证据夺回弱关联，各自对应用例失败。
- 服务、仓储、handler、中间件单测与全量门禁结果见交付说明；设计文档“关联流程”“已知限制”“验证”已按修复更新。

