# Large Chat Tool Output Compaction Design

## 背景

生产 fixture `fixtures/chat-completions-large-20260525-113534-archive-2b9ad158-request.json` 代表了一类 Copilot/Codex 代理请求：客户端通过 `/v1/chat/completions` 发送很长的历史上下文，服务端再转换为 OpenAI Responses API。该样本约 1.92 MB，包含 474 条 `messages`、88 个 function tools，其中 243 条 `role=tool` 历史输出约 1.44 MB。对应响应是 HTTP 200 空流，无 content、无 usage、`finish_reason=stop`。

已确认第一层兜底是：上游 HTTP 200 但没有可见输出、tool call、usage 时，不能作为成功空流返回，应进入 failover 或返回明确错误。本设计覆盖第二层和第三层：大请求保护、可控压缩、可观测性。

## 目标

| 目标 | 说明 |
| --- | --- |
| 降低超大 Chat Completions 请求风险 | 针对大量历史 `role=tool` 输出导致的请求膨胀，减少上游空响应和长尾延迟概率 |
| 保持代理语义尽量稳定 | 不修改 `system`/`developer`、最新 `user`、顶层 `tools` 定义、最近关键 tool 输出 |
| 小范围可控启用 | 默认全局只观测，自动压缩只对指定用户、API Key 或分组开启 |
| 可回滚、可审计 | 配置关闭即可恢复；日志和 token 分析展示压缩前后体积和原因 |

## 非目标

| 非目标 | 原因 |
| --- | --- |
| 不做全局默认自动压缩 | Codex/Copilot 类请求对上下文敏感，默认改写历史可能影响任务质量 |
| 不裁剪顶层 `tools` 定义 | 工具 schema/description 影响模型是否正确调用工具，风险高且 fixture 中仅约 100 KB |
| 不自动生成或维护 `previous_response_id` | 需要可靠会话 ID 和状态绑定，代理层贸然注入可能串上下文 |
| 不把 HTTP gzip 当主要方案 | gzip 只减少传输体积，不减少 token 和上下文压力 |
| 不使用模型二次摘要 | 会引入额外成本、延迟和不可重复性；第一版采用确定性 head/tail 压缩 |

## 触发范围

第一版只处理 `/v1/chat/completions` 转 OpenAI Responses 的路径。仅当配置命中指定用户、API Key 或分组时，才允许自动压缩。未命中时只记录大请求风险。

| 条件 | 默认值 | 说明 |
| --- | ---: | --- |
| 请求体触发阈值 | 1 MB | 低于该阈值不做压缩 |
| `role=tool` 总量触发阈值 | 512 KB | 避免对普通长 system/user 请求误处理 |
| 普通单条 tool 压缩阈值 | 128 KB | 最近 20 条 tool 之外，超过该值可压缩 |
| 巨型单条 tool 压缩阈值 | 512 KB | 最近 20 条 tool 内也可压缩，但不能突破绝对保护 |
| 普通最近 tool 保留 | 20 条 | 普通大小的最近工具结果保留原文 |
| 绝对最近 tool 保留 | 6 条 | 永远不压缩，防止破坏刚产生的关键工具结果 |
| 最后一条 user 之后 | 永远保留 | 当前轮问题之后的上下文不压缩 |
| head/tail 保留 | 各 8000 chars | 保留工具输出开头和结尾 |
| 目标请求体预算 | 900 KB | 从最老/最大候选开始压缩，低于目标后停止；第一版可只作为停止条件，不强制达成 |

## 压缩规则

1. 解析 Chat Completions JSON，统计请求体大小、message 数、顶层 tools 数、`role=tool` 条数和字节数。
2. 若未达到请求体阈值或 `role=tool` 总量阈值，则不压缩，只记录观测指标。
3. 找到所有 `role=tool` 消息，建立保护集合：
   - 最近 6 条 `role=tool` 绝对保护；
   - 最后一条 `role=user` 之后的全部消息绝对保护；
   - 最近 20 条 `role=tool` 普通保护。
4. 候选选择：
   - 最近 20 条之外，单条 `role=tool` 内容超过 128 KB，可压缩；
   - 最近 20 条内，若单条超过 512 KB，且不在绝对保护集合中，也可压缩；
   - 非字符串内容、疑似二进制/base64 内容第一版不压缩，只记录告警。
5. 候选按节省潜力排序，从最大历史 tool 输出开始压缩，达到目标请求体预算后停止。
6. 替换内容使用确定性摘要块，不打印或保存原始完整内容。

压缩后的 `role=tool` 内容格式：

```text
[Sub2API compressed historical tool output]
original_bytes: 591817
sha256: <sha256 of original tool content>
kept_head_chars: 8000
kept_tail_chars: 8000
omitted_chars: <count>
reason: large historical role=tool output

--- kept head ---
实际保留的原始 tool 输出开头内容，最多 8000 chars
--- omitted middle ---
--- kept tail ---
实际保留的原始 tool 输出结尾内容，最多 8000 chars
```

## Fixture 试算

使用生产 fixture 离线试算，不打印敏感原文。

| 指标 | 结果 |
| --- | ---: |
| 原始请求体 | 1,922,708 bytes |
| `role=tool` 总内容 | 1,439,387 bytes |
| `role=tool` 条数 | 243 |
| 压缩候选 | 1 条 |
| 实际压缩 | message index 450 |
| 该条原始大小 | 591,817 bytes |
| 该条压缩后大小 | 17,230 bytes |
| 最终请求体 | 1,348,121 bytes |
| 节省 | 574,587 bytes |
| 节省比例 | 29.88% |

index 450 虽然在最近 20 条 `role=tool` 内，但它不在最近 6 条绝对保护内，也不在最后 user 之后，并且超过 512 KB 巨型阈值，因此允许压缩。

## Prompt Cache Key

自动注入 `prompt_cache_key` 可以作为辅助优化，但不能替代压缩。它不减少 HTTP 请求体，也不减少上下文 token，只能帮助上游在重复稳定前缀时降低延迟和成本。

第一版可在大请求命中时自动注入 `prompt_cache_key`，前提是客户端没有传该字段。key 不使用完整 body hash，而使用较稳定的前缀维度：

| 组成 | 说明 |
| --- | --- |
| 模型 | 防止不同模型混用 |
| 用户/API Key/分组 | 防止跨租户缓存语义混杂 |
| system/developer 摘要 hash | 稳定提示前缀 |
| 顶层 tools schema hash | 工具定义前缀 |

如果 Chat Completions 结构体当前不支持 `prompt_cache_key` 字段，需要补充字段并在 Chat Completions 到 Responses 的转换中透传或注入。

## 可观测性

| 字段 | 用途 |
| --- | --- |
| `large_request_detected` | 是否命中大请求保护 |
| `tool_output_bytes` | `role=tool` 内容总量 |
| `compressed_tool_messages` | 压缩条数 |
| `compressed_original_bytes` | 被压缩内容原始总字节 |
| `compressed_final_bytes` | 压缩后总字节 |
| `request_body_size_before` | 压缩前请求体 |
| `request_body_size_after` | 压缩后请求体 |
| `prompt_cache_key_injected` | 是否注入缓存 key |
| `compaction_mode` | `off` / `warn` / `tool_output_compact` |

这些字段应进入结构化日志，并尽量进入 token analysis 的 request summary `summary_json` 或风险原因，方便在运维页面定位。

## 错误处理

| 场景 | 行为 |
| --- | --- |
| JSON 解析失败 | 不压缩，走原有错误处理 |
| 内容不是字符串 | 不压缩该条，记录 skip reason |
| 疑似 base64/二进制 | 不压缩该条，记录 skip reason |
| 压缩后仍超过目标 | 继续转发，但记录仍超预算 |
| 上游 200 空响应 | 由第一层兜底处理为 failover 或明确错误 |

## 配置

建议新增网关配置段：

```yaml
large_request:
  enabled: true
  mode: warn # off | warn | tool_output_compact
  body_threshold_bytes: 1048576
  tool_total_threshold_bytes: 524288
  normal_tool_threshold_bytes: 131072
  giant_tool_threshold_bytes: 524288
  recent_tool_keep: 20
  absolute_recent_tool_keep: 6
  target_body_bytes: 921600
  head_chars: 8000
  tail_chars: 8000
  enabled_user_ids: []
  enabled_api_key_ids: []
  enabled_group_ids: []
  auto_prompt_cache_key: true
```

默认 `mode: warn`，只观测不改请求。生产上先对指定 API Key 切到 `tool_output_compact`。

## 测试

| 测试 | 预期 |
| --- | --- |
| fixture 压缩回归 | 使用脱敏 fixture，确认只压缩 index 450，最终体积约 1.35 MB |
| 最近 6 条 tool 保护 | 即使超过 512 KB，也不压缩 |
| 最后一条 user 之后保护 | 当前轮之后的 tool 不压缩 |
| 最近 20 条普通保护 | 128 KB 到 512 KB 的最近 tool 不压缩 |
| 最近 20 条巨型例外 | 超过 512 KB 且不在绝对保护内，允许压缩 |
| 非字符串/base64 跳过 | 不破坏结构化或二进制内容 |
| prompt_cache_key 不覆盖 | 客户端已传时不覆盖 |
| 默认 warn 模式 | 只记录指标，转发 body 不变 |

## 上线策略

| 阶段 | 动作 |
| --- | --- |
| 1 | 合入第一层空响应兜底 |
| 2 | 上线大请求观测，默认 `warn` |
| 3 | 对单个测试 API Key 开启 `tool_output_compact` |
| 4 | 对比 TTFT、空响应、usage 缺失、压缩命中率 |
| 5 | 视效果扩大到指定用户或分组 |

回滚只需把 `large_request.mode` 改回 `warn` 或 `off`。
