# Token Analysis 请求来源识别

日期：2026-10-24。范围：离线归档索引、请求明细列与详情解释。生产环境仅只读调研；本地实现不代表已部署或已回填历史。

## 目标与口径

请求来源回答“这一条模型请求为何再次发起”，不判断账号是不是机器人，也不判断文字是否由 AI 撰写。

| 展示 | 含义 |
| --- | --- |
| 人工 | 本轮有新的用户输入，且没有命中明确自动执行特征；属于推断，不能证明有人亲自点击发送 |
| AI | 工具结果续执行、Agent 通信、明确的子 Agent 或自动任务 |
| — | 旧行尚未识别、请求截断、未知结构或信号不足；不是第三种业务来源 |

仅用于管理分析，不参与鉴权、计费、风控或人员绩效判断。客户端可伪造 role 和 metadata；无法从普通 `role=user` 请求证明真实操作者。自动脚本完全模拟普通用户消息时，需要客户端额外提供可信来源信息才能解决。

## 只读调研证据

- 已读取线上 `/admin/token-analysis`。2026-10-23 文件的索引状态显示 66,778 条请求、该文件失败 0。
- 指定的 `2026-10-23.json1` 不存在，实际文件为 `/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl`，132,582,213,280 bytes（约 123.48 GiB）。
- 在文件的 16 个均匀字节位置附近有界抽样：每窗口最多 20 条 request，循环开始前检查累计读入是否小于 32 MiB（最后一行可使窗口超出预算）；单行上限 20 MiB。实际读取 532,218,034 bytes（正文循环 463,449,925 + 行边界对齐 68,768,109），解析 272 条 request；窗口 #10 读满 20 MiB 仍无行尾即停止该窗口，这是采样行长限制，不代表归档损坏或请求被截断。可逐条回查的脱敏证据包见 `docs/delivery/2026-10-24-token-origin-samples/`。
- 样本端点：Responses 259、Chat Completions 8、Messages 5。该抽样受请求大小和时段分布影响，不是全日随机请求样本，不据此估计人工/AI 总占比或准确率。
- 仅返回结构统计及少量经清洗的片段；不下载原始归档，不保存请求正文、用户标识、客户端元数据 ID，不调用外部模型。

| 样本特征 | 条数 | 结论 |
| --- | ---: | --- |
| Responses 最后项 `custom_tool_call_output` | 156 | 工具结果续执行 |
| Responses 最后项 `function_call_output` | 54 | 工具结果续执行 |
| Chat 最后消息 `role=tool` | 6 | 工具结果续执行 |
| Responses 最后项 `agent_message` | 22 | Agent 通信，不是人工输入 |
| Messages 最后项是 system | 5 | 前一条 user 仅包含 `tool_result`，需跳过 system 尾注 |

还观察到：纯 `<environment_context>` 被放入 user 消息；审批输入也是 user，但 `x-openai-subagent=guardian`；定时任务 `turn_trigger=automation_cron_scheduled`；Copilot 新用户请求位于带附件的 `<userRequest>` 包装中。

现有 `summaryResponsesRequest` 会将 assistant 文本写入 `LastUserPreview`，Anthropic 工具结果也可能被递归抽入预览。因此 `last_user_preview`、净输入全文与 `duplicate_count` 均不能作为来源依据。本功能不修改这些旧字段的统计语义。

## 规则与优先级

1. 归档 `body_truncated=true` 一律未识别，包括仍能解析的 JSON 和 multipart，避免用缺失尾部的历史推断来源。
2. 使用已解析请求里的客户端元数据有限白名单识别子 Agent、审批、自动化。`client_metadata.x-codex-turn-metadata` 支持 JSON 字符串和对象，实际触发键为 `turn_trigger`；生产样本中有值的 252 条均为字符串，对象格式只是兼容处理，未经生产验证。`x-openai-subagent` 的值域为 `collab_spawn`/`guardian`/`memory_consolidation`，嵌套 `subagent_kind` 的值域为 `thread_spawn`/`guardian`，两者分别匹配、不可互换。
3. 从原始协议消息尾部向前寻找有效事件。跳过任意 item 类型的 system/developer（含 Responses Lite `additional_tools`）与完整、纯客户端注入包装（`<environment_context>`、`<system-reminder>`，可多段连续）；未知尾部不能跳过后用历史用户消息补判。
4. 工具输出、Agent 消息、assistant/model 继续生成、压缩事件归 AI；新 user 文本、图片或文件归人工推断。混合工具结果与独立用户内容以独立新输入为依据；与 tool_result 同处的整块 `<system-reminder>` 不算独立输入。
5. 工具声明、历史工具数、重复正文、短时间间隔、User-Agent、用户 ID、`previous_response_id` 不单独构成来源证据。
6. `count_tokens` 探测请求不是模型调用，由索引器按原始路径固定为 `unknown/count_tokens`，优先于截断与压缩覆盖。

自动触发白名单包括 `automation_cron_scheduled`、`goal`、`memory_consolidation`、`guardian_review`、`guardian_classifier`、`ambient_suggestions`、`thread_title`。`composer`、`user`、`queue`、`edit_user_message`、`resume_interrupted_task` 不能单独证明本次请求是人工；这些值会被同一 turn 的后续工具轮次携带。`thread_source=automation` 也不能单独证明自动触发，因为存在 composer 触发的记录。

## 存储与展示合同

无需 schema migration，复用 `token_analysis_request_summaries.summary_json`：

```json
{"request_origin":{"kind":"ai","reason":"tool_result","confidence":"high","version":1}}
```

| 字段 | 合同 |
| --- | --- |
| kind | `human`、`ai`、`unknown` |
| reason | `subagent`、`automation`、`tool_result`、`agent_message`、`assistant_continuation`、`compaction`、`user_input`、`no_signal`、`unsupported_shape`、`body_truncated`、`count_tokens` |
| confidence | `high`：结构或明确客户端标记；`medium`：普通用户输入推断；`none`：无法判断。不是统计概率 |
| version | 当前为 1；前端遇到未知版本或不合法组合时显示未识别 |

分类器位于 `backend/internal/service/token_analysis_origin.go`，摘要阶段复用已解析的 root，索引器最后执行截断覆盖。分类结果只包含固定枚举，不带正文或标识。

列表 API 已返回 `summary_json`，无需新增接口或逐行请求。`TokenAnalysisRequestTable.vue` 在用户后增加“请求来源”，保留两类业务标签；`TokenAnalysisRequestDrawer.vue` 展示判断依据、推断程度和规则版本。前端公共 helper 负责版本和枚举校验、双语说明；不在前端二次猜测来源。

## 历史数据与上线边界

- 新代码只对之后被索引的 request 生效；旧行缺字段显示未识别，不能默认人工。
- 当前“索引当前范围”按 `last_offset` 增量续读，点击它不会重算已读行。
- 历史回填须另行安排受控生产写操作：先保留目标原始归档，在索引互斥且自动索引不会抢跑的窗口，只重置目标文件的索引状态后重新索引。原始归档绝不就地改写。
- `UpdateIndexState` 累加 processed/failed，不能仅把 offset 设为 0，否则计数重复。现有 `archive_id` upsert 幂等；净输入的 `quality_*` 只在 `content_sha256` 不变时保留，提取口径变化导致内容改变时清空；完整解析且确认没有用户正文的行会删除旧净输入（采集关闭、截断、降级不删）。重索引仍会执行旧的匹配/风险逻辑，因此须备份、明确范围和验收计数，不自动执行。
- 文件已被删除时，不能仅根据历史预览可靠回填。本轮未写生产数据库、未触发索引、未修改服务或部署。

## 审核修订（2026-10-24）

审核发现原调研样本 272 条中 Messages 仅 5 条，Anthropic 规则基本未经验证，随后做了以下修订：

| 问题 | 处理 |
| --- | --- |
| Claude Code 的 `<system-reminder>` 未被识别为注入包装：纯 reminder 的 user 消息会被判人工，若与 tool_result 同条也会把工具续执行判成人工 | `<system-reminder>` 与 `<environment_context>` 同列为可跳过的注入包装；包装外有任何文字、或包装未闭合，仍按用户输入处理。生产只读补充检查（`docs/delivery/2026-10-24-token-origin-samples/messages-audit.md`）证实：生产历史中有 assistant 之后的纯 reminder user 消息（旧规则判人工，新规则跳过后判 AI）和“reminder + 用户正文”（两种规则均判人工）；tool_result 与纯 reminder 同条 **0 命中**，审核时依据客户端行为作出的该判断未获生产证实，此组合按防御规则处理 |
| Claude Code/Copilot 的子 Agent 首轮、自动压缩、标题/话题等内部请求，以及脚本/程序直接发出的普通 user 消息（证据包 #175 翻译模板、#193 `stage=preflight` 结构化 JSON + 图片），外形都是普通 user 输入且无显式标记 | 不用 system prompt 指纹或正文内容硬判；在页面“人工”说明和本文中明确这类请求可能显示为人工，上线前用生产 Messages 样本测量占比 |
| `count_tokens` 探测行会被打上人工/AI 标签 | 新增 reason `count_tokens`（kind=unknown），前端合同与双语说明同步 |
| 预览与净输入取的不是用户输入：Responses 把 assistant/Agent 文本写入预览，Anthropic 把 tool_result 内容抽入预览；`LastUserText` 沿用预览，因此净输入、`content_sha256`、`duplicate_count` 在工具轮次上失真 | 预览/净输入只取用户本人写的文本，跳过工具结果与注入包装，工具续执行请求沿用发起该轮的用户输入；`UserChars` 维持旧口径，风险分不变 |

已知限制：若客户端把用户排队发送的新消息包在 `<system-reminder>` 中，该轮会判为 AI。

影响评估更正：审核时把第 1 项定为高优先级、并称 Claude Code 是主力客户端，缺乏依据。两批抽样都不是按请求随机抽取（272 条中 Messages 5 条；1 GiB 定向读取的 1,246 行含 response 行，其中 Messages 请求 11 条），不能据此推断全天客户端份额，第 1 项的总体影响尚未量化，留待上线观察；规则本身保留。

生产补充检查（2026-10-24）：1 GiB 有界定向采样命中 11 条 Claude CLI Messages 请求，10 条有效尾部为工具结果、1 条普通用户文本，未出现 tool_result+纯 reminder 同条混合。选定 5 条完整历史中确认存在纯 reminder、reminder+普通文本，仍不能将其当作工具结果同条混合的实证。位置、行哈希、脚本及边界见 `docs/delivery/2026-10-24-token-origin-samples/messages-audit.md`。

回填影响：预览/净输入口径变化只作用于新索引的行，新旧行同一范围内的 `duplicate_count` 会暂时不一致；历史回填应与来源回填合并为同一次受控重索引，避免对单日上百 GiB 的归档扫两遍。

## 上线观察（两天）

按当前设计上线，先在生产观察两天，再按发现的具体问题修订规则（届时如改变分类口径，`version` 升为 2）。

**生效范围**：只对上线后被索引的行生效。自动索引每 300 秒增量处理昨天和今天的文件，无需手动触发或回填即可积累观察数据；上线前已索引的行显示 `—`。注意：新行的预览/净输入改为“发起本轮的用户输入”，同一 agent 轮次的工具续执行请求会共享同一净输入，`duplicate_count`（×N 徽标）在新行上会明显变大，这是预期口径，不是重复请求激增。

**只读核对 SQL**（把 `:deployed_at` 换成部署完成时间；预览含用户正文，只在授权终端查看）：

```sql
-- 1. 新代码处理的行是否都有来源（期望 0）
SELECT count(*) FROM token_analysis_request_summaries
WHERE indexed_at >= :deployed_at AND NOT (summary_json ? 'request_origin');

-- 2. 按端点看分类分布
SELECT endpoint,
       summary_json->'request_origin'->>'kind'   AS kind,
       summary_json->'request_origin'->>'reason' AS reason,
       count(*)
FROM token_analysis_request_summaries
WHERE indexed_at >= :deployed_at
GROUP BY 1, 2, 3
ORDER BY 1, 4 DESC;

-- 3. 待查样本：未支持结构，以及 Messages 上的人工
SELECT archive_id, event_time, endpoint, summary_json->'request_origin'->>'reason' AS reason
FROM token_analysis_request_summaries
WHERE indexed_at >= :deployed_at
  AND (summary_json->'request_origin'->>'reason' = 'unsupported_shape'
       OR (endpoint LIKE '/v1/messages%' AND summary_json->'request_origin'->>'kind' = 'human'))
ORDER BY event_time DESC
LIMIT 100;
```

**建议的合格判断**（最终由业务决定）：

| 检查 | 建议标准 |
| --- | --- |
| 新行缺来源 | 0 条 |
| `unsupported_shape` | 占非 `count_tokens` 行的比例很低；每种未支持结构都查明原因，决定加规则或接受为未识别 |
| 抽查 | 每个主要端点（responses、chat/completions、messages）在页面抽屉中各看约 20 条人工、20 条 AI，判断依据与请求结构一致；误判记下 `archive_id` 和端点 |
| 重点观察 | `/v1/messages` 的人工里客户端内部请求（子 Agent、压缩、标题）与脚本调用的占比（审核修订第 2 项）；是否出现 tool_result 与纯 reminder 同条的实例 |
| 回退 | 无 schema 变更；回退代码后新行不再写来源，已写入的 `request_origin` 留在 `summary_json` 中无副作用 |

## 验证

后端覆盖 Responses/Chat/Anthropic/Gemini、新用户输入覆盖工具历史、环境尾注、Agent 消息、自动元数据、混合内容、多模态、未知结构、截断和序列化持久化。前端覆盖人工/AI/未识别/旧行、抽屉解释、未知版本及坏数据。

2026-10-24 本地验证结果：

| 验证 | 结果 |
| --- | --- |
| `go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run TokenAnalysis` | 3 个包通过；按 wiki 使用独立 GOTMPDIR 与 review-cache |
| View、API、双语完整性与编译 Vitest 专项 | 4 文件 / 45 测试通过 |
| `pnpm --dir frontend run typecheck`、变更前端范围 ESLint、`git diff --check` | 通过 |
| 应用内浏览器 | 使用实际 Vue 请求表/抽屉组件和合成数据，验证人工、AI、截断、旧行及点击详情；临时页面与 Vite 进程已清理 |
| 独立 subagent 复核 | 未发现阻塞问题；保留普通 user 输入不可证明物理来源的边界 |
| 审核修订后 | `go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run TokenAnalysis` 通过，`./internal/service` 全包通过，gofmt/vet 无输出；前端 View/API/双语专项 4 文件 45 测试、typecheck、ESLint 通过 |
| 生产证据包回放 | 按 `samples.json` 的脱敏结构为 272 条重建请求体，经实际分类器：非 user 结尾 244 条全部判 AI（0 例外）；user 结尾 28 条中 13 条人工、15 条 AI（自动任务/子 Agent/环境包装后接工具或 Agent 消息），与“28 条 user 结尾不等于 28 次人工”一致；15 条重点案例全部符合证据包的建议判断，#175/#193 按设计为人工（推断）。重点案例中新增覆盖的形态以 `#N` 样本号并入 `TestTokenAnalysisRequestOriginUsesLatestEffectiveInput`，元数据类案例由白名单测试覆盖 |
| 生产 Messages 补充检查 | 1 GiB 有界定向读取：11 条 Messages（全部 `claude-cli`），末尾 10 条为不带纯 reminder 的 tool_result、1 条普通 user 文本；5 条完整历史中有纯 reminder user 消息与“reminder + 正文”，无 tool_result+reminder 同条。两种已观察形态以 “prod history” 标注并入 `TestTokenAnalysisRequestOriginUsesLatestEffectiveInput`，预览口径用例同步覆盖 |
| 上线前全量门禁 | Go default 全量 54 包通过；normal/embed build 通过；`go mod tidy -diff` 仅有既有 18 行 go.sum 旧 checksum（本轮未改 go.mod/go.sum）；前端 lint:check、typecheck、Vitest 411 文件 / 3191 测试、production build（1172 modules）通过 |
| 未执行 | 增量 golangci-lint：本机缓存的 v2.13/v2.14 由 go1.27.0 构建，无法读取 go1.27.2 的导出数据，且本机连不上校验和数据库无法重建，由 CI 执行；gofmt 与 go vet 已通过。第 2 项生产占比、tool_result+reminder 实例与历史回填留待上线观察 |

初版实现轮未进行全仓构建；之后的上线前全量门禁（Go default 全量、normal/embed build、前端 production build 等）见上表“上线前全量门禁”一行，实现审核修订后的复验见下一节。全程未执行生产回填或部署。工作区进入任务前已有 Token Analysis UI 拆分，本轮在其组件结构上追加来源展示并保留原改动。

## 实现审核修订（2026-10-24）

对应 `docs/reviews/2026-10-24-token-analysis-request-origin-implementation-review-cn.md`：

| 发现 | 处理 |
| --- | --- |
| F1 回填时提取为空，旧净输入残留 | 新增 `DeleteUserInput`；索引器仅在“启用留存、未截断、未降级、完整解析后确认没有用户正文”时删除该 `archive_id` 的旧净输入，采集关闭、归档截断、解析降级一律不删 |
| F2 内容改变后旧评分仍绑定新正文 | `UpsertUserInput` 的 `quality_score/quality_findings/quality_version/evaluated_at` 改为仅在 `content_sha256` 不变时保留，否则清空（`quality_version` 置空串）。当前代码没有写入这些字段的评估任务，该风险尚未兑现 |
| F3 排序/分页使用未提交草稿 | 概览、列表、排序/翻页续查、用户趋势与“索引当前范围”统一读取 `appliedFilters` 快照，只有显式提交（回车、查询、筛选控件变更、标签移除、重置）经 `reloadAll` 更新快照 |

复验：F1 状态转换用例（确认为空时删除；截断、不可解析截断、留存关闭时保留）、F2 在嵌入式 PostgreSQL 18 新建空库上的集成测试（同哈希保留评估、换哈希清空、删除后读取为空）、F3 View 回归（草稿 model-b 未提交时排序与翻页仍请求 model-a）均通过；后端 Token Analysis 三包与前端专项 4 文件 / 47 测试、typecheck、ESLint 通过。
