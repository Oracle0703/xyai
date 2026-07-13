# 上游合并复核：`55ed0ab0d` / `0.1.153`

| 字段 | 值 |
|---|---|
| 复核日期 | 2026-07-13 |
| 工作分支 | `feature/hy/10153_同步sub2api主线` |
| 合并提交 | `0d65f65a20df72aa1ec81966898e3be8699270a0` |
| ours / 151 基线 | `5e6e85568792096abfc9d0114b9172dad6256cb7` |
| theirs / 固定上游 | `55ed0ab0da367183d97c15659e33ae9e83f6ff90` |
| merge-base | `42f3c22830b8b15650b12faeb38bbadb1641e6b1` |
| 上游版本 | `0.1.153` |
| 上游增量 | 58 commits，157 files，`+6184 / -318` |
| merge result vs ours | 158 files，`+6182 / -319` |

## 结论

| 等级 | 结论 | 证据 |
|---|---|---|
| P0/P1/P2 | 未发现阻断或需要修复的冲突组合问题 | combined diff、入口扫描、聚焦测试与独立复核 |
| 上游边界 | 正确，只到 `55ed0ab0d` | merge 第二父为目标 SHA |
| 后续提交 | 未合入 | `merge-base --is-ancestor 7d239d62e HEAD` 返回 1 |
| 冲突标记 | 无 | unresolved index 与真实 marker 扫描均为空 |
| 本地能力 | 保留 | RequestArchive/Intercept、options adapter、并发错误、compatible preset 均存在 |
| 上游能力 | 保留 | WS ingress、Grok API Key/media、计费/migration、兼容桥、Apple container 等均存在 |

当前 `upstream/main=7d239d62e` 比目标多 4 个提交，这 4 个提交明确不属于本次范围：

| Commit | 说明 |
|---|---|
| `5aeb03018` | Codex plan-gated model cooldown |
| `bb7341673` | Grok OAuth media 改走 official API |
| `adb5106c1` | 合并 model capability cooldown |
| `7d239d62e` | 合并 Grok OAuth media routing |

## 上游 0.1.153 增量

| 区域 | 稳定变化 |
|---|---|
| OpenAI WS | 每 API Key ingress 连接上限、Redis lease 刷新/丢失关闭、completed turn 间空闲超时 |
| Grok/xAI | API Key 账号、OAuth CLI proxy/可信自定义 base URL、视频 edits/extensions、API Key 上游模型同步和 CLI 配置；OAuth 模型同步返回 unsupported |
| 计费与数据 | alpha search 成功按次计费、分组 `web_search_price_per_call`、API Key latest IP 并发索引 |
| API 兼容 | Codex `additional_tools`、Read 工具参数实时流式、Anthropic/Responses/Chat stop reason 映射 |
| 安全 | 删除泄露内部 AI 渠道信息的旧 payment channels endpoint |
| Web / 部署 | 嵌入式静态资源一年 immutable 缓存、Apple container 脚本/文档/macOS CI |
| 前端 | DataTable 小列表关闭虚拟化并按 row key 缓存行高、本地日期范围、OpenAI OAuth `plan_type`、pool retry 生效范围 |

## 冲突处理复核

| 文件 | 处理方式 | 复核结论 |
|---|---|---|
| `README_CN.md` | 保留本地 Windows 手动重启记录，加入上游 Apple container，并把源码编译顺延为方式四 | 两侧段落和编号均存在 |
| `backend/go.mod` | 保留上游 `x/mod` 直接依赖，同时保留本地直接使用的 `x/sys`、`x/text` | `go mod tidy -diff` 为 0；后续只清理 go.sum 无用校验项 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 删除上游重复入口，保留本地 `ResponsesToChatCompletionsRequestWithOptions`; 新增 `EffectiveResponsesTools` 并由 options adapter 调用 | 本地第三方过滤与上游 `additional_tools` 同时生效 |
| `backend/internal/server/routes/gateway.go` | 保留本地 RequestArchive/RequestIntercept 链，增加 Grok videos edits/extensions 及根级别名 | 非 Grok 仍本地 404 + business-limited；路由测试覆盖 |
| `backend/internal/service/concurrency_service.go` | 保留本地 `ConcurrencyCacheError`，加入上游 API Key 级 WS ingress lease | 普通 turn 并发错误仍为 503；长连接限制独立、fail-close |
| `frontend/src/components/account/CreateAccountModal.vue` | 保留 OpenAI-compatible provider preset 和动态 placeholder，加入 Grok API Key base URL/`xai-...`/提交 fallback | 两条创建链均存在，i18n 与凭据测试覆盖 |

## 本地能力保留检查

| 能力 | 关键入口 | 状态 |
|---|---|---|
| Prompt Risk / LLM judge | `content_moderation.go`、`prompt_risk_judge.go`、risk-control UI | 保留 |
| RequestArchive / RequestIntercept | `routes/gateway.go` 中 `/v1`、root、Codex、Gemini 链 | 保留并覆盖新增视频路由 |
| Token Analysis | admin route、handler/service/repository、前端 route/API | 保留 |
| 图片生成 | `/image-gen`、batch image、OpenAI/Grok images | 保留 |
| 用户并发与 503 语义 | preset/runner、`ConcurrencyCacheError`、统一错误响应 | 保留 |
| OpenAI-compatible provider preset | `CreateAccountModal.vue`、`credentialsBuilder.ts` | 保留 |
| Responses→Chat options filter | `ResponsesToChatCompletionsRequestWithOptions` | 保留并接入 additional tools |
| compatible cache usage | raw/buffered/streaming helper 与 billing usage | 保留 |

## 独立复核

前端冲突独立复核未发现可执行问题：README 两侧段落、Grok API Key 创建链、OpenAI-compatible preset、i18n 和 credentials builder 均保留；5 个聚焦 Vitest 文件共 53 个用例通过，typecheck 通过。

文档复核发现并修正一处知识库表述：Read 工具收到流式 argument delta 后, `.done` 只关闭 block, 不会再次 sanitize/发送完整参数; 只有非流式, 或流式完全没有 delta 时才 sanitize `.done` 的完整参数。修正后的 `backend.md` 与 `security-and-reliability.md` 已对齐 `responses_to_anthropic.go` 和回归测试。

后端增量复核同时校正了 Grok 边界：模型同步只支持 API Key, 通过 `AccountTestService.validateUpstreamBaseURL` 校验；OAuth 返回 unsupported。真实转发中 OAuth 自定义地址走 `ValidateTrustedBaseURL`, API Key 自定义地址走 `Build*URL` / `ValidateBaseURL`, 均不同于模型同步路径。稳定知识还补充了 pool retry 默认值/范围、usage 周窗口字段、分页 offset 规范化、scheduler cache 单账号编码容错以及 embedded frontend 根级 API 旁路。

非阻断测试改进建议：将 Grok 创建测试从源码合同断言扩展为真实挂载后检查提交 payload，并在 compatible preset 测试中直接断言切换后的 placeholder/hint。当前已有测试足以覆盖本次 merge，建议作为后续测试质量改进单独处理。

## 验证状态

| 检查 | 结果 |
|---|---|
| merge parents / ancestor boundary | 通过 |
| unresolved files / conflict markers / `git diff --check` | 通过 |
| `go mod tidy -diff` | 通过 |
| `golangci-lint run --new-from-rev HEAD^1 ./...` | 通过，0 issues |
| `go build -tags embed -trimpath ./cmd/server` | 通过，Windows artifact 145,468,416 bytes |
| 全仓 `golangci-lint run ./...` | 29 issues，均在 151 第一父已存在；不归因本次 merge |
| `go test -tags=unit -p 1 -count=1 ./...` | 完整重跑通过；`internal/service` `101.002s`。首轮被安全软件锁住 `web.test.exe`, 无断言失败 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过；`internal/service` `57.799s` |
| 前端 lint / typecheck | 均通过 |
| 全量 Vitest | 通过：156 files / 997 tests |
| 前端 production build | 通过：926 modules / `15.56s`；仅既有 import/chunk warning |
| 前端 build 后再次执行 embed build | 通过；最终 artifact 145,468,416 bytes |

Windows 本机没有 bash，无法直接运行 Apple container shell test；`.github/workflows/backend-ci.yml` 已增加 macOS job 执行脚本语法检查和 fixture test。

## 复查命令

```powershell
git show --stat --summary 0d65f65a20df
git show --cc 0d65f65a20df -- README_CN.md backend/go.mod backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go backend/internal/server/routes/gateway.go backend/internal/service/concurrency_service.go frontend/src/components/account/CreateAccountModal.vue
git diff --stat 42f3c22830b8..55ed0ab0da36
git log --oneline 55ed0ab0da36..upstream/main
```
