# Chat Completions 缺省 reasoning_effort 注入(固定推理等级)技术留底

> 适用目的: 在 sub2api 上游 main 大版本升级后, 能快速迁移本次本地改造, 或按同一方案重新实现。
> 当前实现形态: `配置开关(默认空=关闭) + 映射后模型门控 + 单注入点覆盖两条上游形状 + 只在真缺失时注入`。

## 1. 背景与需求

客户端走 `/v1/chat/completions`、请求体里**没有** `reasoning_effort` 字段时, 上游会用它自己的默认推理档。需求: 网关自动补一个默认值(目标 `"high"`), 把 OpenAI 推理模型的思考等级**固定**下来。

### 现状关键事实(已核对源码)

- 既有 `ApplyThinkingEnabledFallback`(`gateway_request.go`)、`deriveOpenAIReasoningEffortFromModel`(`openai_gateway_service.go`)只决定**计费/用量日志**里的 effort, **不会**把 `reasoning_effort` 注入发往上游的 body。真正注入上游是新行为。
- 代码里**没有**可复用的"是否推理模型"判定。最接近的 `isReasoningModel`(apicompat 包, 私有, 仅 `gpt-5` 前缀, 无 o 系列)和 `SupportsVerbosity`(`openai_codex_transform.go`, 对非 gpt 模型返回 `true`, 极性相反)都不能直接用。
- `extractOpenAIReasoningEffortFromBody`(`openai_gateway_service.go`)对"缺失"和"显式 none/minimal"**都返回 nil** → 不能用它判断"是否已指定", 否则会覆盖客户端显式的 `minimal`/`none`。

目标产出: 一个**配置开关**(默认空=不改变现有行为), 开启后仅对 OpenAI 推理模型族、且客户端确实没指定 effort 时, 注入配置的默认 effort。

## 2. 注入点与覆盖面

入口在 `ForwardAsChatCompletions`(`openai_gateway_chat_completions.go:89`)。函数体内按账号类型分流成两条上游形状:

1. **raw CC 直转**(APIKey 且 `!ShouldUseResponsesAPI` → `forwardAsRawChatCompletions`, 见 line 107)→ 上游是 CC 形状, 顶层 `reasoning_effort` 透传(sanitize 只删 `thinking`)。
2. **CC→Responses 转换**(OAuth / APIKey 支持 Responses)→ 顶层 `reasoning_effort` 被 `json.Unmarshal` 进 `chatReq.ReasoningEffort`, 再由 `chatcompletions_to_responses.go` 映射成 `reasoning.effort`。

**只在分流之前对入站 `body` 注入一次**(调用点 `openai_gateway_chat_completions.go:101`), 即可同时覆盖两条路; 且因为注入在 `json.Unmarshal` 之前, `chatReq` 与后续计费用量都会自然读到注入值, **无需额外改计费路径**。

## 3. 改动清单

### 3.1 能力判定 `SupportsOpenAIReasoningEffort`(新)

位置: `openai_codex_transform.go:877`(紧邻 `SupportsVerbosity`, 集中放能力判定)。推理模型(gpt-5.x、o 系列 o1/o3/o4)返回 true; 非推理模型(gpt-4o/gpt-4.1/gpt-3.5、非 OpenAI 模型)返回 false。去 `org/` 前缀后按前缀 + `gpt-<major>` 主版本号(≥5)判定。

```go
func SupportsOpenAIReasoningEffort(model string) bool {
    m := strings.ToLower(strings.TrimSpace(model))
    if i := strings.LastIndex(m, "/"); i >= 0 { // 去掉 org/ 前缀
        m = m[i+1:]
    }
    if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
        return true
    }
    if strings.HasPrefix(m, "gpt-") {
        var major int
        if _, err := fmt.Sscanf(m, "gpt-%d", &major); err == nil && major >= 5 {
            return true // gpt-5 / gpt-5.5 / gpt-5-codex / gpt-6...
        }
    }
    return false
}
```

### 3.2 注入函数 `applyDefaultOpenAIReasoningEffort`(新)

位置: `openai_gateway_chat_completions.go:46`(自由函数便于单测)。满足全部条件时才注入: (a) 配置默认非空(经 `normalizeOpenAIReasoningEffort` 归一); (b) body 是标准 CC(含 `messages`, 排除 Cursor 的 Responses-shape 透传); (c) 映射后 billingModel 是推理模型; (d) 客户端没经 `reasoning_effort` / `reasoning.effort` / 模型名后缀指定 effort。任一不满足返回原 body。

```go
func applyDefaultOpenAIReasoningEffort(body []byte, account *Account, defaultMappedModel, configEffort string) []byte {
    def := normalizeOpenAIReasoningEffort(configEffort)
    if def == "" {
        return body
    }
    if !gjson.GetBytes(body, "messages").Exists() { // 排除 Responses-shape(input,无 messages)透传
        return body
    }
    requestedModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
    billingModel := resolveOpenAIForwardModel(account, requestedModel, defaultMappedModel)
    if !SupportsOpenAIReasoningEffort(billingModel) {
        return body
    }
    // 用 Exists() 判断"是否已指定",不能用 extractOpenAIReasoningEffortFromBody
    //(它对显式 none/minimal 也返回 nil,会误覆盖);模型名后缀(gpt-5-high)也视为已指定。
    if gjson.GetBytes(body, "reasoning_effort").Exists() ||
        gjson.GetBytes(body, "reasoning.effort").Exists() ||
        deriveOpenAIReasoningEffortFromModel(requestedModel) != "" {
        return body
    }
    patched, err := sjson.SetBytes(body, "reasoning_effort", def)
    if err != nil {
        logger.L().Warn("Openai chat_completions: inject default reasoning_effort failed", zap.Error(err))
        return body
    }
    return patched
}
```

调用——在 `ForwardAsChatCompletions` 函数体最前(分流之前):

```go
if s.cfg != nil {
    body = applyDefaultOpenAIReasoningEffort(body, account, defaultMappedModel, s.cfg.Gateway.OpenAIDefaultReasoningEffort)
}
```

### 3.3 配置项 `gateway.openai_default_reasoning_effort`(新)

- **结构体字段**——`config.go:742` `GatewayConfig`(紧随 `OpenAIPassthroughAllowTimeoutHeaders` 的扁平命名风格):
  ```go
  OpenAIDefaultReasoningEffort string `mapstructure:"openai_default_reasoning_effort"`
  ```
- **默认值**——`setDefaults()`(`config.go:1909`):
  ```go
  viper.SetDefault("gateway.openai_default_reasoning_effort", "")
  ```
- **校验**——`Validate()`(`config.go:2592`)。注意 config 包不能 import service 包的 `normalizeOpenAIReasoningEffort`, 故就地按 vocabulary 校验:
  ```go
  switch strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(c.Gateway.OpenAIDefaultReasoningEffort))) {
  case "", "none", "minimal", "low", "medium", "high", "xhigh", "extrahigh", "max":
  default:
      return fmt.Errorf("gateway.openai_default_reasoning_effort must be empty or one of: low/medium/high/xhigh")
  }
  ```
- **配置文件**——`backend/config.yaml`(line 244)与 `deploy/config.example.yaml` `gateway:` 段加注释 + 键(默认空)。

## 4. 关键护栏(评审重点)

1. **模型门控是强制的**: 非推理模型注入 `reasoning_effort` 会让官方 OpenAI 返回 400 "unsupported parameter"。`SupportsOpenAIReasoningEffort` 基于**映射后的 billingModel**(`resolveOpenAIForwardModel`)判定, 与 `ApplyThinkingEnabledFallback` 在 billingModel 后判定的既有约定一致。
2. **只在"真缺失"时注入**: 用 `gjson...Exists()` 判断顶层 `reasoning_effort` / `reasoning.effort`, **不**用归一化值, 避免覆盖客户端显式的 `none`/`minimal`; 模型名后缀(`gpt-5-high`)也视为已指定。
3. **排除 Responses-shape 透传**: gate `messages` 存在, 避免给 Cursor 的 `input`-形 body 加上无效顶层字段。
4. **默认空=零行为变更**: 不配置时整条逻辑短路返回原 body, 存量部署不受影响。
5. **第三方上游天然不命中**: DeepSeek/Kimi/GLM 等模型名不匹配 gpt-5/o 前缀, 不会被注入。已知边界: 若运维把第三方模型映射成 `gpt-5*` 别名并开启此开关, 可能向不支持该字段的上游注入——属 opt-in 自担。

## 5. 测试覆盖

`backend/internal/service/openai_default_reasoning_effort_test.go`(新, `//go:build unit`):

- `TestSupportsOpenAIReasoningEffort` 表驱动 18 例: `gpt-5`/`gpt-5.5`/`gpt-5-codex`/`gpt-5-high`/`GPT-5`/`openai/gpt-5.5`/`o1`/`o1-mini`/`o3-mini`/`o4-mini` → true; `gpt-4o`/`gpt-4.1`/`gpt-3.5-turbo`/`claude-sonnet-4.5`/`deepseek-chat`/``/`gpt-`/`o` → false。
- `TestApplyDefaultOpenAIReasoningEffort` 9 子例(account=nil 时 billingModel=body.model):
  - gpt-5.5 缺失 + def=high → 注入 `reasoning_effort:"high"`。
  - def=`max` → 归一为 `xhigh`。
  - def=`""` / def=`none`(归一空)→ 不注入。
  - 已含 `reasoning_effort:"minimal"` → 保留不覆盖。
  - 已含 `reasoning.effort:"low"` → 不注入顶层 `reasoning_effort`。
  - `model:"gpt-5-high"`(后缀已指定)→ 不注入。
  - `model:"gpt-4o"`(非推理)→ 不注入。
  - `{"input":[...]}`(无 messages, Responses-shape)→ 不注入。

`backend/internal/config/config_test.go`:

- `TestLoadDefaultOpenAIDefaultReasoningEffortEmpty`: 默认空。
- `TestLoadOpenAIDefaultReasoningEffortFromEnv`: `GATEWAY_OPENAI_DEFAULT_REASONING_EFFORT=high` 生效。
- Validate 表新增 "gateway openai default reasoning effort invalid" 行: 非法值(如 `ultra`)被拒。

### 验证命令

```bash
cd backend
go build ./...
go test ./internal/service/ -tags unit -run 'SupportsOpenAIReasoning|ApplyDefaultOpenAIReasoning' -count=1
go test ./internal/config/ -run 'OpenAIDefaultReasoning|Validate' -count=1
```

实测结果: `go build` ✅; 全量 service 单测 `-tags unit` ✅(约 100s, 无回归); config 单测 ✅; `gofmt -l` 干净。

## 6. 启用与运维

- **启用方式**: 配置 `gateway.openai_default_reasoning_effort: "high"`(或 low/medium/xhigh), 重启服务。
  - 之后对 OpenAI 推理模型(gpt-5.x / o 系列)、且未带 `reasoning_effort` 的 CC 请求会自动补该值, 用量日志同步显示。
  - 同配置发 `gpt-4o` 等非推理模型 → 不出现 `reasoning_effort`。
- **关闭**: 置空(默认), 整条逻辑短路, 行为回到上游默认。
- **作用域**: 仅 `/v1/chat/completions` 入口; `/v1/responses` 直入路径不受影响。
- **手动验证**(可选): 对一个 OpenAI APIKey/OAuth 账号发不带 `reasoning_effort` 的 `gpt-5.x` CC 请求 → 抓包/用量日志应显示 effort=配置值。

## 7. 不在范围内 / 后续可扩展

- 不做 per-group / per-account 级 effort 覆盖(需要 ent schema + migration, 本次只做全局配置)。如后续要, 可在 Group 实体或 account `Extra` 上加字段。
- 不改 `/v1/responses` 直入路径(需求只覆盖 Chat Completions 入口)。

## 8. 涉及文件

- `backend/internal/service/openai_codex_transform.go`: `SupportsOpenAIReasoningEffort` 能力判定。
- `backend/internal/service/openai_gateway_chat_completions.go`: `applyDefaultOpenAIReasoningEffort` 注入函数 + `ForwardAsChatCompletions` 调用点。
- `backend/internal/config/config.go`: `OpenAIDefaultReasoningEffort` 字段 + 默认值 + 校验。
- `backend/internal/service/openai_default_reasoning_effort_test.go`: 谓词与注入单测(新)。
- `backend/internal/config/config_test.go`: 默认值/env/非法值断言。
- `backend/config.yaml` / `deploy/config.example.yaml`: 配置键与注释。
- `llm-wiki/wiki/backend.md` / `llm-wiki/wiki/security-and-reliability.md`: 知识库同步。
