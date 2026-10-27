# Token Analysis 风险分类分支第五轮复核

日期：2026-10-27。分支：`feature/hy/10221_token_risk_classification`。HEAD 与本地 main 均为 `25bedc620d9783e065ec6efae45d56f984b6b01c`。审核覆盖基线上的全部未提交 backend/deploy 改动（含新增文件），共 58 个文件。

**结论：本轮未发现新的 P1/P2 业务缺陷；第四轮 F1/F2 的原独立复现均通过，当前数据与分类计算阶段可通过本轮代码审核。保留 1 项 P3 非阻断测试补强建议。** 这一结论不代替读接口、页面、生产环境或大规模负载验收。

## 1. 范围与方法

三个子代理分别审核协议/指纹、数据库/并发、分类/金额，主代理核对完整调用链、去重并执行验证。完整范围包括 migration 249–260、摘要、客户端族、转发事实、前缀候选、分类/金额、后台作业、仓储、配置和 Wire 接线。

相对第四轮快照，本次 13 个代码/配置文件变化，主要是拆分输入与依赖版本、更新待分类索引，以及 namespace 子工具别名计数；没有只检查最新修正文件。唯一设计入口仍为 [风险分类设计](../features/token-analysis-risk-classification-design-cn.md)，明确延期的读接口、新版页面、连续活跃和受控重索引不列作本阶段遗漏。

## 2. 上轮问题及版本合同复核

| 核查项 | 当前实现与验证结果 |
| --- | --- |
| F1：旧消费者制造永久父版本失配 | 本行事实变化增加 `cls_input_version` 并登记失效；传播仅增加 `cls_dep_version`。旧批次可以多触发一次重算，但不再改变子请求保存的父输入版本。原独立交错重放通过。 |
| F2：namespace children 漏计 | 非空 `tools` 优先，否则仅 namespace 回退 `children`；顶层 `tools` 与 `input.additional_tools` 共用。原两个子例均得到 10 个工具，并恢复 `tool_heavy_short_output`、score=15。 |
| 真正的源变化 | 摘要、完成事实、暂存合并、候选边提交、账单关联/解除仍在源事务增加 input 并登记全局 seq；祖先变化继续传播到子孙的 dep。 |
| 条件写回 | 校验子 input、dep、cand_checked，以及选中父 input；成功时同时确认输入与依赖两对版本。四个维度均已独立验证。 |
| 待分类与规则扫描 | 任一分类版本对不等即待处理；规则扫描只增加 dep。migration 260 的新部分索引与查询谓词一致，先建新索引再删旧索引，无效索引重试清理已接入。 |
| 第三轮 seq ABA | 全局序列合同保留；原独立交错再次通过，子请求正确重算为 `cache_key_changed:gateway`。 |
| 更早修正 | 工具结果图片、最终请求 `GetBody`、金额最后一次舍入的三个独立复现再次通过；原缓存键、正文哈希校验、候选串行及插件降级保护仍在。 |

F1 的实际输出：

```text
queue=0 child versions=6/6 parent version now=4 used=4 stale delete rows=0
```

其中 `child versions` 是当前正式 fixture 返回的“输入+依赖”版本合计及其确认合计；父版本则直接读取 `cls_input_version` 和 `parent_cls_version_used`。测试仍保留“旧消费者删除 0 行”和“最终父版本一致”的原断言，没有通过削弱正确性条件让测试通过。只将已重命名的传播 helper 调整为 `propagateTokenAnalysisClassification`。

F2 两个原子例（只有 `children`、空 `tools` 加 `children`）均输出：

```text
adapter tools=10, summary tools=10, score=15, tool-heavy reason=true
```

这些都是本机合成 fixture，不是生产样本或线上金额结果。

## 3. P3：正式陈旧写回测试应隔离所验证的版本条件

位置：[token_analysis_classification_integration_test.go](../../backend/internal/repository/token_analysis_classification_integration_test.go)，第 217–222 行。

原测试同时提交“旧子输入版本”和“旧父版本”两条结果，但两条都没有填入新增的 `ClsDepVersion`，其中候选版本也未统一取自当前快照。此前 fixture 已消费依赖通知，实际 dep 为非零；因此结果可以仅因 dep 不匹配被拒绝，即使目标 input/parent 条件失效，`written == 0` 仍可能通过。

这是回归保护的隔离不足，**不是已经确认的业务写回缺陷，也不阻断本轮数据与计算实现的审核结论**。

建议正式测试从完整的当前快照构造结果，先证明可写，再分别只改变一个版本，断言旧快照拒写、刷新该版本后恢复可写。不要用同时包含多项错误的结果验证单项保护。

本轮已在仓库外提供并执行独立验证，四个子例全部通过：

| 单独变化的条件 | 控制方法 | 结果 |
| --- | --- | --- |
| 子输入版本 | 对 C 调真实 `RecordResponse`，不消费登记；其他版本保持不变 | 旧快照 0 行，刷新后 1 行 |
| 子依赖版本 | 调真实 `propagateTokenAnalysisClassification`；input/cand/父 input 不变 | 旧快照 0 行，刷新后 1 行 |
| 父输入版本 | 对 P 调真实 `RecordResponse`，不消费登记；C 的三项条件保持不变 | 旧快照 0 行，刷新后 1 行 |
| 候选确认版本 | 控制变量 SQL 只推进已对齐候选版本对；input/dep/父 input 不变 | 旧快照 0 行，刷新后 1 行 |

最后一个子例是针对写回条件的控制变量验证；正常 `ReplacePrefixEdges` 还会修改 input，无法单独验证 cand_checked 条件。每个子例均有当前快照可写的正向对照，最终分别检查输入与依赖两对版本已确认。

证据：[独立 CAS 验证源码](../delivery/2026-10-27-token-analysis-branch-fifth-review/verify-db-isolated-classification-cas.txt)、[执行日志](../delivery/2026-10-27-token-analysis-branch-fifth-review/cas-verification.log)。正式测试文件保持原样，便于实现者直接复核并补强。

## 4. 结构、文档与验收边界

| 事项 | 结论 |
| --- | --- |
| 两类版本的必要性 | input 表达本行事实变化，dep 表达需要重算，职责不同；这次拆分消除了传播本身制造伪父版本的根因，未发现应为了减少字段而合并的理由。 |
| 代码分层 | 纯分类器、批量事实加载、仓储条件写回与独立失效登记继续保持清楚的职责，不建议为了缩短文件而混合。 |
| 示例配置 | 上轮重复的多实例规则注释已删除。 |
| Wiki 同步 | 本轮修正旧传播 helper 名、补齐 input/dep/cand_checked 写回合同，并澄清 migration 260 删除的是旧索引，账单删除触发器仍保留。 |
| 设计稿残留表述 | 第 10.3 节第 325 行“没有父请求只校验子请求自己的两个版本”仍是旧说法，当前 SQL 校验 input、dep、cand_checked 三项；建议由设计作者同步。本轮未修改设计稿。 |
| 历史升级 | 按分支文档所述“尚未上线”的首次发布范围审核。如果某测试环境曾运行旧版并留下永久失配结果，新增 dep 列本身不会自动修复历史状态，需要另行受控重分类；未验收运行中的旧版混合升级。 |
| 异步可见性 | 源提交到登记消费之间的可见性、`classified_through` 和读路径发布门槛仍须在读接口阶段闭合。本轮不以计算侧通过替代该验收。 |

## 5. 验证结果

| 检查 | 结果 |
| --- | --- |
| 六包专项 unit | 180 个顶层测试通过，0 跳过；routes 没有匹配测试，仅编译。 |
| 独立 PG 仓储专项 | 52 个顶层测试通过，0 跳过；31 个来自 integration-tagged 文件。 |
| 第四轮 F1/F2 独立重放 | 2 个顶层测试通过，F2 含两个子例。 |
| 第三轮 ABA 独立重放 | 1 个顶层测试通过。 |
| 四个写回条件独立验证 | 1 个顶层测试、4 个子例通过。 |
| 第二轮 service 独立重放 | 3 个顶层测试通过；最终正文包含 standard/passthrough 子场景。 |
| 后端构建与格式 | build、受审 Go 文件 gofmt、`git diff --check` 通过；未执行生成的服务程序。 |
| 源码保护与清理 | 58 个 backend/deploy 文件审核前后哈希一致；临时 `zz_codex_fifth*` 测试已清理，本机隔离 PG 已停止。 |

主要命令（在 backend 下执行，integration DSN 仅指向本机隔离库）：

```powershell
go test -tags=unit -p 1 -count=1 -json -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound|MigrationExecutionMode|ApplyMigrationsFS|OpenAI.*Plugin' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware ./internal/handler/admin ./internal/server/routes
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexFourthReviewStaleConsumer' ./internal/repository
go test -tags=unit -p 1 -count=1 -v -run 'TestCodexFourthReviewNamespaceChildrenCount' ./internal/service
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexAuditInvalidationSequence' ./internal/repository
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexAuditClassificationCAS' ./internal/repository
go test -tags=unit -p 1 -count=1 -v -run 'TestCodexRereview' ./internal/service
go build -o E:/tmp/token-analysis-branch-fifth-review-20261027/sub2api-audit.exe ./cmd/server
```

本机 PostgreSQL 首次 initdb 在磁盘同步阶段超过 60 秒；保留失败尝试后，以新目录和仅用于临时库初始化的 `--no-sync` 重建成功，正式数据库测试均实际执行且无跳过。该处理没有修改应用配置或生产服务，不是持久性测试。

证据见 [第五轮证据目录](../delivery/2026-10-27-token-analysis-branch-fifth-review/)。[execution-results.jsonl](../delivery/2026-10-27-token-analysis-branch-fifth-review/execution-results.jsonl) 保存实际命令和退出码；[validation-summary.json](../delivery/2026-10-27-token-analysis-branch-fifth-review/validation-summary.json) 汇总计数与清理状态；[source-hashes.json](../delivery/2026-10-27-token-analysis-branch-fifth-review/source-hashes.json) 标识受审源码。复现与验证源码以 `.txt` 保存，复跑需临时放入对应 package 的 `_test.go`；服务程序、PG 数据目录及完整快照未复制进仓库。

环境为 Windows、本机隔离 PostgreSQL 18.1，不等同于生产 PostgreSQL 15 验收。未运行全仓测试、浏览器或负载压测；未连接生产、部署或执行重索引。本轮只新增审核文档/证据并同步 wiki，未修改业务实现、正式测试或设计稿，未提交、推送。
