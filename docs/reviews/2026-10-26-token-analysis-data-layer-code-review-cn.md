# Token Analysis 数据基础层代码审核

审核日期：2026-10-26。分支：`feature/hy/10221_token_risk_classification`。基线：`25bedc620d9783e065ec6efae45d56f984b6b01c`，审核对象为工作区尚未提交的实现。

结论：当前代码存在 4 项已复现的正确性问题，建议修正后再验收本阶段。代码拆分总体合理，没有需要整体重写或引入新框架的问题；精简应集中在客户端重映射状态和工具输出载荷提取。

## 1. 范围与完成度

以唯一设计入口 [风险分类设计](../features/token-analysis-risk-classification-design-cn.md) 第 16 节为本轮范围，核对了 29 个后端、测试、迁移和配置文件。文件清单及原始 SHA-256 见 [source-hashes.json](../delivery/2026-10-26-token-analysis-code-review/source-hashes.json)。

| 内容 | 当前状态 | 本次评价 |
| --- | --- | --- |
| 摘要器工具输出口径 | 已实现 | `tool_search_output.tools` 仍漏计，见 F2 |
| UA 留存、客户端识别、规则变化重映射 | 已实现 | 晚到旧版本行和条件写回仍有缺口，见 F3、F4 |
| 指纹密钥 bootstrap | 已实现 | 复用既有 `security_secrets`，专项测试通过；本轮未发现需单独报告的问题 |
| 原生 Responses 转发事实、归档、直接与暂存入库 | 已实现 | 插件路径缺少可信边界，见 F1 |
| 前缀指纹、候选边、分类与发布、连续活跃、金额、新版接口与页面 | 设计明确尚未开始 | 属于后续阶段，不作为本阶段实现缺陷；当前也不能称整份设计已经实现 |

## 2. 必须修正的问题

### F1 · P1：插件路径仍被标记为已核实的原生 Responses 路径

位置：`backend/internal/service/token_analysis_forward_facts.go:74–88`；调用点为 `openai_gateway_forward.go:1067` 与 `openai_gateway_passthrough.go:385`。实际 transport 选择在 `openai_plugin_transport.go:11–18`。

两个调用点都在 `doOpenAIUpstream` 之前记录事实，无条件写入 `usage_cache_source=upstream_parsed`，并对交给 transport 之前的缓存键计算哈希。但是 `doOpenAIUpstream` 会优先交给 `PluginManager.RoundTripOpenAIOAuth`；外部插件是否继续改写请求尚未核实。设计第 11.1 节明确要求这类账号在确认前为 unknown。

定向复现将 OAuth 插件路由设为 100% 命中，使用不可用插件 fixture 执行实际 `Forward` 调用。调用进入插件路径并返回错误，但上下文仍得到：

```text
forward_path = openai_responses
cache_key_handling = scoped
usage_cache_source = upstream_parsed
upstream_cache_key_hash = 非空
```

复现证明插件分支没有使事实降级；没有启动外部插件，也没有调用上游模型。这里并非要求每次网络失败都清空路径信息，而是未核实的 transport 不能继承原生路径的可信保证。成功运行的外部插件也经过相同的提前记录流程。

影响：一旦开始积累数据，未经核实的插件请求会留下看似完整的缓存资格证据，后续金额计算可能错误纳入。当前金额功能尚未实现，本轮没有声称已发生金额误算。

建议：让事实有效性与本次实际选中的 transport 绑定；插件未核实时清空或显式降级其资格证据。不要只依据“存在插件管理器”或在较早时刻读取一次配置，误伤未命中插件的原生请求，或遗漏路由变化。补充插件命中、未命中、透传、重试与账号切换测试。

### F2 · P2：`tool_search_output.tools` 的结果被统计成 0 字节

位置：`backend/internal/service/token_analysis_summary.go:326–331`。

`tokenAnalysisIsToolOutputItemType` 已把 `tool_search_output` 识别成工具输出，但随后统一读取 `item["output"]`。该类型的发现结果可以保存在 `tools` 中，仓库已有真实协议 fixture：`openai_gateway_responses_client_tools_test.go:81`、`openai_gateway_grok_tool_protocol_test.go:76`。

复现放入一个 `tools` 载荷，其 description 为 512 KiB：得到 `tool_message_count=1`，但 `tool_output_bytes=0`，相应最大输出大小也漏计。新修正后的旧风险分和后续大型工具结果诊断仍会漏掉这类请求。

建议：按具体 item 类型选择结果字段，`tool_search_output` 提取 `tools`，普通调用输出提取 `output`；字节计算和消息计数继续复用已有 helper。增加符合真实协议结构的 fixture，不要只给各种类型统一构造 `output` 字段。

### F3 · P2：一次追平就永久停止扫描，晚到的旧版本行不再补映射

位置：`backend/internal/service/token_analysis_client_family.go:109–119`；状态定义在 `token_analysis_service.go:30–32`。

当一次查询没有旧版本行，代码设置 `clientFamiliesCaughtUp=true`，此后本进程不再查询。注释的前提是“本进程只写当前版本”，但表由多个实例共享；滚动更新期间，仍运行旧规则的实例可以在新实例追平后继续写入旧版本行。

复现先对空旧版本集合执行一次 refresh，再模拟旧实例写入一条待重映射行，再次 refresh 返回 0，而不是处理这条行。即使旧实例随后退出，新实例也不会恢复检查，除非再次重启。

建议：删除“本次为空即永久追平”的状态假设，采用有界批次与低频复查；若需要游标，完成一轮后允许开始下一轮。保留版本 0 历史行等待受控重索引的原有边界。用“先追平、后新增旧版本行”的测试验证最终能收敛。

### F4 · P2：重映射写回只比较目标版本，不能拒绝读取后已经变化的行

位置：`backend/internal/repository/token_analysis_repo.go:1149–1155`；读取 DTO 为 `token_analysis_types.go:85–91`。

当前 UPDATE 只要求 `client_family_version <> 0 AND client_family_version <> 目标版本`。这只能保护恰好已被写成目标版本的行，不能保护读取之后由其他规则实例或重索引更新的输入；读取结果也没有保留原规则版本用于条件提交。

在独立 PostgreSQL 中复现如下交错：

| 步骤 | 行状态或操作 |
| --- | --- |
| 1 | 原行 UA=Codex，版本标识 5；任务保存读取结果并算出 family=codex，准备按版本标识 7 写回 |
| 2 | 另一次索引将同一行更新为 UA=hertz、family=hertz、版本标识 9 |
| 3 | 旧任务执行 `UpdateClientFamilies(..., 7, ...)` |
| 实际结果 | UA 仍为 hertz，但 family 被覆盖为 codex，版本标识被覆盖为 7 |

5、7、9 仅是测试中的不同版本标识；生产版本由规则内容哈希生成，数值大小不表示时间先后。这项问题也包括不同规则实例覆盖读取后已经发生的重映射，不能用整数大小比较修复。

建议：读取 DTO 携带 observed UA 与 observed rule version，批量写回同时校验二者仍等于读取快照。条件不再匹配的行跳过，由后续轮次重新读取计算。无需引入新分布式锁，但需和 F3 一起保证重试机会；不同规则实例长期并存的目标版本收敛也应有明确运行约束。

## 3. 简洁性与可维护性

| 部位 | 建议 | 理由 |
| --- | --- | --- |
| 客户端重映射 | 优先简化：有界扫描、按快照计算、条件写回；取消永久 caught-up 状态 | 现有优化引入了额外正确性前提，F3/F4 可在同一流程内修正 |
| 工具输出提取 | 按协议类型提取载荷，继续共用计数 helper | 判定“是工具输出”和判定“结果在哪个字段”是两个不同问题；不必扩展成通用框架 |
| HMAC 编码与密钥管理 | 保留长度前缀编码、用途区分和既有 bootstrap 复用 | 这些代码承担消除拼接歧义、稳定密钥和跨实例一致性的职责，不能仅为减少行数删除 |
| 原生转发采集落点 | 保留两个原生调用点的边界，补实际 transport 校验 | 共享构造函数也被兼容转换使用，移入共享构造器可能把不支持的路径误标为原生；设计 11.2 与 helper 注释可同步为实际调用点 |
| 配置加载校验与运行期 matcher | 暂不强行合并 | 分别承担配置报错和运行期防御职责，没有证据表明这一拆分造成实质维护问题 |

性能待验证项：`ListStaleClientFamilies` 每批按 `id` 从头查找非目标版本行，migration 249 未增加相关查询索引。数据量大时可能重复扫描已经处理过的前缀。修复 F3 时应一起评估轮次游标或其他有界扫描方式，再用实际规模的 `EXPLAIN (ANALYZE, BUFFERS)` 判断是否需要索引。本轮未做压测，**不将其列为已证实的性能故障**。

文档小项：设计开头仍写“设计已定稿，未实施”，与第 16 节已有实现冲突；建议改成“数据基础部分已实现，审核待修正，分类/金额/页面尚未实施”。

## 4. 验证与证据

| 检查 | 结果与边界 |
| --- | --- |
| config/service/repository/middleware 专项单元测试 | 通过；选择 `TokenAnalysis` 与密钥 bootstrap 相关测试 |
| handler/admin TokenAnalysis 专项 | 通过；routes 包同轮编译通过，但正则未匹配其中测试，不能称路由行为测试通过 |
| 独立 PostgreSQL 18.1 空集群 | 自动应用现有迁移及新增 249/250，仓储专项 39 个顶层测试通过、0 跳过；其中 19 个来自 integration-tagged 文件，其余为同包单元测试，不能将 39 项全部称作数据库集成测试 |
| 新增审核复现 | service 三项、repository 实际数据库一项均在正确行为断言处失败，对应 F1–F4；这些失败与现有正式测试通过分开记录 |
| 格式与差异检查 | 改动 Go 文件 `gofmt -l` 无输出；`git diff --check` 通过 |
| 改动保护与清理 | 29 个原有 backend/deploy 改动文件 SHA-256 复核一致；临时复现测试已移出源码目录，独立 PostgreSQL 已停止 |

正式测试命令（在 `backend` 目录）：

```powershell
go test -tags=unit -p 1 -count=1 -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware
go test -tags=unit -p 1 -count=1 -run 'TokenAnalysis' ./internal/handler/admin ./internal/server/routes
# 使用本机独立测试数据库，按仓储 integration harness 提供连接配置：
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
```

证据目录：[2026-10-26-token-analysis-code-review](../delivery/2026-10-26-token-analysis-code-review/)。

- [正式仓储测试原始事件](../delivery/2026-10-26-token-analysis-code-review/integration-tests.jsonl)
- [handler/routes 输出](../delivery/2026-10-26-token-analysis-code-review/handler-routes-tests.txt)
- [service 三项复现输出](../delivery/2026-10-26-token-analysis-code-review/service-repros.txt) 与 [复现源码](../delivery/2026-10-26-token-analysis-code-review/service-repros_test.go.txt)
- [repository 复现输出](../delivery/2026-10-26-token-analysis-code-review/repository-repro.txt) 与 [复现源码](../delivery/2026-10-26-token-analysis-code-review/repository-repro_integration_test.go.txt)

复现源码使用原包内既有测试 helper，修复时可临时放回对应包按 `TestCodexAudit` 定向运行；移入正式测试时应改为稳定名称并保留对应边界。所有请求凭据与缓存键均为合成数据。

本轮只审核实现，并新增审核文档、证据及 wiki 说明；没有修改业务代码、提交、推送、部署、连接生产或执行生产历史重索引。没有进行浏览器验收、全仓库测试及真实插件端到端验证。
