# Grok 4.x 标准版自动思考强度转发设计

WeKnora 的界面和智能体继续使用 `auto`。它的 generic 客户端把“开启思考、强度由服务端决定”发成 `chat_template_kwargs.enable_thinking: true`，不发字面量 `auto`。当最终上游型号是 Grok 4.7 及之后的标准版时，中转站把这个开关或字面量 `auto` 换成 `high`。其余请求按客户端原样转发，包括显式档位、关闭思考和其他厂商的开关写法。

## 1. 实际请求与预期出站

下表仅展示与转发有关的字段，假设账号将 `assistant` 映射到 `grok-4.7`，且分组允许 `high`。

| 客户端请求片段 | 上游协议 | 出站片段 |
| --- | --- | --- |
| `{"model":"assistant","chat_template_kwargs":{"enable_thinking":true}}` | Chat Completions | `{"model":"grok-4.7","reasoning_effort":"high"}` |
| 同一份 WeKnora 请求，经 Chat → Responses 转换 | Responses | `{"model":"grok-4.7","reasoning":{"effort":"high"}}` |
| `{"model":"assistant","reasoning":{"effort":"auto"}}` | Chat Completions，经 Responses → Chat 转换 | `{"model":"grok-4.7","reasoning_effort":"high"}` |
| `{"model":"assistant","reasoning_effort":"low","chat_template_kwargs":{"enable_thinking":true}}` | Chat Completions | 本功能不改写，思考字段按原有规则转发 |
| `{"model":"assistant","chat_template_kwargs":{"enable_thinking":false}}` | Chat Completions | 本功能不改写，思考字段按原有规则转发 |

WeKnora generic 默认不发档位。智能体选 low、medium 或 high 时，线上请求同样只有布尔开关，因此也得到 `high`。要保留档位，在 WeKnora 的模型配置里开启 `supports_reasoning_effort`，请求会带显式 `reasoning_effort`，中转站不改写。

## 2. 哪些模型会转换

适用于 Grok 账号和 OpenAI 兼容 API Key 账号。先按当前转发路径解析模型映射，再识别 xAI 支持的提供商前缀及别名。请求名为 `assistant`、最终解析到 4.7 时命中；请求名为 `grok-4.7`、最终映射到 4.6 时不转换。

转换需同时满足两个条件：

1. 最终型号是 `grok-4.N` 或 `grok-4.N-latest`，N 为无前导零的十进制整数，N ≥ 7 且 N ≠ 20。
2. 该型号已列入 Grok 出站能力表 `grokSupportsReasoningEffort`。当前符合的只有 4.7。

能力表随上游 sub2api 同步维护，本功能不修改它。4.8 等新标准版进入能力表后转换自动生效；在此之前对它的请求不转换，避免把 `high` 发给尚未确认接受该参数的型号。`grok-4.6` 及更早版本、`grok-4.20` 系列、带 `-fast` 或 `-build` 等后缀的变体和其他主版本都不转换。

## 3. 触发条件与改写范围

| 请求情形 | 网关处理 |
| --- | --- |
| 显式 effort 为 `auto` | Chat 依次读取 `reasoning_effort`、`reasoningEffort`、`reasoning.effort`。Responses 依次读取 `reasoning.effort`、`reasoning_effort`、`reasoningEffort`。取第一个存在的字段，其值为 `auto` 时转换，忽略大小写及首尾空格 |
| 没有任何 effort 字段，且 `chat_template_kwargs.enable_thinking` 为 JSON 布尔值 `true` | 转换。出现 `thinking:false`、`thinking.type:"disabled"`，或顶层 `enable_thinking`、`reasoning.enabled` 存在且不为 `true` 时视为冲突，不转换 |
| 其余所有请求 | 请求体逐字节不变。包括显式档位（含 `none`、`null`、空串和未知值）、关闭开关，以及只带顶层 `enable_thinking` 或 `reasoning.enabled` 的请求 |

转换时写入对应协议的 effort 字段，删除其余 effort 拼写和 `chat_template_kwargs.enable_thinking`。`chat_template_kwargs` 的其他键、`reasoning.summary` 等独立选项保留，被删空的对象一并移除。

## 4. 转发、分组与计费

1. HTTP 在选定账号、解析上游模型后处理原始 JSON，再进入 Chat / Responses 直转或协议转换。这样类型化转换器丢弃扩展字段之前，网关已经读到 WeKnora 的开关。流式和非流式共用同一处理。
2. Responses WebSocket 按每个 `response.create` 的实际模型处理，覆盖首帧、后续轮次以及 Grok 的 WebSocket → HTTP 转发。上一轮的自动强度不会成为下一轮的默认值。
3. 适用 OpenAI 分组策略时，以**客户端请求模型名**匹配策略，将推导出的 `high` 交给已有精确映射、上限和拒绝规则。例如，上限为 `medium` 且允许降档时，实际发送 `medium`。若策略拒绝，HTTP 在出站前返回 403，WebSocket 拒绝该轮。原生 Grok 分组没有该策略。
4. 对本次转换，用量记录的 `requested_reasoning_effort` 保存 `auto`，`reasoning_effort` 保存最终出站档位。计费使用后者。例如，发生降档时，两列分别为 `auto` 和 `medium`。布尔开启与字面量 `auto` 表达相同意图，在请求强度列中均记为 `auto`。

配置与数据范围限于中转站的请求处理。WeKnora 无需升级或迁移已保存智能体，文档解析用的 `thinking_enabled` 独立于本设计。此功能不需要新增环境变量、管理端映射项或数据库迁移。

## 5. 实现与验证

| 职责 | 仓库位置 |
| --- | --- |
| 模型判定、触发条件、字段改写 | [openai_gateway_grok_effort.go](../../backend/internal/service/openai_gateway_grok_effort.go) 中的 `grokDefaultsAutoEffortToHigh`、`normalizeGrokAutoEffort`、`grokGenericThinkingEnabled` |
| HTTP 入口 | [Chat 入口](../../backend/internal/service/openai_gateway_chat_completions.go)、[Responses 入口](../../backend/internal/service/openai_gateway_forward.go) |
| Grok 出站能力表（只读，未改动） | [openai_gateway_grok.go](../../backend/internal/service/openai_gateway_grok.go) 中的 `grokSupportsReasoningEffort` |
| 分组规则及模型匹配 | [分组上下文](../../backend/internal/handler/composite_platform.go)、[强度策略](../../backend/internal/service/openai_reasoning_effort_policy.go) |
| WebSocket 按轮处理 | [WS 入口](../../backend/internal/service/openai_ws_forwarder_ingress.go)、[WS 透传](../../backend/internal/service/openai_ws_v2_passthrough_adapter.go) |
| 转换、不改写的正常调用、策略拒绝 | [Grok 自动强度测试](../../backend/internal/service/openai_gateway_grok_effort_test.go)、[WS → HTTP 测试](../../backend/internal/service/openai_ws_http_bridge_test.go) |

在 `backend/` 下运行专项回归：

```sh
go test -tags=unit -count=1 ./internal/service -run 'GrokAuto|ProxyResponsesWebSocketFromClientForGrok'
```

2026-11-20 本地验证：上述专项通过，`internal/service`、`internal/handler`、`internal/pkg/xai`、`internal/pkg/apicompat` 四个包的全量 unit 测试通过。模拟上游覆盖 4.7 的四条 HTTP 路径（两种协议直转及双向转换）和两种流式设置，共 8 种组合，并检查出站请求体与用量记录。另有用例确认关闭开关、其他厂商开关和无关 `chat_template_kwargs` 键原样到达第三方上游，4.8 等未入能力表的型号不转换。

当前实现尚未部署。真实 xAI 对 `high` 的接受情况和实际思考强度仍待真实上游验证。
