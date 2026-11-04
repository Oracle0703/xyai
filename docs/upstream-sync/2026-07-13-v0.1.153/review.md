# Sub2API 0.1.153 上游同步记录（固定提交）

合并日期 2026-07-13 ｜ 工作分支 `feature/hy/10153_同步sub2api主线`（第一父 = 0.1.151 分支基线 `5e6e85568792096abfc9d0114b9172dad6256cb7`）｜ 上游 `Wei-Shaw/sub2api main@55ed0ab0da367183d97c15659e33ae9e83f6ff90`（VERSION 0.1.153，merge base `42f3c22830b8`）｜ 合并提交 `0d65f65a20df72aa1ec81966898e3be8699270a0`，另有文档/依赖收口提交 ｜ 状态：已经 PR #16（`177503f60`，2026-07-13）合入 main

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）、`docs/upstream-sync/reviews/2026-07-13-upstream-55ed0ab-merge-review.md` 与 `docs/superpowers/plans/2026-07-13-sub2api-v0.1.153-sync.md` 已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-13 main sync (v0.1.153, pinned)` 节。

## 1. 范围

- 用户要求以 0.1.151 分支为基线，只合入固定提交 `55ed0ab0d`。验证时 `upstream/main` 已到 `7d239d62e`，其后 4 个提交（`5aeb03018` Codex plan-gated model cooldown、`bb7341673` Grok OAuth media 改走 official API、`adb5106c1`、`7d239d62e`）明确不在本轮；`git merge-base --is-ancestor 7d239d62e HEAD` 返回 1。
- 上游 58 commits、157 files（`+6184 / -318`）；合并结果相对第一父 158 files（`+6182 / -319`）。
- 6 个文本冲突。
- 拓扑决策：一个双父 merge commit 保留两侧历史；后续依赖清理与文档用独立收口提交，不改写已推送的 merge commit。
- 不在范围：部署、重启、数据库操作、无关重构、修复 0.1.151 基线 lint 债务。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `README_CN.md` | 保留本地 Windows 手动重启记录，加入上游 Apple container，源码编译顺延为"方式四"。 |
| `backend/go.mod` | 保留上游直接依赖 `x/mod`，同时保留本地直接使用的 `x/sys`、`x/text`；`go mod tidy` 后三者仍为 direct，`go.sum` 清理 4 个不再使用模块的 8 行校验项，`go mod tidy -diff` 为空。 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 删除上游重复入口，保留本地 `ResponsesToChatCompletionsRequestWithOptions` 为唯一入口；新增上游 `EffectiveResponsesTools` 并由 options adapter 调用，本地第三方 temperature/max token 过滤与 Codex `additional_tools` 同时生效。 |
| `backend/internal/server/routes/gateway.go` | 保留 RequestArchive/RequestIntercept 中间件链，加入 Grok `/v1/videos/edits`、`/v1/videos/extensions` 及根级别名；非 Grok 仍本地 404 + business-limited，路由测试覆盖。 |
| `backend/internal/service/concurrency_service.go` | 保留本地 `ConcurrencyCacheError` 与 503 语义，加入上游 API Key 级 WS ingress lease（申请/刷新/丢失关闭）；普通 turn 并发错误仍为 503，长连接限制独立且 fail-close。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 保留本地 OpenAI-compatible provider preset、动态 placeholder 与 credentials metadata，加入 Grok API Key base URL、`xai-...` 与提交 fallback；两条创建链均有 i18n 与凭据测试覆盖。 |

## 3. 保留的本地能力与语义适配

| 能力 | 关键入口 | 状态 |
| --- | --- | --- |
| Prompt Risk / LLM judge | `content_moderation.go`、`prompt_risk_judge.go`、risk-control UI | 保留，fail-open 约束不变 |
| RequestArchive / RequestIntercept | `routes/gateway.go` 的 `/v1`、root、Codex、Gemini 链 | 保留并覆盖新增视频路由 |
| Token Analysis | admin route、handler/service/repository、前端 route/API | 保留 |
| 图片生成 | `/image-gen`、batch image、OpenAI/Grok images | 保留 |
| 用户并发与 503 语义 | preset/runner、`ConcurrencyCacheError`、统一错误响应 | 保留 |
| OpenAI-compatible provider preset | `CreateAccountModal.vue`、`credentialsBuilder.ts` | 保留 |
| Responses→Chat options filter | `ResponsesToChatCompletionsRequestWithOptions` | 保留并接入 `additional_tools` |
| compatible cache usage | raw/buffered/streaming helper 与 billing usage | 保留 |

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| OpenAI WS | `gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds`（默认 300，0 关闭，只限制已完成 turn 之间的客户端空闲）；`gateway.openai_ws.max_ingress_connections_per_api_key`（默认 64，0 关闭）。Redis lease key `concurrency:openai_ws_ingress:api_key:{id}`，TTL 60 秒、20 秒刷新，lease 丢失时关闭连接 |
| Grok/xAI | API Key 账号；OAuth 官方 API URL 归一到 CLI proxy，自定义 URL 仅在可信校验通过时使用；视频 edits/extensions；API Key 上游模型同步（经 `AccountTestService.validateUpstreamBaseURL`），OAuth 模型同步返回 unsupported；真实转发中 OAuth 自定义地址走 `ValidateTrustedBaseURL`，API Key 自定义地址走 `Build*URL` / `ValidateBaseURL`；`UseKeyModal` 生成 Grok CLI / OpenCode 配置 |
| 计费与数据 | alpha search 仅成功 2xx 按次计费；分组 `web_search_price_per_call`（可空 decimal，NULL = 0.01 USD/次），Ent schema、migration、DTO、管理 UI、billing 同步；API Key latest IP 并发索引 |
| API 兼容 | Codex `additional_tools` 与 top-level tools 合并后转换；Read 工具参数 delta 实时透传——收到流式 delta 后 `.done` 只关闭 block，只有非流式或流式完全无 delta 时才 sanitize `.done` 的完整参数；Anthropic/Responses/Chat 的 max token / content filter stop reason 映射 |
| 安全 | 删除向用户端泄露内部 AI 渠道配置的旧 payment channels 接口 |
| Web / 部署 | 嵌入式静态资源一年 immutable 缓存；Apple container 脚本/文档与 macOS CI job |
| 前端 | DataTable 小列表关闭虚拟化并按 row key 缓存行高；日期范围用本地日期；OpenAI OAuth `plan_type`；pool retry 覆盖更多转发路径 |

## 5. 验证

| 命令 / 检查 | 结果 |
| --- | --- |
| `git rev-list --parents -n 1 0d65f65a`、目标/后续提交 ancestor 检查 | 通过：父为 `5e6e85568` + `55ed0ab0d`，`7d239d62e` 非祖先 |
| 未解决文件、冲突标记、`git diff --check` | 通过 |
| `go mod tidy -diff` | 通过 |
| 冲突专项（apicompat / routes / concurrency；前端 5 files / 53 tests）+ `pnpm typecheck` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 完整重跑通过（`internal/service` 101.0s）；首轮被安全软件锁住 `web.test.exe`（`unlinkat ... Access is denied`），无断言失败，换 `E:\tmp\xyai-unit-final-*` 作 `GOTMPDIR` 后通过 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过（`internal/service` 57.8s） |
| `pnpm --dir frontend run lint:check`、`run typecheck` | 通过 |
| `pnpm --dir frontend exec vitest run` | 156 files / 997 tests 通过 |
| `pnpm --dir frontend run build` | 通过，926 modules / 15.56s，仅既有 import/chunk warning |
| 前端构建后 `go build -tags embed -trimpath ./cmd/server` | 通过，Windows 产物 145,468,416 bytes |
| `golangci-lint run --new-from-rev HEAD^1 ./...` | 0 issues，本次未新增 |
| 全仓 `golangci-lint run ./...` | 29 issues，均在 0.1.151 第一父已存在，记为基线债务（见 `docs/features/technical-debt-board-cn.md` TD-004） |

Apple container shell test 需要 bash，Windows 本机未运行；由 `.github/workflows/backend-ci.yml` 的 macOS job 执行 `/bin/bash -n` 与 fixture test。

前端冲突独立复核未发现可执行问题；文档复核校正了 Read 工具 `.done` 语义和 Grok 模型同步/转发校验边界（已写入第 4 节与 llm-wiki），并补充了 pool retry 默认值/范围、usage 周窗口字段、分页 offset 规范化、scheduler cache 单账号编码容错以及嵌入式前端根级 API 旁路等稳定知识。

## 6. 遗留问题

- 全仓 lint 基线债务（当时 29 项）仍在，见 `docs/features/technical-debt-board-cn.md` TD-004。
- 非阻断测试改进：Grok 创建测试从源码合同断言改为真实挂载后检查提交 payload；compatible preset 测试直接断言切换后的 placeholder/hint。未在本轮处理。
