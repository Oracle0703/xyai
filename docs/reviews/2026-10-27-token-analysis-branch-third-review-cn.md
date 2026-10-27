# Token Analysis 风险分类分支第三轮复核

日期：2026-10-27。分支：`feature/hy/10221_token_risk_classification`。HEAD 与本地 main 均为 `25bedc620d9783e065ec6efae45d56f984b6b01c`；本次审核覆盖基线上的全部未提交后端与部署配置改动（含新增文件），共 55 个文件。

**结论：上一轮 4 项问题的原场景已通过回归，但仍有 1 项 P2 并发一致性问题：失效登记删除后复用 seq，旧消费者可能误删新一代通知，导致子请求持续保留旧分类。建议修正这一项后再验收当前数据与分类计算阶段。** 本轮没有发现其他证据充分、需要阻断该阶段验收的新增问题。

## 1. 审核范围与修复确认

三个子代理分别审核协议与指纹、数据库并发与迁移、分类与金额；主代理核对实际调用链、去重并执行独立复现。范围包括 migration 249–257、摘要与客户端族、转发事实、前缀候选、分类与金额、后台作业、仓储、配置和 Wire 接线。[当前唯一设计文档](../features/token-analysis-risk-classification-design-cn.md) 明确延期的读接口、新版页面、连续活跃和受控重索引，不列作本阶段代码遗漏。

| 上轮问题 | 本轮核验 | 结果 |
| --- | --- | --- |
| N1 工具结果图片遗漏 | 输出载荷补充图片计数；原独立摘要→分类→金额复现原样重放 | 通过，图片请求不会按该路径误入纯文本金额计算 |
| N2 最终提示词哈希取样过早 | 由构造完成请求的 `GetBody` 取得正文；标准与透传两条路径均与实际交给 transport 的正文比较 | 原独立复现的两个子场景通过 |
| N3 普通父事实变化与首次建边漏通知 | 源事务只给自身加版本并登记；源提交后作业沿依赖边传播 | 正式 PG 回归 `TestTokenAnalysisNewEdgeCannotMissConcurrentParentFactChange` 通过；新方案的多消费者边界见 T1 |
| N4 中间单价提前舍入 | 合并为有理式，最后一次 `DivRound(10)` | 原独立金额复现原样重放通过 |

上述“通过”针对原复现场景，不表示所有并发交错均已穷尽。N3 依据新合同先消费登记再分类；`RunClassification` 只是分类入口，不会消费登记，不能将单独调用该方法当作完整后台作业。

## 2. T1 · P2：失效登记删后重建会复用 seq，旧消费者可误删新通知

| 位置 | 相关行为 |
| --- | --- |
| [migration 257](../../backend/migrations/257_token_analysis_cls_invalidation_seq.sql)，第 8 行及触发器 | `seq` 默认值为 1；插入新行没有指定新代次，仅冲突更新时递增 |
| [token_analysis_fingerprint_repo.go](../../backend/internal/repository/token_analysis_fingerprint_repo.go)，第 389–391 行 | 源事务同样使用默认值插入，已存在时才执行 `seq + 1` |
| [token_analysis_classification_repo.go](../../backend/internal/repository/token_analysis_classification_repo.go)，第 280–283、309–311 行 | 事务外无锁读取 `(archive_id, seq)`；传播完成后按旧快照中的二元组删除 |

`seq` 只保证同一行尚未删除期间不回退。登记被消费删除后，新的登记又从 1 开始；旧消费者无法区分两代记录。这是 ABA 问题：值经历“1 → 不存在 → 1”，最终比较相等，实际已经是不同通知。

### 已复现的交错

| 顺序 | 操作及可见状态 |
| --- | --- |
| 1 | 消费者 A、B 都读到 P 的旧通知 `seq=1`。 |
| 2 | A 正常传播并删除旧通知；B 仍持有旧批次。 |
| 3 | B 的依赖发现完成，此时没有 C→P 边；在给 P 加锁前暂停。 |
| 4 | 新源事务 S 更新 P 的最终缓存键，持有 P 行锁但尚未提交；恢复 B 后，B 等待该锁。 |
| 5 | C 正常建立 C→P，先消费 C 自己的建边登记，再按 P 的旧 MVCC 事实完成分类，确认为有资格父请求。 |
| 6 | S 提交新事实并重建 P 的通知，新的 `seq` 再次为 1。 |
| 7 | B 沿先前发现的集合只更新 P，再用旧 `(P, 1)` 删除了 S 刚创建的新通知。 |
| 8 | 再消费登记并分类：P/C 均无待处理通知，C 仍版本对齐并保留旧原因。 |

独立 PostgreSQL 18.1 的实际输出：

```text
old seq=1, P/C queue rows=0, child versions=6/6,
cause=missed_with_eligible_parent
```

正确行为断言失败：新通知应保留并促使 C 重算，或者 C 已经反映新父事实。实际两者均未发生。失败不是超时或测试前置条件不成立。

影响是子请求可能持续保留陈旧的资格原因及分类结果；后续普通分类轮次不能自动修复这个已丢失通知的状态，除非另有输入变化重新触发失效。没有据此声称已发生生产金额误算。设计明确单实例锁是 best-effort，只减少重复工作，因此不能用该锁排除多消费者交错。

### 复现方法与证据边界

- A 使用真实 `DrainClassificationInvalidations`，S 使用真实 `RecordResponse`，C 使用正常 Query/Select 路径的 `ReplacePrefixEdges`、登记消费与 `RunClassification`。
- B 使用真实 `bumpTokenAnalysisClassification`。测试 wrapper 只在依赖发现后、最后的加锁 UPDATE 前暂停；随后执行与当前 Drain 相同的条件删除 SQL，模拟已读出的旧批次。它不是两个未改装 Drain 的黑盒压力测试。
- 测试专用 trigger/advisory gate 只固定源事务的提交时机；所有数据都是本机隔离数据库中的合成 fixture，没有连接生产或调用模型。
- [完整复现源码](../delivery/2026-10-27-token-analysis-branch-third-review/repro-db-invalidation-aba.txt) 与 [失败日志](../delivery/2026-10-27-token-analysis-branch-third-review/repository-repro.log) 已保留。源码依赖当前分支的分类 fixture 与并发等待 helper。

复跑时可把该源码临时放入 `backend/internal/repository/zz_codex_third_aba_integration_test.go`，将 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 指向单独的本机测试库，在 backend 下执行以下命令，结束后删除该临时测试文件：

```powershell
go test -tags=integration -p 1 -count=1 -v -run 'TestCodexAuditInvalidationSequence' ./internal/repository
```

### 修正建议

让每次登记使用不会因删除重建而复用的代次，例如数据库全局 sequence/唯一 token，或独立持久化的高水位。所有登记路径（包括账单删除触发器）须遵守同一合同；消费仍按读出的代次条件删除。仅保留当前行内的 `seq + 1` 不足以修复。

补充上述多消费者回归，并确保测试先消费 C 自己的建边通知，避免它在最后一轮碰巧修复状态而掩盖 P 通知丢失。若 migration 257 已在某环境应用，应通过新增迁移升级，避免修改已应用迁移。

## 3. 结构、简洁性与发布边界

| 维度 | 本轮结论 |
| --- | --- |
| 分层与简洁性 | 纯分类器、批量证据加载、仓储条件写回、独立失效登记的职责分离合理；不建议为了减少函数或文件数把这些职责重新混合。 |
| 协议与最终证据 | 工具定义位置、工具输出图片、最终请求正文取样的已知缺口已覆盖；本轮没有新增证据充分的协议/指纹 P1/P2。 |
| 异步传播窗口 | 源提交至登记消费之间，子孙可能仍显示版本对齐。设计已承认该窗口，并要求 `classified_through` 纳入未处理登记；读接口上线前还须明确旧结果的可见性策略，水位本身不会阻止旧结果展示。这是延期读接口的发布合同，不另列为当前阶段缺陷。 |
| 请求成本 | `GetBody` 读取与哈希是在请求协程内响应结束之后计算，代码注释已说明。没有做压测，不能把复制/哈希成本写成已证实性能故障。 |
| 业务口径 | 当前只验证数据与计算实现；未验收生产金额、真实页面展示或历史重索引结果。 |

## 4. 验证与源码保护

| 检查 | 结果与边界 |
| --- | --- |
| 六个相关包专项 unit | 178 个顶层测试通过，0 跳过；routes 没有匹配测试，仅完成编译。 |
| 独立 PostgreSQL 仓储专项 | 50 个顶层测试通过，0 跳过；其中 29 个来自 integration-tagged 文件。 |
| 上轮独立 service 复现重放 | 3 个顶层测试通过；最终哈希测试包含 standard/passthrough 两个子场景。 |
| 本轮 ABA 复现 | 1 个顶层测试在正确行为断言处失败，见 T1。 |
| 后端 build | 通过；未执行生成的服务程序。 |
| 格式与 diff | 受审 Go 文件的 gofmt 与 `git diff --check` 通过。 |
| 源码保护 | 55 个 backend/deploy 受审文件在审核前后 SHA-256 全部一致。 |
| 清理 | 无残留 `zz_codex_third*` 临时测试；本机隔离 PG 已停止，`postmaster.pid` 已消失。 |

正式验证命令（在 backend 下执行）：

```powershell
go test -tags=unit -p 1 -count=1 -json -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound|MigrationExecutionMode|ApplyMigrationsFS|OpenAI.*Plugin' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware ./internal/handler/admin ./internal/server/routes
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
go build -o E:/tmp/token-analysis-branch-third-review-20261027/sub2api-audit.exe ./cmd/server
```

验证使用 Windows、本机隔离 PostgreSQL 18.1；不等同于生产 PostgreSQL 15 的兼容性验收。没有运行全仓测试、浏览器验证或负载压测，也没有部署、生产查询或历史重索引。本轮只新增审核文档与证据、同步 backend wiki；未改业务实现、正式测试或设计稿，未提交或推送。

证据目录：[2026-10-27-token-analysis-branch-third-review](../delivery/2026-10-27-token-analysis-branch-third-review/)。其中 [validation-summary.json](../delivery/2026-10-27-token-analysis-branch-third-review/validation-summary.json) 汇总测试计数、复现结果与清理状态，[source-hashes.json](../delivery/2026-10-27-token-analysis-branch-third-review/source-hashes.json) 标识受审源码。旧复现源码在本目录一并保留，方便后续修复复跑；服务程序、数据库数据目录及完整源码快照均未复制进仓库。
