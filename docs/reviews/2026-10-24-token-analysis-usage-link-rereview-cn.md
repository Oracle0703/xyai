# Token Analysis 计费关联修复：第二次独立复核

日期：2026-10-24。审核对象为 `feature/hy/10218_organization_usage_rollup`、HEAD `bc45a7e03298d0a1643452b8cf12629edefa8eb3` 上 Claude 最新未提交修改。源码 SHA-256 见交付目录 `usage-link-rereview-source-hashes.json`。本轮仅审核和本机隔离验证，未连接生产、未部署、未改业务代码。

> 最新审核：本文 6 个原始场景已在第三次独立复核中通过；仍有 pending 生命周期的两类遗漏、3 个新失败场景。见 `docs/reviews/2026-10-24-token-analysis-usage-link-third-review-cn.md`。本文保留第二次审核与当时修复记录。

## 结论

上次主要复现已得到针对性修复：精确账单保护/夺回、歧义不猜测、同账单原子认领、逆序跨日响应暂存，以及旧 `method=''` 失败行清理，均有实际通过的集成用例。

**仍有 4 类遗漏、6 个独立失败场景，不建议将当前实现标记为完整交付。** 本轮补充测试为 5 个顶层测试，其中失败响应测试分无关联键/新增 response.id 两个子用例；这 6 个场景全部在独立 PostgreSQL 18.3 中复现。影响分析页关联、统计与风险显示，没有生产已发生或实际重复扣费的证据。

| 编号 | 优先级 | 遗漏 | 实际影响 |
| --- | --- | --- | --- |
| R1 | P2 | 已有 timing 关联未随较晚到达的失败响应解除 | 错误账单继续显示 linked、参与用量统计 |
| R2 | P2 | 同一请求的陈旧写入可覆盖精确关联 | 多索引进程交错时，精确账单被换成邻居账单，或被清空 |
| R3 | P2 | 单日升序索引跨日请求未补关联请求日 | 完成事实/response.id 已保存，但历史请求仍未关联 |
| R4 | P2 | 夺回范围外账单后未重算旧行风险 | 账单已移除，原行仍保留由该账单产生的 70 分风险 |

## R1：失败响应晚到时，timing 关联仍被保留

位置：`backend/internal/repository/token_analysis_repo.go:193-194`，`backend/internal/service/token_analysis_usage_link.go:118-120`。

复现 `TestRereviewTimingClaimRechecksLateFailedResponse`：历史请求无关联键，尚未补回 response。首次库内关联将附近 `client:unrelated` 账单关联为 timing；之后读取该请求的 502 response，再运行索引，错误关联仍保留。

- 没有新增关联键：候选 SQL 直接跳过无键的 timing 行。
- response 同时补回 `resp_failed_owner`：虽然进入候选并只查精确账单，但精确匹配不存在时，`match == nil && weakLink` 提前返回，仍保留旧账单。

最终输出中两种形态分别保留账单 26、27，均为该用例故意放入的无关账单。现有 `TestTokenAnalysisUsageLinkFailedRequestsKeepOnlyExactBills` 只覆盖 `usage_match_method=''` 的旧窗口错链，不覆盖已经经过新规则变为 timing 后才补齐 response 的状态。DB-only 恢复之后再做 JSONL 完整恢复即可到达此路径，不需要多实例。

修复要求：响应状态变化需要触发弱关联复核；没有精确计费证据的失败行解除弱关联并重算风险。保留有精确账单的失败请求，不能按失败状态一刀切清空真实用量。仅修改候选 SQL 不够，服务层提前返回也需修正。

## R2：账单锁未防住同一摘要的陈旧结果覆盖

位置：`backend/internal/repository/token_analysis_repo.go:415-418`、`:428-432`、`:440-445`、`:474`。

本轮原子认领已经阻止两个请求并发绑定同一账单，但没有保护同一请求被不同查询结果覆盖。两个复现都用真实仓储方法顺序执行合法多进程交错，不靠 sleep：

1. `TestRereviewStaleTimingCannotOverwriteExactClaim`：A 先查询到邻居账单 28（timing），保留结果暂不写。自身账单 29 到达后，B 查询并精确绑定 29（已断言 request_id）。A 最后提交旧结果，写入仍返回 Applied=true，实际关联回退到 28。
2. `TestRereviewStaleReleaseCannotClearExactClaim`：A 从已有旧窗口错链查询到 nil。B 随后按新到精确账单修好同一请求；A 提交旧 nil，精确关联被直接清空。普通未关联行的 nil 查询不会调用写入，本用例明确以已有旧错链构造可达路径。

原因：正向写入按目标账单 ID 加锁，两个不同账单不会互斥；占用检查排除本 archive；最终 UPDATE 仅按 archive_id，没有核对查询时旧值。nil 清空分支更是无锁直接更新。

修复要求：写入时核对目标摘要的当前关联，使用旧 usage ID/method 或版本的条件更新，在冲突时重新查询；必须阻止陈旧 timing/clear 覆盖较新的精确关联。只新增摘要锁而不核对陈旧结果仍不充分。下一次索引有机会恢复，但不能将这一点作为并发正确性的替代。

此问题要求多个服务/索引实例交错，不能写成当前单实例生产已经发生，也不是重复扣费。

## R3：跨日历史按单日升序索引，响应补齐但请求不参与关联

位置：`backend/internal/service/token_analysis_indexer.go:60`，`backend/internal/repository/token_analysis_repo.go:191`。

`TestRereviewAscendingSingleDayIndexLinksCrossDayRequest` 使用真实临时 JSONL：D 日 23:59:59 request 带 client_request_id，D+1 日 00:00:20 response 带 response.id，账单采用该 response.id。

依次单独索引 D、D+1，再索引一次 D+1：第一轮缺响应事实，无法精确找到 WS 账单；第二轮补回完成时间和 response.id，但候选仅包含 D+1 开始的请求，D 日行被排除。测试先确认 response.id 已保存，再断言关联，结果仍为 NULL。

逆序暂存逻辑已修好，这里是另一个方向。一次选中两日完整范围正常；自动昨天/今天的共同扫描通常也可覆盖。问题限定为历史日期分次单日恢复，历史日不在自动范围内时，必须额外重新索引请求日才能恢复。

修复要求：将本轮更新完成事实的 archive_id 加入补关联集合，不应只根据本轮文件日期筛选请求。至少回归升序、逆序、单次全范围三种方式，不要只验证逆序。

## R4：跨日期夺回弱账单后，旧风险仍保留

位置：`backend/internal/repository/token_analysis_repo.go:467-470`、`:191`；重跑范围见 `backend/internal/service/token_analysis_usage_link.go:45-79`。

`TestRereviewDisplacedPreviousDayRiskIsRecomputed`：先只索引 D 日，在库内把 D 日午夜前的旧请求按 timing 关联到次日账单。该账单 input=100000、output=80、cache_read=0，真实风险计算产生 70 分，测试先确认该分数。再插入 D+1 日真实 owner 及 response.id，只索引 D+1。

精确夺回成功，旧行 usage_log_id 变 NULL、method 变 released；但 risk_score 仍为 70，risk_reasons 仍包含 `huge_input_tiny_output` 45 分与 `low_cache_hit_large_input` 25 分。两条理由完全依赖已被移走的账单。

原因：事务只清账单字段；服务尝试重跑时仍限定 D+1，D 日 released 行不会被处理。在未再次索引 D 日前，它继续污染风险筛选、风险请求数和风险原因分布。

修复要求：让被夺回行的风险重算独立于请求日期范围，例如返回 displaced archive IDs 并显式处理。保留 body 自身产生的风险，不应无条件把所有风险清零。

## 验证与复跑

| 检查 | 结果 |
| --- | --- |
| Go service / repository / handler/admin / server/middleware 的 TokenAnalysis、RequestArchive 专项 | 4 包通过，默认 vet 随 go test 执行 |
| PostgreSQL 关联、来源筛选、输入质量专项 | 12 个顶层测试全部通过，没有 Docker/DSN 跳过 |
| 前端页面/API/locale/编译专项 | 4 文件、49 测试通过 |
| 前端 typecheck、相关 ESLint | 通过 |
| 二次独立边界复现 | 5 个顶层测试、6 个场景全部失败，分别覆盖 R1 两种、R2 两种、R3、R4 |
| git diff --check | 通过 |

前端测试有既有 Browserslist 过期提示及测试 stub 的 Vue emits 警告，不影响测试结果。本轮未用生产浏览器验证未部署代码。

交付目录 `docs/delivery/2026-10-24-token-origin-human-input/`：

- `usage-link-rereview-repros.go.txt`：本轮全部复现源码，复用当前 integration fixture。
- `usage-link-rereview-results.txt`：本轮 6 个场景的实际失败输出。
- `usage-link-rereview-integration-results.txt`：当前 12 项正式数据库测试通过输出。
- `usage-link-rereview-source-hashes.json`：审核源码基线。

复跑：将 .go.txt 临时复制为 `backend/internal/repository/token_analysis_rereview_temp_test.go`，按 wiki 设置 Go 缓存和新的 GOTMPDIR，然后在 backend 执行：

```text
go test -tags integration -p 1 -count=1 ./internal/repository -run '^TestRereview' -v
```

`SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 必须指向独立测试数据库。测试 harness 自动迁移并写入测试数据，不得指向生产或未知数据库。本轮新建集群只监听 127.0.0.1:55485，使用新空集群的 postgres 数据库；测试完成已停止。临时测试 .go 已删除，仅保留交付附件。原六个复现保留为首轮历史证据，没有改写为本轮结果。

## 处理结果（2026-10-24，Claude，本地修复，未提交、未部署）

六个场景在新建 PostgreSQL 18 空库上按附带复现原样确认失败（源码哈希与本复核基线一致），四类遗漏均已采纳。

| 发现 | 处理 |
| --- | --- |
| R1 晚到的失败响应未解除时序关联 | 写回/合并完成事实时，若改变了未精确关联行的状态、完成时间或 response.id，置 `usage_link_pending`；带标记的时序关联行不再视为“已确认”，按新事实完整复核：失败且无精确账单则解除并重算风险，有精确账单仍关联。候选 SQL 与服务层提前返回同时修正 |
| R2 同一请求的陈旧写入覆盖较新关联 | `SetUsageLink` 增加 `expected`（查找前读到的 `usage_log_id`/`usage_match_method`），关联与解除都按它条件更新；状态已变时返回未生效、整个认领事务回滚，留待下一轮重新查找 |
| R3 升序分日索引跨日请求未关联 | 同 R1 的 `usage_link_pending`：只索引 D+1 时写回 D 日行的完成事实即置标记，关联轮次不受日期范围限制地处理带标记行。回归覆盖逆序、升序分日、整段三种方式 |
| R4 跨日夺回账单后旧行风险未重算 | 夺回时原占用行置 `released` 并置 `usage_link_pending`，同一次索引的下一轮按标记处理、解除并重算风险；body 自身风险保留（用例验证 90→20，只余 `oversized_system_prompt`） |

迁移改动：247 增加 `usage_link_pending` 列；248 增加 `idx_token_analysis_usage_link_pending` 部分索引（CONCURRENTLY）。

验证：

- 本复核六个场景：R1 两种、R3、R4 原样通过；R2 两个复现直接调用 `SetUsageLink`，新签名需要传入各自查找前读到的状态（A、B 都读到未关联，或都读到旧错链），补上该参数后通过，陈旧写入返回 `Applied=false`、精确关联保留。已改写为永久回归 `TestTokenAnalysisUsageLinkLateFailedResponseClearsTimingLink`、`...StaleWritesCannotOverrideNewerLink`、`...CrossDayRequestInAnyIndexOrder`、`...DisplacedRowOnOtherDayIsRescored`。
- 关联集成测试共 14 个顶层用例在新空库通过；变异检查分别去掉条件更新、完成事实置标记、夺回置标记、跨日期处理带标记行，各自对应用例失败。

