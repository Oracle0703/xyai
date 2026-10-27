# Token Analysis 风险分类分支严格审核

审核日期：2026-10-27。分支：`feature/hy/10221_token_risk_classification`。本地 main 与分支 HEAD 均为 `25bedc620d9783e065ec6efae45d56f984b6b01c`；本轮审核包含其上的全部未提交后端与配置改动，以及未跟踪的新源码、测试和 migration。

**结论：本阶段尚不能通过验收。确认 8 项可复现问题：2 项 P1、6 项 P2。** 上轮四项问题已有对应修正，本轮问题集中在新增的最终转发证据、候选并发一致性、分类失效传播，以及新金额逻辑与既有摘要的衔接。整体分层合理，不建议整体重写。

## 1. 审核范围与证据边界

| 维度 | 审核范围与方式 |
| --- | --- |
| 分支范围 | 51 个 backend/deploy 文件，含所有新增未跟踪源码、测试、migration；清单见证据中的 reviewed-source-hashes.json |
| 协议与指纹子代理 | additional_tools、工具输出、单次 UseNumber 解码、规范化、前缀与去尾边、归档重读 |
| 数据层子代理 | migration 249–254、非事务索引恢复、仓储事务、候选版本、锁顺序、分类传播与账单删除 |
| 分类与金额子代理 | 冷暖、父子资格、多跳回溯、三桶口径、历史单价、窗口 W、作业与设计覆盖 |
| 主代理复核 | 去重和代码交叉核对、转发事实与隐私边界、构建、专项测试、真实本机 PG 并发复现、源码保护 |
| 已实施 | 数据基础、指纹与候选、分类与金额计算、版本传播、分类作业 |
| 明确延期 | 新读接口及发布可见性、页面、连续活跃、受控重索引；不列为本阶段代码缺陷 |

唯一设计入口：[风险分类设计](../features/token-analysis-risk-classification-design-cn.md)。本轮没有连接生产、调用真实模型、读取原始生产正文、部署或历史重索引。转发测试使用仓库已有 recorder，所有凭据与正文均为合成 fixture；数据库复现使用全新本机 PostgreSQL 18.1 集群。

审核开始后的 SHA 比对发现分类器及其测试两个文件发生外部变化（mtime 为 11:28:51）；主代理及子代理均未修改它们。正式测试、构建和定向复现均在该时间之后执行，分类子代理再次复读确认。最终 51 个文件另存审核快照及哈希，收尾按该版本复核，不能把较早哈希视为最终验收版本。

## 2. 发现总表

| 编号 | 级别 | 问题 | 实际复现结果 |
| --- | --- | --- | --- |
| R1 | P1 | 入口前缀证据被无条件用于发生正文变换的原生转发 | encrypted reasoning 被成功重试删除，分类资格仍未降级 |
| R2 | P1 | 新父与新子并发入库时，候选失效通知可能永久丢失 | 父提交后查询可找到它，但子 cand 版本仍为 1/1、空候选已确认 |
| R3 | P2 | 源行已先持锁，后续按 id 排序不能消除逆序死锁 | 实际 WriteClassifications 返回 `pq: deadlock detected` |
| R4 | P2 | 父账单删除被下一次分类覆盖，子请求收不到失效通知 | 父为 no_usage；sweep=0；子版本 6/6，原因仍为 missed_with_eligible_parent |
| R5 | P2 | 缓存键哈希先 TrimSpace，混淆不同实际发送值 | 标准/透传两条真实 Forward 管线均保留不同原值，却生成相同哈希 |
| R6 | P2 | 归档重读只比对哈希字段，没有验证实际正文 | 正文 A 改为 B，保留旧哈希字段，读取仍被接受 |
| R7 | P2 | 合法纯文本字符串被既有图片计数器误计，新金额门槛因此拒绝 | 应为 row_exact，实际为 unavailable:image_input |
| R8 | P2 | 固定 128 MiB 行上限覆盖不了合法未截断请求的 JSONL 编码 | 小于 40 MiB 的正文经过转义超过行上限，重匹配失败 |

### R1：原生转发的最终提示词未必保留入口前缀

位置：`backend/internal/service/openai_gateway_forward.go:1067`，以及 `token_analysis_classification.go:588`。相关正文变换位于前者 `:743`、`:1139`。

指纹来自客户端入口正文，转发事实却把原生 Responses 路径作为可继承这些前缀证据的路径。该路径会发生真正改变提示词内容/顺序的处理：

- `invalid_encrypted_content` 后剥离 reasoning 内容，再次发送。
- `NormalizeCompactionTriggerInputOrder` 把 compaction_trigger 移到 input 最后。

定向测试执行实际 `Forward`：第一次返回上游 400，第二次成功，已断言第二次发送内容中的 encrypted_content 消失。该请求没有 previous_response_id，不能靠续接引用门槛排除；最终事实仍使 `tokenAnalysisChildSideEligible` 返回 true。

结构反例也很直接：入口 `P=[user, trigger]`、`C=[user, trigger, assistant, user]` 是完整前缀；触发器重排后实际发送 `P=[user, trigger]`、`C=[user, assistant, user, trigger]`，前缀已不成立。当前原生路径、缓存键和 usage 来源检查不能识别这个变化。

影响：已核实的是“提示词改变但资格证据未降级”；结合当前分类器，存在把不成立的入口前缀作为完整缓存前缀并给出条件金额的路径。本轮没有声称生产已经误算或改变实际收费。

建议：区分“原生协议”与“入口提示词在最终请求中仍可比较”。对会改写提示词条目/内容/顺序的处理记录变换事实并保守降级；或为最终发送内容建立可验证的指纹合同。不能仅以路径名或未改缓存键作为前缀保持证明。覆盖正常规范化与重试两类变换，保留插件已实现的降级逻辑。

### R2：并发新增的父请求可能永久漏出候选集合

位置：`backend/internal/repository/token_analysis_repo.go:194–204`。

当前晚到父请求在自身事务内更新已可见的后续行。下面的交错不受现有子版本 CAS 保护：

1. P 的事务 INSERT P，并完成“标记后续行”；此时 C 尚不存在。
2. P 暂未提交。C 完成 Upsert 并提交，查父候选看不到 P，确认空候选集合。
3. P 恢复执行分类传播；边表没有 C→P，无法通知 C；随后 P 提交。
4. 重新查询父候选已经能查到 P，但 C 的 cand 版本仍对齐，后台重新匹配只查脏行，因此不会补回。

独立 PG 通过测试专用 trigger/advisory gate 固定此交错，使用真实 Upsert、FindPrefixCandidates、ReplacePrefixEdges，结果为 C 的版本 1/1。测试 gate 只用于控制执行时点，没有修改生产逻辑。

建议：新父提交和新子候选确认之间需要可证明不会漏掉的失效登记，例如适当粒度的串行化边界、提交后可持久重试的失效任务，或有界周期复核机制。只增强子行 CAS、仅加进程内互斥或继续依赖 best-effort leader lock 都不能覆盖这个交错。

### R3：实际锁顺序仍会死锁

位置：`backend/internal/repository/token_analysis_classification_repo.go:204–212` 与 `token_analysis_fingerprint_repo.go:249–258`；源行在 Upsert/RecordResponse 等流程前面已被锁住。

晚到历史父请求可能 `P.id > C.id`。源事务先持有 P；分类批量写回按 id 先持 C，再等待 P；源传播再按 id 尝试 C，形成循环等待。传播函数“最后按 id 排序”的注释没有考虑调用者已经持有的行锁。

测试通过 pg_stat_activity/pg_blocking_pids 确认真实分类 UPDATE 已阻塞，再调用真实传播 helper。PG 实际中止了分类写回事务，返回 `pq: deadlock detected`。本次受害者为分类批次，它可以下一轮重试；不能将这次结果描述为已复现索引丢行。另需注意索引错误路径会推进 offset，若其他交错使源索引事务成为受害者，不能假定它具有相同重试能力。

建议：从第一个相关行锁开始统一锁顺序，或重新划定源写入与依赖传播的事务方式。增加真实并发回归测试；如果保留可重试事务冲突，应在适当边界实现完整重试，而非只记录日志后推进索引水位。

### R4：账单删除失效可能被分类阶段吞掉

位置：`backend/internal/repository/token_analysis_classification_repo.go:275–278`；作业顺序见 `token_analysis_classification_job.go` 的 classificationOnce。

低 id 子 C 可以先被一批分类确认，使用高 id 父 P 当前的事实；P 自身仍 dirty 是合法状态，因为资格并不要求父的 cls_checked 已追平。此时删除 P 的账单，后续批次将 P 分类成 no_usage 并对齐，随后维护扫描就不再匹配 P：它只寻找空账单且旧 cache_obs_state 为 small/hit/cold/unsupported 的行。

实际复现：父 no_usage，维护扫描标记 0 行；子仍为 6/6，原因保留 missed_with_eligible_parent。之后也没有新的源事件保证它重算。未来读接口的父 usage JOIN 能隐藏旧金额，但不能代替子原因与备选父请求的重算。

建议：账单消失需要独立、不可被分类覆盖的失效登记或输入快照差异检测，并向依赖传播。仅把 sweep 调到 classify 之前仍不能封闭两者之间发生删除的窗口。

### R5：不同最终缓存键被归并成相同证据

位置：`backend/internal/service/token_analysis_forward_facts.go:78`，哈希写入见 `:90`。

普通 API Key 请求在标准与透传管线均可原样发送 JSON 字符串 `"cache-key"` 和 `" cache-key "`。实际 Forward 测试已分别验证 lastBody 的两个值不同；分析记录对它们先 TrimSpace，导致最终缓存键哈希相等。入口 client_cache_key_hash 同样做 trim，prompt_cache_key 又被排除出 h0，因此其他指纹检查不会纠正这个归并。

建议：缓存键是需要精确比较的值，应对实际发送的字符串字节计算 HMAC；只有已在真实转发路径执行、可验证的规范化才可以复用。这个测试证明的是网关证据丢失了区别，不依赖猜测上游是否自行规范化空白。

### R6：归档元数据的哈希相等不代表正文未变

位置：`backend/internal/service/token_analysis_candidates.go:226–234`。

readTokenAnalysisArchiveRequest 比较的是 event.BodySHA256 与摘要存下的 BodySHA256，没有对 event.Body 重算。测试把完整正文中的 A 同长度替换为 B，保留 archive_id、旧 body_sha256 和正确行位置，当前读取仍返回 ok=true。

如果真实 C 是 `[A,Z]`、父 P 是 `[B]`，重读的 C 被改成 `[B,Z]` 而元数据不变，重新匹配就可能向仍保存旧指纹的 C 提交错误 full_prefix 边。该场景要求归档内容发生漂移，不代表观察到了生产篡改；它违反的是设计承诺的源内容校验边界。

建议：对未截断 event.Body 重算 SHA-256，再与摘要快照比较；同时核对重算的 fp/version 与读到的存储指纹，避免用当前规则的新链给旧快照确认候选。captureBytes 对完整原始数据计算哈希，截断行应保持 unsupported，不能把截断正文当作完整正文验证。

### R7：新金额门槛依赖了不正确的纯文本图片计数

位置：新增金额门槛 `backend/internal/service/token_analysis_classification.go:634–636`；来源为 `token_analysis_summary.go:348` 和 `:486–490`。

Responses 的 `content:string` 是合法格式。既有 tokenAnalysisImageCount 却把任意非空字符串记为图片。摘要与分类组合测试中，只有纯文本的请求得到 ImageCount=1，于是本来其他资格全部满足的行返回 `unavailable:image_input`，金额为 null，预期为 row_exact。

图片计数器的行为早于本分支，但本分支首次用它阻断新金额计算，属于新增逻辑的整合缺口。不能为了避免误报直接删除图片门槛。

建议：让图片计数根据明确的图片字段/内容块类型计数，纯文本字符串为 0；保留 URL/inline image 等真实图片入口。增加“合法纯文本摘要 → 分类金额”串联测试。

### R8：请求体限制与归档行限制不一致

位置：`backend/internal/service/token_analysis_candidates.go:26–28`、`:212–220`、`:254`。

固定 128 MiB 读取上限按“40 MiB 请求体加元数据”估算，但外层 JSONL 编码会继续转义字符串。例如 `<` 会成为 `\u003c`；测试中 23 MiB 的合法正文小于 40 MiB 请求限制，未截断，写入外层归档后却超过 138 MiB。首次索引没有该读取上限，后续重新匹配返回 body_unavailable。运行时请求体上限还允许配置到 512 MiB。

建议：明确请求体预算与外层归档行预算的关系，采用与实际编码/配置一致的有界读取或流式方案。如果产品有意只支持较小的重读行，应明确单独的不可重读状态、配置与验收边界，不能把仍存在的合法归档误报为原文不可用。

## 3. 简洁性、隐私和覆盖判断

| 部位 | 判断 |
| --- | --- |
| 摘要、规范化、候选、纯分类器、仓储写回分层 | 职责清楚，建议保留；不要合成一个大服务 |
| additional_tools 与 tool_search_output | 依据样本补上了正确载荷位置；namespace 递归叶子计数、UseNumber 共用解码方向正确 |
| HMAC/长度前缀/数值类型标签 | 有明确的歧义消除与隐私作用；指纹不在当前 API DTO 中返回，不应仅为减行删除 |
| 批量 jsonb_to_recordset 写回与 decimal 金额 | 比逐行更新或浮点金额更合适；当前主要问题是证据与并发合同，不是缺少抽象层 |
| 归档重读的局部精简 | `json.Unmarshal([]byte(strings.TrimSpace(string(line))), ...)` 有不必要的全量转换；json.Unmarshal 本身接受首尾空白，可以直接传 line |
| 迁移与构建接线 | 249–254、_notx 无效索引重试登记、Wire 接线均已核对；独立空库迁移与后端构建通过 |
| 性能待验证 | FindPrefixCandidates 无 SQL 行数上限，传播只有深度上限而无宽度上限；未做接近生产规模的执行计划/压测，不把静态疑点写成已实测性能故障 |

另有低优先级合同差异：设计第 7 节要求当前请求缺 completed_at 时为 pending；分类器目前只检查 usage 版本。这类精确账单已关联但 response 归档丢失的请求通常也没有转发事实，金额仍被其他门槛阻断。本轮未将它扩张为金额误算证据，后续应统一状态语义并补例。

上轮四项修正本轮认可其方向：插件按实际 transport 显式降级、tool_search_output 取 tools、客户端族游标与低频复查、observed UA/version 条件写回。修正后没有必要继续坚持旧临时测试中的“必须返回 nil”或“立即复查”等实现细节断言。

## 4. 验证结果

| 验证 | 结果 |
| --- | --- |
| 六个相关包专项 unit 检查 | config/service/repository/middleware/admin handler 共 169 个顶层测试通过；routes 无匹配用例，仅编译通过 |
| 独立 PostgreSQL 18.1 仓储专项 | 42 个顶层测试通过、0 跳过；22 个来自 integration-tagged 文件，其余为同包单元测试 |
| 新增定向复现 | 8 个顶层测试在正确行为断言处失败；缓存键另含 standard/passthrough 两个子场景；日志分别保留，不混入正式测试通过数 |
| 后端构建 | `go build ./cmd/server` 成功，构建产物留在仓库外且未执行 |
| 格式 | 51 文件中的 Go 源码 gofmt -l 无输出；git diff --check 通过 |
| 清理 | 临时测试已从源码目录删除；本机独立 PostgreSQL 已停止，postmaster.pid 消失 |
| 业务源码保护 | 最终 51 个文件与 reviewed-source-hashes.json 核对；本轮未修改业务实现或正式测试 |

正式命令（backend 目录，仓库本地 Go 缓存和独立 GOTMPDIR）：

```powershell
go test -tags=unit -p 1 -count=1 -json -run 'TokenAnalysis|BootstrapSecrets|SecuritySecret|GenerateHexSecret|SecretNotFound|MigrationExecutionMode|ApplyMigrationsFS|OpenAI.*Plugin' ./internal/config ./internal/service ./internal/repository ./internal/server/middleware ./internal/handler/admin ./internal/server/routes
# SUB2API_POSTGRES_ONLY_INTEGRATION_DSN 指向本次全新本机集群：
go test -tags=integration -p 1 -count=1 -json -run 'TestTokenAnalysis' ./internal/repository
go build -o E:/tmp/token-analysis-branch-review-20261027/sub2api-audit.exe ./cmd/server
```

本轮没有执行全仓库测试、浏览器验收、真实插件端到端或生产规模压测；数据库运行验证仅为 PG 18.1，未把它等同于生产 PG 15 验收。当前也尚无新前端业务代码需要浏览器验收。

## 5. 证据入口与后续修正顺序

证据目录：[2026-10-27-token-analysis-branch-review](../delivery/2026-10-27-token-analysis-branch-review/)。

- 正式测试：[unit-tests.jsonl](../delivery/2026-10-27-token-analysis-branch-review/unit-tests.jsonl)、[integration-tests.jsonl](../delivery/2026-10-27-token-analysis-branch-review/integration-tests.jsonl)。
- 实际复现输出：[service-repros.log](../delivery/2026-10-27-token-analysis-branch-review/service-repros.log)、[repository-repros.log](../delivery/2026-10-27-token-analysis-branch-review/repository-repros.log)。
- 复现源码：同目录 `repro-*.txt`；使用当前包内 fixture，恢复为临时 `_test.go` 后可定向运行。数据库复现只能在独立测试库执行。
- 基线：[reviewed-source-hashes.json](../delivery/2026-10-27-token-analysis-branch-review/reviewed-source-hashes.json)；initial-source-hashes.json 仅用于记录审核开始时的状态。

建议先修 R1/R2，再处理 R3/R4 的并发与失效合同，随后补齐 R5–R8；保留本轮复现作为回归测试。读接口发布金额之前，还必须单独实现和验证设计 10.4 的所有可见性条件。本次只新增审核材料并同步 wiki，未提交、推送或部署。
