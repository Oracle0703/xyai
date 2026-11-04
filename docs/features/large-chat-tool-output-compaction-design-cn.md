# 大 Chat 请求历史 tool 输出压缩设计

更新：2026-11-04。状态：**已合入 main（`4ae1d4830` 配置、`5a1251b5f` 压缩器、`d5df827b9` 范围控制、`6f9779fc1` warn 只读，均 2026-05-25）。默认 `mode: warn`，只观测不改写；`tool_output_compact` 只对名单内的用户/API Key/分组生效。原设计中的 Token Analysis 可观测性（执行计划 Task 6）未实现，`deploy/config.example.yaml` 也没有本配置段（§10）。**

本文是大 Chat Completions 请求历史 `role=tool` 输出压缩功能的唯一文档，包含规则、配置、代码位置、易错点和验证方式。原设计与执行计划已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/superpowers/specs/2026-05-25-large-chat-tool-output-compaction-design.md`、`git show 26f6363af:docs/superpowers/plans/2026-05-25-large-chat-tool-output-compaction.md` 查看）。执行计划除 Task 6 外已落地，代码为准。

阅读顺序建议：
- 改压缩规则：先读 §3、§8；
- 改配置或开启范围：读 §5；
- 定位代码：读 §7；
- 跑测试：读 §9。

## 1. 背景

生产样本（`fixtures/chat-completions-large-20260525-113534-archive-2b9ad158-*.json`，来自请求归档；`fixtures/` 在 `.gitignore` 中，只在本机，含生产内容，不入库）代表一类 Copilot/Codex 代理请求：客户端通过 `/v1/chat/completions` 发送很长的历史上下文，网关转换为 OpenAI Responses API。样本约 1.92MB，474 条 `messages`、88 个 function tools，其中 243 条 `role=tool` 历史输出约 1.44MB。对应响应是 HTTP 200 空流：无 content、无 usage、`finish_reason=stop`。

问题分三层：第一层是“上游 200 但无可见输出、tool call、usage 时不能当作成功空流返回，应 failover 或明确报错”，不在本文范围；本文覆盖第二、三层：大请求观测与可控压缩。

## 2. 目标与非目标

| 目标 | 说明 |
| --- | --- |
| 降低超大请求风险 | 针对大量历史 `role=tool` 输出导致的请求膨胀，减少上游空响应与长尾延迟 |
| 尽量保持代理语义 | 不改 `system`/`developer`、最新 `user`、顶层 `tools`、最近的关键 tool 输出 |
| 小范围可控启用 | 默认全局只观测，压缩只对名单开启 |
| 可回滚 | 改配置即可恢复 |

| 非目标 | 原因 |
| --- | --- |
| 全局默认自动压缩 | Codex/Copilot 类请求对上下文敏感，默认改写历史可能影响任务质量 |
| 裁剪顶层 `tools` 定义 | schema/description 影响模型是否正确调用工具；样本中仅约 100KB |
| 自动生成/维护 `previous_response_id` | 需要可靠会话 ID 和状态绑定，代理层贸然注入可能串上下文 |
| 以 HTTP gzip 为主要方案 | 只减传输体积，不减 token 和上下文 |
| 模型二次摘要 | 额外成本、延迟、不可重复；采用确定性 head/tail 截取 |

## 3. 规则

只处理 `/v1/chat/completions` 转 OpenAI Responses 的路径（`OpenAIGatewayService` 的 Chat Completions 转发，模型映射之后、兼容层 prompt_cache_key 派生之前）。

### 3.1 检测（`CompactLargeChatToolOutputs`）

1. `enabled=false` 或 `mode=off`：直接返回，不统计。
2. 统计每条消息的 `content` 原始 JSON 字节数（`collectLargeToolStats`，即 JSON 编码后的长度，含引号与转义），累计 `role=tool` 条数与字节数，记录最后一条 `role=user` 的下标。
3. 请求体 `> body_threshold_bytes` **且** tool 总量 `> tool_total_threshold_bytes` 才算大请求（`LargeRequestDetected`），否则返回。
4. `mode=warn` 或请求不在开启范围：只返回观测结果，**不改 body**。

### 3.2 候选选择（`selectLargeToolCandidates`，按顺序判断，命中即跳过并计入 `SkipReasons`）

| 顺序 | 条件 | skip reason |
| --- | --- | --- |
| 1 | 最近 `absolute_recent_tool_keep`（6）条 tool 之一 | `absolute_recent_tool_keep` |
| 2 | 位于最后一条 user 之后 | `after_last_user_keep` |
| 3 | content 不是 JSON 字符串（数组/对象等结构化内容） | `non_string_tool_content` |
| 4 | 疑似 base64/二进制（`looksLikeBase64ToolContent`：以 `data:` 开头；或去空白后 ≥32768 字符、长度为 4 的倍数且全为 base64 字符） | `base64_or_binary` |
| 5 | 最近 `recent_tool_keep`（20）条之一且 `<= giant_tool_threshold_bytes`（512KB） | `recent_tool_keep_below_giant_threshold` |
| 6 | `<= normal_tool_threshold_bytes`（128KB） | `below_normal_threshold` |
| — | 其余 | 成为候选 |

即：最近 20 条之外超过 128KB 可压缩；最近 20 条内只有超过 512KB 且不在绝对保护内的才可压缩。

### 3.3 压缩

- 候选按字节数降序（相同时按下标升序），逐条替换；每替换一条重新序列化请求，当前体积 `<= target_body_bytes`（900KB）即停止。压完所有候选仍超预算时照常转发。
- 替换内容（`compactToolContent`）：按 rune 计数，长度不超过 `head_chars + tail_chars` 时不替换；否则保留前 `head_chars`、后 `tail_chars` 个字符：

```text
[Sub2API compressed historical tool output]
original_bytes: <原 content 的 JSON 字节数>
sha256: <原始字符串的 sha256>
kept_head_chars: 8000
kept_tail_chars: 8000
omitted_chars: <省略的字符数>
reason: large historical role=tool output

--- kept head ---
<原文开头>
--- omitted middle ---
--- kept tail ---
<原文结尾>
```

- 被省略的原文不打印、不保存。
- 压缩失败（序列化错误）只记 warn 日志，转发原请求。

### 3.4 prompt_cache_key 注入（`MaybeInjectLargeRequestPromptCacheKey`）

- 条件：`auto_prompt_cache_key=true`、请求在开启范围内（`scope.Enabled`，隐含 `mode=tool_output_compact`）、`LargeRequestDetected`、客户端未传 `prompt_cache_key`。warn 模式不注入（`6f9779fc1`：warn 必须只读）。
- key = `sub2api-large-` + sha256(种子) 的前 32 个十六进制字符；种子 = 模型 + user_id + api_key_id + group_id + 全部 system/developer 消息内容 + 顶层 tools 的 JSON。不用完整 body hash，使稳定前缀可复用；带身份维度防止跨租户共用。
- 注入后重新序列化 body；序列化失败则回滚字段，避免“结构体有 key、透传 body 没有”的不一致。注入的 key 同时作为本次请求的 `promptCacheKey`，此时不再走兼容层 `deriveCompatPromptCacheKey`。
- `apicompat.ChatCompletionsRequest.PromptCacheKey` 在 Chat → Responses 转换中透传（`chatcompletions_to_responses.go`）。
- 它只能降低重复前缀的延迟与成本，不减少请求体和上下文 token，不能替代压缩。

### 3.5 生产样本试算（2026-05-25，离线，未打印原文）

| 指标 | 结果 |
| --- | ---: |
| 原始请求体 | 1,922,708 bytes |
| `role=tool` 总量 / 条数 | 1,439,387 bytes / 243 |
| 候选 / 实际压缩 | 1 条（message index 450） |
| 该条原始 → 压缩后 | 591,817 → 17,230 bytes |
| 最终请求体 | 1,348,121 bytes（节省 574,587 bytes，29.88%） |

index 450 在最近 20 条 tool 内，但不在最近 6 条、不在最后 user 之后，且超过 512KB，因此可压缩。压缩后仍高于 900KB 目标，按规则照常转发。

## 4. 开启范围（`LargeRequestScopeFromConfig`）

- 仅当 `enabled=true` 且 `mode=tool_output_compact`，且请求的 user_id、api_key_id、group_id 任一命中对应名单时为开启；三个名单都为空时视为不开启（不会变成全局压缩）。
- 身份来自 `largeRequestIdentityFromContext`：从 gin 上下文 `api_key`（`*APIKey`）取 `ID`、`UserID`、`GroupID`；上下文显式设置的 `user_id`、`api_key_id`、`group_id` int64 值优先（单测后备路径）。

## 5. 配置

`gateway.large_request`（`backend/internal/config/config.go` 的 `GatewayLargeRequestConfig`）：

| 配置 | 默认 | 说明 |
| --- | ---: | --- |
| `enabled` | `true` | 总开关；关闭等同 `off` |
| `mode` | `warn` | `off` / `warn` / `tool_output_compact`；空值按 `warn`，其他值启动校验失败 |
| `body_threshold_bytes` | 1048576 | 必须为正 |
| `tool_total_threshold_bytes` | 524288 | 必须为正 |
| `normal_tool_threshold_bytes` | 131072 | 必须为正 |
| `giant_tool_threshold_bytes` | 524288 | 必须 `>= normal_tool_threshold_bytes` |
| `recent_tool_keep` | 20 | 非负 |
| `absolute_recent_tool_keep` | 6 | 非负且 `<= recent_tool_keep` |
| `target_body_bytes` | 921600 | 压缩停止条件；≤0 时压完所有候选 |
| `head_chars` / `tail_chars` | 8000 / 8000 | 必须为正 |
| `enabled_user_ids` / `enabled_api_key_ids` / `enabled_group_ids` | `[]` | 开启名单 |
| `auto_prompt_cache_key` | `true` | 仅在开启范围内生效 |

配置在启动时加载，修改需重启；没有后台热改入口。`deploy/config.example.yaml` 目前没有该段。

上线顺序（原设计）：先合入第一层空响应兜底 → 默认 `warn` 观测 → 对单个测试 API Key 开 `tool_output_compact` → 对比 TTFT、空响应、usage 缺失、压缩命中率 → 再扩大到指定用户或分组。回滚：`mode` 改回 `warn` 或 `off` 并重启。

## 6. 可观测性

现状：命中大请求（`LargeRequestDetected`）时，`logLargeChatToolCompactionResult` 输出一条 Info 结构化日志 `openai chat_completions: large request observed`，字段：`mode`、`request_body_size_before`、`request_body_size_after`、`tool_output_bytes`、`tool_message_count`、`compacted`、`compressed_tool_messages`、`compressed_original_bytes`、`compressed_final_bytes`、`prompt_cache_key_injected`、`account_id`。

未实现：原设计要求把这些字段写入 Token Analysis 的 request summary `summary_json` 或风险原因（执行计划 Task 6，涉及 `token_analysis_types.go`、`token_analysis_summary.go`、`token_analysis_risk.go`），当前代码中没有任何相关字段。`SkipReasons` 与 `CompressedMessageIndices` 只存在于结果结构体，没有输出到日志。

## 7. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/internal/service/large_chat_tool_compaction.go` | 常量 `LargeRequestModeOff`/`Warn`/`ToolOutputCompact`；`LargeRequestScope`、`LargeRequestIdentity`、`LargeChatToolCompactionResult`；`LargeRequestScopeFromConfig`、`CompactLargeChatToolOutputs`、`MaybeInjectLargeRequestPromptCacheKey`、`collectLargeToolStats`、`selectLargeToolCandidates`、`normalizeLargeRequestMode`、`largeRequestPromptCacheSeed`、`compactToolContent`、`looksLikeBase64ToolContent` |
| `backend/internal/service/openai_gateway_chat_completions.go` | 接入点（转发函数内 `if s.cfg != nil` 段）、`largeRequestIdentityFromContext`、`ginContextInt64Value`、`logLargeChatToolCompactionResult` |
| `backend/internal/pkg/apicompat/types.go`、`backend/internal/pkg/apicompat/chatcompletions_to_responses.go` | `ChatCompletionsRequest.PromptCacheKey` 与转换透传 |
| `backend/internal/config/config.go` | `GatewayLargeRequestConfig`、默认值与 `Validate` 中的 `gateway.large_request.*` 校验 |

## 8. 关键约束与易错点

1. **warn 模式必须只读**：不改 body、不注入 prompt_cache_key；只有 `Compacted` 或 `PromptCacheKeyInjected` 为真时才替换转发用的 `chatReq`/`body`。依据：提交 `6f9779fc1` 修复了 warn 模式改写请求的问题。
2. **名单全空不得等于全局开启**：`LargeRequestScopeFromConfig` 在三个名单都为空时返回关闭。
3. **最近 6 条 tool 与最后一条 user 之后的消息永不压缩**，即使超过巨型阈值。依据：刚产生的工具结果是当前任务的关键上下文。
4. **只压缩字符串内容**；结构化 content 与疑似 base64/二进制跳过，避免破坏结构或产生无意义的截断。
5. **字节阈值按 content 的原始 JSON 长度计算**（含转义），不是解码后的 UTF-8 长度；head/tail 按 rune 截取，不会切断多字节字符。
6. **客户端已传 `prompt_cache_key` 时不得覆盖**。
7. **注入失败必须回滚结构体字段**，保持 struct 与 body 一致。
8. 压缩改变了发往上游的历史内容，被压缩消息之后的上游 prompt cache 前缀会失效；扩大名单前应对比缓存命中率。
9. 修改配置需重启；不要假设后台可热改。

已否决：模型二次摘要、裁剪顶层 tools、自动维护 `previous_response_id`、以 gzip 代替压缩、全局默认压缩，原因见 §2。

## 9. 验证

### 9.1 命令

```bash
cd backend
go test -tags=unit ./internal/service/ -run 'LargeChatToolOutputs|LargeRequestScope|LargeRequestPromptCacheKey|ForwardAsChatCompletionsCompactsLargeToolOutput|ForwardAsChatCompletionsWarnModeDoesNotMutateLargeRequest'
go test -tags=unit ./internal/config/ -run TestLoadDefaultGatewayLargeRequestConfig
go test -tags=unit ./internal/pkg/apicompat/
```

### 9.2 测试清单

- `backend/internal/service/large_chat_tool_compaction_test.go`：`TestCompactLargeChatToolOutputsSyntheticProductionShape`（合成的生产形状，不读取 `fixtures/`）、`TestCompactLargeChatToolOutputsWarnModeDoesNotMutate`、`TestCompactLargeChatToolOutputsRespectsAbsoluteRecentToolKeep`、`TestCompactLargeChatToolOutputsKeepsMessagesAfterLastUser`、`TestLargeRequestScopeFromConfigMatchesAPIKey`、`TestLargeRequestScopeFromConfigNoMatch`、`TestMaybeInjectLargeRequestPromptCacheKeyDoesNotOverrideClientValue`、`TestMaybeInjectLargeRequestPromptCacheKeyUsesStablePrefix`。
- `backend/internal/service/openai_gateway_chat_completions_test.go`：`TestForwardAsChatCompletionsCompactsLargeToolOutputForEnabledAPIKey`、`TestForwardAsChatCompletionsWarnModeDoesNotMutateLargeRequest`。
- `backend/internal/config/config_test.go`：`TestLoadDefaultGatewayLargeRequestConfig`。
- 缺口：没有“最近 20 条普通保护”“非字符串/base64 跳过”“目标预算停止”的独立用例。

### 9.3 最近结果

本次文档合并未运行；没有保留的历史运行记录。

## 10. 状态与待办

- [ ] 生产是否已对任何名单开启 `tool_output_compact`：仓库内无记录，需向运维确认。
- [ ] Token Analysis 可观测性（原执行计划 Task 6）：把大请求字段写入 request summary 或风险原因。
- [ ] 在 `deploy/config.example.yaml` 补 `gateway.large_request` 示例段。
- [ ] 补 §9.2 列出的测试缺口；日志补 `SkipReasons`。
