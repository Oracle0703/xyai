# Sub2API 0.2.7 上游同步记录（固定提交）

合并日期 2026-09-20 ｜ 工作分支 `feature/hy/10207_merge_sub2api_207`（第一父 = 本地 `main@de5a3e383cd8eb197c1a83f12a71fb04d9e4e049`）｜ 上游 `Wei-Shaw/sub2api main@fbb9006adef852c46f0c7f18b0a8a740722cfac7`（VERSION 0.2.7，上游重写历史后共同祖先退至 `efe9aab1e4ec`）｜ 合并提交 `d120d499e837e53abc8caa8da9f6fc73d9f6daf3`（父 `de5a3e383` + `fbb9006ad`）｜ 状态：在 main 第一父历史上

本文是该轮同步的唯一记录。独立复审任务书 `claude-review-request.md` 与复审结果 `claude-review-result.md`（本目录）已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-09-20 v0.2.7 exact-SHA merge awaiting review` 节。

## 1. 范围

- 上游相对共同祖先 34 commits、96 paths（`+7652 / -615`）。初始三方集合：双方修改 30、仅本地 540、仅上游 66（最终均与目标 blob 一致）。提交前 staged 164 个路径（`+8050 / -3019`）。
- 本轮按"仅解决冲突"执行：合入固定上游、适配本地接口冲突、同步文档、运行测试；完整测试并非全绿，剩余红项均已归属（第 5 节）。
- 独立复审（Claude，2026-09-20）结论 GO：在固定 SHA、仅解决冲突、按用户选择移除 Codex ticket 的范围内，未发现阻断本次合并的缺陷；复审确认 backend/frontend/deploy 三个代码子树（`4e25d22d...` / `31b1fc95...` / `c3fd269d...`）在审核前后无漂移。GO 只覆盖已验证范围，不代表上游无 bug 或数据库集成全覆盖。用户随后授权提交。

### 上游历史重写与用户选择

抓取时 `8b69738d7...fbb9006ad main -> upstream/main (forced update)`。旧 0.2.6 的 8 个提交（ticket PR #7315 及 VERSION 同步）不在目标历史中；共同祖先的 VERSION 是 0.2.5，直接 merge 会把已合入的 ticket 当作本地增量保留。用户明确选择"跟随指定上游移除 Codex ticket"：用旧上游 `8b69738d` → 共同祖先的精确反向差异移除 ticket，再保留目标 0.2.7 的新增逻辑。

- 移除范围：harvester、请求注入、调度 gate、设置缓存/读写/DTO、账号票据状态、前端展示、专属测试；`backend`/`frontend`/`deploy` 中无 `CodexTicket` / `openai_codex_ticket` / harvest 命中。
- 保留：普通 turn-state echo guard（`guardOpenAICodexTurnStateEcho` 仍在 passthrough 出站路径，`openai_codex_turn_state.go` 及测试保留）、response affinity、Codex 指纹、HTTP/2 keepalive、model rewrite、租户隔离。
- 数据边界：没有 ticket 清理 migration，没有操作数据库或改线上配置；已部署库里旧 ticket 的 settings/extra 行可能仍在，只是代码不再读取。

## 2. 冲突与处理

| 项目 | 处理 |
| --- | --- |
| `backend/cmd/server/VERSION`（普通 merge 文本冲突） | 采用 0.2.7，不恢复被上游撤下的 0.2.6 边界 |
| `backend/internal/service/setting_service.go`（移除 ticket 三方补丁的文本冲突） | 精确删除 4 个 ticket cache/singleflight 字段，保留本地 `onRiskControlUpdate func(bool)` 与 `requestArchiveRuntimeCache`；`SetRiskControlUpdateCallback` 仍由 `service/wire.go` 注册，`setting_update.go` 保存成功后调用 |
| Prompt Risk 语义冲突 | 聚焦测试报 `addModerationText` / `collectContentValue` / `collectAnthropicUserContentValue` undefined——上游已改为 `moderationTextCollector` 方法。在本地 `prompt_risk_input.go` 加三个共 14 行的薄转接，一律 `filterReminders=true`，与第一父"遇 `<system-reminder>` 丢弃、Anthropic reminder 按 prefix 判断"的合同等价；newest/full、连续 user items、尾部 tool output、wrapper 剥离仍是原函数。上游关键词路径 `extractContentModerationKeywordText` 仍走 `filterReminders=false`，未被改回 |
| `backend/cmd/server/wire_gen.go` | 从源图生成，去掉 ticket 参数/cleanup，保留本地 provider（相对目标本地 `wire.go` 多 21 行 cleanup：Token Analysis、并发预设、Prompt Metrics、quota flusher 等）；**保留目标生成物中手写的 `pluginManager.SetAccountDirectory(openAIGatewayService)`**——目标源图没有等价接线，`go generate` 会删掉这行（见第 6 节 W1）|

Wire 工具新增的两个 `google/subcommands` 校验和已恢复，`go.sum` 沿本地 main；无 Ent schema 变化；新增 SQL 只有上游原始 `238b_content_moderation_engine_meta.sql`，不改历史迁移。

### 双方修改路径（30 个）

| 文件 | 处理 |
| --- | --- |
| `backend/internal/handler/admin/content_moderation_handler.go` | 接入 TypeSafe engine/profile/test 参数，保留本地 Prompt Risk 三个 handler |
| `backend/internal/handler/endpoint.go` | 加 Seedance 归一化，保留本地 Responses input_tokens/subpath 合同 |
| `backend/internal/repository/content_moderation_repo.go` / `_test.go` | 新增 engine_meta 读写及测试；保留 Prompt Risk block/observe 筛选、封禁计数排除及其断言 |
| `backend/internal/repository/http_upstream.go` | 采用上游 HTTP/2 keepalive 容错，按授权移除 ticket harvest transport |
| `backend/internal/repository/wire.go` | 新增 `PluginKVStore`，保留组织用量、Token Analysis、并发预设等 provider |
| `backend/internal/server/routes/admin.go` | 增加插件只读 `GET /admin/plugins/:id/status`（挂 `adminAuth`，不在子管理员白名单，默认拒绝；仅对完整管理员免 step-up） |
| `backend/internal/server/routes/gateway.go` | Seedance 四组别名（相对第一父新增 12 条路由）走既有 `rootRoute`，带 API Key、allowlist、archive、intercept；`/v1` 链仍是 archive → `guardResponsesSubpath(intercept)` |
| `backend/internal/service/account_usage_service.go` / `_batch_test.go` | 采用上游"查询不清除 refresh error"及用例，保留本地配额/usage 逻辑与聚合断言 |
| `backend/internal/service/content_moderation.go` | 采用上游独立引擎与关键词输入逻辑；保留 Prompt Risk 前置阶段（`evaluatePromptRiskStage` 在语义引擎前）、LLM judge、密钥掩码、judge 回环头、runtime hash、日志/action、热更新 |
| `openai_gateway_cc_pipeline.go`、`openai_gateway_responses_chat_fallback.go` | 合入 DeepSeek 缺失 `reasoning_content` 占位；本地 Responses→Chat 过滤与 options adapter 保留 |
| `openai_gateway_chat_completions_raw.go` / `_test.go` | 增加响应公开模型名还原及断言；本地 thinking 清理、compatible usage/cache normalization 保留 |
| `openai_gateway_passthrough.go` | 采用上游 model 重写（`needModelReplace` 不再先 `Contains(mappedModel)` 的 SSE 重写，目标行为），移除 ticket 注入；本地 body 清洗 / `Retry-After` 校验保留 |
| `openai_gateway_request_body.go`、`openai_gateway_response_handling.go`、`openai_gateway_service_test.go` | 采用上游 model 别名还原与顶层/nested model 重写及新断言；本地 official body 清洗、cache alias/fill-missing、reasoning-only failover、compatible cache 测试保留 |
| `setting_gateway_runtime.go` | 采用上游"非法 UA 保留独立 client version"，按授权移除 ticket cache methods |
| `frontend/src/api/admin/riskControl.ts` | 新增引擎/profile/metadata 参数，保留 Prompt Risk client 与类型 |
| `frontend/src/components/account/CreateAccountModal.vue`、`EditAccountModal.vue` | 合入 Seedance opt-in（默认 capabilities 为 chat/embeddings，`seedance` 需显式勾选），移除 ticket 展示，保留 OpenAI-compatible preset |
| `frontend/src/i18n/locales/{en,zh}/admin/channels.ts`、`zh/common.ts` | 合入上游标签与"内容审计"文案，本地 key 保留 |
| `frontend/src/types/index.ts` | 合入 Seedance capability、移除 ticket DTO，保留 `UserRole`/`sub_admin`/`AdminPermission` |
| `frontend/src/views/admin/RiskControlView.vue` | 合入 TypeSafe 独立配置与草稿测试 key；`PromptRiskPanel` 独立保存/结果筛选保留，未并入 TypeSafe 草稿 |

## 3. 保留的本地能力与语义适配

25 个 tracked `docs/features/` 文件全部保留（含本地 main 最新的部门用量/额度重置设计，本轮不实现），仅台账追加。相对上一版纯上游的本地代码增量涉及 369 个 backend/frontend/deploy 路径，归一化 +/- 行比对除 Wire ticket 参数、Prompt Risk 适配和两个组件 README 外均一致；抽查 65 个仅上游代码路径 index blob 与目标一致（加 `docs/seedance-api.md` 共 66）。

| 本地能力 | 保留依据 |
| --- | --- |
| RequestArchive / RequestIntercept | 路由、middleware、provider、运行态设置与设置页；Seedance 走 `rootRoute`；Responses path guard 先于拦截 |
| Prompt Risk / LLM judge | 独立配置、前置阶段、newest/full、掩码/回环保护、热更新、action 筛选；只适配 collector 接口 |
| Prompt Metrics / Token Analysis | service/repository/API/router/Wire/后台生命周期 |
| 组织用量 / 并发预设 / quota flusher | 独立仓储、管理 API、页面、定时 runner、cleanup |
| 子管理员 | `UserRole`/`AdminPermission`、后端默认拒绝白名单、前端 guards |
| OpenAI-compatible | usage/cache alias 与 fill-missing、thinking/options/schema 清洗、preset、默认 reasoning、大请求保护、空响应 failover |
| 图片工具与本地运维 | 工具页/Pinia 状态持久化、Redis 7+ 与依赖错误分类 |

本地 Prompt Risk 与新增 TypeSafe 内容审计的输入范围、动作、配置不同，不按重复能力删除，两者并存且不能互相替代。

## 4. 采用与未采用的上游行为

- 采用：公开模型名还原、DeepSeek reasoning 占位、关键词 reminder 检查、TypeSafe 独立内容审计引擎（engine_meta）、插件 HostService / KV / 账号目录、HTTP/2 keepalive 容错。
- Seedance：创建走 `BindGrokMediaVideoRequestAccount`；查询/删除走 `IsVideoLookupRequest` 的 user/API Key/group 绑定，缺绑定 404；只在 status 成功且 `completion_tokens>0` 时 claim 一次计费；创建失败不重试；账号需 `seedance` capability。
- 未采用：0.2.6 已合入的 Codex ticket（随上游撤下而移除，见第 1 节）。

## 5. 验证

| 验证 | 命令 / 结果 |
| --- | --- |
| 后端专项 | unit tag，`cmd/server`、service、repository、handler/admin、routes、apicompat、typesafe、pluginapi 按 Wire/Seedance/Plugin/Moderation/PromptRisk/本地能力过滤，通过（TypeSafe client/pluginapi 无匹配用例，由完整测试覆盖） |
| 后端 default | `go test -json -p 1 -count=1 ./...` 退出 0：54 个测试包通过，14 个显式 skip |
| 后端 unit | `go test -json -tags=unit -p 1 -count=1 ./...`：58 包通过，server、service 2 包失败（`/auth/me` golden 与 Ollama CAS，父 `TestAPIContracts` 同时标 fail 不另计）；18 skip |
| 后端 integration | `CI=true go test -json -tags=integration -p 1 -count=1 ./...`：53 包通过，repository 因 Docker 不可用在 `TestMain` fail-closed；18 skip。不能写成全量集成通过 |
| 后端构建 | `go build ./cmd/server`、`go build -tags=embed ./cmd/server` 通过 |
| 依赖 / lint | `go mod tidy -diff` 退出 0；golangci-lint v2.13.0 / Go 1.27 `run --new-from-rev=HEAD --timeout=30m ./...` 0 issues |
| 前端 | lint、typecheck 通过；`pnpm exec vitest run` 325/326 files、2427/2433 tests（6 个第一父 Pinia 装配失败）；build 通过（i18n 3/3，1089 modules，仅既有大 chunk 提示） |
| Wiki 图谱 | 34 nodes、72 edges、54 wikilinks、0 unresolved，READY |
| 复审补跑 | `go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'PromptRisk|ContentModeration|Seedance|PluginHost|PluginAccount|RequestArchive|RequestIntercept'` 四包 ok；`RiskControlView.spec.ts`、`EditAccountModal.spec.ts`、`BulkEditAccountModal.spec.ts` 3 files / 125 tests 通过 |

| 未通过 / 覆盖边界 | 归属与处理 |
| --- | --- |
| `/auth/me` golden 缺 `admin_permissions:null` | 第一父：当前 unit 与导出的 `main@de5a3e383` 精确单测均失败；未改 DTO/golden |
| Ollama CAS（`ratelimit_service_ollama_429.go`） | 上游/第一父：当前 `-count=3` 3 次失败，第一父快照 1 通过 / 2 失败；实现与测试四方 blob 一致。一次转绿不能当修复 |
| `SubscriptionsView.userUsageLink.spec.ts` 6 个 no active Pinia | 第一父：第一父快照同样 6 失败，测试与页面本轮未改 |
| repository integration 整包 | 环境：`docker is not available (CI=true); failing integration tests`。不通过去掉 CI 标志把失败伪装为成功 |
| 18 个 integration 显式 skip | DingTalk sentinel 1、Windows symlink 权限 1、Docker 依赖 middleware/routes 3、TLS capture URL 1、Prompt Audit Redis 3 / PostgreSQL 6、TypeSafe live 凭据 1、OpenAI API key 1、本地插件 fixture 1 |

结论支持提交前人工审核，不代表完整集成或生产验收通过。

## 6. 遗留问题

| ID | 等级 / 归属 | 说明 | 当前状态 |
| --- | --- | --- | --- |
| W1 | P2 / 上游 | `SetAccountDirectory` 只在上游手改的 `wire_gen.go` 中（上游 `c63bd14a0`）；重新生成会丢账号目录注入，插件拿不到 `ListAccounts` / `ResolveOutboundIdentity` | 2026-09-23 本地把这一步移到 `backend/internal/service/wire.go#ProvidePluginManager`，由 Wire 源定义生成（当前源码第 199 行 `manager.SetAccountDirectory(gateway)`）；合并规则见 playbook |
| D1 | P3 / 上游 | `AccountInfo` 注释写"非机密"，实现故意保留 Extra 与 Proxy（含代理密码），不能当作完全脱敏的管理 DTO；`pluginapi/README.md` 的 HostService 只写了 KV、未写账号目录 | 上游文档缺口，未改 |
| T1–T3 | P3 | `/auth/me` golden、Ollama CAS、订阅 Pinia 装配，见第 5 节 | `/auth/me` golden 已随部门功能（`a9c53a9f6`，2026-09-28 进入 main）更新；其余材料中无后续结论 |
| E1/E2 | 环境 | repository integration 未跑；TypeSafe live、OpenAI key、Prompt Audit Redis/PG、插件 fixture 未覆盖 | — |
| F3（0.2.6） | 未确认 | rollup 时区边界仍因无 Docker 未验证，不是本轮回归 | 见 `docs/upstream-sync/2026-09-18-v0.2.6/review.md` |

阅读提示：llm-wiki 中 0.2.6 的 ticket 段落是历史记录，现行行为以 0.2.7 段为准。
