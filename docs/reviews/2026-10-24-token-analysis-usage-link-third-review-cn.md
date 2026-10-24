# Token Analysis 计费关联修复：第三次独立复核

日期：2026-10-24。审核对象：`feature/hy/10218_organization_usage_rollup`、HEAD `bc45a7e03298d0a1643452b8cf12629edefa8eb3` 上的最新未提交实现。源码快照见交付目录 `usage-link-review3-source-hashes.json`。本轮未连接生产、未部署、未修改业务代码。

> 最新状态（2026-10-25）：本文两类问题、3 个原始复现已在第四次独立复核中全部通过，当前未发现新的实质遗漏。见 `docs/reviews/2026-10-25-token-analysis-usage-link-fourth-review-cn.md`。本文保留第三次审核与当时修复记录。

## 结论

**第二次复核的 4 类问题、6 个原始场景全部通过，确认相应修复有效。** 上轮两个直接调用 SetUsageLink 的并发复现仅适配新增 expected 参数，原触发顺序和预期结果保留。原失败时序链已解除；陈旧关联写返回 Applied=false；跨日关联成功；跨范围账单夺回后旧风险由 70 降为 0。永久回归还验证了 body 自身风险的保留。

新增的 `usage_link_pending` 机制仍有 **2 类遗漏、3 个真实数据库失败场景**，建议继续补修后再交付。两项均为 P2，影响分析页数据正确性；本轮没有生产已发生或实际重复扣费的证据。

| 编号 | 遗漏 | 实测结果 |
| --- | --- | --- |
| T1 | 跨日请求尚未找到账单，就清除跨日期重试标记 | response 已索引、账单晚到后继续索引响应日及当前日期，历史请求仍未关联 |
| T2 | 旧处理结果可以清掉新的复核标记 | 较晚到达的 502 响应被旧 timing 写入覆盖；或精确夺回后的新标记被旧 Clear 清掉，旧风险 70 分残留 |

## T1：未找到晚到账单也清标记，跨日期重试丢失

位置：`backend/internal/service/token_analysis_usage_link.go:121-125`；日期候选过滤位于 `backend/internal/repository/token_analysis_repo.go:204`。

复现 `TestReview3CrossDayLateBillRemainsRetryable` 使用真实临时 JSONL：

1. D 日 23:59:59 的 request 带 client_request_id，D+1 日 00:00:20 的 response 带 response.id，先分两次索引 D 和 D+1。
2. 此时账单尚未落库。D 日请求被 pending 标记带入跨日期关联，但 FindUsageLink 返回 nil，服务把 pending 清为 false。实际日志为 `response indexed before bill arrives: pending=false`。
3. 随后插入完全匹配 response.id 的账单，再索引 D+1，随后索引当前日期。请求仍没有 usage_log_id，精确关联断言失败。

原因：`match == nil && pending` 分支把“本次还没有账单”当成“已经完成复核”，删除了不受日期限制的唯一触发标记；后续请求日期不在扫描范围，便没有机会再查。

范围：这是分日恢复/补关联与异步晚到账单组合的边界。自动扫描昨天和今天时，如果请求日仍在该范围内，可以通过日期候选恢复；一次选择整个跨日范围也有机会恢复。不能把此结论表述成所有跨日请求都会失败。

修复要求：仍等待计费证据的成功请求，应保留可重试状态，或放入持久重试队列；只有已有确定结论时才确认完成。若采用重试期限/退避，需明确终止合同，不能默默因一次 nil 查询丢失任务。补回归：响应先到、账单后到，并在请求日之外运行后续索引。

## T2：关联状态的条件更新没有保护完成事实和复核版本

位置：`backend/internal/repository/token_analysis_repo.go:430-434`、`:511-514`；服务调用位于 `backend/internal/service/token_analysis_usage_link.go:125`、`:145`。

当前 expected 只包含 `usage_log_id` 与 `usage_match_method`，足以挡住上轮 R2 的“另一进程已经改了关联”场景；但完成事实变化时这两个字段可以完全不变。布尔 pending 也不能区分旧任务与新任务。

### T2a：首次失败响应夹在查找与认领之间

`TestReview3NewResponseInvalidatesStaleTimingWrite`：

1. 索引实例 A 读取无关联键、尚无完成事实的历史请求，FindUsageLink 得到邻近账单的 timing 结果，暂不提交。
2. 实例 B 索引到该请求的第一次 502 response，将 pending 置 true。此时 usage ID 仍为空、method 仍为空。
3. A 使用旧 expected 和旧 timing 结果写入，条件更新仍成功，顺带把 pending 清为 false。日志为 `stale write after 502: applied=true pending=false bill=37`。
4. 再正常索引两次，错误关联仍保留。无键 timing 且 pending=false 的请求被候选条件跳过。

这是首次响应补齐的合法交错，不需要伪造多条互相矛盾的响应，也不依赖 sleep。条件是多个索引服务实例并行。

### T2b：旧 Clear 吞掉精确夺回产生的新任务

`TestReview3StaleClearCannotAcknowledgeNewDisplacement`：

1. D 日旧请求先按 timing 绑定账单，并由账单产生 70 分风险。
2. 第一次 response 到达，标记 pending。A 读取后重新查询，结果仍是同一账单，准备走“无需改动 → 清标记”的分支。
3. B 此后发现 D+1 日的精确 owner，成功夺回账单，把旧请求置为 `released/pending=true`。
4. A 调用旧的 ClearUsageLinkPending。该方法仅按 archive_id 清标记，没有核对读到的关联或复核版本，把 B 的新任务一并清掉。
5. 再索引 D+1，D 日行仍为 `released/pending=false/risk=70`。账单已经清掉，但范围外旧行没有机会重算风险，预期 0 的断言失败。

修复要求：将完成事实变化、账单释放等事件纳入版本化的复核状态。候选必须携带读取版本，Set 和 Clear 均只允许确认该版本；版本变化时拒绝陈旧结果，并保留新任务。也可比较完整相关事实/关联快照实现同等语义。仅比较 pending 布尔值不能覆盖 true→true 的新任务，且只修 Set 而保留无条件 Clear 仍会遗漏 T2b。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| 第二次复核的原始复现 | 5 个顶层测试、6 个场景全部通过；只适配新增 expected 参数 |
| 当前 PostgreSQL 正式专项 | 15 个顶层测试通过（14 个关联/来源用例及 1 个输入质量用例），无数据库跳过 |
| Go TokenAnalysis / RequestArchive 专项 | service、repository、handler/admin、server/middleware 四包通过；默认 vet 随 go test 执行 |
| 前端页面/API/locale 专项 | 4 文件、49 测试通过 |
| 前端 typecheck、相关 ESLint | 通过 |
| 新增边界复现 | 3 个顶层测试全部失败，分别覆盖 T1、T2a、T2b |
| git diff --check | 通过 |

前端仍有既有 Browserslist 提示和测试 stub 的 Vue emits 警告。浏览器当前部署版本不用于证明本轮本地实现已上线。

## 证据与复跑

目录：`docs/delivery/2026-10-24-token-origin-human-input/`。

| 文件 | 内容 |
| --- | --- |
| `usage-link-review3-repros.go.txt` | 本轮 3 个失败复现源码 |
| `usage-link-review3-results.txt` | 实际失败输出 |
| `usage-link-review3-prior-repros.go.txt` | 适配新签名后的上轮 6 场景，预期不变 |
| `usage-link-review3-prior-results.txt` | 上轮场景与当前正式 PostgreSQL 专项全部通过的输出 |
| `usage-link-review3-source-hashes.json` | 当前审核源码 SHA-256 |

将失败复现附件临时复制为 `backend/internal/repository/token_analysis_review3_temp_test.go`，按 wiki 配置 Go 缓存及独立 GOTMPDIR，在 backend 运行：

```text
go test -tags integration -p 1 -count=1 ./internal/repository -run '^TestReview3' -v
```

`SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 只能指向独立测试数据库。harness 会执行 migration 并写测试数据，不得指向生产或未知数据库。本轮使用新建 PostgreSQL 18.3 空集群，仅监听 127.0.0.1:55486，已停止；两个临时 Go 测试文件已删除。业务实现保持 Claude 的原样，旧轮次证据不改写。

## 处理结果（2026-10-25，Claude，本地修复，未提交、未部署）

三个场景在新建 PostgreSQL 18 空库上按附带复现确认失败（源码哈希与本复核基线一致），两类遗漏均已采纳。

| 发现 | 处理 |
| --- | --- |
| T1 跨日请求未找到账单就清掉重试标记 | 明确重试合同：带关联键、响应未失败的请求复核后仍找不到账单时不确认，`usage_link_retry_until`（复核事件时刻加 48 小时）之前每轮不受日期范围限制地继续精确查找；期满仍无证据才确认为未关联。无关联键的存量行账单早已落库，一次查不到即确认，避免历史回填后每轮重查大量歧义行 |
| T2 条件更新未保护完成事实与复核任务 | 布尔标记改为复核版本：完成事实变化、账单被夺回时 `usage_link_version` 加一，处理结果写回 `usage_link_checked_version`；关联写入与“无需改动”的确认都必须携带查找前读到的版本并按它条件更新，确认只写回读到的版本。查找之后登记的复核使陈旧时序写入被拒（T2a），陈旧确认也不能吞掉夺回产生的新任务（T2b） |

迁移改动（247/248 均未部署，直接改写）：`usage_link_pending` 换成 `usage_link_version`、`usage_link_checked_version`、`usage_link_retry_until`；待复核部分索引条件改为两版本不等。仓储接口：`TokenAnalysisUsageLinkState` 增加 `Version`、`KeepPending`，`ClearUsageLinkPending` 增加 `version` 参数。

验证：

- 本复核三个场景全部通过。复现文件需要两处等价改写：读取的旧 `usage_link_pending` 列改为 `usage_link_version <> usage_link_checked_version`；T2b 中 A 的确认传入它读到的版本，B 的认领传入 B 写回 response 后读到的状态（原复现传版本 0，B 的合法写入会被版本条件拒绝）。T2a 原样通过。
- 第三轮附带的上轮六个场景（`usage-link-review3-prior-repros.go.txt`）全部通过；其中两个 R2 复现的 A、B 都在写回 response 之后读取，改为传入这次读取的状态与版本。
- 永久回归新增 `TestTokenAnalysisUsageLinkLateBillIsRetriedUntilDeadline`（含截止后确认未关联）与 `...NewRecheckRejectsStaleResults`（T2a、T2b），关联集成测试共 16 个顶层用例在新空库通过。
- 变异检查：去掉复核版本条件、确认改为确认当前版本、夺回不登记复核、不等待晚到账单，各自对应用例失败。去掉确认的版本条件本身不会失败，因为确认只写回读到的版本，旧确认无法覆盖新版本。

