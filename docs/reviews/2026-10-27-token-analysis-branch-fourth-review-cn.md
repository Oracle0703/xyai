# Token Analysis 风险分类分支第四轮复核

日期：2026-10-27。分支：`feature/hy/10221_token_risk_classification`。HEAD 与本地 main 均为 `25bedc620d9783e065ec6efae45d56f984b6b01c`。审核范围为基线上的全部未提交 backend/deploy 改动（含新增文件），共 56 个文件。

**结论：第三轮 T1 的 seq ABA 已修复，原并发场景重放通过；全分支复核仍发现 2 项 P2，建议修正后再验收当前数据与分类计算阶段。** 一项是旧消费者仍可产生没有后续通知的父版本变化，另一项是已支持的 namespace `children` 工具定义被漏计。二者均已独立复现；不将本轮新发现等同于本次修改新引入。

## 1. 范围、方法与修复确认

继续由三个子代理分别检查协议/指纹、数据库/并发、分类/金额；主代理核对调用链、交叉去重并统一执行测试。对照第三轮源码快照，本次只有 4 个 backend 文件变化：登记仓储、其 sqlmock、并发回归和新增 migration 258；其他分支增量仍重新纳入审核。

唯一设计入口为 [token-analysis-risk-classification-design-cn.md](../features/token-analysis-risk-classification-design-cn.md)。明确延期的读接口、新版页面、连续活跃和受控重索引，不作为本阶段遗漏。

| 既有问题/维度 | 当前结果 |
| --- | --- |
| 第三轮 T1：删除后复用 seq | migration 258 将新建默认值、普通冲突登记、账单删除触发器统一改为全局 `nextval`；原 ABA 交错重放通过。 |
| 原 ABA 独立复现 | `old seq=331, P/C queue rows=0, child versions=7/7, cause=cache_key_changed:gateway`，子请求正确追平并反映新事实。只删除旧测试中“seq 必须等于 1”的过时前置断言，其余交错与正确行为断言保留。 |
| 第二轮工具图片、最终正文哈希、金额舍入 | 三个原独立 service 复现再次通过；最终正文覆盖 standard/passthrough。 |
| 首次建边与父事实变化、候选锁、分类写回死锁、删除账单传播 | 当前正式 PG 专项通过，已知原场景未退化。 |
| 结构与简洁性 | 纯分类器、批量加载、仓储条件写回、独立失效登记职责清晰；无需为了减少函数数目合并这些职责。 |

## 2. F1 · P2：过期消费者删除 0 行，仍提交版本传播，子请求可持续失配

位置：[token_analysis_classification_repo.go](../../backend/internal/repository/token_analysis_classification_repo.go)，第 304–315 行；传播范围及更新见 [token_analysis_fingerprint_repo.go](../../backend/internal/repository/token_analysis_fingerprint_repo.go)，第 259–303 行。

`DrainClassificationInvalidations` 在事务外读出登记。进入事务后先无条件执行 `bumpTokenAnalysisClassification`，最后才按旧 `(archive_id, seq)` 删除。删除没有匹配任何行时仍返回 commit=true。全局 seq 防止误删新登记，但没有约束旧批次已经产生的版本修改。

### 已复现交错

| 顺序 | 操作 |
| --- | --- |
| 1 | B 保留 P 登记的旧快照；A 通过真实 Drain 完成该登记并删除。 |
| 2 | B 开始处理旧批次，依赖发现只找到 P；在加锁 UPDATE 前暂停。 |
| 3 | C 通过正常 Query/Select 路径建立 C→P，消费自己的建边登记，按当前 P 版本 6 完成分类。此时 C 自身版本对齐，记录的父版本也是 6。 |
| 4 | B 恢复，只对早先发现的 P 加版本，P 变为 7；按旧 seq 删除返回 0 行，事务仍提交。 |
| 5 | 再次消费登记并分类：队列为空，C 自身仍为 6/6，引用的父版本仍为 6，P 当前版本为 7。 |

独立 PostgreSQL 18.1 输出：

```text
queue=0 child versions=6/6 parent version now=7 used=6 stale delete rows=0
```

测试在“最终所用父版本应追平”的正确行为断言处失败，不是阻塞或超时。此场景没有新源事件，也没有 seq 重用；额外的父版本变化由旧消费者自身造成。

现有后台路径不会必然修复：待分类查询只看子请求自己的 cls 两版本，C 不会被选中；P 的分类写回不再传播；C 的候选版本、规则版本正常，维护扫描也不会因此选中它；队列已无对应通知。除非另有输入变化触发失效，该父版本失配将持续存在。

影响须准确区分：本复现没有证明金额数值算错，也没有证明当前页面展示错误；它证明已计算结果持续不能满足设计 10.4 的父版本发布条件。这个状态不会随队列正常消费收敛，区别于设计已接受的短暂异步窗口，因此属于当前数据一致性缺陷。

### 修正建议与证据

把有效登记的消费资格与传播副作用绑定在同一事务中，过期批次不能留下已提交的版本修改。可考虑先原子认领有效登记，再只对成功认领的集合传播；同时检查源写入锁顺序和整体重试。批量中只有部分登记仍有效时，也必须保证其他过期登记不产生副作用，不能仅检查整批删除总数是否为零。

复现的 A/C 使用真实仓储与分类接口；B 使用真实 `bumpTokenAnalysisClassification`、当前 Drain 的条件删除 SQL，以及正式测试已有的暂停 wrapper，固定“依赖发现后、行锁前”这一时机。它是受控交错复现，不是两个未改装后台作业的黑盒压测。

证据：[完整复现源码](../delivery/2026-10-27-token-analysis-branch-fourth-review/repro-stale-invalidation-consumer.txt)、[执行日志](../delivery/2026-10-27-token-analysis-branch-fourth-review/stale-consumer-repro.log)。

## 3. F2 · P2：namespace 的 children 别名漏计，工具数和现有风险条件不准确

位置：[token_analysis_summary.go](../../backend/internal/service/token_analysis_summary.go)，第 568–572 行。

项目现有 [namespace 适配器](../../backend/internal/pkg/apicompat/responses_namespace.go) 第 147–152 行明确采用“非空 `tools` 优先，否则回退 `children`”；原生 Responses 适配路径实际调用该逻辑，Chat 桥也支持此别名。摘要的 `tokenAnalysisToolEntries` 只递归 `tools`，会把只有 `children` 的 namespace 当作 1 个工具，遇到 `tools:[]` 则直接计为 0。

合成请求中放入一个 namespace，内含 10 个合法 function 定义。复现先调用真实 `FlattenResponsesNamespaces` 确认适配后产生 10 个工具，再对原始请求调用摘要器，并串联现有风险计算（input=20000、output=100）：

| 结构 | 实际适配工具数 | 摘要 tools_count | 现有工具风险条件 |
| --- | --- | --- | --- |
| `namespace.children=[10 个 function]` | 10 | 1 | 缺失，score=0 |
| `namespace.tools=[]` 且 `children=[10 个 function]` | 10 | 0 | 缺失，score=0 |

当前 [token_analysis_risk.go](../../backend/internal/service/token_analysis_risk.go) 第 49 行在 `ToolsCount >= 10`、输入不少于 20000、输出不超过 500 时应产生 `tool_heavy_short_output`。这两个子例均在工具数量和风险条件断言处失败。它们不是任意构造的未知协议，而是仓库已有适配器明确支持的结构；没有据此推断生产出现频率。

建议 namespace 子项提取遵循现有适配器的优先级，保留叶子计数与空 namespace 语义；不要对任意对象的 `children` 字段无条件递归。补充 `children` 单独存在、空 `tools` 回退以及两者同时非空时的优先级回归。当前共用的叶子计数也用于 `additional_tools`，修复时须保持两个入口一致。

证据：[完整复现源码](../delivery/2026-10-27-token-analysis-branch-fourth-review/repro-namespace-children.txt)、[执行日志](../delivery/2026-10-27-token-analysis-branch-fourth-review/namespace-repro.log)。

## 4. 非阻塞建议与边界

| 项目 | 结论 |
| --- | --- |
| 示例配置重复注释 | `deploy/config.example.yaml:870–871` 的“多实例部署必须使用相同规则”重复一行，可顺手删除；不另列缺陷。 |
| migration 258 首次发布 | 新序列在事务内建立并设为默认值；成功后 runner 按迁移记录跳过，不会在每次启动重复 setval。序列回滚产生空洞，不复用值，符合代次用途。 |
| 从运行中的 257 升级 | 当前分支未上线，不能据此假设生产存在旧消费者。若某预发布环境已运行 257，258 的 MAX(seq) 种子不覆盖已删除但被旧消费者持有的历史代次，旧程序的 seq+1 也不遵守新合同；应停旧消费者/旧登记写入并清理旧批次后切换，不能宣称支持新旧程序混跑。未对此场景作兼容性验收。 |
| 读接口发布合同 | 异步传播窗口内的可见性、`classified_through` 对待处理登记的覆盖仍需在读接口实现时验收；不将明确延期的接口本身当作本轮缺陷。 |
| 性能与前端 | 没有压测或浏览器验证，不把哈希、复制、递归传播的成本写成已证实性能故障，也不以本轮后端测试替代页面验收。 |

## 5. 验证与源码保护

| 检查 | 结果 |
| --- | --- |
| 六包专项 unit | 178 个顶层测试通过，0 跳过；routes 没有匹配测试，仅编译。 |
| 独立 PG 仓储专项 | 51 个顶层测试通过，0 跳过，其中 30 个来自 integration-tagged 文件。 |
| 第三轮 ABA 独立重放 | 1 个顶层测试通过。 |
| 第二轮 service 独立重放 | 3 个顶层测试通过；最终正文含 standard/passthrough 两子场景。 |
| 本轮 F1/F2 独立复现 | 2 个顶层测试在正确行为断言处失败；F2 含两个失败子场景。 |
| 后端构建、格式 | build、受审 Go 文件 gofmt、`git diff --check` 通过。未执行服务产物。 |
| 源码保护与清理 | 56 个 backend/deploy 文件审核前后哈希一致；临时 `zz_codex_fourth*` 测试已删除，本机隔离 PG 已停止。 |

主要命令（在 backend 下，integration DSN 只指向本机隔离库）：

```powershell
go test -tags=unit -p 1 -count=1 -json -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound|MigrationExecutionMode|ApplyMigrationsFS|OpenAI.*Plugin' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware ./internal/handler/admin ./internal/server/routes
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexAuditInvalidationSequence' ./internal/repository
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexFourthReviewStaleConsumer' ./internal/repository
go test -tags=unit -p 1 -count=1 -v -run 'TestCodexFourthReviewNamespaceChildrenCount' ./internal/service
go test -tags=unit -p 1 -count=1 -v -run 'TestCodexRereview' ./internal/service
go build -o E:/tmp/token-analysis-branch-fourth-review-20261027/sub2api-audit.exe ./cmd/server
```

独立复现源码以 `.txt` 保存；复跑时临时放入对应 package 的 `zz_codex_fourth_*_test.go`，结束后删除，且不要对业务库运行 integration 测试。F1 依赖当前分支正式测试的 fixture 和暂停 wrapper。所有复现使用合成数据，没有外部模型调用。

完整证据见 [第四轮证据目录](../delivery/2026-10-27-token-analysis-branch-fourth-review/)，其中 [validation-summary.json](../delivery/2026-10-27-token-analysis-branch-fourth-review/validation-summary.json) 汇总结果，[source-hashes.json](../delivery/2026-10-27-token-analysis-branch-fourth-review/source-hashes.json) 标识受审源码，[execution-results.jsonl](../delivery/2026-10-27-token-analysis-branch-fourth-review/execution-results.jsonl) 保留实际命令与退出码。

环境为 Windows、本机 PostgreSQL 18.1；不等同于生产 PostgreSQL 15 验收。未运行全仓测试、生产查询、部署或重索引。本轮只增加审核文档与证据、同步 backend wiki，未修改业务代码、正式测试或设计稿，未提交、推送。
