# Token Analysis 风险分类分支第二轮复核

日期：2026-10-27。分支：`feature/hy/10221_token_risk_classification`。HEAD 与本地 main 均为 `25bedc620d9783e065ec6efae45d56f984b6b01c`，审核范围是该基线上的全部未提交后端/部署配置代码及新增文件，共 54 个文件。

**结论：上轮主要复现场景已修正，但仍有 3 项 P2 正确性缺口和 1 项 P3 精度问题。建议先修完三项 P2，再验收当前数据与分类计算阶段。** 这些是复核中的新增发现或修正尚未覆盖的路径，不意味着它们全部由本轮修改新引入。

## 1. 范围、方法与上轮修正

继续由三个子代理分别审核协议/指纹、数据库并发、分类/金额，主代理独立核对源码、去重并执行测试。完整范围包括 migration 249–256、摘要、客户端族、转发记录、候选、分类、作业、仓储与 Wire 接线；同时对照上轮保存的源码快照检查本次修正。当前 [设计第 16 节](../features/token-analysis-risk-classification-design-cn.md) 明确延期的读接口、页面、连续活跃和受控重索引，不作为本阶段遗漏。

| 上轮编号 | 本轮复核 |
| --- | --- |
| R1 原生变换缺资格保护 | 新的联合门槛成立：父子各自最终 items 等于各自入口 items，且父子最终 root 相同。原 encrypted reasoning 重试场景的正式回归通过；但构造器内部改写仍未被采集，见 N2。 |
| R2 新父/新子漏候选 | 同 Key 咨询锁在摘要行锁之前取得，候选查询与写边处于同一把锁内；原并发新增交错的 PG 回归通过。普通父事实变化与新建边是另一处未覆盖交错，见 N3。 |
| R3 分类写回死锁 | 分类 UPDATE 使用 SKIP LOCKED，源事务对 40P01/40001 整体重试；原锁顺序交错回归及重试单测通过。 |
| R4 删除账单漏传播 | migration 256 触发器在源行 usage 非空变空时增加源版本并登记独立失效队列；消费过程传播与删除登记在同一事务，原场景回归通过。 |
| R5 缓存键空白归并 | 标准与透传的实际发送键保持区别，按原值哈希；正式回归与上轮独立复现重放通过。 |
| R6 归档正文漂移未验证 | 已重算实际正文 SHA-256，并核对重读指纹与存储指纹；正式回归与上轮独立复现重放通过。 |
| R7 纯文本误计图片 | 原合法 content:string 已不再计为图片，摘要→金额复现重放通过；工具结果图片的反向遗漏仍在，见 N1。 |
| R8 合法大归档行无法重读 | 上限已改为 6×512 MiB+8 MiB；另重放实际超过旧 128 MiB 上限的归档读取，验证已通过。 |

上轮临时复现只在需要时调整已改变的合同，例如大行测试仍验证“超过旧 128 MiB”，不再错误要求超过新的读取上限；未把合理改变后的实现细节当作新缺陷。

## 2. 尚需修正的问题

### N1 · P2：工具结果中的图片可绕过纯文本金额门槛

位置：`backend/internal/service/token_analysis_summary.go:341–344`；下游纯文本门槛为 `token_analysis_classification.go` 的 computeTokenAnalysisReuseAmount。

Responses 的工具输出分支在统计工具字节后直接 continue，跳过 output 内图片的识别。仓库已有受支持的媒体结果结构：

```json
{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}
```

本轮组合复现使用含上述图片的父子请求，真实计算入口指纹、验证完整前缀、新的最终 items/root 哈希门槛，并串联当前分类器。普通 OpenAI 原生路径保留该工具结果结构；当上游没有提供可选的图片 token 明细时，Usage.ImageInputTokens 为 0。

实际结果：`ImageCount=0`，价格证据为 `row_exact`，而非预期的 `unavailable:image_input`。因此已知图文请求能够进入承诺只覆盖纯文本的金额路径。该测试使用合成数据，没有声称生产已经发生金额误算。

建议：在工具结果载荷上补齐图片计数，覆盖 function_call_output 等实际支持的输出块；同时保留“文本工具结果为 0 张图片”的回归。不要通过撤掉图片资格门槛修复。

### N2 · P2：最终提示词哈希仍可能取到构造器改写前的正文

位置：`backend/internal/service/openai_gateway_forward.go:1067`、`openai_gateway_passthrough.go:385`。真正的构造器内改写位于前者 `:1423`、后者 `:624`。

调用者把自己的 body 交给 recordTokenAnalysisNativeForwardFacts，但 buildUpstreamRequest/buildUpstreamRequestOpenAIPassthrough 内部会对局部 body 再执行 normalizeDeepSeekResponsesRequestBody，再用新值构造 HTTP 请求。构造器返回后，调用者的旧字节切片不会自动变成实际请求正文。

复现使用仓库现有 PlatformOpenAI、base_url 映射至 DeepSeek 的账号 fixture，以及真实 Forward 和 fake HTTPUpstream。标准与透传两条路径都先确认实际发送内容已提升工具图片、补充图片字段，再从 recorder.lastBody 计算权威哈希并比较。

两条路径均失败：实际最终 items 哈希为 `f1b3080b2a3ae69eda7b4146bd85ae93`，记录却为 `9811c20e7dda296392df4b2b3804e698`。它证明“最终提示词证据”与真正发送内容不一致；没有实际请求外部供应商，也不把供应商 TTL 假设或生产金额误算作为已验证结论。

建议：从构造器返回真正最终正文，或通过请求的 GetBody 获取其一致视图，再计算/暂存最终提示词证据。保留不消耗主 Body 的转发语义，覆盖两个构造分支；算法门槛本身正确，关键是取样点必须位于最后一次正文改写之后。

### N3 · P2：父事实更新与首次建边交错，子请求可能永久漏重算

位置：`backend/internal/repository/token_analysis_repo.go:227` 的 RecordResponse，以及 `token_analysis_fingerprint_repo.go:259–302` 的依赖发现与版本更新。分类写回对父版本的检查见 `token_analysis_classification_repo.go:211`。

新增同 Key 候选锁保护了父 Upsert 与候选构建，但 RecordResponse 不取该锁；其版本传播先查询依赖边，再锁行增加版本。下面的交错仍可发生：

1. P 更新完成事实/最终缓存键，传播查询当时没有子边，事务尚未提交。
2. C 通过正常 Query/Select 路径在候选锁内建立 C→P 并提交。
3. 分类读到 P 的旧 MVCC 事实，将 C 判为 eligible 并确认；写回只锁 C，父版本 EXISTS 不会等待 P 的更新。
4. P 提交新事实和新版本，但已经完成的依赖发现没有包含 C。
5. 再运行分类，C 版本仍对齐，不会重新计算。

独立 PostgreSQL 复现使用真实 RecordResponse、ReplacePrefixEdges、RunClassification；测试专用 trigger/advisory gate 只固定时序。已核对当前边表没有 FK，因此 C 插边不会因外键取得 P 的 KEY SHARE 锁而被阻止；测试也先检查这个实际前提。未以超时或阻塞代替成功复现。

实际结果：父版本 `3 → 4`，子版本仍为 `4/4`，使用的父版本仍为 3，原因保留 `missed_with_eligible_parent`。未来读接口的父版本门槛可以隐藏旧金额，但不能补回分类重算，也不能更新原因或寻找其他有效父请求。

建议：所有会改变父资格的源写入与新依赖边的建立之间，都需要不丢通知的序列化/版本登记机制；也可将源版本变化可靠地登记为可重试失效。仅把新父 Upsert 与候选查询串行、或依赖 SKIP LOCKED，都不覆盖本交错。

### N4 · P3：中间单价提前舍入，最终金额末位可能不准确

位置：`backend/internal/service/token_analysis_classification.go:684–686`。

decimal.Div 默认先保留 16 位小数，两种单价分别被舍入后再相减、乘可挽回量，可能偏离最终保留 10 位的精确结果。本轮实测：

| 输入/输出 | 值 |
| --- | --- |
| 父总提示词 | 800000 |
| 子 input / cache_read | 1000000 / 1024 |
| 子 input_cost / cache_read_cost | 1.0000000000 / 0.0001024001 |
| 可挽回 token | 798976 |
| 精确计算、最后舍入 10 位 | 0.7190783220 |
| 当前代码 | 0.7190783219 |

差值只有 `0.0000000001`，属于低影响精度问题，不应和前三项等量齐观，更不代表实际扣费错误。建议合并为精确有理式，最后只做一次 DivRound(10)，或明确足够高的中间精度。

## 3. 结构、性能和版本边界

| 部位 | 复核结论 |
| --- | --- |
| 最终 items/root 联合证明 | 逻辑成立，前提是确实读取最终发送正文；不需要把所有顶层改写一概拒绝 |
| 候选查询与写边同锁 | 修复了新父提交窗口；每类 K+1 限量与完整前缀/去尾/关联优先级一致，值得保留 |
| 账单删除独立失效队列 | 避免用可被分类覆盖的状态充当通知；触发器、事务消费和重试方向合理 |
| 事务重试封装 | inTx 减少重复事务代码，当前写入回调仅操作数据库，适合整体重放 |
| 纯分类器、事实加载、仓储写回分层 | 继续保留，没有理由整体重构 |
| 哈希延迟计算注释 | 当前仍由 RequestArchive 在 c.Next 返回后、HTTP 请求协程内同步调用；“不占热路径/不延迟转发”应准确描述为后移计算，不能等同后台异步或零成本。本轮没有实测性能故障 |
| 规则版本 | 分支明确未上线，当前版本仍为 1 不单列首次上线缺陷；若某预发布环境已经运行旧版分类，应安排版本递增/受控重算。未来最终转发哈希也要考虑算法版本与旧归档的可比性 |

本轮没有进行生产规模压力测试。新的 3 GiB 以上最大行预算只是编码覆盖上限，不等于已验证该体量的峰值内存与吞吐。

## 4. 验证与保护

| 检查 | 结果 |
| --- | --- |
| 相关包专项 unit | 175 个顶层测试通过，0 跳过；routes 无匹配用例，仅编译通过 |
| 独立 PG 18.1 空库 | 自动应用完整迁移至 256；仓储专项 49 个顶层测试通过，0 跳过，其中 28 个来自 integration-tagged 文件，其余为同包单元测试 |
| 上轮独立复现重放 | 原值缓存键、正文哈希、纯文本金额、实际超旧行上限读取均通过；其余按正式修正后合同的回归测试核对 |
| 本轮新增复现 | 4 个顶层测试在正确行为断言处失败，对应 N1–N4；最终哈希含 standard/passthrough 两个子场景 |
| 构建与格式 | 后端 go build 成功；改动 Go 文件 gofmt -l 无输出；git diff --check 通过 |
| 工作区保护 | 54 个业务/配置文件保存 SHA-256 快照并在收尾比对；没有修改业务实现或正式测试 |
| 清理 | 临时测试从源码目录删除；独立 PostgreSQL 已停止，postmaster.pid 消失 |

正式命令与上轮一致：

```powershell
# backend 目录，使用仓库本地 Go 缓存和独立 GOTMPDIR
go test -tags=unit -p 1 -count=1 -json -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound|MigrationExecutionMode|ApplyMigrationsFS|OpenAI.*Plugin' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware ./internal/handler/admin ./internal/server/routes
# SUB2API_POSTGRES_ONLY_INTEGRATION_DSN 指向本轮新建本机测试库
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
go build -o E:/tmp/token-analysis-branch-rereview-20261027/sub2api-audit.exe ./cmd/server
```

证据目录：[2026-10-27-token-analysis-branch-rereview](../delivery/2026-10-27-token-analysis-branch-rereview/)。

- 正式测试：[unit-tests.jsonl](../delivery/2026-10-27-token-analysis-branch-rereview/unit-tests.jsonl)、[integration-tests.jsonl](../delivery/2026-10-27-token-analysis-branch-rereview/integration-tests.jsonl)。
- 新增复现：[service-repros.log](../delivery/2026-10-27-token-analysis-branch-rereview/service-repros.log)、[repository-repros.log](../delivery/2026-10-27-token-analysis-branch-rereview/repository-repros.log)，源码为同目录 repro-*.txt。
- 上轮修正重放：[fixed-replay.log](../delivery/2026-10-27-token-analysis-branch-rereview/fixed-replay.log)。
- 审核基线：[source-hashes.json](../delivery/2026-10-27-token-analysis-branch-rereview/source-hashes.json)。

仅新增审核报告/证据并同步 wiki；未提交、推送、部署、调用真实模型、连接生产或执行生产重索引。本轮数据库运行验证仅为 PG 18.1，不替代生产 PG 15 验收；没有进行全仓库测试或浏览器验收。
