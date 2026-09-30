# 上游合并复核报告：`9a2f11b4e` / `0.1.150`

- 复核日期：2026-07-10
- 复核人：Copilot（人工复核 Codex 合并结果）
- 工作分支：`feature/hy/0621_敏感词过滤`
- 合并提交：`61fec21ade8c1214f2426158ea698b8f2f5f1e9a`
- 合并父母：
  - ours：`4b1bb6cb09605fbdc2194b7804b2212c3bee084e`
  - theirs / 上游 head：`9a2f11b4e21763cb7003ea29921d9a672ab50b1f`
- merge-base：`ddb1a210ce6742ebd4bd5339b9a0c8f309bcbbf0`
- 上游版本：`0.1.150`

---

## 1. 结论摘要

| 等级 | 结论 |
|---|---|
| 冲突标记残留 | 无 |
| 冲突文件测试合并 | 正确（两边测试都保留） |
| 上游 GPT-5.6 cache write 能力表面合入 | 是 |
| **冲突/组合路径语义** | **有错误** |
| 影响 | 官方嵌套 `cache_write_tokens` 可能被本地兼容 helper 清零，导致 cache creation 计费/统计丢失 |
| 证据 | 合并提交上的 `TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes` 失败：`expected 4, actual 0` |

**一句话：**

> Codex 把上游 `openAIUsageFromGJSON`（能识别 `cache_write_tokens`）和本地 `applyOpenAICompatibleCacheUsageFromJSON`（兼容 `prompt_cache_hit_tokens` 等）叠在一起时，本地 helper 用“直接赋值”覆盖了上游已解析出的 cache write，造成功能回退。

---

## 2. 合并范围

### 2.1 上游增量（相对上一轮 `ddb1a210c` / 0.1.149）

主要提交：

| Commit | 说明 |
|---|---|
| `4a2b10c94` | feat(openai): support GPT-5.6 cache write billing |
| `383f61d0e` | fix(openai): align GPT-5.6 billing with official pricing |
| `062af81fb` | fix(openai): preserve explicit GPT-5.6 cache write prices |
| `0a5f34a2e` | fix(openai): recognize Windows websocket resets |
| `b9b013a08` | fix(usage): WS passthrough effort 提取补入映射后模型候选 |
| `dda8f7873` / `ea9f40b63` | admin user breakdown `request_type` 解析与前端类型修复 |
| `0dec1ad29` | 消除 `isOpenAIGPT56Model` 重复声明，统一到 `openai_model_alias.go` |
| `9a2f11b4e` | VERSION → `0.1.150` |

### 2.2 合并提交声明的冲突文件

```text
# Conflicts:
#       backend/internal/pkg/apicompat/chatcompletions_responses_test.go
#       backend/internal/service/openai_gateway_chat_completions_raw.go
```

### 2.3 双边改动（MM）相关文件

除 2 个声明冲突文件外，以下文件在合并中也是双方都改过（自动/手工合并）：

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`
- `backend/internal/pkg/apicompat/types.go`
- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/openai_gateway_request_body.go`
- `backend/internal/service/openai_gateway_response_handling.go`
- `backend/internal/service/openai_gateway_service_test.go`
- `backend/internal/service/openai_gateway_usage.go`

---

## 3. 冲突文件逐项复核

### 3.1 `chatcompletions_responses_test.go` — 正确

`git show --cc 61fec21ad -- .../chatcompletions_responses_test.go` 显示两边测试都保留：

| 侧 | 测试 | 状态 |
|---|---|---|
| 本地 | `TestChatCompletionsToResponses_PromptCacheKey` | 保留 |
| 上游 | `TestUsageConversionsPreserveCacheWriteTokens` | 保留 |

这部分没有问题。

### 3.2 `openai_gateway_chat_completions_raw.go` — 有错误

冲突点在 `extractCCStreamUsage`。

#### 上游意图（`9a2f11b4e`）

把旧的手写字段提取：

```go
u := OpenAIUsage{
    InputTokens:  int(gjson.Get(payload, "usage.prompt_tokens").Int()),
    OutputTokens: int(gjson.Get(payload, "usage.completion_tokens").Int()),
}
if cached := gjson.Get(payload, "usage.prompt_tokens_details.cached_tokens"); cached.Exists() {
    u.CacheReadInputTokens = int(cached.Int())
}
```

改成统一解析器：

```go
u, ok := openAIUsageFromGJSON(usageResult)
if !ok {
    return nil
}
return &u
```

`openAIUsageFromGJSON` 能识别：

- cache read：`input/prompt_tokens_details.cached_tokens`、`cache_read_input_tokens` 等
- cache write/creation：`*.cache_write_tokens`、`cache_creation_input_tokens` 等

#### 本地意图（ours）

在 usage 提取后继续调用：

```go
applyOpenAICompatibleCacheUsageFromJSON([]byte(payload), &u, "usage")
```

目的是兼容第三方字段，尤其是：

- `prompt_cache_hit_tokens`（DeepSeek 等）
- 顶层 `cache_read_input_tokens` / `cache_creation_input_tokens`
- Anthropic 风格 5m/1h cache creation 明细

#### 合并结果（`61fec21ad`）

```go
func extractCCStreamUsage(payload string) *OpenAIUsage {
    usageResult := gjson.Get(payload, "usage")
    if !usageResult.Exists() || !usageResult.IsObject() {
        return nil
    }
    u, ok := openAIUsageFromGJSON(usageResult)
    if !ok {
        return nil
    }
    applyOpenAICompatibleCacheUsageFromJSON([]byte(payload), &u, "usage")
    return &u
}
```

**表面看是“两边都保留”，语义上却互相打架。**

---

## 4. 根因：本地 helper 无条件覆盖上游已解析值

### 4.1 合并提交中的本地 helper（有问题版本）

文件：`backend/internal/service/openai_gateway_response_handling.go`

```go
func applyOpenAICompatibleCacheUsageFromJSON(data []byte, usage *OpenAIUsage, usagePath string) {
    // ...
    cacheCreation := int(gjson.GetBytes(data, path("cache_creation_input_tokens")).Int())
    if cacheCreation == 0 {
        // 仅回退 ephemeral 5m/1h
        cacheCreation5m := ...
        cacheCreation1h := ...
        if cacheCreation5m > 0 || cacheCreation1h > 0 {
            cacheCreation = cacheCreation5m + cacheCreation1h
        }
    }
    usage.CacheCreationInputTokens = cacheCreation // 直接赋值，可为 0

    cacheRead := int(gjson.GetBytes(data, path("cache_read_input_tokens")).Int())
    // 若干 fallback，含 prompt_cache_hit_tokens
    usage.CacheReadInputTokens = cacheRead // 直接赋值
}
```

问题点：

1. **只认** 顶层 `cache_creation_input_tokens` 和 5m/1h ephemeral
2. **不认** 上游新增的官方嵌套字段：
   - `input_tokens_details.cache_write_tokens`
   - `prompt_tokens_details.cache_write_tokens`
   - `cache_write_tokens` / `cache_write_input_tokens` 等
3. 即使 `openAIUsageFromGJSON` 已经把 `CacheCreationInputTokens` 设为正确值，helper 仍会用 `0` **覆盖**

### 4.2 上游解析器（正确识别 cache write）

同文件中的：

```go
func openAICacheCreationTokensFromUsage(value gjson.Result) int {
    return firstPositiveGJSONInt(
        value.Get("input_tokens_details.cache_write_tokens"),
        value.Get("prompt_tokens_details.cache_write_tokens"),
        value.Get("input_tokens_details.cache_creation_tokens"),
        value.Get("prompt_tokens_details.cache_creation_tokens"),
        value.Get("cache_write_tokens"),
        value.Get("cache_creation_input_tokens"),
        value.Get("cache_write_input_tokens"),
        value.Get("cache_creation_tokens"),
    )
}
```

### 4.3 失败路径时序

典型 payload（上游测试用例）：

```json
{
  "id": "resp_1",
  "usage": {
    "input_tokens": 9,
    "output_tokens": 5,
    "input_tokens_details": {
      "cached_tokens": 2,
      "cache_write_tokens": 4
    }
  }
}
```

| 步骤 | 函数 | `CacheCreationInputTokens` |
|---|---|---|
| 1 | `openAIUsageFromGJSON` | `4`（正确） |
| 2 | `applyOpenAICompatibleCacheUsageFromJSON` 找不到顶层 `cache_creation_input_tokens` | 算出 `0` |
| 3 | 直接赋值覆盖 | **`0`（错误）** |

同一问题不仅在冲突文件 `extractCCStreamUsage`，也在：

```go
func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
    if usage, ok := openAIUsageFromGJSON(gjson.GetBytes(body, "usage")); ok {
        applyOpenAICompatibleCacheUsageFromJSON(body, &usage, "usage")
        return usage, true
    }
    usage, ok := openAIUsageFromGJSON(gjson.GetBytes(body, "response.usage"))
    if ok {
        applyOpenAICompatibleCacheUsageFromJSON(body, &usage, "response.usage")
    }
    return usage, ok
}
```

这是更关键的统一入口，影响非流式 / SSE usage 提取与计费。

---

## 5. 证据

### 5.1 合并提交上的失败测试

命令：

```bash
cd backend
go test -tags=unit -count=1 ./internal/service \
  -run "TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes|TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback"
```

结果（合并提交语义 / 修复前）：

```text
=== RUN   TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes
    openai_gateway_service_test.go:2822:
                Error Trace: .../openai_gateway_service_test.go:2822
                Error:          Not equal:
                                expected: 4
                                actual  : 0
                Test:           TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes
--- FAIL: TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes (0.00s)
=== RUN   TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback
--- PASS: TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback (0.00s)
FAIL
```

说明：

- 上游 GPT-5.6 cache write 回归 **失败**
- 本地兼容 cache 字段回归 **仍通过**
- 这是典型的“保留本地 helper，但 helper 语义过时/过强，冲掉上游新字段”

### 5.2 相关测试位置

- `backend/internal/service/openai_gateway_service_test.go`
  - `TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes`
  - `TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback`
- `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`
  - `TestUsageConversionsPreserveCacheWriteTokens`
- 本地兼容：
  - `TestForwardAsRawChatCompletions_StreamPreservesCompatibleCacheUsage`

### 5.3 其他双边文件扫描

对关键双边改动文件做了“双方新增长行是否仍在 HEAD”扫描：

| 文件 | 结果 |
|---|---|
| apicompat bridge/types/responses_to_chatcompletions | OK（cache write 字段映射在） |
| billing/pricing GPT-5.6 cache write | OK |
| WS passthrough effort candidates | OK |
| `isOpenAIGPT56Model` 去重 | OK |
| 本地 prompt_cache_hit 兼容路径 | OK（函数还在） |

**不是整次合并都坏了，而是 cache usage 两条路径叠在一起时语义冲突。**

---

## 6. 影响面

| 场景 | 影响 |
|---|---|
| 官方/兼容上游返回 `input_tokens_details.cache_write_tokens` | `CacheCreationInputTokens` 可能被清零 |
| GPT-5.6 cache write 计费/统计（0.1.150 主功能） | 少记 cache creation，账单与 dashboard 偏差 |
| 仅有顶层 `cache_creation_input_tokens` 的兼容上游 | 仍可工作 |
| 仅有 `prompt_cache_hit_tokens` 的 DeepSeek 类上游 | 本地 helper 仍可补 cache read |
| Chat↔Responses usage 结构转换（apicompat） | 本身正确；问题在 service 层提取后覆盖 |

---

## 7. 建议修复（给 Codex）

### 7.1 原则

本地 `applyOpenAICompatibleCacheUsageFromJSON` 应是：

- **补缺 / 统一解析**
- **不要无条件覆盖** 已由 `openAIUsageFromGJSON` 解析出的正值

### 7.2 推荐改法

修改 `backend/internal/service/openai_gateway_response_handling.go` 中的 `applyOpenAICompatibleCacheUsageFromJSON`：

1. 复用上游已有：
   - `openAICacheCreationTokensFromUsage`
   - `openAICacheReadTokensFromUsage`
2. 仅在解析结果 `> 0` 时写回 `usage`
3. 继续保留 ephemeral 5m/1h 汇总作为 cache creation 回退
4. 确保 `prompt_cache_hit_tokens` 进入 cache read 候选（可放进 `openAICacheReadTokensFromUsage`）

示意：

```go
func applyOpenAICompatibleCacheUsageFromJSON(data []byte, usage *OpenAIUsage, usagePath string) {
    if usage == nil || len(data) == 0 {
        return
    }
    base := strings.TrimSpace(usagePath)
    if base == "" {
        base = "usage"
    }
    usageResult := gjson.GetBytes(data, base)

    cacheCreation := openAICacheCreationTokensFromUsage(usageResult)
    if cacheCreation == 0 {
        cacheCreation5m := int(usageResult.Get("cache_creation.ephemeral_5m_input_tokens").Int())
        cacheCreation1h := int(usageResult.Get("cache_creation.ephemeral_1h_input_tokens").Int())
        if cacheCreation5m > 0 || cacheCreation1h > 0 {
            cacheCreation = cacheCreation5m + cacheCreation1h
        }
    }
    if cacheCreation > 0 {
        usage.CacheCreationInputTokens = cacheCreation
    }

    cacheRead := openAICacheReadTokensFromUsage(usageResult)
    if cacheRead > 0 {
        usage.CacheReadInputTokens = cacheRead
    }
}
```

并建议：

```go
func openAICacheReadTokensFromUsage(value gjson.Result) int {
    return firstPositiveGJSONInt(
        value.Get("input_tokens_details.cached_tokens"),
        value.Get("prompt_tokens_details.cached_tokens"),
        value.Get("cache_read_input_tokens"),
        value.Get("cache_read_tokens"),
        value.Get("cached_tokens"),
        value.Get("prompt_cache_hit_tokens"), // 本地兼容字段
    )
}
```

### 7.3 验证命令

```bash
cd backend

go test -tags=unit -count=1 ./internal/service -run \
"TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes|TestExtractOpenAIUsageFromJSONBytes_CompatibleCacheFieldsFallback|StreamPreservesCompatibleCacheUsage|TestParseSSEUsage_CompatibleCacheFieldsFallback"

go test -tags=unit -count=1 ./internal/pkg/apicompat -run \
"UsageConversionsPreserveCacheWrite|ChatUsageToResponses|PromptCacheKey"
```

期望：

- 上游 cache write 用例 PASS
- 本地 `prompt_cache_hit_tokens` / compatible cache 用例 PASS

### 7.4 流程补记（AGENTS 规则）

本次合并后还应追加：

1. `docs/features/sub2api -merage-list.md`
   - 记录 `9a2f11b4e` / `61fec21ad` / `0.1.150`
   - 冲突文件与处理方式
   - 本复核发现的 cache write 覆盖问题与修复
2. `llm-wiki/wiki/*`
   - 版本基线从 `0.1.149` 更新到 `0.1.150`
   - 补充 GPT-5.6 cache write 计费与 usage 字段兼容说明

---

## 8. 工作区现状说明（给 Codex 对照）

复核过程中发现：工作区 **未提交修改** 已对 `openai_gateway_response_handling.go` 做了与第 7 节一致的修复方向。

| 状态 | 说明 |
|---|---|
| `git rev-parse HEAD` | 仍是合并提交 `61fec21ad` |
| 工作区 diff | `backend/internal/service/openai_gateway_response_handling.go` 已改 |
| 修复后定向测试 | `TestExtractOpenAIUsageFromJSONBytes_*` 与 `StreamPreservesCompatibleCacheUsage` 已 PASS |

因此：

- **合并提交本身** 存在本报告描述的问题
- **当前工作树** 可能已经修掉该问题，但尚未形成独立 commit / merge-list 记录

请 Codex 以本报告为准核对：

1. 合并提交是否确实有该回归（是）
2. 工作区修复是否完整、是否还需补测试/文档
3. 是否把修复与 `0.1.150` 合并记录一并提交

---

## 9. 总评

| 项 | 判定 |
|---|---|
| 冲突文件选择“两边都留” | 方向对 |
| 测试文件合并 | 正确 |
| 上游 cache write 解析器合入 | 正确 |
| 本地兼容 helper 与上游解析器组合 | **错误** |
| 是否需要 Codex 继续处理 | **是：确认/完善修复 + 补 merge-list/wiki + 提交** |

**核心错误不是“漏合某个文件”，而是“合入后两条 usage 路径语义未统一，本地 helper 把上游 cache_write 清零”。**
